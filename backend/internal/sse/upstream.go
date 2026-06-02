package sse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/skua-app/skua/internal/events"
)

// Frigate /ws speaks a thin wrapper over MQTT-style topic frames:
//
//	{"topic":"<topic>","payload":"<string>"}
//
// Useful topics (probed against Frigate 0.17 on 2026-05-20):
//
//   - "events"                 — payload is a JSON-encoded string
//     { "type":"new"|"update"|"end", "before":<event>, "after":<event> }
//     where <event> matches events.FrigateEvent.
//   - "<cam_id>/status/detect" — payload is the literal string "online" / "offline"
//
// We translate those upstream frames into four SSE event names
// (event.new, event.end, camera.online, camera.offline) plus a
// synthetic "reconnected" event on every successful reconnect.

// upstreamFrame is the outer envelope sent on Frigate /ws.
// Payload is left as RawMessage because it is sometimes a quoted JSON
// string (events / status) and sometimes a number (e.g. zone occupancy).
type upstreamFrame struct {
	Topic   string          `json:"topic"`
	Payload json.RawMessage `json:"payload"`
}

// eventsPayload is the parsed shape of the JSON-encoded string inside
// the "events" topic's payload field.
type eventsPayload struct {
	Type  string              `json:"type"`
	After events.FrigateEvent `json:"after"`
}

// Upstream maintains a single WebSocket connection to Frigate and forwards
// translated frames to a Hub for fan-out.
type Upstream struct {
	wsURL  string
	hub    *Hub
	logger *slog.Logger
	dialer *websocket.Dialer
}

func NewUpstream(wsURL string, hub *Hub, logger *slog.Logger) *Upstream {
	return &Upstream{
		wsURL:  wsURL,
		hub:    hub,
		logger: logger,
		dialer: &websocket.Dialer{HandshakeTimeout: 10 * time.Second},
	}
}

// Run drives the upstream connection loop until ctx is cancelled. It blocks,
// so callers should run it in a goroutine. The first connection is silent;
// every subsequent successful reconnect broadcasts a synthetic
// "reconnected" event so clients can refresh state if they care to.
func (u *Upstream) Run(ctx context.Context) {
	const (
		initialBackoff = time.Second
		maxBackoff     = 30 * time.Second
	)
	backoff := initialBackoff
	firstConnect := true

	for {
		if err := ctx.Err(); err != nil {
			return
		}

		err := u.runOnce(ctx, firstConnect)
		if err == nil || errors.Is(err, context.Canceled) {
			return
		}
		u.logger.Warn("frigate ws disconnected", "error", err, "retry_in", backoff)
		firstConnect = false

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// runOnce holds a single connection. Resets backoff (via the caller) on a
// clean exit isn't done — we always retry with whatever the loop's current
// backoff is, because a Frigate that flap-disconnects shouldn't be polled
// at sub-second cadence.
func (u *Upstream) runOnce(ctx context.Context, firstConnect bool) error {
	c, _, err := u.dialer.DialContext(ctx, u.wsURL, nil)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = c.Close() }()

	u.logger.Info("frigate ws connected", "url", u.wsURL)
	if !firstConnect {
		u.hub.Broadcast("reconnected", struct{}{})
	}

	// Per-read deadline keeps a half-dead socket from hanging forever; a
	// real Frigate publishes /status/* and zone activity continuously so 60s
	// of silence is a strong signal the connection is gone.
	for {
		if err := c.SetReadDeadline(time.Now().Add(60 * time.Second)); err != nil {
			return fmt.Errorf("set read deadline: %w", err)
		}
		_, msg, err := c.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		u.handleFrame(msg)
	}
}

func (u *Upstream) handleFrame(raw []byte) {
	var frame upstreamFrame
	if err := json.Unmarshal(raw, &frame); err != nil {
		u.logger.Debug("frigate ws frame parse failed", "error", err)
		return
	}

	switch {
	case frame.Topic == "events":
		u.handleEvents(frame.Payload)
	case strings.HasSuffix(frame.Topic, "/status/detect"):
		camID := strings.TrimSuffix(frame.Topic, "/status/detect")
		u.handleDetectStatus(camID, frame.Payload)
	}
}

func (u *Upstream) handleEvents(rawPayload json.RawMessage) {
	// The "events" payload is a JSON-encoded STRING — unwrap one layer.
	var encoded string
	if err := json.Unmarshal(rawPayload, &encoded); err != nil {
		u.logger.Debug("frigate events payload not a string", "error", err)
		return
	}
	var ev eventsPayload
	if err := json.Unmarshal([]byte(encoded), &ev); err != nil {
		u.logger.Debug("frigate events payload parse failed", "error", err)
		return
	}

	item := events.ToItem(ev.After)
	switch ev.Type {
	case "new":
		u.hub.Broadcast("event.new", item)
	case "end":
		u.hub.Broadcast("event.end", map[string]any{
			"id":               item.ID,
			"ended_at":         item.EndedAt,
			"duration_seconds": item.DurationSeconds,
		})
	}
	// "update" frames are noisy and not surfaced — clients refetch on
	// event.end or rely on cached snapshots for visible state.
}

func (u *Upstream) handleDetectStatus(camID string, rawPayload json.RawMessage) {
	var status string
	if err := json.Unmarshal(rawPayload, &status); err != nil {
		return
	}
	switch status {
	case "online":
		u.hub.Broadcast("camera.online", map[string]string{"cam_id": camID})
	case "offline":
		u.hub.Broadcast("camera.offline", map[string]string{"cam_id": camID})
	}
}

// DeriveWSURL converts an HTTP(S) base URL to ws://host/ws (or wss).
// Returns an error if the base URL is malformed.
func DeriveWSURL(httpBase string) (string, error) {
	u, err := url.Parse(httpBase)
	if err != nil {
		return "", fmt.Errorf("parse base url: %w", err)
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
		// already a ws scheme
	default:
		return "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	u.Path = "/ws"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

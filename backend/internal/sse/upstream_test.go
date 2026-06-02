package sse

import (
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/skua-app/skua/internal/events"
)

func newTestUpstream(t *testing.T) (*Upstream, *Client) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	hub := NewHub(logger)
	u := NewUpstream("ws://unused/ws", hub, logger)
	c := hub.Subscribe()
	t.Cleanup(func() { hub.Unsubscribe(c) })
	return u, c
}

// readFrame waits briefly for the next frame on c.C.
func readFrame(t *testing.T, c *Client) (string, bool) {
	t.Helper()
	select {
	case b := <-c.C:
		return string(b), true
	case <-time.After(200 * time.Millisecond):
		return "", false
	}
}

// expectNoFrame asserts no frame is delivered within a short window.
func expectNoFrame(t *testing.T, c *Client) {
	t.Helper()
	select {
	case b := <-c.C:
		t.Fatalf("expected no frame, got %q", string(b))
	case <-time.After(50 * time.Millisecond):
	}
}

// parseFrame splits an SSE frame into event name and JSON data.
func parseFrame(t *testing.T, frame string) (eventName, data string) {
	t.Helper()
	if !strings.HasSuffix(frame, "\n\n") {
		t.Fatalf("frame missing trailing blank line: %q", frame)
	}
	lines := strings.Split(strings.TrimSuffix(frame, "\n\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("frame must be 2 lines, got %d: %q", len(lines), frame)
	}
	if !strings.HasPrefix(lines[0], "event: ") {
		t.Fatalf("missing event prefix: %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "data: ") {
		t.Fatalf("missing data prefix: %q", lines[1])
	}
	return strings.TrimPrefix(lines[0], "event: "), strings.TrimPrefix(lines[1], "data: ")
}

// buildEventsFrame wraps an inner events payload as a Frigate /ws frame.
// The payload field is JSON-encoded twice as Frigate publishes it: the
// inner object is marshaled to JSON, that JSON is marshaled again as a
// string, and the resulting quoted-string bytes become the outer envelope's
// payload field. handleEvents undoes both layers.
func buildEventsFrame(t *testing.T, inner any) []byte {
	t.Helper()
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		t.Fatalf("marshal inner: %v", err)
	}
	payload, err := json.Marshal(string(innerJSON))
	if err != nil {
		t.Fatalf("marshal payload string: %v", err)
	}
	frame, err := json.Marshal(upstreamFrame{Topic: "events", Payload: payload})
	if err != nil {
		t.Fatalf("marshal frame: %v", err)
	}
	return frame
}

// buildStatusFrame builds a "<cam>/status/detect" frame where the payload
// is a JSON-encoded string like "online" / "offline".
func buildStatusFrame(t *testing.T, camID, status string) []byte {
	t.Helper()
	payload, err := json.Marshal(status)
	if err != nil {
		t.Fatalf("marshal status payload: %v", err)
	}
	frame, err := json.Marshal(upstreamFrame{Topic: camID + "/status/detect", Payload: payload})
	if err != nil {
		t.Fatalf("marshal status frame: %v", err)
	}
	return frame
}

func sampleFrigateEvent() events.FrigateEvent {
	score := 0.91
	e := events.FrigateEvent{
		ID:          "1748800000.0-abcdef",
		Camera:      "cam3",
		Label:       "person",
		StartTime:   1748800000.0,
		HasSnapshot: true,
		HasClip:     true,
	}
	e.Data.TopScore = &score
	return e
}

func TestUpstream_HandleEvents_New(t *testing.T) {
	u, c := newTestUpstream(t)
	after := sampleFrigateEvent()
	inner := eventsPayload{Type: "new", After: after}

	u.handleFrame(buildEventsFrame(t, inner))

	frame, ok := readFrame(t, c)
	if !ok {
		t.Fatal("expected event.new broadcast")
	}
	name, data := parseFrame(t, frame)
	if name != "event.new" {
		t.Errorf("event name = %q, want event.new", name)
	}
	wantJSON, err := json.Marshal(events.ToItem(after))
	if err != nil {
		t.Fatalf("marshal want: %v", err)
	}
	if data != string(wantJSON) {
		t.Errorf("data = %q, want %q", data, wantJSON)
	}
}

func TestUpstream_HandleEvents_End(t *testing.T) {
	u, c := newTestUpstream(t)
	after := sampleFrigateEvent()
	endTime := after.StartTime + 12.0
	after.EndTime = &endTime
	inner := eventsPayload{Type: "end", After: after}

	u.handleFrame(buildEventsFrame(t, inner))

	frame, ok := readFrame(t, c)
	if !ok {
		t.Fatal("expected event.end broadcast")
	}
	name, data := parseFrame(t, frame)
	if name != "event.end" {
		t.Errorf("event name = %q, want event.end", name)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(data), &payload); err != nil {
		t.Fatalf("unmarshal event.end data: %v", err)
	}
	if payload["id"] != after.ID {
		t.Errorf("id = %v, want %v", payload["id"], after.ID)
	}
	wantItem := events.ToItem(after)
	if wantItem.EndedAt == nil {
		t.Fatal("test fixture: ToItem produced nil EndedAt")
	}
	if payload["ended_at"] != *wantItem.EndedAt {
		t.Errorf("ended_at = %v, want %v", payload["ended_at"], *wantItem.EndedAt)
	}
	dur, ok := payload["duration_seconds"].(float64)
	if !ok {
		t.Fatalf("duration_seconds missing or wrong type: %v", payload["duration_seconds"])
	}
	if dur != 12 {
		t.Errorf("duration_seconds = %v, want 12", dur)
	}
}

func TestUpstream_HandleEvents_UpdateDropped(t *testing.T) {
	u, c := newTestUpstream(t)
	inner := eventsPayload{Type: "update", After: sampleFrigateEvent()}

	u.handleFrame(buildEventsFrame(t, inner))

	expectNoFrame(t, c)
}

func TestUpstream_HandleEvents_Malformed(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
	}{
		{
			name: "payload not a string",
			raw:  []byte(`{"topic":"events","payload":{"type":"new"}}`),
		},
		{
			name: "payload string not valid inner json",
			raw:  []byte(`{"topic":"events","payload":"not-json"}`),
		},
		{
			name: "payload missing entirely",
			raw:  []byte(`{"topic":"events"}`),
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u, c := newTestUpstream(t)
			u.handleFrame(tc.raw) // must not panic
			expectNoFrame(t, c)
		})
	}
}

func TestUpstream_HandleDetectStatus(t *testing.T) {
	tests := []struct {
		name      string
		camID     string
		status    string
		wantEvent string
	}{
		{"online", "cam3", "online", "camera.online"},
		{"offline", "cam5_main", "offline", "camera.offline"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			u, c := newTestUpstream(t)
			u.handleFrame(buildStatusFrame(t, tc.camID, tc.status))
			frame, ok := readFrame(t, c)
			if !ok {
				t.Fatalf("expected %s broadcast", tc.wantEvent)
			}
			name, data := parseFrame(t, frame)
			if name != tc.wantEvent {
				t.Errorf("event name = %q, want %q", name, tc.wantEvent)
			}
			var payload map[string]string
			if err := json.Unmarshal([]byte(data), &payload); err != nil {
				t.Fatalf("unmarshal status data: %v", err)
			}
			if payload["cam_id"] != tc.camID {
				t.Errorf("cam_id = %q, want %q", payload["cam_id"], tc.camID)
			}
		})
	}
}

func TestUpstream_HandleDetectStatus_Unknown(t *testing.T) {
	u, c := newTestUpstream(t)
	u.handleFrame(buildStatusFrame(t, "cam9", "weird"))
	expectNoFrame(t, c)
}

func TestUpstream_HandleDetectStatus_NonStringPayload(t *testing.T) {
	u, c := newTestUpstream(t)
	u.handleFrame([]byte(`{"topic":"cam1/status/detect","payload":123}`))
	expectNoFrame(t, c)
}

func TestUpstream_HandleFrame_UnknownTopic(t *testing.T) {
	u, c := newTestUpstream(t)
	u.handleFrame([]byte(`{"topic":"zone/front/occupancy","payload":3}`))
	expectNoFrame(t, c)
}

func TestUpstream_HandleFrame_Malformed(t *testing.T) {
	u, c := newTestUpstream(t)
	u.handleFrame([]byte(`not even json`))
	expectNoFrame(t, c)
}

// DeriveWSURL extra branches not exercised by TestDeriveWSURL in sse_test.go:
// ws/wss passthrough, unsupported scheme, and malformed URL.
func TestDeriveWSURL_ExtraBranches(t *testing.T) {
	t.Run("ws_passthrough", func(t *testing.T) {
		got, err := DeriveWSURL("ws://host:5000/foo")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "ws://host:5000/ws" {
			t.Errorf("got %q, want ws://host:5000/ws", got)
		}
	})
	t.Run("wss_passthrough", func(t *testing.T) {
		got, err := DeriveWSURL("wss://host/api")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "wss://host/ws" {
			t.Errorf("got %q, want wss://host/ws", got)
		}
	})
	t.Run("unsupported_scheme", func(t *testing.T) {
		if _, err := DeriveWSURL("ftp://host"); err == nil {
			t.Error("expected error for unsupported scheme")
		}
	})
	t.Run("malformed_url", func(t *testing.T) {
		// Embedded control character forces url.Parse to error.
		if _, err := DeriveWSURL("http://\x00host"); err == nil {
			t.Error("expected error for malformed url")
		}
	})
}

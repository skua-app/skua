// Package sse implements a single-upstream-to-many-clients event fan-out.
//
// Topology: one persistent WebSocket from BFF → Frigate /ws (in upstream.go)
// feeds Hub.Broadcast. Each HTTP SSE client opened against handler.go gets
// its own bounded channel; slow consumers are evicted rather than backing
// up the broadcast loop.
package sse

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
)

// clientChanCap bounds each subscriber's queue. Broadcasts that can't be
// delivered within this cap mark the subscriber as slow and evict it.
const clientChanCap = 32

// Client is a subscriber's handle on the hub. The HTTP handler reads from C
// and writes the bytes to its ResponseWriter; Done closes when the hub has
// evicted this subscriber (slow consumer or shutdown).
type Client struct {
	C    chan []byte
	done chan struct{}
}

// Done returns a channel closed when the hub evicts this client.
func (c *Client) Done() <-chan struct{} { return c.done }

// Hub holds the live set of subscribers and dispatches broadcasts to them.
// The zero value is unusable; call NewHub.
type Hub struct {
	logger *slog.Logger
	mu     sync.RWMutex
	subs   map[*Client]struct{}
}

func NewHub(logger *slog.Logger) *Hub {
	return &Hub{
		logger: logger,
		subs:   make(map[*Client]struct{}),
	}
}

// Subscribe registers a new subscriber. Always pair with Unsubscribe to
// release the slot (typically via defer in the SSE handler).
func (h *Hub) Subscribe() *Client {
	c := &Client{
		C:    make(chan []byte, clientChanCap),
		done: make(chan struct{}),
	}
	h.mu.Lock()
	h.subs[c] = struct{}{}
	h.mu.Unlock()
	return c
}

// Unsubscribe removes a subscriber and closes its done channel. Safe to call
// after the hub has already evicted the subscriber.
func (h *Hub) Unsubscribe(c *Client) {
	h.mu.Lock()
	if _, ok := h.subs[c]; ok {
		delete(h.subs, c)
		close(c.done)
	}
	h.mu.Unlock()
}

// Broadcast serialises payload as JSON, formats it as an SSE event frame,
// and pushes it to every subscriber. Subscribers whose channel is full are
// considered slow and are evicted on the spot (their Done channel closes).
func (h *Hub) Broadcast(eventName string, payload any) {
	data, err := json.Marshal(payload)
	if err != nil {
		h.logger.Debug("sse marshal failed", "event", eventName, "error", err)
		return
	}
	frame := []byte(fmt.Sprintf("event: %s\ndata: %s\n\n", eventName, data))

	h.mu.RLock()
	var slow []*Client
	for c := range h.subs {
		select {
		case c.C <- frame:
		default:
			slow = append(slow, c)
		}
	}
	h.mu.RUnlock()

	if len(slow) == 0 {
		return
	}
	h.mu.Lock()
	for _, c := range slow {
		if _, ok := h.subs[c]; !ok {
			continue
		}
		delete(h.subs, c)
		close(c.done)
		h.logger.Warn("sse subscriber dropped (slow)", "remaining", len(h.subs))
	}
	h.mu.Unlock()
}

// SubscriberCount returns the current number of registered subscribers.
// Intended for diagnostics and tests.
func (h *Hub) SubscriberCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.subs)
}

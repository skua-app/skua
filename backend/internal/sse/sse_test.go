package sse

import (
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func newTestHub() *Hub {
	return NewHub(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestHub_BroadcastReachesSubscriber(t *testing.T) {
	h := newTestHub()
	c := h.Subscribe()
	defer h.Unsubscribe(c)

	h.Broadcast("event.new", map[string]string{"id": "abc"})

	select {
	case frame := <-c.C:
		got := string(frame)
		if !strings.HasPrefix(got, "event: event.new\n") {
			t.Errorf("missing event line: %q", got)
		}
		if !strings.Contains(got, `data: {"id":"abc"}`) {
			t.Errorf("missing data line: %q", got)
		}
		if !strings.HasSuffix(got, "\n\n") {
			t.Errorf("frame must terminate with blank line: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive broadcast")
	}
}

func TestHub_SlowSubscriberDropped(t *testing.T) {
	h := newTestHub()
	c := h.Subscribe()
	// Intentionally do not drain c.C; the bounded channel will fill.

	for i := 0; i < clientChanCap+5; i++ {
		h.Broadcast("event.new", i)
	}

	select {
	case <-c.Done():
		// expected — hub evicted slow subscriber
	case <-time.After(time.Second):
		t.Fatal("slow subscriber was not evicted")
	}
	if got := h.SubscriberCount(); got != 0 {
		t.Errorf("SubscriberCount after eviction = %d, want 0", got)
	}
}

func TestHub_UnsubscribeIdempotent(t *testing.T) {
	h := newTestHub()
	c := h.Subscribe()
	h.Unsubscribe(c)
	// A second Unsubscribe (e.g. after eviction) must not panic.
	h.Unsubscribe(c)
}

func TestDeriveWSURL(t *testing.T) {
	cases := map[string]string{
		"http://10.0.0.1:5000":       "ws://10.0.0.1:5000/ws",
		"https://frigate.local":      "wss://frigate.local/ws",
		"http://host:5000/":          "ws://host:5000/ws",
		"http://host:5000/api/foo?x": "ws://host:5000/ws",
	}
	for in, want := range cases {
		got, err := DeriveWSURL(in)
		if err != nil {
			t.Errorf("DeriveWSURL(%q): unexpected error %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("DeriveWSURL(%q) = %q, want %q", in, got, want)
		}
	}
}

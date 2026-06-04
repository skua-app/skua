package sse

import (
	"io"
	"log/slog"
	"strings"
	"sync"
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

// TestHub_ConcurrentSubscribeBroadcastUnsubscribeRace drives Subscribe,
// Broadcast, and Unsubscribe from many goroutines for a bounded window so
// the race detector sees the slow-consumer eviction path in Broadcast
// overlap with explicit Unsubscribe calls — both routes delete-and-close
// the client's done channel, and both must coordinate through h.mu without
// a double close. The test passes if -race finds nothing, no goroutine
// panics on close(done), and the hub quiesces with no leaked subscribers.
func TestHub_ConcurrentSubscribeBroadcastUnsubscribeRace(t *testing.T) {
	h := newTestHub()
	deadline := time.Now().Add(200 * time.Millisecond)

	var wg sync.WaitGroup

	// Broadcasters: push frames continuously so slow subscribers fill their
	// bounded channel and the eviction path fires concurrently with Unsubscribe.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; time.Now().Before(deadline); j++ {
				h.Broadcast("event.new", map[string]int{"id": id, "n": j})
			}
		}(i)
	}

	// Draining churners: short-lived subscribers that read a few frames and
	// then Unsubscribe themselves — exercises Subscribe / Unsubscribe under
	// concurrent broadcast load.
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				c := h.Subscribe()
				for drained := 0; drained < 4; {
					select {
					case <-c.C:
						drained++
					case <-c.Done():
						drained = 4
					case <-time.After(2 * time.Millisecond):
						drained = 4
					}
				}
				h.Unsubscribe(c)
			}
		}()
	}

	// Slow consumers: never drain, so the hub's eviction path closes their
	// done channel. The explicit Unsubscribe races against that eviction —
	// both Broadcast and Unsubscribe must observe the membership check
	// under h.mu so close(c.done) only happens once.
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(deadline) {
				c := h.Subscribe()
				// Brief pause so a Broadcast almost certainly fills the
				// 32-cap channel and reaches the eviction path before
				// Unsubscribe runs.
				time.Sleep(time.Millisecond)
				h.Unsubscribe(c)
			}
		}()
	}

	wg.Wait()

	if got := h.SubscriberCount(); got != 0 {
		t.Errorf("SubscriberCount after quiesce = %d, want 0", got)
	}
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

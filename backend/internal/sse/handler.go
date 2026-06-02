package sse

import (
	"net/http"
	"time"
)

// pingInterval keeps middleboxes and Service Workers from idling out a
// quiet SSE stream. The wire frame is an SSE comment, ignored by EventSource.
const pingInterval = 15 * time.Second

// ServeHTTP implements the GET /api/stream handler. The router must allow
// long-lived responses (no write deadline) — see api/router.go's Unwrap on
// the request-logging response wrapper, which exposes the underlying
// ResponseWriter to http.NewResponseController.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	// X-Accel-Buffering: no disables nginx response buffering so each event
	// frame is forwarded immediately. NPM (our reverse proxy) honours it.
	w.Header().Set("X-Accel-Buffering", "no")

	rc := http.NewResponseController(w)
	// Disable write deadlines so the long-lived stream can outlive the
	// server's WriteTimeout.
	if err := rc.SetWriteDeadline(time.Time{}); err != nil {
		h.logger.Debug("sse: SetWriteDeadline unsupported", "error", err)
	}

	// Initial comment forces headers to flush so the browser opens the
	// EventSource immediately rather than waiting for the first event.
	if _, err := w.Write([]byte(": connected\n\n")); err != nil {
		return
	}
	if err := rc.Flush(); err != nil {
		// If we can't flush we can't stream — give up early.
		h.logger.Debug("sse: initial flush failed", "error", err)
		return
	}

	client := h.Subscribe()
	defer h.Unsubscribe(client)

	ctx := r.Context()
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-client.Done():
			return
		case frame := <-client.C:
			if _, err := w.Write(frame); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		case <-ticker.C:
			if _, err := w.Write([]byte(": ping\n\n")); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
}

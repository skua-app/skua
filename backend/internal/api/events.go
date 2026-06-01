package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/skua-app/skua/internal/events"
)

const (
	eventsListTimeout  = 5 * time.Second
	eventsImageTimeout = 5 * time.Second
	eventsClipTimeout  = 30 * time.Second
	eventsDefaultLimit = 50
	eventsMaxLimit     = 200
)

// handleEvents proxies GET /api/events to Frigate with normalised filters
// and cursor pagination. See docs/epics/E3 §D2.
func (h *Handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit := eventsDefaultLimit
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			writeError(w, h.logger, http.StatusBadRequest, "limit must be a positive integer", nil)
			return
		}
		if n > eventsMaxLimit {
			n = eventsMaxLimit
		}
		limit = n
	}

	var before time.Time
	if s := q.Get("before"); s != "" {
		t, err := time.Parse(time.RFC3339, s)
		if err != nil {
			writeError(w, h.logger, http.StatusBadRequest, "before must be ISO 8601", err)
			return
		}
		before = t
	}

	params := events.ListParams{
		Cameras: splitCSV(q["camera"]),
		Labels:  splitCSV(q["label"]),
		Before:  before,
		Limit:   limit,
	}

	ctx, cancel := context.WithTimeout(r.Context(), eventsListTimeout)
	defer cancel()

	resp, err := h.events.List(ctx, params)
	if err != nil {
		status := http.StatusBadGateway
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		writeError(w, h.logger, status, "events upstream error", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("encode events response", "error", err)
	}
}

// handleEventThumbnail proxies a per-event thumbnail from Frigate with an
// immutable cache so the client only fetches each thumb once. See §D3.
func (h *Handler) handleEventThumbnail(w http.ResponseWriter, r *http.Request) {
	h.proxyEventImage(w, r, "thumbnail.jpg")
}

// handleEventSnapshot proxies a per-event snapshot from Frigate with an
// immutable cache. See §D3.
func (h *Handler) handleEventSnapshot(w http.ResponseWriter, r *http.Request) {
	h.proxyEventImage(w, r, "snapshot.jpg")
}

// handleEventClip serves the per-event MP4 clip from Frigate. With
// ?download=1 the response is served as an attachment with a meaningful
// filename; otherwise inline for <video> playback. The BFF buffers the
// upstream clip and serves it via http.ServeContent so that HTTP Range
// requests work end-to-end — required for iOS Safari inline playback. See
// events.Client.ServeClip for the buffering rationale and size cap.
func (h *Handler) handleEventClip(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, h.logger, http.StatusBadRequest, "missing event id", nil)
		return
	}
	if !validUpstreamID(id) {
		writeError(w, h.logger, http.StatusBadRequest, "invalid event id", nil)
		return
	}
	var downloadName string
	if r.URL.Query().Get("download") == "1" {
		downloadName = "frigate-" + id + ".mp4"
	}

	ctx, cancel := context.WithTimeout(r.Context(), eventsClipTimeout)
	defer cancel()

	if err := h.events.ServeClip(ctx, id, w, r, downloadName); err != nil {
		status := http.StatusBadGateway
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusGatewayTimeout
		}
		writeError(w, h.logger, status, "event clip upstream error", err)
		return
	}
}

func (h *Handler) proxyEventImage(w http.ResponseWriter, r *http.Request, name string) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, h.logger, http.StatusBadRequest, "missing event id", nil)
		return
	}
	if !validUpstreamID(id) {
		writeError(w, h.logger, http.StatusBadRequest, "invalid event id", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), eventsImageTimeout)
	defer cancel()

	body, etag, err := h.events.FetchImage(ctx, id, name)
	if err != nil {
		writeError(w, h.logger, http.StatusBadGateway, "event image upstream error", err)
		return
	}
	defer func() {
		if err := body.Close(); err != nil {
			h.logger.Debug("failed to close event image body", "error", err)
		}
	}()

	if etag == "" {
		// Event snapshots are write-once, so the id is a safe validator.
		etag = `"` + id + `"`
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("ETag", etag)
	if _, err := io.Copy(w, body); err != nil {
		h.logger.Debug("stream event image", "id", id, "name", name, "error", err)
	}
}

// validUpstreamID reports whether s is a safe path segment to interpolate
// into an upstream Frigate URL: non-empty, only [A-Za-z0-9._-]. Frigate
// event ids are "<unix>-<hash>" and camera ids are config keys, both of
// which satisfy this; anything else (raw or percent-encoded "/", "?", "#",
// whitespace, etc.) is malformed or hostile and must be rejected at the
// HTTP boundary before it reaches any upstream client method.
func validUpstreamID(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '.' || c == '_' || c == '-':
		default:
			return false
		}
	}
	return true
}

// splitCSV flattens chi-style repeated query values into a deduped list,
// allowing both ?camera=cam5&camera=cam7 and ?camera=cam5,cam7 callers.
func splitCSV(values []string) []string {
	seen := make(map[string]struct{})
	out := make([]string, 0, len(values))
	for _, v := range values {
		for _, p := range strings.Split(v, ",") {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			if _, ok := seen[p]; ok {
				continue
			}
			seen[p] = struct{}{}
			out = append(out, p)
		}
	}
	return out
}

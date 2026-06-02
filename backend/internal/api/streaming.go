package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/image/draw"

	"github.com/skua-app/skua/internal/streamoverrides"
)

// handleWhep proxies a WHEP SDP offer to go2rtc and returns the SDP answer.
// It translates the cam_id URL parameter to the camera's main stream name.
func (h *Handler) handleWhep(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	camID := chi.URLParam(r, "cam_id")

	cam, ok := h.cameras.Find(camID)
	if !ok {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "camera not found", nil)
		return
	}
	if h.go2rtcURL == "" {
		writeError(w, h.logger, http.StatusServiceUnavailable, "go2rtc_unavailable", "go2rtc not configured", nil)
		return
	}

	quality := r.URL.Query().Get("quality")
	if quality == "" {
		quality = "main"
	}
	var override streamoverrides.Override
	if h.streamOverrides != nil {
		override = h.streamOverrides.Get(camID)
	}
	var (
		streamName   string
		usedOverride bool
	)
	switch quality {
	case "main":
		if override.Main != "" {
			streamName = override.Main
			usedOverride = true
		} else {
			streamName = cam.StreamMain
		}
	case "sub":
		if override.Sub != "" {
			streamName = override.Sub
			usedOverride = true
		} else {
			streamName = cam.StreamSub
		}
	default:
		writeError(w, h.logger, http.StatusBadRequest, "bad_request", "quality must be main or sub", nil)
		return
	}
	if streamName == "" {
		writeError(w, h.logger, http.StatusBadRequest, "no_stream", "no stream configured for quality", nil)
		return
	}

	ctx := r.Context()
	if h.whepTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, h.whepTimeout)
		defer cancel()
	}

	upstream := fmt.Sprintf("%s/api/webrtc?src=%s&type=whep", h.go2rtcURL, url.QueryEscape(streamName))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, upstream, r.Body)
	if err != nil {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "create upstream request", err)
		return
	}
	req.Header.Set("Content-Type", "application/sdp")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			writeError(w, h.logger, http.StatusGatewayTimeout, "upstream_timeout", "whep upstream timeout", err)
		} else {
			writeError(w, h.logger, http.StatusBadGateway, "upstream_error", "whep upstream error", err)
		}
		return
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			h.logger.Debug("failed to close whep upstream body", "error", err)
		}
	}()

	answer, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, h.logger, http.StatusBadGateway, "upstream_error", "read whep answer", err)
		return
	}

	h.logger.Debug("whep exchange",
		"cam_id", camID,
		"quality", quality,
		"stream_name", streamName,
		"override", usedOverride,
		"offer_size_bytes", r.ContentLength,
		"answer_size_bytes", len(answer),
		"upstream_status", resp.StatusCode,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(resp.StatusCode)
	if _, err := w.Write(answer); err != nil {
		h.logger.Debug("write whep answer failed", "error", err)
	}
}

// handleGetPrefs returns the current shared preferences as JSON.
func (h *Handler) handleGetPrefs(w http.ResponseWriter, r *http.Request) {
	p := h.prefsStore.Get()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(p); err != nil {
		h.logger.Error("encode prefs response", "error", err)
	}
}

// handleTileJPEG fetches latest.jpg from Frigate, resizes to 320 px wide, and
// returns it as a JPEG at quality 60. Used by the grid in eco mode to reduce
// bandwidth compared to the full-resolution snapshot.
func (h *Handler) handleTileJPEG(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, ok := h.cameras.Find(id); !ok {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "camera not found", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	body, _, err := h.frigate.GetSnapshot(ctx, id)
	if err != nil {
		writeError(w, h.logger, http.StatusBadGateway, "upstream_error", "upstream error", err)
		return
	}
	defer func() {
		if err := body.Close(); err != nil {
			h.logger.Debug("failed to close tile upstream body", "error", err)
		}
	}()

	src, err := jpeg.Decode(body)
	if err != nil {
		writeError(w, h.logger, http.StatusBadGateway, "upstream_error", "failed to decode jpeg", err)
		return
	}

	srcB := src.Bounds()
	if srcB.Dx() <= 0 || srcB.Dy() <= 0 {
		writeError(w, h.logger, http.StatusBadGateway, "upstream_error", "invalid source image dimensions", nil)
		return
	}
	dstW := 320
	dstH := srcB.Dy() * dstW / srcB.Dx()
	// Clamp dstH to 1 in case an extremely wide/short source rounds the
	// scaled height to zero — image.NewRGBA with a zero-area rect would
	// produce a blank tile that jpeg.Encode rejects.
	if dstH < 1 {
		dstH = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, srcB, draw.Over, nil)

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	if err := jpeg.Encode(w, dst, &jpeg.Options{Quality: 60}); err != nil {
		h.logger.Debug("failed to encode tile jpeg", "error", err)
	}
}

// handlePutPrefs accepts a partial JSON body and merges it into the stored prefs.
func (h *Handler) handlePutPrefs(w http.ResponseWriter, r *http.Request) {
	var partial map[string]any
	if err := json.NewDecoder(r.Body).Decode(&partial); err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_body", "invalid JSON body", err)
		return
	}

	updated, err := h.prefsStore.Update(partial)
	if err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_prefs", err.Error(), nil)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(updated); err != nil {
		h.logger.Error("encode prefs update response", "error", err)
	}
}

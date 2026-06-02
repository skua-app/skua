package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/skua-app/skua/internal/streamoverrides"
)

// go2rtcListTimeout bounds the upstream lookup used by both /api/go2rtc/streams
// and the validation step in PUT /api/stream-overrides/{cam_id}.
const go2rtcListTimeout = 5 * time.Second

// streamOverridesResponse is the JSON shape for GET /api/stream-overrides.
// It mirrors cameraNamesResponse: only cameras with at least one explicit
// override show up; the empty map serialises as {}, not null.
type streamOverridesResponse struct {
	Overrides map[string]streamoverrides.Override `json:"overrides"`
}

// putStreamOverrideRequest is the body shape for PUT /api/stream-overrides/{cam_id}.
// Both fields are required and may be empty strings; sending the entry with
// both fields blank clears the override.
type putStreamOverrideRequest struct {
	Main *string `json:"main"`
	Sub  *string `json:"sub"`
}

func (h *Handler) handleListGo2RTCStreams(w http.ResponseWriter, r *http.Request) {
	if h.go2rtc == nil {
		writeError(w, h.logger, http.StatusBadGateway, "go2rtc_unreachable", "go2rtc is unreachable", errors.New("go2rtc client not configured"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), go2rtcListTimeout)
	defer cancel()

	names, err := h.go2rtc.GetStreams(ctx)
	if err != nil {
		writeError(w, h.logger, http.StatusBadGateway, "go2rtc_unreachable", "go2rtc is unreachable", err)
		return
	}
	if names == nil {
		names = []string{}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(names); err != nil {
		h.logger.Error("encode go2rtc streams list", "error", err)
	}
}

func (h *Handler) handleListStreamOverrides(w http.ResponseWriter, _ *http.Request) {
	overrides := map[string]streamoverrides.Override{}
	if h.streamOverrides != nil {
		overrides = h.streamOverrides.All()
		if overrides == nil {
			overrides = map[string]streamoverrides.Override{}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(streamOverridesResponse{Overrides: overrides}); err != nil {
		h.logger.Error("encode stream overrides list", "error", err)
	}
}

// handlePutStreamOverride accepts PUT /api/stream-overrides/{cam_id} with
// body { main, sub } (both required strings; empty strings allowed). Sets
// the override pair atomically; sending both fields empty clears the entry.
// Each non-empty value is validated against the live go2rtc /api/streams
// list — unknown aliases return 400 stream_not_found with a message
// naming which field and value was rejected.
func (h *Handler) handlePutStreamOverride(w http.ResponseWriter, r *http.Request) {
	camID := chi.URLParam(r, "cam_id")
	if camID == "" {
		writeError(w, h.logger, http.StatusBadRequest, "missing_id", "Camera id is required", nil)
		return
	}
	if h.streamOverrides == nil {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "Internal error", errors.New("streamOverrides store not configured"))
		return
	}

	var req putStreamOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_body", "Invalid request body", err)
		return
	}
	if req.Main == nil || req.Sub == nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_body", "Invalid request body", errors.New("main and sub are required"))
		return
	}

	if _, ok := h.cameras.Find(camID); !ok {
		writeError(w, h.logger, http.StatusNotFound, "camera_not_found", "Unknown camera", nil)
		return
	}

	main := strings.TrimSpace(*req.Main)
	sub := strings.TrimSpace(*req.Sub)

	if main == "" && sub == "" {
		if err := h.streamOverrides.Forget(camID); err != nil {
			writeError(w, h.logger, http.StatusInternalServerError, "internal", "Internal error", err)
			return
		}
		writeStreamOverrideOK(w, h.logger, streamoverrides.Override{})
		return
	}

	if h.go2rtc == nil {
		writeError(w, h.logger, http.StatusBadGateway, "go2rtc_unreachable", "go2rtc is unreachable", errors.New("go2rtc client not configured"))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), go2rtcListTimeout)
	defer cancel()
	available, err := h.go2rtc.GetStreams(ctx)
	if err != nil {
		writeError(w, h.logger, http.StatusBadGateway, "go2rtc_unreachable", "go2rtc is unreachable", err)
		return
	}

	if main != "" && !slices.Contains(available, main) {
		msg := fmt.Sprintf("Main stream not found in go2rtc: %s", main)
		writeError(w, h.logger, http.StatusBadRequest, "stream_not_found", msg, nil)
		return
	}
	if sub != "" && !slices.Contains(available, sub) {
		msg := fmt.Sprintf("Sub stream not found in go2rtc: %s", sub)
		writeError(w, h.logger, http.StatusBadRequest, "stream_not_found", msg, nil)
		return
	}

	if err := h.streamOverrides.Set(camID, main, sub); err != nil {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "Internal error", err)
		return
	}
	writeStreamOverrideOK(w, h.logger, streamoverrides.Override{Main: main, Sub: sub})
}

func writeStreamOverrideOK(w http.ResponseWriter, logger *slog.Logger, saved streamoverrides.Override) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(saved); err != nil {
		logger.Error("encode stream override response", "error", err)
	}
}

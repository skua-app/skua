package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/skua-app/skua/internal/names"
)

// cameraNamesResponse is the JSON shape for GET /api/camera-names.
// It returns ONLY the overrides currently set; cameras without an
// override are absent. Default names live in /api/cameras (already
// merged) and the UI never re-derives them here.
type cameraNamesResponse struct {
	Names map[string]string `json:"names"`
}

type updateCameraNameRequest struct {
	Name string `json:"name"`
}

type updateCameraNameResponse struct {
	CamID string `json:"cam_id"`
	Name  string `json:"name"`
}

func (h *Handler) handleListCameraNames(w http.ResponseWriter, _ *http.Request) {
	overrides := map[string]string{}
	if h.names != nil {
		overrides = h.names.All()
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(cameraNamesResponse{Names: overrides}); err != nil {
		h.logger.Error("encode camera names list", "error", err)
	}
}

// handlePutCameraName accepts PUT /api/camera-names/{cam_id} with body
// {"name": "..."}. An empty trimmed name clears the override and the
// camera reverts to its config.Name. Response carries the merged result
// so the frontend doesn't need to re-fetch /api/cameras to read it.
func (h *Handler) handlePutCameraName(w http.ResponseWriter, r *http.Request) {
	camID := chi.URLParam(r, "cam_id")
	if camID == "" {
		writeError(w, h.logger, http.StatusBadRequest, "missing_id", "Camera id is required", nil)
		return
	}
	if h.names == nil {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "Internal error", errors.New("names store not configured"))
		return
	}

	var req updateCameraNameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_body", "Invalid request body", err)
		return
	}

	stored, err := h.names.Set(camID, req.Name)
	if err != nil {
		mapNameError(w, h.logger, err)
		return
	}

	// Resolve so the response carries the merged name (override if set,
	// otherwise the config default). UI binds straight to this.
	resolved := stored
	if resolved == "" {
		// Override cleared → look up the registry default.
		if cam, ok := h.cameras.Find(camID); ok {
			resolved = cam.Name
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(updateCameraNameResponse{CamID: camID, Name: resolved}); err != nil {
		h.logger.Error("encode camera name update", "error", err)
	}
}

func mapNameError(w http.ResponseWriter, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, names.ErrCameraNotFound):
		writeError(w, logger, http.StatusBadRequest, "camera_not_found", "Unknown camera", nil)
	case errors.Is(err, names.ErrNameTooLong):
		writeError(w, logger, http.StatusBadRequest, "name_too_long", "Name is too long (max 30 characters)", nil)
	default:
		writeError(w, logger, http.StatusInternalServerError, "internal", "Internal error", err)
	}
}

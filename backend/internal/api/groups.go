package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/skua-app/skua/internal/groups"
)

// groupResponse is the JSON shape returned for a single group.
type groupResponse struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	CameraIDs []string `json:"camera_ids"`
}

func toGroupResponse(g groups.Group) groupResponse {
	ids := g.CameraIDs
	if ids == nil {
		ids = []string{}
	}
	return groupResponse{ID: g.ID, Name: g.Name, CameraIDs: ids}
}

type createGroupRequest struct {
	Name string `json:"name"`
}

type updateGroupRequest struct {
	Name      *string   `json:"name,omitempty"`
	CameraIDs *[]string `json:"camera_ids,omitempty"`
}

func (h *Handler) handleListGroups(w http.ResponseWriter, _ *http.Request) {
	list := h.groups.List()
	out := make([]groupResponse, 0, len(list))
	for _, g := range list {
		out = append(out, toGroupResponse(g))
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(out); err != nil {
		h.logger.Error("encode groups list", "error", err)
	}
}

func (h *Handler) handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	var req createGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_body", "Invalid request body", err)
		return
	}
	g, err := h.groups.Create(req.Name)
	if err != nil {
		mapGroupError(w, h.logger, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(toGroupResponse(g)); err != nil {
		h.logger.Error("encode group create response", "error", err)
	}
}

func (h *Handler) handleUpdateGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, h.logger, http.StatusBadRequest, "missing_id", "Group id is required", nil)
		return
	}
	var req updateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_body", "Invalid request body", err)
		return
	}
	if req.Name == nil && req.CameraIDs == nil {
		writeError(w, h.logger, http.StatusBadRequest, "empty_patch", "Nothing to update", nil)
		return
	}
	g, err := h.groups.Update(id, req.Name, req.CameraIDs)
	if err != nil {
		mapGroupError(w, h.logger, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(toGroupResponse(g)); err != nil {
		h.logger.Error("encode group update response", "error", err)
	}
}

func (h *Handler) handleDeleteGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, h.logger, http.StatusBadRequest, "missing_id", "Group id is required", nil)
		return
	}
	if err := h.groups.Delete(id); err != nil {
		mapGroupError(w, h.logger, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// mapGroupError maps validation/not-found errors to HTTP responses with
// snake_case codes the frontend can react to inline.
func mapGroupError(w http.ResponseWriter, logger *slog.Logger, err error) {
	switch {
	case errors.Is(err, groups.ErrNotFound):
		writeError(w, logger, http.StatusNotFound, "not_found", "Group not found", nil)
	case errors.Is(err, groups.ErrNameEmpty):
		writeError(w, logger, http.StatusBadRequest, "name_empty", "Group name is required", nil)
	case errors.Is(err, groups.ErrNameTooLong):
		writeError(w, logger, http.StatusBadRequest, "name_too_long", "Name is too long (max 30 characters)", nil)
	case errors.Is(err, groups.ErrNameDuplicate):
		writeError(w, logger, http.StatusBadRequest, "name_duplicate", "A group with this name already exists", nil)
	case errors.Is(err, groups.ErrCameraNotFound):
		writeError(w, logger, http.StatusBadRequest, "camera_not_found", "Unknown camera in list", nil)
	case errors.Is(err, groups.ErrDuplicateCamera):
		writeError(w, logger, http.StatusBadRequest, "duplicate_camera", "List contains duplicate cameras", nil)
	default:
		writeError(w, logger, http.StatusInternalServerError, "internal", "Internal error", err)
	}
}

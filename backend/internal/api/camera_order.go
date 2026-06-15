package api

import (
	"encoding/json"
	"errors"
	"net/http"
)

// cameraOrderResponse is the JSON shape for GET/PUT /api/camera-order. It
// returns the effective saved order — unknown ids are stripped server-side
// at write time, so clients never need to reconcile a "ghost id" set.
type cameraOrderResponse struct {
	Order []string `json:"order"`
}

// putCameraOrderRequest carries the desired order as a list of cam ids.
// Duplicates are de-duplicated server-side (first occurrence wins) and
// unknown ids are silently dropped, mirroring the contract that the
// client sends its full desired order and the server is the validator.
type putCameraOrderRequest struct {
	Order []string `json:"order"`
}

func (h *Handler) handleGetCameraOrder(w http.ResponseWriter, _ *http.Request) {
	order := []string{}
	if h.cameraOrder != nil {
		order = h.cameraOrder.All()
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(cameraOrderResponse{Order: order}); err != nil {
		h.logger.Error("encode camera order list", "error", err)
	}
}

// handlePutCameraOrder replaces the saved camera order with the body's
// `order` list. Unknown ids are silently dropped server-side. The
// response carries the effective saved order so the client doesn't
// need to re-fetch.
func (h *Handler) handlePutCameraOrder(w http.ResponseWriter, r *http.Request) {
	if h.cameraOrder == nil {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "Internal error", errors.New("camera order store not configured"))
		return
	}
	var req putCameraOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_body", "Invalid request body", err)
		return
	}
	saved, err := h.cameraOrder.Set(req.Order)
	if err != nil {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "Failed to save camera order", err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(cameraOrderResponse{Order: saved}); err != nil {
		h.logger.Error("encode camera order update", "error", err)
	}
}

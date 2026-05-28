package api

import (
	"encoding/json"
	"net/http"
)

// appConfigResponse is the JSON shape for GET /api/config.
// Fields here are public — never put secrets in this payload.
type appConfigResponse struct {
	FrigateUIURL string `json:"frigate_ui_url"`
}

func (h *Handler) handleConfig(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(appConfigResponse{FrigateUIURL: h.frigateUIURL}); err != nil {
		h.logger.Error("encode config response", "error", err)
	}
}

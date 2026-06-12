package api

import (
	"encoding/json"
	"net/http"

	"github.com/skua-app/skua/internal/version"
)

// appConfigResponse is the JSON shape for GET /api/config.
// Fields here are public — never put secrets in this payload.
type appConfigResponse struct {
	FrigateUIURL string `json:"frigate_ui_url"`
	Version      string `json:"version"`
}

func (h *Handler) handleConfig(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	resp := appConfigResponse{
		FrigateUIURL: h.frigateUIURL,
		Version:      version.Version,
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("encode config response", "error", err)
	}
}

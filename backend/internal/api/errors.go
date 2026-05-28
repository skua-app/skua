package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

func writeError(w http.ResponseWriter, logger *slog.Logger, status int, msg string, err error) {
	if err != nil {
		logger.Error(msg, "error", err.Error(), "status", status)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

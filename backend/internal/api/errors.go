package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

// writeError emits the unified {error, message} body used by every BFF
// endpoint. `error` carries a stable snake_case code the frontend may
// switch on (e.g. not_found, upstream_timeout, invalid_id); `message`
// is a human-readable string suitable for display. `cause`, when non-nil,
// is logged but not sent to the client.
func writeError(w http.ResponseWriter, logger *slog.Logger, status int, code, message string, cause error) {
	if cause != nil {
		logger.Error(message, "error", cause.Error(), "code", code, "status", status)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": message})
}

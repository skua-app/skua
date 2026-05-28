package api

import (
	"fmt"
	"log/slog"
	"net/http"
)

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	if _, err := fmt.Fprint(w, "ok"); err != nil {
		// best-effort; client may have disconnected
		slog.Default().Debug("healthz write failed", "error", err)
	}
}

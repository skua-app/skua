// Package emergency serves a styled setup-required page when the BFF hits a
// startup blocker that would otherwise leave the operator looking at a
// connection-refused error in the browser.
//
// Two blockers route here:
//
//   - ReasonDataNotWritable: cameras.New saw fs.ErrPermission on the data
//     directory (typical cause: root-owned bind mount conflicting with the
//     distroless nonroot uid 65532).
//   - ReasonFrigateUnreachable: cameras.New could not pull the camera list
//     and there is no cameras.yaml snapshot on disk yet — i.e. first start
//     with Frigate down or misconfigured.
//
// Emergency mode serves only the styled HTML page (status 200 so it renders
// cleanly) plus a 503 on /healthz and on every /api/* path. The page polls
// /healthz every 3 seconds and reloads on a 200 response — only the normal
// server's healthz returns 200, so a restart into a healthy BFF auto-recovers
// any open browser tabs.
package emergency

import (
	_ "embed"
	"encoding/json"
	"html/template"
	"net/http"
	"strings"
	"time"
)

// Reason identifies which blocker triggered emergency mode. The string value
// is also what the /api/* fallback returns to clients.
type Reason string

const (
	ReasonDataNotWritable    Reason = "data_not_writable"
	ReasonFrigateUnreachable Reason = "frigate_unreachable"
)

//go:embed page.html
var pageHTML string

var pageTemplate = template.Must(template.New("emergency").Parse(pageHTML))

type pageData struct {
	Reason string
	Detail string
	Title  string
}

func titleFor(r Reason) string {
	switch r {
	case ReasonDataNotWritable:
		return "Skua — setup required"
	case ReasonFrigateUnreachable:
		return "Skua — waiting for Frigate"
	}
	return "Skua — setup required"
}

func apiMessageFor(r Reason) string {
	switch r {
	case ReasonDataNotWritable:
		return "Skua's data directory is not writable by the container user (uid/gid 65532)."
	case ReasonFrigateUnreachable:
		return "Skua cannot reach Frigate and has no on-disk camera snapshot to fall back to."
	}
	return "Skua is not ready."
}

// Handler returns the emergency-mode HTTP handler. Exposed so tests can drive
// it via httptest without binding a real port.
func Handler(reason Reason, detail string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			// 503 (not 200) so the page's polling JS knows we are still in
			// emergency mode; reload fires only when the normal server's
			// 200 "ok" response replaces this.
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("emergency"))

		case strings.HasPrefix(r.URL.Path, "/api/"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "setup_required",
				"reason":  string(reason),
				"message": apiMessageFor(reason),
			})

		default:
			data := pageData{
				Reason: string(reason),
				Detail: detail,
				Title:  titleFor(reason),
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Cache-Control", "no-store")
			w.WriteHeader(http.StatusOK)
			_ = pageTemplate.Execute(w, data)
		}
	})
}

// Serve binds an http.Server on :port and runs the emergency handler. It
// blocks until ListenAndServe returns (typically: the container is killed by
// docker restart sending SIGTERM, and the Go runtime tears the process down
// before ListenAndServe ever returns). A bind failure surfaces as the error
// return; the caller is expected to fall back to a stderr message and exit.
func Serve(port string, reason Reason, detail string) error {
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           Handler(reason, detail),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return srv.ListenAndServe()
}

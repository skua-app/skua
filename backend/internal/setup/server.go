// Package setup serves the first-run wizard for entering Frigate and
// go2rtc URLs from the browser instead of the .env file. It evolves the
// emergency page pattern: the same single-port handler, the same styled
// shell, the same /healthz-poll-and-reload script — plus an interactive
// panel that test-connects to the entered URLs and persists them to the
// runtimeconfig overlay (/data/config.yaml by default).
//
// Two entry modes:
//
//   - ModeInitial: cfg.NeedsSetup is true. Empty fields, no banner.
//   - ModeUnreachable: cameras.New saw a frigate-unreachable error AND
//     the URL is editable (came from the overlay file, not env). Fields
//     are prefilled with the current cfg values and an error banner sits
//     above the form.
//
// Env-locked URLs render read-only with a "set via FRIGATE_URL
// environment variable" note and are excluded from save POSTs — so the
// operator can't try to overwrite a value the container env will win
// next boot anyway.
//
// Reconfiguration is restart-based, not hot-reload. After a successful
// save the handler closes a restart channel; Serve calls Shutdown and
// returns, and main.go exits 0. The container's restart policy
// (unless-stopped/always) brings the process back up against the new
// config file.
package setup

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/skua-app/skua/internal/frigate"
	"github.com/skua-app/skua/internal/go2rtc"
	"github.com/skua-app/skua/internal/runtimeconfig"
)

// Mode discriminates the wizard's two entry paths.
type Mode string

const (
	ModeInitial     Mode = "initial"
	ModeUnreachable Mode = "unreachable"
)

// Options configures the wizard handler.
type Options struct {
	Mode        Mode
	Prefill     runtimeconfig.Values
	Locked      LockedFields
	ErrMessage  string        // shown in a banner on ModeUnreachable
	TestTimeout time.Duration // per-target probe timeout; defaults to 3s
}

// LockedFields tells the wizard which inputs to render read-only. True
// here means "this field is set via an environment variable" — the form
// strips locked values from POSTs and the operator sees a hint instead
// of an editable input.
type LockedFields struct {
	FrigateURL   bool
	FrigateUIURL bool
	Go2RTCURL    bool
}

//go:embed page.html
var pageHTML string

var pageTemplate = template.Must(template.New("setup").Parse(pageHTML))

type pageData struct {
	Title        string
	Mode         string
	ErrMessage   string
	FrigateURL   string
	FrigateUIURL string
	Go2RTCURL    string
	LockFrigate  bool
	LockUI       bool
	LockGo2RTC   bool
}

type testRequest struct {
	FrigateURL string `json:"frigate_url"`
	Go2RTCURL  string `json:"go2rtc_url"`
}

type targetResult struct {
	OK      bool   `json:"ok"`
	Skipped bool   `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
}

type testResponse struct {
	Frigate targetResult `json:"frigate"`
	Go2RTC  targetResult `json:"go2rtc"`
}

type saveRequest struct {
	FrigateURL   string `json:"frigate_url"`
	FrigateUIURL string `json:"frigate_ui_url"`
	Go2RTCURL    string `json:"go2rtc_url"`
}

type apiError struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// Handler returns the wizard's HTTP handler. The restart channel is
// closed exactly once on the first successful save; Serve uses it as the
// shutdown signal.
func Handler(store *runtimeconfig.Store, opts Options, restart chan<- struct{}) http.Handler {
	if opts.TestTimeout <= 0 {
		opts.TestTimeout = 3 * time.Second
	}

	var once sync.Once
	signalRestart := func() {
		once.Do(func() {
			if restart != nil {
				close(restart)
			}
		})
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		// 503 so the page's polling JS only reloads after the normal
		// server replaces this handler with one that answers 200.
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("setup"))
	})

	mux.HandleFunc("/api/setup/test", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
			return
		}
		var req testRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
			return
		}
		resp := testResponse{
			Frigate: probeFrigate(r.Context(), req.FrigateURL, opts.TestTimeout),
			Go2RTC:  probeGo2RTC(r.Context(), req.Go2RTCURL, opts.TestTimeout),
		}
		writeJSON(w, http.StatusOK, resp)
	})

	mux.HandleFunc("/api/setup/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeAPIError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
			return
		}
		var req saveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
			return
		}

		values := runtimeconfig.Values{
			FrigateURL:   strings.TrimSpace(req.FrigateURL),
			FrigateUIURL: strings.TrimSpace(req.FrigateUIURL),
			Go2RTCURL:    strings.TrimSpace(req.Go2RTCURL),
		}

		// Drop env-locked fields server-side so a tampered POST can't
		// write a stale value into the overlay file. The next boot would
		// ignore them anyway, but storing them would confuse a future
		// operator reading config.yaml.
		if opts.Locked.FrigateURL {
			values.FrigateURL = opts.Prefill.FrigateURL
		}
		if opts.Locked.FrigateUIURL {
			values.FrigateUIURL = opts.Prefill.FrigateUIURL
		}
		if opts.Locked.Go2RTCURL {
			values.Go2RTCURL = opts.Prefill.Go2RTCURL
		}

		if values.FrigateURL == "" {
			writeAPIError(w, http.StatusBadRequest, "frigate_url_required", "Frigate URL is required.")
			return
		}
		if err := validateURL(values.FrigateURL); err != nil {
			writeAPIError(w, http.StatusBadRequest, "frigate_url_invalid", fmt.Sprintf("Frigate URL: %s", err))
			return
		}
		if values.Go2RTCURL != "" {
			if err := validateURL(values.Go2RTCURL); err != nil {
				writeAPIError(w, http.StatusBadRequest, "go2rtc_url_invalid", fmt.Sprintf("go2rtc URL: %s", err))
				return
			}
		}
		if values.FrigateUIURL != "" {
			if err := validateURL(values.FrigateUIURL); err != nil {
				writeAPIError(w, http.StatusBadRequest, "frigate_ui_url_invalid", fmt.Sprintf("Frigate UI URL: %s", err))
				return
			}
		}

		if err := store.Save(values); err != nil {
			if errors.Is(err, fs.ErrPermission) {
				writeAPIError(w, http.StatusInternalServerError, "data_not_writable",
					"Skua's data directory is not writable by the container user (uid/gid 65532). "+
						"Run chown -R 65532:65532 on the host data directory, or switch to a named Docker volume.")
				return
			}
			writeAPIError(w, http.StatusInternalServerError, "save_failed", err.Error())
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"status": "saved"})
		signalRestart()
	})

	// Catch-all /api/* under setup mode: always 503, never proxy.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		writeAPIError(w, http.StatusServiceUnavailable, "setup_required",
			"Skua is in first-run setup mode. Open / to configure.")
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data := pageData{
			Title:        titleFor(opts.Mode),
			Mode:         string(opts.Mode),
			ErrMessage:   opts.ErrMessage,
			FrigateURL:   opts.Prefill.FrigateURL,
			FrigateUIURL: opts.Prefill.FrigateUIURL,
			Go2RTCURL:    opts.Prefill.Go2RTCURL,
			LockFrigate:  opts.Locked.FrigateURL,
			LockUI:       opts.Locked.FrigateUIURL,
			LockGo2RTC:   opts.Locked.Go2RTCURL,
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Robots-Tag", "noindex")
		w.WriteHeader(http.StatusOK)
		_ = pageTemplate.Execute(w, data)
	})

	return mux
}

// Serve binds an http.Server on :port, runs the wizard handler, and
// blocks until either ListenAndServe returns (bind error / process
// killed) or a successful save closes the restart channel. The latter
// triggers a graceful Shutdown and returns nil so main.go can exit 0.
func Serve(port string, store *runtimeconfig.Store, opts Options) error {
	restart := make(chan struct{})
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           Handler(store, opts, restart),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	listenErr := make(chan error, 1)
	go func() {
		err := srv.ListenAndServe()
		if errors.Is(err, http.ErrServerClosed) {
			listenErr <- nil
			return
		}
		listenErr <- err
	}()

	select {
	case <-restart:
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
		return <-listenErr
	case err := <-listenErr:
		return err
	}
}

func titleFor(m Mode) string {
	switch m {
	case ModeUnreachable:
		return "Skua — finish setup"
	default:
		return "Skua — first-run setup"
	}
}

func probeFrigate(ctx context.Context, raw string, timeout time.Duration) targetResult {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return targetResult{Error: "Frigate URL is empty"}
	}
	if err := validateURL(trimmed); err != nil {
		return targetResult{Error: err.Error()}
	}
	base := strings.TrimRight(trimmed, "/")

	client := frigate.NewClient(base, timeout)
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if _, err := client.GetStats(probeCtx); err != nil {
		return targetResult{Error: humanError(err)}
	}
	return targetResult{OK: true}
}

func probeGo2RTC(ctx context.Context, raw string, timeout time.Duration) targetResult {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return targetResult{Skipped: true}
	}
	if err := validateURL(trimmed); err != nil {
		return targetResult{Error: err.Error()}
	}
	base := strings.TrimRight(trimmed, "/")

	httpClient := &http.Client{Timeout: timeout}
	client := go2rtc.New(base, httpClient)
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if _, err := client.GetStreams(probeCtx); err != nil {
		return targetResult{Error: humanError(err)}
	}
	return targetResult{OK: true}
}

func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("not a valid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("must start with http:// or https://")
	}
	if u.Host == "" {
		return fmt.Errorf("missing host")
	}
	return nil
}

// humanError strips the deepest layer of network noise off a wrapped
// error so the wizard panel shows a short, readable line. Go's net
// package nests several layers (op > addr > dial > syscall); the
// outermost frame is usually the most useful.
func humanError(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, ": "); i > 0 && i < len(msg)-2 {
		// keep last segment ("connection refused", "no such host", ...)
		last := msg
		for {
			j := strings.Index(last, ": ")
			if j < 0 || j >= len(last)-2 {
				break
			}
			last = last[j+2:]
		}
		if last != "" {
			return last
		}
	}
	return msg
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, apiError{Error: code, Message: message})
}

// Package probe encapsulates the ephemeral test-connect logic used by the
// first-run setup wizard (internal/setup) and the runtime-config editor
// (api /api/runtime-config/test). Both paths need to spin up a temporary
// frigate.Client / go2rtc.Client, call a known endpoint with a short
// per-call timeout, and report a structured result — but they don't share
// any state otherwise. Pulling this into a neutral package lets either
// caller drive a probe without dragging the other's dependencies in.
//
// External behaviour mirrors what internal/setup carried before the
// extraction:
//   - empty Frigate URL → Result with Error "Frigate URL is empty"
//   - empty go2rtc URL  → Result with Skipped: true
//   - invalid URL       → validation error in Result.Error
//   - upstream error    → last segment of the wrapped error (humanError)
//   - success           → Result with OK: true
package probe

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/skua-app/skua/internal/frigate"
	"github.com/skua-app/skua/internal/go2rtc"
)

// Result carries one upstream probe outcome. Skipped is only meaningful
// for optional targets (currently go2rtc) when the caller passes an
// empty URL — the upstream wasn't tried, so OK is false and Error is
// empty.
type Result struct {
	OK      bool   `json:"ok"`
	Skipped bool   `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Report bundles both probe results. The wizard and the runtime-config
// editor return this shape directly to the browser.
type Report struct {
	Frigate Result `json:"frigate"`
	Go2RTC  Result `json:"go2rtc"`
}

// Frigate probes the Frigate REST API at rawURL by calling GetStats with
// a per-call context timeout. Returns the structured outcome; never
// panics on input. An empty rawURL is a hard error (Frigate is required).
func Frigate(ctx context.Context, rawURL string, timeout time.Duration) Result {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return Result{Error: "Frigate URL is empty"}
	}
	if err := ValidateURL(trimmed); err != nil {
		return Result{Error: err.Error()}
	}
	base := strings.TrimRight(trimmed, "/")

	client := frigate.NewClient(base, timeout)
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if _, err := client.GetStats(probeCtx); err != nil {
		return Result{Error: humanError(err)}
	}
	return Result{OK: true}
}

// Go2RTC probes the go2rtc REST API at rawURL by calling GetStreams
// with a per-call context timeout. An empty rawURL is treated as
// "operator chose to skip" — Result.Skipped is true and Error is empty.
func Go2RTC(ctx context.Context, rawURL string, timeout time.Duration) Result {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return Result{Skipped: true}
	}
	if err := ValidateURL(trimmed); err != nil {
		return Result{Error: err.Error()}
	}
	base := strings.TrimRight(trimmed, "/")

	httpClient := &http.Client{Timeout: timeout}
	client := go2rtc.New(base, httpClient)
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	if _, err := client.GetStreams(probeCtx); err != nil {
		return Result{Error: humanError(err)}
	}
	return Result{OK: true}
}

// ValidateURL enforces a permissive but non-empty URL shape: parseable,
// http or https scheme, host present. Trailing-slash normalisation is
// the caller's job — see internal/config.
func ValidateURL(raw string) error {
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
// error so the wizard / editor shows a short, readable line. Go's net
// package nests several layers (op > addr > dial > syscall); the
// innermost frame ("connection refused", "no such host", ...) is the
// most useful for an operator.
func humanError(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, ": "); i > 0 && i < len(msg)-2 {
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

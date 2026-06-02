package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/skua-app/skua/internal/cameras"
	"github.com/skua-app/skua/internal/capabilities"
	"github.com/skua-app/skua/internal/config"
	"github.com/skua-app/skua/internal/frigate"
	applog "github.com/skua-app/skua/internal/log"
	"github.com/skua-app/skua/internal/sse"
)

// newCSRFRouter builds the real router with the minimum dependencies the
// CSRF guard tests need. The cameras store is wired against a fake
// always-failing Frigate so same-origin POST /api/cameras/refresh exits
// cleanly through the 502 path (not a nil-deref panic), which keeps the
// "passed the guard" assertion focused on the middleware rather than the
// recovery middleware.
func newCSRFRouter(t *testing.T) http.Handler {
	t.Helper()
	logger := applog.New("error", "text")

	fakeFrigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(fakeFrigate.Close)

	frigateClient := frigate.NewClient(fakeFrigate.URL, &http.Client{Timeout: 2 * time.Second})
	camerasStore := cameras.NewForTestWithFrigate(
		filepath.Join(t.TempDir(), "cameras.yaml"),
		frigateClient,
		[]config.CameraSpec{{ID: "cam1", Name: "cam1", StreamMain: "cam1_main"}},
	)

	h := NewHandler(HandlerDeps{
		Logger:       logger,
		Frigate:      frigateClient,
		Cameras:      camerasStore,
		HTTPClient:   &http.Client{},
		Capabilities: capabilities.NewForTest(nil),
		Runtime:      RuntimeConfigDeps{RequestRestart: func() {}},
	})
	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	return NewRouter(h, sse.NewHub(logger), logger, staticFS)
}

// crossSiteBlocked reports whether rec is specifically the 403 emitted by
// the cross-site guard, not some coincidental 403 from a downstream
// handler. Body absence is treated as "not blocked".
func crossSiteBlocked(rec *httptest.ResponseRecorder) bool {
	if rec.Code != http.StatusForbidden {
		return false
	}
	var body struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		return false
	}
	return body.Error == "cross_site_blocked"
}

func TestCrossSiteGuard_RejectsCrossSiteMutating(t *testing.T) {
	r := newCSRFRouter(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"POST refresh", http.MethodPost, "/api/cameras/refresh", ""},
		{"POST restart", http.MethodPost, "/api/runtime-config/restart", ""},
		{"PUT prefs", http.MethodPut, "/api/prefs", `{}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var bodyReader *strings.Reader
			if c.body != "" {
				bodyReader = strings.NewReader(c.body)
			}
			var req *http.Request
			if bodyReader != nil {
				req = httptest.NewRequest(c.method, c.path, bodyReader)
				req.Header.Set("Content-Type", "application/json")
			} else {
				req = httptest.NewRequest(c.method, c.path, nil)
			}
			req.Header.Set("Sec-Fetch-Site", "cross-site")
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)

			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
			}
			var body struct {
				Error   string `json:"error"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode body: %v; raw = %s", err, rec.Body.String())
			}
			if body.Error != "cross_site_blocked" {
				t.Errorf("error code = %q, want cross_site_blocked", body.Error)
			}
			if body.Message == "" {
				t.Errorf("message must be non-empty")
			}
		})
	}
}

func TestCrossSiteGuard_AllowsSameOriginMutating(t *testing.T) {
	r := newCSRFRouter(t)
	req := httptest.NewRequest(http.MethodPost, "/api/cameras/refresh", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if crossSiteBlocked(rec) {
		t.Fatalf("same-origin POST was blocked as cross-site (status %d body %s)",
			rec.Code, rec.Body.String())
	}
}

func TestCrossSiteGuard_AllowsAbsentHeader(t *testing.T) {
	r := newCSRFRouter(t)
	// POST /api/runtime-config/restart succeeds with the test rig's
	// RequestRestart stub, giving a clean non-403 response shape.
	req := httptest.NewRequest(http.MethodPost, "/api/runtime-config/restart", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if crossSiteBlocked(rec) {
		t.Fatalf("POST with no Sec-Fetch-Site was blocked as cross-site (status %d body %s)",
			rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202 (fail-open path should reach handler)", rec.Code)
	}
}

func TestCrossSiteGuard_SafeMethodsBypass(t *testing.T) {
	r := newCSRFRouter(t)
	// GET /api/config is a real, store-free endpoint and the guard must
	// pass it through regardless of the Sec-Fetch-Site value, since the
	// drive-by CSRF risk only applies to state-changing methods.
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if crossSiteBlocked(rec) {
		t.Fatalf("cross-site GET was blocked (status %d body %s)",
			rec.Code, rec.Body.String())
	}
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (safe method must pass guard)", rec.Code)
	}
}

func TestCrossSiteGuard_AllowsSameSiteAndNone(t *testing.T) {
	r := newCSRFRouter(t)
	for _, site := range []string{"same-site", "none"} {
		t.Run(site, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/runtime-config/restart", nil)
			req.Header.Set("Sec-Fetch-Site", site)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if crossSiteBlocked(rec) {
				t.Fatalf("Sec-Fetch-Site %q was blocked (status %d body %s)",
					site, rec.Code, rec.Body.String())
			}
		})
	}
}

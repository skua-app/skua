package emergency

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandler covers both reasons against the three response branches the
// emergency handler implements: HTML page, /healthz, /api/*. Exercising the
// HTML render here also catches any template syntax error at go test time
// rather than at process boot in production.
func TestHandler(t *testing.T) {
	cases := []struct {
		name         string
		reason       Reason
		detail       string
		pageContains []string
	}{
		{
			name:   "data_not_writable",
			reason: ReasonDataNotWritable,
			detail: "/data",
			pageContains: []string{
				"Skua can't write to its data directory",
				"65532",
				"chown -R 65532:65532 /data",
				"docker compose restart skua",
				"Troubleshooting",
			},
		},
		{
			name:   "frigate_unreachable",
			reason: ReasonFrigateUnreachable,
			detail: "http://10.0.0.5:5000",
			pageContains: []string{
				"Skua can't reach Frigate",
				"http://10.0.0.5:5000",
				"FRIGATE_URL",
				"5000",
				"docker compose restart skua",
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			h := Handler(tc.reason, tc.detail)

			// GET / → 200 text/html, body carries the reason-specific copy.
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("GET / status = %d, want 200", rr.Code)
			}
			if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Fatalf("GET / Content-Type = %q, want text/html", ct)
			}
			body, _ := io.ReadAll(rr.Body)
			for _, want := range tc.pageContains {
				if !strings.Contains(string(body), want) {
					t.Errorf("GET / body missing %q", want)
				}
			}

			// A deep SPA route should still render the page (catch-all default).
			req = httptest.NewRequest(http.MethodGet, "/cam/cam1", nil)
			rr = httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("GET /cam/cam1 status = %d, want 200", rr.Code)
			}

			// /healthz → 503 so the polling JS on the page does NOT reload
			// until the normal server takes over and returns 200.
			req = httptest.NewRequest(http.MethodGet, "/healthz", nil)
			rr = httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusServiceUnavailable {
				t.Errorf("GET /healthz status = %d, want 503", rr.Code)
			}

			// /api/cameras → 503 JSON envelope.
			req = httptest.NewRequest(http.MethodGet, "/api/cameras", nil)
			rr = httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if rr.Code != http.StatusServiceUnavailable {
				t.Fatalf("GET /api/cameras status = %d, want 503", rr.Code)
			}
			if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
				t.Fatalf("GET /api/cameras Content-Type = %q, want application/json", ct)
			}
			var resp map[string]string
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("decode /api/cameras JSON: %v", err)
			}
			if resp["error"] != "setup_required" {
				t.Errorf("api error = %q, want %q", resp["error"], "setup_required")
			}
			if resp["reason"] != string(tc.reason) {
				t.Errorf("api reason = %q, want %q", resp["reason"], tc.reason)
			}
			if resp["message"] == "" {
				t.Error("api message is empty")
			}
		})
	}
}

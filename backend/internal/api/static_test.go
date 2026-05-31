package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/skua-app/skua/internal/capabilities"
	applog "github.com/skua-app/skua/internal/log"
	"github.com/skua-app/skua/internal/sse"
)

// testSpaFS returns a minimal in-memory FS that mirrors the real embed layout.
func testSpaFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html": {
			Data: []byte("<!doctype html><html><body>app</body></html>"),
		},
		"favicon.ico": {
			Data: []byte("\x00\x00\x01\x00"), // stub ICO header
		},
		"manifest.webmanifest": {
			Data: []byte(`{"name":"cams"}`),
		},
		"icons/icon-192.png": {
			Data: []byte("\x89PNG\r\n\x1a\n"), // stub PNG header
		},
		"_app/immutable/entry/start.fake.js": {
			Data: []byte("// js chunk"),
		},
	}
}

func testRouter(fsys fstest.MapFS) http.Handler {
	logger := applog.New("error", "text")
	h := NewHandler(logger, nil, nil, nil, nil, "", nil, "", 0, &http.Client{}, nil, nil, nil, capabilities.NewForTest(nil), nil, RuntimeConfigDeps{})
	return NewRouter(h, sse.NewHub(logger), logger, fsys)
}

func TestSPA_RootServesHTML(t *testing.T) {
	srv := httptest.NewServer(testRouter(testSpaFS()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("close body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "<!doctype html") {
		t.Fatalf("body does not contain <!doctype html: %q", body[:min(len(body), 80)])
	}
}

func TestSPA_RoutePathFallsBackToHTML(t *testing.T) {
	srv := httptest.NewServer(testRouter(testSpaFS()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/cam/cam5")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("close body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("want text/html content-type, got %q", ct)
	}
}

func TestSPA_MissingJSChunkReturns404(t *testing.T) {
	srv := httptest.NewServer(testRouter(testSpaFS()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/_app/immutable/entry/missing.CKeaS50O.js")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("close body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404 for missing JS chunk, got %d", resp.StatusCode)
	}
}

func TestSPA_MissingCSSReturns404(t *testing.T) {
	srv := httptest.NewServer(testRouter(testSpaFS()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/foo.css")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("close body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404 for missing CSS, got %d", resp.StatusCode)
	}
}

func TestSPA_IconServed(t *testing.T) {
	srv := httptest.NewServer(testRouter(testSpaFS()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/icons/icon-192.png")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("close body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for existing icon, got %d", resp.StatusCode)
	}
}

func TestSPA_ManifestServed(t *testing.T) {
	srv := httptest.NewServer(testRouter(testSpaFS()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/manifest.webmanifest")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("close body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
}

func TestSPA_APINotFoundReturns404(t *testing.T) {
	srv := httptest.NewServer(testRouter(testSpaFS()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/nonexistent")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("close body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404 for unknown API path, got %d", resp.StatusCode)
	}
}

func TestSPA_FaviconServed(t *testing.T) {
	srv := httptest.NewServer(testRouter(testSpaFS()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/favicon.ico")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Logf("close body: %v", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for favicon.ico, got %d", resp.StatusCode)
	}
}

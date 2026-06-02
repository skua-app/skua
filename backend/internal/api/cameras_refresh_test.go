package api

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
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

// frigateConfigJSON renders the cameras subtree of /api/config from a flat
// {id: mainAlias} map; enabled is always true.
func frigateConfigJSON(cams map[string]string) string {
	body := `{"cameras":{`
	first := true
	for id, main := range cams {
		if !first {
			body += ","
		}
		first = false
		body += fmt.Sprintf(`%q:{"name":%q,"enabled":true,"live":{"streams":{"Main":%q,"Sub":%q}}}`,
			id, id, main, id+"_sub")
	}
	body += `}}`
	return body
}

func newRefreshRouter(
	t *testing.T,
	frigateURL string,
	initialSpecs []config.CameraSpec,
) (http.Handler, *cameras.Store, *sse.Hub) {
	t.Helper()
	logger := applog.New("error", "text")
	client := frigate.NewClient(frigateURL, &http.Client{Timeout: 5 * time.Second})
	camerasStore := cameras.NewForTestWithFrigate(filepath.Join(t.TempDir(), "cameras.yaml"), client, initialSpecs)
	checker := makeChecker(client, initialSpecs, map[string]bool{})
	hub := sse.NewHub(logger)

	// Wire hooks the same way main.go does so the test exercises the real
	// fan-out path. capabilitiesStore is a no-op tmp store.
	capsStore := capabilities.NewForTest(nil)
	camerasStore.SetOnAdded(func(specs []cameras.CameraSpec) {
		for _, spec := range specs {
			hub.Broadcast("camera.added", map[string]string{"cam_id": spec.ID, "name": spec.Name})
		}
	})
	camerasStore.SetOnRemoved(func(ids []string) {
		for _, id := range ids {
			hub.Broadcast("camera.removed", map[string]string{"cam_id": id})
		}
	})

	h := NewHandler(HandlerDeps{
		Logger:       logger,
		Frigate:      client,
		Checker:      checker,
		Cameras:      camerasStore,
		HTTPClient:   &http.Client{},
		Capabilities: capsStore,
	})
	staticFS := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	return NewRouter(h, hub, logger, staticFS), camerasStore, hub
}

// readSSEEvent consumes one "event: <name>\ndata: <json>\n\n" block from r.
// Returns ("", "", false) on timeout or EOF.
func readSSEEvent(t *testing.T, r *bufio.Reader, timeout time.Duration) (name, data string, ok bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		line, err := r.ReadString('\n')
		if err != nil {
			return "", "", false
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "event: ") {
			name = strings.TrimPrefix(line, "event: ")
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
			continue
		}
		if line == "" && name != "" {
			return name, data, true
		}
	}
	return "", "", false
}

func TestCamerasRefresh_AddedBroadcastsAndReturnsDiff(t *testing.T) {
	frigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/config" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(frigateConfigJSON(map[string]string{
			"cam1": "cam1_main_h264",
			"cam2": "cam2_main_h264",
		})))
	}))
	defer frigate.Close()

	router, _, hub := newRefreshRouter(t, frigate.URL, []config.CameraSpec{
		{ID: "cam1", Name: "cam1", StreamMain: "cam1_main_h264"},
	})

	// Spin up the SSE handler so we can capture the broadcast. Use the
	// /api/stream route via the router. ResponseRecorder doesn't flush, so
	// we use httptest.NewServer wrapping the router.
	srv := httptest.NewServer(router)
	defer srv.Close()

	streamCtx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()
	streamReq, _ := http.NewRequestWithContext(streamCtx, http.MethodGet, srv.URL+"/api/stream", nil)
	streamResp, err := http.DefaultClient.Do(streamReq)
	if err != nil {
		t.Fatalf("stream connect: %v", err)
	}
	defer func() { _ = streamResp.Body.Close() }()

	// Wait briefly for the hub to register the subscriber.
	deadline := time.Now().Add(2 * time.Second)
	for hub.SubscriberCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if hub.SubscriberCount() == 0 {
		t.Fatal("SSE client never registered with hub")
	}

	// Trigger the refresh.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/cameras/refresh", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", w.Code, w.Body.String())
	}
	var diff refreshDiffResponse
	if err := json.NewDecoder(w.Body).Decode(&diff); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(diff.Added) != 1 || diff.Added[0] != "cam2" {
		t.Errorf("added = %v, want [cam2]", diff.Added)
	}
	if len(diff.Removed) != 0 {
		t.Errorf("removed = %v, want []", diff.Removed)
	}

	br := bufio.NewReader(streamResp.Body)
	name, data, ok := readSSEEvent(t, br, 2*time.Second)
	if !ok {
		t.Fatal("no SSE event received within 2s")
	}
	if name != "camera.added" {
		t.Errorf("event name = %q, want camera.added", name)
	}
	if !strings.Contains(data, `"cam_id":"cam2"`) {
		t.Errorf("event payload missing cam_id: %s", data)
	}
}

func TestCamerasRefresh_FrigateErrorReturns502NoBroadcast(t *testing.T) {
	frigate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer frigate.Close()

	router, store, hub := newRefreshRouter(t, frigate.URL, []config.CameraSpec{
		{ID: "cam1", Name: "cam1", StreamMain: "cam1_main_h264"},
	})

	// Count broadcasts via a fan-out subscriber. The hub fans out per-message,
	// so subscribing and then triggering a refresh that fails must yield zero
	// messages on the subscriber channel.
	client := hub.Subscribe()
	defer hub.Unsubscribe(client)
	var received int32
	go func() {
		for range client.C {
			atomic.AddInt32(&received, 1)
		}
	}()

	before := store.Snapshot()

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/cameras/refresh", nil)
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body %s", w.Code, w.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["error"] != "frigate_unreachable" {
		t.Errorf("error code = %q, want frigate_unreachable", body["error"])
	}

	if got := store.Snapshot(); len(got) != len(before) || got[0].ID != before[0].ID {
		t.Errorf("in-memory snapshot mutated after failure: %v vs %v", got, before)
	}

	// Give the broadcast goroutine a moment to flush any pending sends.
	time.Sleep(50 * time.Millisecond)
	if n := atomic.LoadInt32(&received); n != 0 {
		t.Errorf("expected 0 broadcasts, got %d", n)
	}
}

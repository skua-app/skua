package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/skua-app/skua/internal/cameras"
	"github.com/skua-app/skua/internal/capabilities"
	"github.com/skua-app/skua/internal/events"
	"github.com/skua-app/skua/internal/frigate"
	"github.com/skua-app/skua/internal/go2rtc"
	"github.com/skua-app/skua/internal/groups"
	"github.com/skua-app/skua/internal/names"
	"github.com/skua-app/skua/internal/prefs"
	"github.com/skua-app/skua/internal/runtimeconfig"
	"github.com/skua-app/skua/internal/streamoverrides"
)

// cameraResponse is the JSON shape for GET /api/cameras elements.
type cameraResponse struct {
	ID           string               `json:"id"`
	Name         string               `json:"name"`
	Online       bool                 `json:"online"`
	SnapshotURL  string               `json:"snapshot_url"`
	Capabilities capabilitiesResponse `json:"capabilities"`
	Streams      streamsResponse      `json:"streams"`
	Groups       []string             `json:"groups"`
}

type capabilitiesResponse struct {
	TalkBack bool `json:"talk_back"`
	PTZ      bool `json:"ptz"`
}

type streamsResponse struct {
	Main string `json:"main"`
	Sub  string `json:"sub"`
}

// OnlineChecker maintains a background-refreshed cache of camera online status.
// It polls /api/stats every ttl and marks each camera online iff camera_fps > 0.
// When detection is disabled on a camera, it falls back to a snapshot probe.
type OnlineChecker struct {
	client  *frigate.Client
	cameras *cameras.Store
	ttl     time.Duration
	logger  *slog.Logger
	mu      sync.RWMutex
	status  map[string]bool
	nowFn   func() time.Time // injectable for testing; currently unused in logic
}

func NewOnlineChecker(client *frigate.Client, camerasStore *cameras.Store, ttl time.Duration, logger *slog.Logger) *OnlineChecker {
	oc := &OnlineChecker{
		client:  client,
		cameras: camerasStore,
		ttl:     ttl,
		logger:  logger,
		status:  make(map[string]bool),
		nowFn:   time.Now,
	}
	oc.refreshAll()
	go oc.loop()
	return oc
}

func (oc *OnlineChecker) loop() {
	ticker := time.NewTicker(oc.ttl)
	defer ticker.Stop()
	for range ticker.C {
		oc.refreshAll()
	}
}

// onlineResult carries the result of a per-camera online check.
type onlineResult struct {
	id     string
	online bool
}

func (oc *OnlineChecker) refreshAll() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cams := oc.cameras.Snapshot()

	stats, err := oc.client.GetStats(ctx)
	if err != nil {
		oc.logger.Warn("frigate stats unavailable, marking all cameras offline", "error", err)
		oc.mu.Lock()
		oc.pruneLocked(cams)
		for _, cam := range cams {
			oc.status[cam.ID] = false
		}
		oc.mu.Unlock()
		return
	}

	// Separate cameras into synchronous results and those needing a snapshot probe.
	syncResults := make([]onlineResult, 0, len(cams))
	probeIDs := make([]string, 0)

	for _, cam := range cams {
		cs, ok := stats.Cameras[cam.ID]
		if !ok {
			syncResults = append(syncResults, onlineResult{cam.ID, false})
			continue
		}
		if cs.CameraFPS > 0 {
			syncResults = append(syncResults, onlineResult{cam.ID, true})
			continue
		}
		if !cs.DetectionEnabled {
			// fps may be 0 when Frigate detection is off; fall back to snapshot probe
			probeIDs = append(probeIDs, cam.ID)
			continue
		}
		syncResults = append(syncResults, onlineResult{cam.ID, false})
	}

	// Fan-out snapshot probes for detection-disabled cameras.
	probeCh := make(chan onlineResult, len(probeIDs))
	for _, id := range probeIDs {
		go func(camID string) {
			probeCtx, probeCancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer probeCancel()
			probeCh <- onlineResult{camID, oc.client.ProbeSnapshot(probeCtx, camID)}
		}(id)
	}

	oc.mu.Lock()
	oc.pruneLocked(cams)
	for _, r := range syncResults {
		oc.status[r.id] = r.online
	}
	for range probeIDs {
		r := <-probeCh
		oc.status[r.id] = r.online
	}
	oc.mu.Unlock()
}

// pruneLocked removes status entries for cameras no longer present in the
// snapshot. Caller must hold oc.mu for writing.
func (oc *OnlineChecker) pruneLocked(cams []cameras.CameraSpec) {
	if len(oc.status) == 0 {
		return
	}
	present := make(map[string]struct{}, len(cams))
	for _, c := range cams {
		present[c.ID] = struct{}{}
	}
	for id := range oc.status {
		if _, ok := present[id]; !ok {
			delete(oc.status, id)
		}
	}
}

func (oc *OnlineChecker) IsOnline(id string) bool {
	oc.mu.RLock()
	defer oc.mu.RUnlock()
	return oc.status[id]
}

// RuntimeConfigDeps bundles the runtime-config wiring threaded through
// NewHandler in E7.1. Grouped into one struct so the constructor stays
// readable as the URL fields, provenance booleans, and the restart
// closer would otherwise add seven positional args.
type RuntimeConfigDeps struct {
	Store               *runtimeconfig.Store
	FrigateURL          string // effective resolved URL (env or overlay or "")
	FrigateUIURL        string // effective resolved URL with FrigateURL fallback
	Go2RTCURL           string // effective resolved URL (may be "")
	FrigateURLFromEnv   bool
	FrigateUIURLFromEnv bool
	Go2RTCURLFromEnv    bool
	// RequestRestart closes the main shutdown-select's restart channel
	// exactly once. Calling it from /api/runtime-config/restart triggers
	// a graceful Shutdown and a process exit so the container restart
	// policy boots the new overlay file.
	RequestRestart func()
}

// Handler holds dependencies for the API routes.
type Handler struct {
	logger          *slog.Logger
	frigate         *frigate.Client
	events          *events.Client
	checker         *OnlineChecker
	cameras         *cameras.Store
	go2rtcURL       string
	go2rtc          *go2rtc.Client
	frigateUIURL    string
	whepTimeout     time.Duration
	httpClient      *http.Client
	prefsStore      *prefs.Store
	groups          *groups.Store
	names           *names.Store
	capabilities    *capabilities.Store
	streamOverrides *streamoverrides.Store
	runtime         RuntimeConfigDeps
}

func NewHandler(
	logger *slog.Logger,
	frigateClient *frigate.Client,
	eventsClient *events.Client,
	checker *OnlineChecker,
	camerasStore *cameras.Store,
	go2rtcURL string,
	go2rtcClient *go2rtc.Client,
	frigateUIURL string,
	whepTimeout time.Duration,
	httpClient *http.Client,
	prefsStore *prefs.Store,
	groupsStore *groups.Store,
	namesStore *names.Store,
	capabilitiesStore *capabilities.Store,
	streamOverridesStore *streamoverrides.Store,
	runtime RuntimeConfigDeps,
) *Handler {
	return &Handler{
		logger:          logger,
		frigate:         frigateClient,
		events:          eventsClient,
		checker:         checker,
		cameras:         camerasStore,
		go2rtcURL:       go2rtcURL,
		go2rtc:          go2rtcClient,
		frigateUIURL:    frigateUIURL,
		whepTimeout:     whepTimeout,
		httpClient:      httpClient,
		prefsStore:      prefsStore,
		groups:          groupsStore,
		names:           namesStore,
		capabilities:    capabilitiesStore,
		streamOverrides: streamOverridesStore,
		runtime:         runtime,
	}
}

func (h *Handler) handleCameras(w http.ResponseWriter, r *http.Request) {
	cams := h.cameras.Snapshot()
	resp := make([]cameraResponse, 0, len(cams))
	for _, cam := range cams {
		camGroups := []string{}
		if h.groups != nil {
			if gid := h.groups.GroupFor(cam.ID); gid != "" {
				camGroups = append(camGroups, gid)
			}
		}
		caps := capabilities.Capabilities{}
		if h.capabilities != nil {
			caps = h.capabilities.Get(cam.ID)
		}
		resp = append(resp, cameraResponse{
			ID:          cam.ID,
			Name:        h.names.Resolve(cam.ID, cam.Name),
			Online:      h.checker.IsOnline(cam.ID),
			SnapshotURL: fmt.Sprintf("/api/cameras/%s/snapshot.jpg", cam.ID),
			Capabilities: capabilitiesResponse{
				TalkBack: caps.TalkBack,
				PTZ:      caps.PTZ,
			},
			Streams: streamsResponse{
				Main: cam.StreamMain,
				Sub:  cam.StreamSub,
			},
			Groups: camGroups,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		h.logger.Error("encode cameras response", "error", err)
	}
}

// refreshDiffResponse is the JSON shape returned by POST /api/cameras/refresh.
// Both slices are present (possibly empty) — never null — so the frontend
// can iterate without a null guard.
type refreshDiffResponse struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
}

// handleRefreshCameras re-pulls Frigate /api/config, persists the new
// snapshot, broadcasts camera.added / camera.removed, and triggers orphan
// cleanup in groups / names / capabilities (all wired in main.go via the
// cameras.Store hooks). Body is ignored. On Frigate unreachable → 502 with
// the {error, message} envelope and no on-disk / in-memory mutation.
func (h *Handler) handleRefreshCameras(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	diff, err := h.cameras.Refresh(ctx)
	if err != nil {
		writeRefreshError(w, h.logger, http.StatusBadGateway, "frigate_unreachable", "Could not refresh cameras from Frigate", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(refreshDiffResponse{Added: diff.Added, Removed: diff.Removed}); err != nil {
		h.logger.Error("encode refresh response", "error", err)
	}
}

// writeRefreshError emits the structured {error, message} body used by the
// other store-backed endpoints, so the frontend can switch on .code.
func writeRefreshError(w http.ResponseWriter, logger *slog.Logger, status int, code, message string, cause error) {
	if cause != nil {
		logger.Error("cameras refresh failed", "error", cause.Error(), "code", code, "status", status)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "message": message})
}

func (h *Handler) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	body, etag, err := h.frigate.GetSnapshot(ctx, id)
	if err != nil {
		writeError(w, h.logger, http.StatusBadGateway, "snapshot unavailable", err)
		return
	}
	defer func() {
		if err := body.Close(); err != nil {
			h.logger.Debug("failed to close snapshot body", "error", err)
		}
	}()

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-store")
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	if _, err := io.Copy(w, body); err != nil {
		h.logger.Error("stream snapshot", "camera", id, "error", err)
	}
}

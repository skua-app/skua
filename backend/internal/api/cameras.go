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

// NewOnlineChecker constructs the checker but does NOT start the polling
// loop — call Start(ctx) in a goroutine for that. Splitting construction
// from lifecycle mirrors internal/sse.Upstream and gives callers a way to
// stop the loop on graceful shutdown.
func NewOnlineChecker(client *frigate.Client, camerasStore *cameras.Store, ttl time.Duration, logger *slog.Logger) *OnlineChecker {
	return &OnlineChecker{
		client:  client,
		cameras: camerasStore,
		ttl:     ttl,
		logger:  logger,
		status:  make(map[string]bool),
		nowFn:   time.Now,
	}
}

// Start drives the polling loop until ctx is cancelled. Blocks, so callers
// should run it in a goroutine. Performs one immediate refresh so /api/cameras
// has data before the first tick fires; each subsequent refresh runs at the
// configured ttl. Refreshes are bounded by per-call timeouts derived from ctx,
// so a cancelled parent aborts in-flight Frigate I/O promptly.
func (oc *OnlineChecker) Start(ctx context.Context) {
	oc.refreshAll(ctx)
	ticker := time.NewTicker(oc.ttl)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			oc.refreshAll(ctx)
		}
	}
}

// onlineResult carries the result of a per-camera online check.
type onlineResult struct {
	id     string
	online bool
}

// refreshAll polls Frigate /api/stats and (for detection-disabled cameras) the
// per-camera snapshot endpoint, then atomically swaps the result into
// oc.status. The full new map is computed off-lock — including draining the
// probe fan-out — so concurrent IsOnline readers never block on network I/O.
// Pruning is implicit: the new map contains exactly the cameras present in the
// current snapshot, so cameras removed since the previous refresh disappear.
func (oc *OnlineChecker) refreshAll(ctx context.Context) {
	cams := oc.cameras.Snapshot()

	statsCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	stats, err := oc.client.GetStats(statsCtx)
	if err != nil {
		// Suppress the noisy warn when shutdown is cancelling mid-poll.
		if ctx.Err() == nil {
			oc.logger.Warn("frigate stats unavailable, marking all cameras offline", "error", err)
		}
		offline := make(map[string]bool, len(cams))
		for _, cam := range cams {
			offline[cam.ID] = false
		}
		oc.mu.Lock()
		oc.status = offline
		oc.mu.Unlock()
		return
	}

	next := make(map[string]bool, len(cams))
	probeIDs := make([]string, 0)

	for _, cam := range cams {
		cs, ok := stats.Cameras[cam.ID]
		if !ok {
			next[cam.ID] = false
			continue
		}
		if cs.CameraFPS > 0 {
			next[cam.ID] = true
			continue
		}
		if !cs.DetectionEnabled {
			// fps may be 0 when Frigate detection is off; fall back to snapshot probe
			probeIDs = append(probeIDs, cam.ID)
			continue
		}
		next[cam.ID] = false
	}

	if len(probeIDs) > 0 {
		probeCh := make(chan onlineResult, len(probeIDs))
		for _, id := range probeIDs {
			go func(camID string) {
				probeCtx, probeCancel := context.WithTimeout(ctx, 2*time.Second)
				defer probeCancel()
				probeCh <- onlineResult{camID, oc.client.ProbeSnapshot(probeCtx, camID)}
			}(id)
		}
		for range probeIDs {
			r := <-probeCh
			next[r.id] = r.online
		}
	}

	oc.mu.Lock()
	oc.status = next
	oc.mu.Unlock()
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

// HandlerDeps bundles every dependency NewHandler needs into one struct so
// callers pass them by name rather than position. Two adjacent same-typed
// strings (Go2RTCURL, FrigateUIURL) could otherwise be transposed with no
// compiler complaint; named-field initialisation makes that class of bug
// unrepresentable. RuntimeConfigDeps stays nested as Runtime — it is its
// own deliberate grouping (see the type comment above).
type HandlerDeps struct {
	Logger          *slog.Logger
	Frigate         *frigate.Client
	Events          *events.Client
	Checker         *OnlineChecker
	Cameras         *cameras.Store
	Go2RTCURL       string
	Go2RTC          *go2rtc.Client
	FrigateUIURL    string
	WHEPTimeout     time.Duration
	HTTPClient      *http.Client
	Prefs           *prefs.Store
	Groups          *groups.Store
	Names           *names.Store
	Capabilities    *capabilities.Store
	StreamOverrides *streamoverrides.Store
	Runtime         RuntimeConfigDeps
}

func NewHandler(d HandlerDeps) *Handler {
	return &Handler{
		logger:          d.Logger,
		frigate:         d.Frigate,
		events:          d.Events,
		checker:         d.Checker,
		cameras:         d.Cameras,
		go2rtcURL:       d.Go2RTCURL,
		go2rtc:          d.Go2RTC,
		frigateUIURL:    d.FrigateUIURL,
		whepTimeout:     d.WHEPTimeout,
		httpClient:      d.HTTPClient,
		prefsStore:      d.Prefs,
		groups:          d.Groups,
		names:           d.Names,
		capabilities:    d.Capabilities,
		streamOverrides: d.StreamOverrides,
		runtime:         d.Runtime,
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
		writeError(w, h.logger, http.StatusBadGateway, "frigate_unreachable", "Could not refresh cameras from Frigate", err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(refreshDiffResponse{Added: diff.Added, Removed: diff.Removed}); err != nil {
		h.logger.Error("encode refresh response", "error", err)
	}
}

func (h *Handler) handleSnapshot(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if !validUpstreamID(id) {
		writeError(w, h.logger, http.StatusBadRequest, "invalid_id", "invalid camera id", nil)
		return
	}
	if _, ok := h.cameras.Find(id); !ok {
		writeError(w, h.logger, http.StatusNotFound, "not_found", "camera not found", nil)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	body, etag, err := h.frigate.GetSnapshot(ctx, id)
	if err != nil {
		writeError(w, h.logger, http.StatusBadGateway, "upstream_error", "snapshot unavailable", err)
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

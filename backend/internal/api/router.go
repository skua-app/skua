package api

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/skua-app/skua/internal/sse"
)

func NewRouter(h *Handler, hub *sse.Hub, logger *slog.Logger, staticFS fs.FS) http.Handler {
	r := chi.NewRouter()

	r.Use(requestLogger(logger))
	r.Use(middleware.Recoverer)

	r.Get("/healthz", handleHealthz)

	r.Route("/api", func(r chi.Router) {
		r.Use(crossSiteGuard)
		r.Get("/config", h.handleConfig)
		r.Get("/runtime-config", h.handleGetRuntimeConfig)
		r.Put("/runtime-config", h.handlePutRuntimeConfig)
		r.Post("/runtime-config/test", h.handleTestRuntimeConfig)
		r.Post("/runtime-config/restart", h.handleRestartRuntimeConfig)
		r.Get("/cameras", h.handleCameras)
		r.Post("/cameras/refresh", h.handleRefreshCameras)
		r.Get("/go2rtc/streams", h.handleListGo2RTCStreams)
		r.Get("/cameras/{id}/snapshot.jpg", h.handleSnapshot)
		r.Get("/cameras/{id}/tile.jpg", h.handleTileJPEG)
		r.Method(http.MethodGet, "/cameras/{id}/vod/{start}/{end}/*", http.HandlerFunc(h.handleTimelineVOD))
		r.Method(http.MethodHead, "/cameras/{id}/vod/{start}/{end}/*", http.HandlerFunc(h.handleTimelineVOD))
		r.Get("/cameras/{id}/preview-frame-list", h.handleTimelinePreviewFrameList)
		r.Get("/cameras/{id}/preview-clips", h.handleTimelinePreviewClips)
		r.Method(http.MethodGet, "/cameras/{id}/preview-clip/{file}", http.HandlerFunc(h.handleTimelinePreviewClip))
		r.Method(http.MethodHead, "/cameras/{id}/preview-clip/{file}", http.HandlerFunc(h.handleTimelinePreviewClip))
		r.Method(http.MethodGet, "/cameras/{id}/preview-frame/{file}", http.HandlerFunc(h.handleTimelinePreviewFrame))
		r.Method(http.MethodHead, "/cameras/{id}/preview-frame/{file}", http.HandlerFunc(h.handleTimelinePreviewFrame))
		r.Get("/cameras/{id}/recordings-summary", h.handleRecordingsSummary)
		r.Get("/cameras/{id}/review", h.handleTimelineReview)
		r.Post("/webrtc/{cam_id}/whep", h.handleWhep)
		r.Get("/prefs", h.handleGetPrefs)
		r.Put("/prefs", h.handlePutPrefs)
		r.Get("/events", h.handleEvents)
		r.Get("/glance", h.handleGlance)
		r.Post("/glance/seen", h.handleGlanceSeen)
		r.Post("/glance/seen-all", h.handleGlanceSeenAll)
		r.Post("/glance/heartbeat", h.handleGlanceHeartbeat)
		r.Method(http.MethodGet, "/glance/{id}/preview.mp4", http.HandlerFunc(h.handleGlancePreview))
		r.Method(http.MethodHead, "/glance/{id}/preview.mp4", http.HandlerFunc(h.handleGlancePreview))
		r.Method(http.MethodGet, "/glance/{id}/clip.mp4", http.HandlerFunc(h.handleGlanceClip))
		r.Get("/events/{id}/thumbnail.jpg", h.handleEventThumbnail)
		r.Get("/events/{id}/snapshot.jpg", h.handleEventSnapshot)
		r.Get("/events/{id}/review", h.handleEventReview)
		r.Method(http.MethodGet, "/events/{id}/clip.mp4", http.HandlerFunc(h.handleEventClip))
		r.Method(http.MethodHead, "/events/{id}/clip.mp4", http.HandlerFunc(h.handleEventClip))
		r.Get("/groups", h.handleListGroups)
		r.Post("/groups", h.handleCreateGroup)
		r.Patch("/groups/{id}", h.handleUpdateGroup)
		r.Delete("/groups/{id}", h.handleDeleteGroup)
		r.Get("/camera-names", h.handleListCameraNames)
		r.Put("/camera-names/{cam_id}", h.handlePutCameraName)
		r.Get("/stream-overrides", h.handleListStreamOverrides)
		r.Put("/stream-overrides/{cam_id}", h.handlePutStreamOverride)
		r.Get("/camera-order", h.handleGetCameraOrder)
		r.Put("/camera-order", h.handlePutCameraOrder)
		r.Get("/stream", hub.ServeHTTP)
	})

	r.Mount("/", spaHandler(staticFS))

	return r
}

// spaHandler serves static files and falls back to index.html for SPA routes.
// Asset paths (has extension, or starts with /_app/ or /icons/) get 404 when missing.
// Extensionless paths that don't exist in the FS are assumed to be SPA routes.
func spaHandler(fsys fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(fsys))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "."
		}
		_, err := fs.Stat(fsys, path)
		if err != nil {
			if isAssetPath(r.URL.Path) {
				http.NotFound(w, r)
				return
			}
			// Extensionless unknown path → SPA route fallback
			r2 := r.Clone(r.Context())
			r2.URL.Path = "/"
			fileServer.ServeHTTP(w, r2)
			return
		}
		fileServer.ServeHTTP(w, r)
	})
}

// isAssetPath returns true for paths that should 404 when missing rather than
// falling back to index.html. Anything with a file extension, or under known
// static prefixes, is an asset.
func isAssetPath(p string) bool {
	if strings.HasPrefix(p, "/_app/") || strings.HasPrefix(p, "/icons/") {
		return true
	}
	// Has a file extension
	last := strings.LastIndex(p, "/")
	segment := p[last+1:]
	return strings.Contains(segment, ".")
}

type responseWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (rw *responseWriter) WriteHeader(status int) {
	if !rw.wrote {
		rw.status = status
		rw.wrote = true
	}
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wrote {
		rw.status = http.StatusOK
		rw.wrote = true
	}
	return rw.ResponseWriter.Write(b)
}

// Unwrap exposes the underlying ResponseWriter so that
// http.NewResponseController can reach Flush / SetWriteDeadline on the real
// connection — required for the long-lived SSE handler.
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

func requestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(ww, r)

			ua := r.UserAgent()
			if len(ua) > 120 {
				ua = ua[:120]
			}
			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"remote_addr", r.RemoteAddr,
				"user_agent", ua,
			)
		})
	}
}

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/skua-app/skua/internal/api"
	"github.com/skua-app/skua/internal/cameras"
	"github.com/skua-app/skua/internal/capabilities"
	"github.com/skua-app/skua/internal/config"
	"github.com/skua-app/skua/internal/events"
	"github.com/skua-app/skua/internal/frigate"
	"github.com/skua-app/skua/internal/go2rtc"
	"github.com/skua-app/skua/internal/groups"
	applog "github.com/skua-app/skua/internal/log"
	"github.com/skua-app/skua/internal/names"
	"github.com/skua-app/skua/internal/prefs"
	"github.com/skua-app/skua/internal/sse"
	"github.com/skua-app/skua/internal/static"
	"github.com/skua-app/skua/internal/streamoverrides"
)

func main() {
	healthcheck := flag.Bool("healthcheck", false, "probe /healthz and exit")
	flag.Parse()

	if *healthcheck {
		runHealthcheck()
		return
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	logger := applog.New(cfg.LogLevel, cfg.LogFormat)
	slog.SetDefault(logger)

	staticFS := static.FS()
	if logger.Enabled(context.Background(), slog.LevelDebug) {
		logEmbedTree(logger, staticFS)
	}

	prefsStore, err := prefs.New(cfg.PrefsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prefs: %v\n", err)
		os.Exit(1)
	}

	frigateClient := frigate.NewClient(cfg.FrigateURL, cfg.HTTPTimeout)

	camerasStore, err := cameras.New(cfg.CamerasPath, frigateClient)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cameras: %v\n", err)
		os.Exit(1)
	}
	logger.Info("cameras loaded", "path", cfg.CamerasPath, "count", len(camerasStore.Snapshot()))

	groupsStore, err := groups.New(cfg.GroupsPath, func(id string) bool {
		_, ok := camerasStore.Find(id)
		return ok
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "groups: %v\n", err)
		os.Exit(1)
	}
	logger.Info("groups loaded", "path", cfg.GroupsPath, "count", len(groupsStore.List()))

	namesStore, err := names.New(cfg.NamesPath, func(id string) bool {
		_, ok := camerasStore.Find(id)
		return ok
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "camera names: %v\n", err)
		os.Exit(1)
	}
	logger.Info("camera names loaded", "path", cfg.NamesPath, "count", len(namesStore.All()))

	capabilitiesStore, err := capabilities.New(cfg.CapabilitiesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "capabilities: %v\n", err)
		os.Exit(1)
	}
	logger.Info("capabilities loaded", "path", cfg.CapabilitiesPath, "count", len(capabilitiesStore.All()))

	streamOverridesStore, err := streamoverrides.New(cfg.StreamOverridesPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stream overrides: %v\n", err)
		os.Exit(1)
	}
	logger.Info("stream overrides loaded", "path", cfg.StreamOverridesPath, "count", len(streamOverridesStore.All()))

	checker := api.NewOnlineChecker(frigateClient, camerasStore, cfg.SnapshotCacheTTL, logger)
	httpClient := &http.Client{}
	go2rtcClient := go2rtc.New(cfg.Go2RTCURL, httpClient)
	eventsClient := events.NewClient(cfg.FrigateURL, &http.Client{Timeout: cfg.HTTPTimeout})

	hub := sse.NewHub(logger)
	wsURL, err := sse.DeriveWSURL(cfg.FrigateURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ws url: %v\n", err)
		os.Exit(1)
	}
	upstream := sse.NewUpstream(wsURL, hub, logger)
	upstreamCtx, upstreamCancel := context.WithCancel(context.Background())
	defer upstreamCancel()
	go upstream.Run(upstreamCtx)

	// Wire refresh hooks AFTER every store exists. OnAdded broadcasts
	// camera.added so clients can refetch immediately. OnRemoved broadcasts
	// camera.removed FIRST, then strips the cam from groups / names /
	// capabilities — so clients see the removal before the dependent stores
	// are mutated. Forget errors are logged but never propagated to the API
	// client; by the time cleanup runs, the refresh has already succeeded.
	camerasStore.SetOnAdded(func(specs []cameras.CameraSpec) {
		for _, spec := range specs {
			hub.Broadcast("camera.added", map[string]string{
				"cam_id": spec.ID,
				"name":   namesStore.Resolve(spec.ID, spec.Name),
			})
		}
	})
	camerasStore.SetOnRemoved(func(camIDs []string) {
		for _, id := range camIDs {
			hub.Broadcast("camera.removed", map[string]string{"cam_id": id})
		}
		for _, id := range camIDs {
			if err := groupsStore.Forget(id); err != nil {
				logger.Warn("groups Forget failed", "cam_id", id, "error", err)
			}
			if err := namesStore.Forget(id); err != nil {
				logger.Warn("names Forget failed", "cam_id", id, "error", err)
			}
			if err := capabilitiesStore.Forget(id); err != nil {
				logger.Warn("capabilities Forget failed", "cam_id", id, "error", err)
			}
			if err := streamOverridesStore.Forget(id); err != nil {
				logger.Warn("stream overrides Forget failed", "cam_id", id, "error", err)
			}
		}
	})

	h := api.NewHandler(
		logger,
		frigateClient,
		eventsClient,
		checker,
		camerasStore,
		cfg.Go2RTCURL,
		go2rtcClient,
		cfg.FrigateUIURL,
		cfg.WHEPTimeout,
		httpClient,
		prefsStore,
		groupsStore,
		namesStore,
		capabilitiesStore,
		streamOverridesStore,
	)
	router := api.NewRouter(h, hub, logger, staticFS)

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  cfg.HTTPTimeout,
		WriteTimeout: cfg.HTTPTimeout * 2,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		logger.Info("server starting", "port", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
	logger.Info("server stopped")
}

func logEmbedTree(logger *slog.Logger, fsys fs.FS) {
	if err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		logger.Debug("embed", "path", path, "dir", d.IsDir())
		return nil
	}); err != nil {
		logger.Debug("embed walk failed", "error", err)
	}
}

func runHealthcheck() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "3200"
	}
	resp, err := http.Get("http://localhost:" + port + "/healthz")
	if err != nil {
		fmt.Fprintf(os.Stderr, "healthcheck: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Debug("failed to close response body", "error", err)
		}
	}()
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "healthcheck: unexpected status %d\n", resp.StatusCode)
		os.Exit(1)
	}
	os.Exit(0)
}

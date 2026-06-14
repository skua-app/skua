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
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/skua-app/skua/internal/api"
	"github.com/skua-app/skua/internal/cameraorder"
	"github.com/skua-app/skua/internal/cameras"
	"github.com/skua-app/skua/internal/capabilities"
	"github.com/skua-app/skua/internal/config"
	"github.com/skua-app/skua/internal/emergency"
	"github.com/skua-app/skua/internal/events"
	"github.com/skua-app/skua/internal/frigate"
	"github.com/skua-app/skua/internal/glance"
	"github.com/skua-app/skua/internal/go2rtc"
	"github.com/skua-app/skua/internal/groups"
	applog "github.com/skua-app/skua/internal/log"
	"github.com/skua-app/skua/internal/names"
	"github.com/skua-app/skua/internal/prefs"
	"github.com/skua-app/skua/internal/runtimeconfig"
	"github.com/skua-app/skua/internal/session"
	"github.com/skua-app/skua/internal/setup"
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

	// runtimeconfig overlay is the second tier of the env > file precedence
	// chain. We build it here regardless of NeedsSetup so both the setup
	// wizard and the unreachable-wizard branch below can share one Store.
	runtimeConfigStore, err := runtimeconfig.New(cfg.RuntimeConfigPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "runtime config: %v\n", err)
		os.Exit(1)
	}

	// First-run setup: no env-supplied or file-supplied FrigateURL. Serve
	// the wizard in place of the normal stack. A successful save triggers
	// a clean process exit so the container's restart policy boots the
	// fresh config.
	if cfg.NeedsSetup {
		logger.Info("entering setup mode", "reason", "frigate_url_not_set", "runtime_config_path", cfg.RuntimeConfigPath)
		opts := setup.Options{
			Mode:    setup.ModeInitial,
			Prefill: runtimeConfigStore.Get(),
			Locked: setup.LockedFields{
				FrigateURL:   cfg.FrigateURLFromEnv,
				FrigateUIURL: cfg.FrigateUIURLFromEnv,
				Go2RTCURL:    cfg.Go2RTCURLFromEnv,
			},
		}
		if err := setup.Serve(cfg.Port, runtimeConfigStore, opts); err != nil {
			fmt.Fprintf(os.Stderr, "setup: %v\n", err)
			os.Exit(1)
		}
		logger.Info("setup complete, exiting for restart")
		os.Exit(0)
	}

	prefsStore, err := prefs.New(cfg.PrefsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "prefs: %v\n", err)
		os.Exit(1)
	}

	glanceStore, err := glance.New(cfg.GlanceStatePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "glance: %v\n", err)
		os.Exit(1)
	}

	sessionStore := session.New(cfg.AwaySessionGap)

	// httpClient is shared by frigate, go2rtc, and the WHEP signaling path.
	// Timeouts on the Frigate side are governed by each call's context (e.g.
	// cameras.New uses a 10s ctx for GetConfig, OnlineChecker uses 5s/2s, the
	// snapshot handler uses 3s); a client-level Timeout would fight those.
	httpClient := &http.Client{}
	frigateClient := frigate.NewClient(cfg.FrigateURL, httpClient)

	camerasStore, err := cameras.New(cfg.CamerasPath, frigateClient)
	if err != nil {
		// Print the operator-facing stderr diagnostic first (matches the
		// previous behaviour for ops logs), then route the browser into
		// either the setup wizard or the informational emergency page
		// depending on what the operator can actually fix from the
		// browser.
		printCameraStoreDiagnostic(err, cfg.CamerasPath)

		switch {
		case errors.Is(err, fs.ErrPermission):
			// Data dir is read-only. The wizard couldn't persist anything
			// anyway — keep the informational chown page.
			detail := filepath.Dir(cfg.CamerasPath)
			logger.Error("entering emergency mode", "reason", string(emergency.ReasonDataNotWritable), "error", err)
			if serveErr := emergency.Serve(cfg.Port, emergency.ReasonDataNotWritable, detail); serveErr != nil {
				fmt.Fprintf(os.Stderr, "emergency: %v\n", serveErr)
			}
			os.Exit(1)

		case !cfg.FrigateURLFromEnv:
			// URL came from the overlay file (or was implicitly empty —
			// but NeedsSetup would have caught that). Operator can fix
			// it in the browser, so serve the wizard prefilled with the
			// current values and an explanatory banner.
			logger.Error("entering setup mode", "reason", "frigate_unreachable", "frigate_url", cfg.FrigateURL, "error", err)
			opts := setup.Options{
				Mode: setup.ModeUnreachable,
				Prefill: runtimeconfig.Values{
					FrigateURL:   cfg.FrigateURL,
					FrigateUIURL: cfg.FrigateUIURL,
					Go2RTCURL:    cfg.Go2RTCURL,
				},
				Locked: setup.LockedFields{
					FrigateURL:   cfg.FrigateURLFromEnv,
					FrigateUIURL: cfg.FrigateUIURLFromEnv,
					Go2RTCURL:    cfg.Go2RTCURLFromEnv,
				},
				ErrMessage: fmt.Sprintf("Cannot reach Frigate at %s. Update the URL below, test the connection, then save to restart.", cfg.FrigateURL),
			}
			if serveErr := setup.Serve(cfg.Port, runtimeConfigStore, opts); serveErr != nil {
				fmt.Fprintf(os.Stderr, "setup: %v\n", serveErr)
				os.Exit(1)
			}
			logger.Info("setup complete, exiting for restart")
			os.Exit(0)

		default:
			// FRIGATE_URL is env-locked. The operator can't fix it from
			// the browser — env will win at next boot — so the
			// informational emergency page is the honest response.
			detail := cfg.FrigateURL
			logger.Error("entering emergency mode", "reason", string(emergency.ReasonFrigateUnreachable), "error", err)
			if serveErr := emergency.Serve(cfg.Port, emergency.ReasonFrigateUnreachable, detail); serveErr != nil {
				fmt.Fprintf(os.Stderr, "emergency: %v\n", serveErr)
			}
			os.Exit(1)
		}
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

	cameraOrderStore, err := cameraorder.New(cfg.CameraOrderPath, func(id string) bool {
		_, ok := camerasStore.Find(id)
		return ok
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "camera order: %v\n", err)
		os.Exit(1)
	}
	logger.Info("camera order loaded", "path", cfg.CameraOrderPath, "count", len(cameraOrderStore.All()))

	// restart is closed by the /api/runtime-config/restart handler when the
	// operator presses "Apply" in /settings. main's shutdown select watches
	// both this and the SIGINT/SIGTERM quit channel; either branch funnels
	// into the same srv.Shutdown. On the restart branch main exits 0 after
	// Shutdown so the container restart policy boots the new overlay file.
	// The sync.Once wrapper keeps the closer idempotent under concurrent
	// Apply presses from multiple tabs.
	restart := make(chan struct{})
	var restartOnce sync.Once
	requestRestart := func() {
		restartOnce.Do(func() { close(restart) })
	}

	checker := api.NewOnlineChecker(frigateClient, camerasStore, cfg.OnlineCheckInterval, logger)
	go2rtcClient := go2rtc.New(cfg.Go2RTCURL, httpClient)
	// No Client.Timeout here: clip fetches must honour handleEventClip's 30s
	// per-call context, not a 5s blanket cap (which would bound every Range
	// request body read). List and FetchImage already set their own 5s
	// contexts, so their effective bound is unchanged. Per-clip buffer cap
	// is configurable via CLIP_MAX_MIB (default 256 MiB).
	eventsClient := events.NewClient(cfg.FrigateURL, nil, cfg.ClipMaxBytes)

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

	// OnlineChecker mirrors sse.Upstream's lifecycle: the constructor does
	// not start the loop; Start(ctx) does and exits on ctx cancel. The
	// defer covers both shutdown paths (SIGTERM and the restart channel).
	// First refresh runs inside Start, so /api/cameras may briefly see
	// "all offline" between server-up and the first Frigate stats round-
	// trip — acceptable, and the same race the SSE upstream already
	// tolerates.
	checkerCtx, checkerCancel := context.WithCancel(context.Background())
	defer checkerCancel()
	go checker.Start(checkerCtx)

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
			if err := cameraOrderStore.Forget(id); err != nil {
				logger.Warn("camera order Forget failed", "cam_id", id, "error", err)
			}
		}
	})

	h := api.NewHandler(api.HandlerDeps{
		Logger:          logger,
		Frigate:         frigateClient,
		Events:          eventsClient,
		Checker:         checker,
		Cameras:         camerasStore,
		Go2RTCURL:       cfg.Go2RTCURL,
		Go2RTC:          go2rtcClient,
		FrigateUIURL:    cfg.FrigateUIURL,
		WHEPTimeout:     cfg.WHEPTimeout,
		HTTPClient:      httpClient,
		Prefs:           prefsStore,
		Groups:          groupsStore,
		Names:           namesStore,
		Capabilities:    capabilitiesStore,
		StreamOverrides: streamOverridesStore,
		CameraOrder:     cameraOrderStore,
		Glance:          glanceStore,
		Session:         sessionStore,
		Runtime: api.RuntimeConfigDeps{
			Store:               runtimeConfigStore,
			FrigateURL:          cfg.FrigateURL,
			FrigateUIURL:        cfg.FrigateUIURL,
			Go2RTCURL:           cfg.Go2RTCURL,
			FrigateURLFromEnv:   cfg.FrigateURLFromEnv,
			FrigateUIURLFromEnv: cfg.FrigateUIURLFromEnv,
			Go2RTCURLFromEnv:    cfg.Go2RTCURLFromEnv,
			RequestRestart:      requestRestart,
		},
	})
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

	var restarting bool
	select {
	case <-quit:
		logger.Info("shutting down")
	case <-restart:
		// Apply pressed in /settings → restart-bounce via the container's
		// restart policy. Fall through to the same graceful Shutdown the
		// SIGTERM path runs, then exit 0 below.
		restarting = true
		logger.Info("restart requested via settings")
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
	}
	logger.Info("server stopped")

	if restarting {
		logger.Info("restart requested via settings, exiting for restart")
		os.Exit(0)
	}
}

// printCameraStoreDiagnostic emits the operator-facing stderr block for a
// cameras.New failure. The /data volume must be writable by uid/gid 65532
// (the distroless `nonroot` user); a root-owned bind mount makes cameras.New
// fail on first write with a terse fs.ErrPermission deep in the chain. We
// surface the two concrete remedies (chown the host dir, or switch to a
// named Docker volume) plus a link to the README troubleshooting section.
// All other error paths fall back to the original "cameras: <err>" one-liner.
//
// Does NOT exit — the caller decides whether to enter emergency mode or quit.
func printCameraStoreDiagnostic(err error, camerasPath string) {
	if errors.Is(err, fs.ErrPermission) {
		dir := filepath.Dir(camerasPath)
		fmt.Fprintf(os.Stderr, "cameras: cannot write to data directory: %s\n", dir)
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "The /data volume must be writable by the container user (uid/gid 65532).")
		fmt.Fprintln(os.Stderr, "Fix with one of:")
		fmt.Fprintf(os.Stderr, "  chown -R 65532:65532 %s\n", dir)
		fmt.Fprintln(os.Stderr, "  Or switch to a named Docker volume (Docker manages permissions automatically).")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "See: https://github.com/skua-app/skua#container-wont-start--data-folder-stays-empty")
		return
	}
	fmt.Fprintf(os.Stderr, "cameras: %v\n", err)
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

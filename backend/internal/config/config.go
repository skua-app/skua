package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/skua-app/skua/internal/probe"
	"github.com/skua-app/skua/internal/runtimeconfig"
)

type Config struct {
	Port                string
	LogLevel            string
	LogFormat           string
	FrigateURL          string
	FrigateUIURL        string
	Go2RTCURL           string
	OnlineCheckInterval time.Duration
	HTTPTimeout         time.Duration
	ShutdownTimeout     time.Duration
	WHEPTimeout         time.Duration
	PrefsPath           string
	GroupsPath          string
	NamesPath           string
	CamerasPath         string
	CapabilitiesPath    string
	StreamOverridesPath string
	RuntimeConfigPath   string
	GlanceStatePath     string

	// Provenance flags — true when the corresponding URL came from a
	// non-empty environment variable. The setup wizard renders env-sourced
	// fields read-only so the operator can't try to "save" over them via
	// the browser when the container env will win on next boot.
	FrigateURLFromEnv   bool
	Go2RTCURLFromEnv    bool
	FrigateUIURLFromEnv bool

	// NeedsSetup is true when the effective FrigateURL is empty after
	// merging env and the on-disk overlay. main.go routes the browser
	// into the setup wizard instead of normal boot.
	NeedsSetup bool
}

func Load() (*Config, error) {
	var errs []string

	cfg := &Config{
		Port:                env("PORT", "3200"),
		LogLevel:            env("LOG_LEVEL", "info"),
		LogFormat:           env("LOG_FORMAT", "json"),
		OnlineCheckInterval: parseDuration("ONLINE_CHECK_INTERVAL", env("ONLINE_CHECK_INTERVAL", "15s"), &errs),
		HTTPTimeout:         parseDuration("HTTP_TIMEOUT", env("HTTP_TIMEOUT", "5s"), &errs),
		ShutdownTimeout:     parseDuration("SHUTDOWN_TIMEOUT", env("SHUTDOWN_TIMEOUT", "10s"), &errs),
		WHEPTimeout:         parseDuration("WHEP_TIMEOUT", env("WHEP_TIMEOUT", "10s"), &errs),
		PrefsPath:           env("PREFS_PATH", "/data/prefs.json"),
		GroupsPath:          env("GROUPS_CONFIG_PATH", "/data/groups.yaml"),
		NamesPath:           env("CAMERA_NAMES_CONFIG_PATH", "/data/camera_names.yaml"),
		CamerasPath:         env("CAMERAS_CONFIG_PATH", "/data/cameras.yaml"),
		CapabilitiesPath:    env("CAPABILITIES_CONFIG_PATH", "/data/capabilities.yaml"),
		StreamOverridesPath: env("STREAM_OVERRIDES_CONFIG_PATH", "/data/stream_overrides.yaml"),
		RuntimeConfigPath:   env("RUNTIME_CONFIG_PATH", "/data/config.yaml"),
		GlanceStatePath:     env("GLANCE_STATE_PATH", "/data/glance.json"),
	}

	frigateEnv := os.Getenv("FRIGATE_URL")
	frigateUIEnv := os.Getenv("FRIGATE_UI_URL")
	go2rtcEnv := os.Getenv("GO2RTC_URL")

	cfg.FrigateURLFromEnv = frigateEnv != ""
	cfg.FrigateUIURLFromEnv = frigateUIEnv != ""
	cfg.Go2RTCURLFromEnv = go2rtcEnv != ""

	overlay, err := runtimeconfig.New(cfg.RuntimeConfigPath)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	file := overlay.Get()

	cfg.FrigateURL = firstNonEmpty(frigateEnv, file.FrigateURL)
	cfg.Go2RTCURL = firstNonEmpty(go2rtcEnv, file.Go2RTCURL)
	cfg.FrigateUIURL = firstNonEmpty(frigateUIEnv, file.FrigateUIURL)

	cfg.FrigateURL = strings.TrimRight(cfg.FrigateURL, "/")
	if cfg.FrigateUIURL == "" {
		cfg.FrigateUIURL = cfg.FrigateURL
	}
	cfg.FrigateUIURL = strings.TrimRight(cfg.FrigateUIURL, "/")
	if cfg.Go2RTCURL != "" {
		cfg.Go2RTCURL = strings.TrimRight(cfg.Go2RTCURL, "/")
	}

	cfg.NeedsSetup = cfg.FrigateURL == ""

	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[cfg.LogLevel] {
		errs = append(errs, fmt.Sprintf("LOG_LEVEL must be one of debug|info|warn|error, got %q", cfg.LogLevel))
	}
	validFormats := map[string]bool{"json": true, "text": true}
	if !validFormats[cfg.LogFormat] {
		errs = append(errs, fmt.Sprintf("LOG_FORMAT must be json|text, got %q", cfg.LogFormat))
	}

	// Only validate non-empty URLs. An empty FrigateURL is the legitimate
	// first-run NeedsSetup state — the wizard handles it; failing Load here
	// would break boot. Same logic for Go2RTCURL (optional) and FrigateUIURL
	// (falls back to FrigateURL when blank, so empty is already impossible
	// post-resolution unless FrigateURL is also empty).
	if cfg.FrigateURL != "" {
		if err := probe.ValidateURL(cfg.FrigateURL); err != nil {
			errs = append(errs, fmt.Sprintf("FRIGATE_URL %s", err))
		}
	}
	if cfg.Go2RTCURL != "" {
		if err := probe.ValidateURL(cfg.Go2RTCURL); err != nil {
			errs = append(errs, fmt.Sprintf("GO2RTC_URL %s", err))
		}
	}
	if cfg.FrigateUIURL != "" {
		if err := probe.ValidateURL(cfg.FrigateUIURL); err != nil {
			errs = append(errs, fmt.Sprintf("FRIGATE_UI_URL %s", err))
		}
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("config: %s", strings.Join(errs, "; "))
	}
	return cfg, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// parseDuration parses raw as a Go duration string. On failure it appends a
// readable diagnostic to errs and returns 0; the caller's len(errs) gate then
// turns the bad value into a Load error rather than silently disabling a
// timeout (a zero Duration means "no timeout").
func parseDuration(key, raw string, errs *[]string) time.Duration {
	d, err := time.ParseDuration(raw)
	if err != nil {
		*errs = append(*errs, fmt.Sprintf("%s must be a valid duration (e.g. 5s, 200ms), got %q", key, raw))
		return 0
	}
	return d
}

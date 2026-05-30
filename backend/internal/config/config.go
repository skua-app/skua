package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/skua-app/skua/internal/runtimeconfig"
)

type Config struct {
	Port                string
	LogLevel            string
	LogFormat           string
	FrigateURL          string
	FrigateUIURL        string
	Go2RTCURL           string
	SnapshotCacheTTL    time.Duration
	HTTPTimeout         time.Duration
	ShutdownTimeout     time.Duration
	WHEPTimeout         time.Duration
	StreamProxyTimeout  time.Duration
	PrefsPath           string
	GroupsPath          string
	NamesPath           string
	CamerasPath         string
	CapabilitiesPath    string
	StreamOverridesPath string
	RuntimeConfigPath   string

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
	cfg := &Config{
		Port:                env("PORT", "3200"),
		LogLevel:            env("LOG_LEVEL", "info"),
		LogFormat:           env("LOG_FORMAT", "json"),
		SnapshotCacheTTL:    mustDuration(env("SNAPSHOT_CACHE_TTL", "15s")),
		HTTPTimeout:         mustDuration(env("HTTP_TIMEOUT", "5s")),
		ShutdownTimeout:     mustDuration(env("SHUTDOWN_TIMEOUT", "10s")),
		WHEPTimeout:         mustDuration(env("WHEP_TIMEOUT", "10s")),
		StreamProxyTimeout:  mustDuration(env("STREAM_PROXY_TIMEOUT", "0")),
		PrefsPath:           env("PREFS_PATH", "/data/prefs.json"),
		GroupsPath:          env("GROUPS_CONFIG_PATH", "/data/groups.yaml"),
		NamesPath:           env("CAMERA_NAMES_CONFIG_PATH", "/data/camera_names.yaml"),
		CamerasPath:         env("CAMERAS_CONFIG_PATH", "/data/cameras.yaml"),
		CapabilitiesPath:    env("CAPABILITIES_CONFIG_PATH", "/data/capabilities.yaml"),
		StreamOverridesPath: env("STREAM_OVERRIDES_CONFIG_PATH", "/data/stream_overrides.yaml"),
		RuntimeConfigPath:   env("RUNTIME_CONFIG_PATH", "/data/config.yaml"),
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

	var errs []string
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLevels[cfg.LogLevel] {
		errs = append(errs, fmt.Sprintf("LOG_LEVEL must be one of debug|info|warn|error, got %q", cfg.LogLevel))
	}
	validFormats := map[string]bool{"json": true, "text": true}
	if !validFormats[cfg.LogFormat] {
		errs = append(errs, fmt.Sprintf("LOG_FORMAT must be json|text, got %q", cfg.LogFormat))
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

func mustDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0
	}
	return d
}

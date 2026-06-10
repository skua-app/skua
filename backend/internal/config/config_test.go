package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// withIsolatedRuntimeConfig points RUNTIME_CONFIG_PATH at a per-test temp
// file and clears any URL env vars the test does not explicitly set. This
// keeps test execution hermetic from the developer's real /data and from
// shell-level FRIGATE_URL exports.
func withIsolatedRuntimeConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	t.Setenv("RUNTIME_CONFIG_PATH", path)
	t.Setenv("FRIGATE_URL", "")
	t.Setenv("FRIGATE_UI_URL", "")
	t.Setenv("GO2RTC_URL", "")
	return path
}

func writeOverlay(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write overlay: %v", err)
	}
}

func TestLoad_Defaults(t *testing.T) {
	withIsolatedRuntimeConfig(t)
	t.Setenv("FRIGATE_URL", "http://frigate:5000")
	t.Setenv("PORT", "")
	t.Setenv("LOG_LEVEL", "")
	t.Setenv("LOG_FORMAT", "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Port != "3200" {
		t.Errorf("Port: want 3200, got %s", cfg.Port)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel: want info, got %s", cfg.LogLevel)
	}
	if cfg.LogFormat != "json" {
		t.Errorf("LogFormat: want json, got %s", cfg.LogFormat)
	}
	if cfg.OnlineCheckInterval != 15*time.Second {
		t.Errorf("OnlineCheckInterval: want 15s, got %v", cfg.OnlineCheckInterval)
	}
	if cfg.NeedsSetup {
		t.Errorf("NeedsSetup: want false, got true")
	}
	if !cfg.FrigateURLFromEnv {
		t.Errorf("FrigateURLFromEnv: want true, got false")
	}
	if cfg.AwaySessionGap != 30*time.Minute {
		t.Errorf("AwaySessionGap: want 30m, got %v", cfg.AwaySessionGap)
	}
}

func TestLoad_AwaySessionGapOverride(t *testing.T) {
	withIsolatedRuntimeConfig(t)
	t.Setenv("FRIGATE_URL", "http://frigate:5000")
	t.Setenv("AWAY_SESSION_GAP", "2h")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AwaySessionGap != 2*time.Hour {
		t.Errorf("AwaySessionGap: want 2h, got %v", cfg.AwaySessionGap)
	}
}

func TestLoad_StripsTrailingSlash(t *testing.T) {
	withIsolatedRuntimeConfig(t)
	t.Setenv("FRIGATE_URL", "http://frigate:5000/")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.FrigateURL != "http://frigate:5000" {
		t.Errorf("want trailing slash stripped, got %s", cfg.FrigateURL)
	}
}

func TestLoad_InvalidLogLevel(t *testing.T) {
	withIsolatedRuntimeConfig(t)
	t.Setenv("FRIGATE_URL", "http://frigate:5000")
	t.Setenv("LOG_LEVEL", "verbose")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid LOG_LEVEL")
	}
}

func TestLoad_InvalidLogFormat(t *testing.T) {
	withIsolatedRuntimeConfig(t)
	t.Setenv("FRIGATE_URL", "http://frigate:5000")
	t.Setenv("LOG_FORMAT", "xml")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid LOG_FORMAT")
	}
}

func TestLoad_CustomDurations(t *testing.T) {
	withIsolatedRuntimeConfig(t)
	t.Setenv("FRIGATE_URL", "http://frigate:5000")
	t.Setenv("ONLINE_CHECK_INTERVAL", "30s")
	t.Setenv("HTTP_TIMEOUT", "10s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OnlineCheckInterval != 30*time.Second {
		t.Errorf("OnlineCheckInterval: want 30s, got %v", cfg.OnlineCheckInterval)
	}
	if cfg.HTTPTimeout != 10*time.Second {
		t.Errorf("HTTPTimeout: want 10s, got %v", cfg.HTTPTimeout)
	}
}

func TestLoad_EnvSetNoFile(t *testing.T) {
	withIsolatedRuntimeConfig(t)
	t.Setenv("FRIGATE_URL", "http://frigate:5000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.FrigateURL != "http://frigate:5000" {
		t.Errorf("FrigateURL = %q, want env value", cfg.FrigateURL)
	}
	if !cfg.FrigateURLFromEnv {
		t.Errorf("FrigateURLFromEnv: want true")
	}
	if cfg.NeedsSetup {
		t.Errorf("NeedsSetup: want false")
	}
}

func TestLoad_FileOnlyNoEnv(t *testing.T) {
	path := withIsolatedRuntimeConfig(t)
	writeOverlay(t, path, "frigate_url: http://from-file:5000\n")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.FrigateURL != "http://from-file:5000" {
		t.Errorf("FrigateURL = %q, want file value", cfg.FrigateURL)
	}
	if cfg.FrigateURLFromEnv {
		t.Errorf("FrigateURLFromEnv: want false (came from file)")
	}
	if cfg.NeedsSetup {
		t.Errorf("NeedsSetup: want false (file supplied the URL)")
	}
}

func TestLoad_NeitherEnvNorFile(t *testing.T) {
	withIsolatedRuntimeConfig(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.FrigateURL != "" {
		t.Errorf("FrigateURL = %q, want empty", cfg.FrigateURL)
	}
	if !cfg.NeedsSetup {
		t.Errorf("NeedsSetup: want true when no source supplies a URL")
	}
	if cfg.FrigateURLFromEnv {
		t.Errorf("FrigateURLFromEnv: want false")
	}
}

func TestLoad_EnvWinsOverFile(t *testing.T) {
	path := withIsolatedRuntimeConfig(t)
	writeOverlay(t, path, "frigate_url: http://from-file:5000\ngo2rtc_url: http://from-file:1984\n")
	t.Setenv("FRIGATE_URL", "http://from-env:5000")
	t.Setenv("GO2RTC_URL", "http://from-env:1984")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.FrigateURL != "http://from-env:5000" {
		t.Errorf("FrigateURL = %q, env should win", cfg.FrigateURL)
	}
	if !cfg.FrigateURLFromEnv {
		t.Errorf("FrigateURLFromEnv: want true")
	}
	if cfg.Go2RTCURL != "http://from-env:1984" {
		t.Errorf("Go2RTCURL = %q, env should win", cfg.Go2RTCURL)
	}
	if !cfg.Go2RTCURLFromEnv {
		t.Errorf("Go2RTCURLFromEnv: want true")
	}
}

func TestLoad_FrigateUIURLFallsBackToFrigateURL(t *testing.T) {
	withIsolatedRuntimeConfig(t)
	t.Setenv("FRIGATE_URL", "http://frigate:5000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.FrigateUIURL != "http://frigate:5000" {
		t.Errorf("FrigateUIURL = %q, want FrigateURL fallback", cfg.FrigateUIURL)
	}
	if cfg.FrigateUIURLFromEnv {
		t.Errorf("FrigateUIURLFromEnv: want false (not from env)")
	}
}

func TestLoad_FrigateUIURLFileOverrides(t *testing.T) {
	path := withIsolatedRuntimeConfig(t)
	writeOverlay(t, path, "frigate_url: http://frigate:5000\nfrigate_ui_url: http://frigate:8971\n")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.FrigateUIURL != "http://frigate:8971" {
		t.Errorf("FrigateUIURL = %q, want file value", cfg.FrigateUIURL)
	}
}

func TestLoad_FrigateUIURLEnvWinsOverFile(t *testing.T) {
	path := withIsolatedRuntimeConfig(t)
	writeOverlay(t, path, "frigate_url: http://frigate:5000\nfrigate_ui_url: http://from-file:8971\n")
	t.Setenv("FRIGATE_UI_URL", "http://from-env:8971")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.FrigateUIURL != "http://from-env:8971" {
		t.Errorf("FrigateUIURL = %q, env should win", cfg.FrigateUIURL)
	}
	if !cfg.FrigateUIURLFromEnv {
		t.Errorf("FrigateUIURLFromEnv: want true")
	}
}

func TestLoad_InvalidDurationIsFatal(t *testing.T) {
	withIsolatedRuntimeConfig(t)
	t.Setenv("FRIGATE_URL", "http://frigate:5000")
	t.Setenv("HTTP_TIMEOUT", "5000") // missing unit — must fail loud, not silently 0

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid HTTP_TIMEOUT")
	}
}

func TestLoad_InvalidFrigateURLIsFatal(t *testing.T) {
	withIsolatedRuntimeConfig(t)
	t.Setenv("FRIGATE_URL", "frigate:5000") // missing scheme

	if _, err := Load(); err == nil {
		t.Fatal("expected error for scheme-less FRIGATE_URL")
	}
}

func TestLoad_EmptyFrigateURLIsNotValidated(t *testing.T) {
	// Empty FrigateURL is the legitimate first-run NeedsSetup state and must
	// not fail Load; the wizard handles it.
	withIsolatedRuntimeConfig(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error for empty FRIGATE_URL: %v", err)
	}
	if !cfg.NeedsSetup {
		t.Errorf("NeedsSetup: want true when no URL is supplied")
	}
}

func TestLoad_InvalidGo2RTCURLIsFatal(t *testing.T) {
	withIsolatedRuntimeConfig(t)
	t.Setenv("FRIGATE_URL", "http://frigate:5000")
	t.Setenv("GO2RTC_URL", "not a url")

	if _, err := Load(); err == nil {
		t.Fatal("expected error for malformed GO2RTC_URL")
	}
}

func TestLoad_MalformedOverlayIsFatal(t *testing.T) {
	path := withIsolatedRuntimeConfig(t)
	writeOverlay(t, path, "frigate_url: [not a string")
	t.Setenv("FRIGATE_URL", "http://frigate:5000")

	if _, err := Load(); err == nil {
		t.Fatal("expected error on malformed overlay even when env supplies a URL")
	}
}

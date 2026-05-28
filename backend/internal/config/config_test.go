package config

import (
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
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
	if cfg.SnapshotCacheTTL != 15*time.Second {
		t.Errorf("SnapshotCacheTTL: want 15s, got %v", cfg.SnapshotCacheTTL)
	}
}

func TestLoad_RequiresFrigateURL(t *testing.T) {
	t.Setenv("FRIGATE_URL", "")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error when FRIGATE_URL is missing")
	}
}

func TestLoad_StripsTrailingSlash(t *testing.T) {
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
	t.Setenv("FRIGATE_URL", "http://frigate:5000")
	t.Setenv("LOG_LEVEL", "verbose")
	_, err := Load()
	if err == nil {
		t.Fatal("expected error for invalid LOG_LEVEL")
	}
}

func TestLoad_CustomDurations(t *testing.T) {
	t.Setenv("FRIGATE_URL", "http://frigate:5000")
	t.Setenv("SNAPSHOT_CACHE_TTL", "30s")
	t.Setenv("HTTP_TIMEOUT", "10s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.SnapshotCacheTTL != 30*time.Second {
		t.Errorf("SnapshotCacheTTL: want 30s, got %v", cfg.SnapshotCacheTTL)
	}
	if cfg.HTTPTimeout != 10*time.Second {
		t.Errorf("HTTPTimeout: want 10s, got %v", cfg.HTTPTimeout)
	}
}

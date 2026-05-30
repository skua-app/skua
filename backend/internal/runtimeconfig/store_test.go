package runtimeconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func tempStorePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "config.yaml")
}

func TestNew_MissingFileIsEmptyStore(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := s.Get(); got != (Values{}) {
		t.Errorf("expected empty values, got %+v", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file not to exist, stat err = %v", err)
	}
}

func TestNew_MalformedYAMLFailsFast(t *testing.T) {
	path := tempStorePath(t)
	if err := os.WriteFile(path, []byte("frigate_url: [not a string"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := New(path); err == nil {
		t.Fatal("expected error on malformed YAML")
	}
}

func TestSave_RoundTripsAndPersists(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	want := Values{
		FrigateURL:   "http://frigate:5000",
		FrigateUIURL: "http://frigate:8971",
		Go2RTCURL:    "http://frigate:1984",
	}
	if err := s.Save(want); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if got := s.Get(); got != want {
		t.Errorf("Get after Save = %+v, want %+v", got, want)
	}

	s2, err := New(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := s2.Get(); got != want {
		t.Errorf("after reload = %+v, want %+v", got, want)
	}
}

func TestSave_EmptyValuesRoundTrip(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Save(Values{}); err != nil {
		t.Fatalf("Save empty: %v", err)
	}
	s2, err := New(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := s2.Get(); got != (Values{}) {
		t.Errorf("after reload empty = %+v, want zero", got)
	}
}

func TestSave_OverwritesExistingFile(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Save(Values{FrigateURL: "http://old:5000"}); err != nil {
		t.Fatalf("Save first: %v", err)
	}
	if err := s.Save(Values{FrigateURL: "http://new:5000", Go2RTCURL: "http://new:1984"}); err != nil {
		t.Fatalf("Save second: %v", err)
	}
	s2, err := New(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got := s2.Get()
	if got.FrigateURL != "http://new:5000" || got.Go2RTCURL != "http://new:1984" {
		t.Errorf("after reload = %+v, want overwritten values", got)
	}
	if got.FrigateUIURL != "" {
		t.Errorf("FrigateUIURL should be empty after overwrite, got %q", got.FrigateUIURL)
	}
}

func TestPath_ReturnsConfiguredPath(t *testing.T) {
	path := tempStorePath(t)
	s, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if s.Path() != path {
		t.Errorf("Path() = %q, want %q", s.Path(), path)
	}
}

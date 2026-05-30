// Package runtimeconfig persists the URLs entered through the first-run
// setup wizard. The on-disk file is the second tier of the env > file
// precedence chain — env vars always win when set, so the overlay only
// supplies values the operator left unset.
//
// Storage shape (see fileSchema):
//
//	frigate_url: "http://frigate:5000"
//	frigate_ui_url: "http://frigate:8971"
//	go2rtc_url: "http://frigate:1984"
//
// The file is not created until the first Save call. URL trimming is not
// this layer's job — raw values are stored as written; internal/config
// normalises them after merging with the environment.
package runtimeconfig

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Values is the resolved runtime overlay. Empty strings mean "not set"
// and let env vars or fallbacks take over upstream.
type Values struct {
	FrigateURL   string `yaml:"frigate_url" json:"frigate_url"`
	FrigateUIURL string `yaml:"frigate_ui_url" json:"frigate_ui_url"`
	Go2RTCURL    string `yaml:"go2rtc_url" json:"go2rtc_url"`
}

// fileSchema is the on-disk YAML envelope. Keeping the schema flat (no
// outer key) matches the streamoverrides house style for single-document
// config files.
type fileSchema = Values

// Store is a thread-safe, YAML-backed wrapper over a single Values record.
type Store struct {
	path   string
	mu     sync.RWMutex
	values Values
}

// New loads the overlay file at path. Missing file → empty store (the
// file is not created until the first Save). Malformed YAML → error
// (fail-fast), matching streamoverrides.New.
func New(path string) (*Store, error) {
	s := &Store{path: path}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("runtimeconfig: read %s: %w", path, err)
	}

	var fs fileSchema
	if err := yaml.Unmarshal(data, &fs); err != nil {
		return nil, fmt.Errorf("runtimeconfig: parse %s: %w", path, err)
	}
	s.values = fs
	return s, nil
}

// Path returns the on-disk path the store writes to.
func (s *Store) Path() string {
	return s.path
}

// Get returns a copy of the current values under a read lock.
func (s *Store) Get() Values {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.values
}

// Save writes values atomically and updates the in-memory copy on success.
func (s *Store) Save(values Values) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.atomicWrite(values); err != nil {
		return err
	}
	s.values = values
	return nil
}

// atomicWrite serialises values via temp + rename, matching the
// streamoverrides house style (MkdirAll 0o755, SetIndent(2), chmod 0o644).
func (s *Store) atomicWrite(v Values) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("runtimeconfig: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "config-*.tmp")
	if err != nil {
		return fmt.Errorf("runtimeconfig: create temp: %w", err)
	}
	tmpPath := tmp.Name()

	enc := yaml.NewEncoder(tmp)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("runtimeconfig: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("runtimeconfig: close encoder: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("runtimeconfig: close temp: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("runtimeconfig: chmod temp: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("runtimeconfig: rename: %w", err)
	}
	return nil
}

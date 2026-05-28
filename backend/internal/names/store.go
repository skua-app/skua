// Package names persists user-supplied per-camera display names as a
// YAML map[cam_id]name. The store is the override layer: a non-empty
// entry replaces the static name from config.Cameras for /api/cameras
// responses; cameras without an entry continue to use the config name.
//
// Storage shape (see fileSchema):
//
//	names:
//	  cam1: "Yard"
//	  cam5: "Driveway"
//
// The top-level `names` key is forward-compatible — sibling keys can
// land later without a migration.
package names

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

const (
	nameMinLen = 1
	nameMaxLen = 30
)

// Errors returned by Set so the API layer can map them to snake_case
// codes / HTTP statuses without leaking the store internals.
var (
	ErrNameTooLong    = errors.New("name_too_long")
	ErrCameraNotFound = errors.New("camera_not_found")
)

// CameraExistsFunc is injected from the api layer to validate cam ids
// against config.Cameras without pulling a config dependency in here.
type CameraExistsFunc func(id string) bool

// fileSchema is the on-disk YAML envelope.
type fileSchema struct {
	Names map[string]string `yaml:"names"`
}

// Store is a thread-safe, YAML-backed map[cam_id]display_name.
type Store struct {
	path        string
	cameraExist CameraExistsFunc
	mu          sync.RWMutex
	names       map[string]string
}

// New loads the names file at path. Missing → empty store (the file is
// not created until the first Set). Malformed YAML → error (fail-fast).
func New(path string, cameraExist CameraExistsFunc) (*Store, error) {
	s := &Store{path: path, cameraExist: cameraExist, names: map[string]string{}}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("names: read %s: %w", path, err)
	}

	var fs fileSchema
	if err := yaml.Unmarshal(data, &fs); err != nil {
		return nil, fmt.Errorf("names: parse %s: %w", path, err)
	}
	if fs.Names != nil {
		// Trim values on load — defensive against hand-edited files.
		for id, n := range fs.Names {
			trimmed := strings.TrimSpace(n)
			if trimmed != "" {
				s.names[id] = trimmed
			}
		}
	}
	return s, nil
}

// All returns a defensive copy of the current override map.
func (s *Store) All() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]string, len(s.names))
	for k, v := range s.names {
		out[k] = v
	}
	return out
}

// Get returns the stored override for camID, or "" if none is set.
func (s *Store) Get(camID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.names[camID]
}

// Resolve returns the override for camID if present, otherwise fallback.
// Used by the cameras handler to merge config.Name with user overrides
// in a single call site.
func (s *Store) Resolve(camID, fallback string) string {
	if s == nil {
		return fallback
	}
	if n := s.Get(camID); n != "" {
		return n
	}
	return fallback
}

// Set assigns name to camID. An empty name (after trim) deletes the
// override so the camera falls back to its config default. Returns the
// effective stored value (or "" if the override was cleared) and any
// validation error.
func (s *Store) Set(camID, name string) (string, error) {
	if s.cameraExist != nil && !s.cameraExist(camID) {
		return "", ErrCameraNotFound
	}
	trimmed := strings.TrimSpace(name)
	if len([]rune(trimmed)) > nameMaxLen {
		return "", ErrNameTooLong
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	next := cloneMap(s.names)
	if trimmed == "" {
		delete(next, camID)
	} else {
		next[camID] = trimmed
	}
	if err := s.atomicWrite(next); err != nil {
		return "", err
	}
	s.names = next
	return trimmed, nil
}

// Forget removes camID's override entry, writing the file atomically.
// No-op (no disk write) when camID is not present, so refresh cleanup over
// many never-overridden cams does not thrash the filesystem.
func (s *Store) Forget(camID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.names[camID]; !ok {
		return nil
	}
	next := cloneMap(s.names)
	delete(next, camID)
	if err := s.atomicWrite(next); err != nil {
		return err
	}
	s.names = next
	return nil
}

func cloneMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// atomicWrite serialises the override map via temp + rename.
func (s *Store) atomicWrite(m map[string]string) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("names: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "camera_names-*.tmp")
	if err != nil {
		return fmt.Errorf("names: create temp: %w", err)
	}
	tmpPath := tmp.Name()

	// Make missing-map encode as an empty map rather than `null` so the
	// file stays readable when every override is cleared.
	payload := fileSchema{Names: m}
	if payload.Names == nil {
		payload.Names = map[string]string{}
	}

	enc := yaml.NewEncoder(tmp)
	enc.SetIndent(2)
	if err := enc.Encode(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("names: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("names: close encoder: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("names: close temp: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("names: chmod temp: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("names: rename: %w", err)
	}
	return nil
}

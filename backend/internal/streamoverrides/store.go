// Package streamoverrides persists per-camera go2rtc stream-name overrides
// as a YAML map[cam_id]Override. The override layer applies only inside the
// WHEP handler — GET /api/cameras still surfaces the Frigate-truth stream
// names from internal/cameras. Cameras absent from the file fall through to
// the registry value.
//
// Storage shape (see fileSchema):
//
//	stream_overrides:
//	  cam5: { main: "cam1_sub", sub: "" }
//	  cam6: { main: "cam6_main_h264", sub: "cam6_sub" }
//
// An entry is only kept when at least one of main/sub is non-empty after
// trim — Set with both empty removes the row, matching internal/names'
// "empty clears the override" semantics.
package streamoverrides

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"
)

// Override is the per-camera stream-name override pair. Either field may
// be empty independently; an empty field means "fall through to the
// registry value for that quality".
type Override struct {
	Main string `yaml:"main" json:"main"`
	Sub  string `yaml:"sub" json:"sub"`
}

// fileSchema is the on-disk YAML envelope.
type fileSchema struct {
	StreamOverrides map[string]Override `yaml:"stream_overrides"`
}

// Store is a thread-safe, YAML-backed map[cam_id]Override.
type Store struct {
	path      string
	mu        sync.RWMutex
	overrides map[string]Override
}

// New loads the overrides file at path. Missing → empty store (the file
// is not created until the first Set mutation). Malformed YAML → error
// (fail-fast).
func New(path string) (*Store, error) {
	s := &Store{path: path, overrides: map[string]Override{}}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("streamoverrides: read %s: %w", path, err)
	}

	var fs fileSchema
	if err := yaml.Unmarshal(data, &fs); err != nil {
		return nil, fmt.Errorf("streamoverrides: parse %s: %w", path, err)
	}
	if fs.StreamOverrides != nil {
		for id, o := range fs.StreamOverrides {
			s.overrides[id] = o
		}
	}
	return s, nil
}

// NewForTest builds a Store backed by the supplied map without touching
// disk. Tests use this to skip filesystem setup.
func NewForTest(initial map[string]Override) *Store {
	return &Store{overrides: cloneMap(initial)}
}

// Get returns the stored override for camID, or the zero value when no
// entry exists. Never returns ok — absence == zero, consistent with the
// "passive override" semantics.
func (s *Store) Get(camID string) Override {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.overrides[camID]
}

// All returns a defensive copy of the override map.
func (s *Store) All() map[string]Override {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneMap(s.overrides)
}

// Set assigns main and sub to camID. Both are trimmed first; if both end
// up empty the entry is removed (and the file rewritten without it), so
// the override store never accumulates dead rows.
func (s *Store) Set(camID string, main, sub string) error {
	main = strings.TrimSpace(main)
	sub = strings.TrimSpace(sub)
	if main == "" && sub == "" {
		return s.Forget(camID)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	next := cloneMap(s.overrides)
	next[camID] = Override{Main: main, Sub: sub}
	if err := s.atomicWrite(next); err != nil {
		return err
	}
	s.overrides = next
	return nil
}

// Forget removes camID's override entry, writing the file atomically.
// No-op (no disk write) when camID is not present, so refresh cleanup over
// many never-overridden cams does not thrash the filesystem.
func (s *Store) Forget(camID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.overrides[camID]; !ok {
		return nil
	}
	next := cloneMap(s.overrides)
	delete(next, camID)
	if err := s.atomicWrite(next); err != nil {
		return err
	}
	s.overrides = next
	return nil
}

func cloneMap(in map[string]Override) map[string]Override {
	out := make(map[string]Override, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// atomicWrite serialises the override map via temp + rename.
func (s *Store) atomicWrite(m map[string]Override) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("streamoverrides: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "stream_overrides-*.tmp")
	if err != nil {
		return fmt.Errorf("streamoverrides: create temp: %w", err)
	}
	tmpPath := tmp.Name()

	payload := fileSchema{StreamOverrides: m}
	if payload.StreamOverrides == nil {
		payload.StreamOverrides = map[string]Override{}
	}

	enc := yaml.NewEncoder(tmp)
	enc.SetIndent(2)
	if err := enc.Encode(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("streamoverrides: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("streamoverrides: close encoder: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("streamoverrides: close temp: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("streamoverrides: chmod temp: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("streamoverrides: rename: %w", err)
	}
	return nil
}

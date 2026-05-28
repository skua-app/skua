// Package capabilities persists per-camera capability flags (talk_back, ptz)
// as a YAML map[cam_id]Capabilities. These flags are NOT exposed by Frigate's
// /api/config — they are local, hand-edited overrides. Cameras absent from
// the file get the zero value {talk_back: false, ptz: false}.
//
// Storage shape (see fileSchema):
//
//	capabilities:
//	  cam5: { talk_back: true, ptz: false }
//	  cam6: { talk_back: true }
//
// The store is a passive override layer — no CameraExistsFunc, no validation:
// the cameras.Store registry refresh path drives all cleanup via Forget.
package capabilities

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Capabilities is the per-camera capability set served back to the API
// layer's /api/cameras response.
type Capabilities struct {
	TalkBack bool `yaml:"talk_back" json:"talk_back"`
	PTZ      bool `yaml:"ptz" json:"ptz"`
}

// fileSchema is the on-disk YAML envelope.
type fileSchema struct {
	Capabilities map[string]Capabilities `yaml:"capabilities"`
}

// Store is a thread-safe, YAML-backed map[cam_id]Capabilities.
type Store struct {
	path string
	mu   sync.RWMutex
	caps map[string]Capabilities
}

// New loads the capabilities file at path. Missing → empty store (the file
// is not created until the first Forget mutation). Malformed YAML → error
// (fail-fast). The store has no Set method in sprint B — entries are
// hand-edited on the host until a future editor lands.
func New(path string) (*Store, error) {
	s := &Store{path: path, caps: map[string]Capabilities{}}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("capabilities: read %s: %w", path, err)
	}

	var fs fileSchema
	if err := yaml.Unmarshal(data, &fs); err != nil {
		return nil, fmt.Errorf("capabilities: parse %s: %w", path, err)
	}
	if fs.Capabilities != nil {
		for id, c := range fs.Capabilities {
			s.caps[id] = c
		}
	}
	return s, nil
}

// NewForTest builds a Store backed by the supplied map without touching
// disk. Tests use this to skip filesystem setup.
func NewForTest(initial map[string]Capabilities) *Store {
	return &Store{caps: cloneMap(initial)}
}

// Get returns the stored capability set for camID, or the zero value when
// no entry exists. Never returns ok — absence == zero, consistent with the
// "passive override" semantics.
func (s *Store) Get(camID string) Capabilities {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.caps[camID]
}

// All returns a defensive copy of the override map. Intended for tests and
// future read-only API surfaces.
func (s *Store) All() map[string]Capabilities {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneMap(s.caps)
}

// Forget removes camID's capability entry, writing the file atomically.
// No-op (no disk write) when camID is not present, so refresh cleanup over
// many never-overridden cams does not thrash the filesystem.
func (s *Store) Forget(camID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.caps[camID]; !ok {
		return nil
	}
	next := cloneMap(s.caps)
	delete(next, camID)
	if err := s.atomicWrite(next); err != nil {
		return err
	}
	s.caps = next
	return nil
}

func cloneMap(in map[string]Capabilities) map[string]Capabilities {
	out := make(map[string]Capabilities, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// atomicWrite serialises the override map via temp + rename.
func (s *Store) atomicWrite(m map[string]Capabilities) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("capabilities: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "capabilities-*.tmp")
	if err != nil {
		return fmt.Errorf("capabilities: create temp: %w", err)
	}
	tmpPath := tmp.Name()

	payload := fileSchema{Capabilities: m}
	if payload.Capabilities == nil {
		payload.Capabilities = map[string]Capabilities{}
	}

	enc := yaml.NewEncoder(tmp)
	enc.SetIndent(2)
	if err := enc.Encode(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("capabilities: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("capabilities: close encoder: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("capabilities: close temp: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("capabilities: chmod temp: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("capabilities: rename: %w", err)
	}
	return nil
}

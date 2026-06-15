// Package cameraorder persists the household-shared display order for
// cameras as a YAML list of cam_ids. The list is the user's preference
// layer for camera ordering; the API handler applies it on top of the
// registry from internal/cameras to produce the GET /api/cameras response.
//
// Storage shape (see fileSchema):
//
//	order:
//	  - cam5
//	  - cam1
//	  - cam3
//
// The top-level `order` key is forward-compatible — sibling keys can land
// later without a migration.
package cameraorder

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// CameraExistsFunc is injected from main.go to validate cam ids against
// the live camera registry without pulling a cameras dependency in here.
type CameraExistsFunc func(id string) bool

// fileSchema is the on-disk YAML envelope.
type fileSchema struct {
	Order []string `yaml:"order"`
}

// Store is a thread-safe, YAML-backed ordered list of cam ids. Unknown
// ids (cameras that have been removed from Frigate) are stripped at
// write time and on Set; the file is the user's preference, not a
// registry.
type Store struct {
	path        string
	cameraExist CameraExistsFunc
	mu          sync.RWMutex
	order       []string
}

// New loads the order file at path. Missing → empty store (the file is
// not created until the first Set). Malformed YAML → error (fail-fast).
// Unknown ids found in the on-disk file are dropped on load so a stale
// snapshot from before a refresh never re-introduces a ghost camera.
func New(path string, cameraExist CameraExistsFunc) (*Store, error) {
	s := &Store{path: path, cameraExist: cameraExist, order: []string{}}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return nil, fmt.Errorf("cameraorder: read %s: %w", path, err)
	}

	var fs fileSchema
	if err := yaml.Unmarshal(data, &fs); err != nil {
		return nil, fmt.Errorf("cameraorder: parse %s: %w", path, err)
	}
	s.order = dedupAndFilter(fs.Order, cameraExist)
	return s, nil
}

// All returns a defensive copy of the current order list.
func (s *Store) All() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, len(s.order))
	copy(out, s.order)
	return out
}

// Apply returns ids reordered according to the saved order. Ids present
// in the saved order appear first in that order; ids not present in the
// saved order are appended in the input order (so newly discovered
// cameras never vanish). Ids in the saved order that are absent from
// the input are dropped.
func (s *Store) Apply(ids []string) []string {
	if s == nil || len(ids) == 0 {
		out := make([]string, len(ids))
		copy(out, ids)
		return out
	}
	s.mu.RLock()
	saved := s.order
	s.mu.RUnlock()

	present := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		present[id] = struct{}{}
	}

	out := make([]string, 0, len(ids))
	placed := make(map[string]struct{}, len(ids))
	for _, id := range saved {
		if _, ok := present[id]; !ok {
			continue
		}
		out = append(out, id)
		placed[id] = struct{}{}
	}
	for _, id := range ids {
		if _, ok := placed[id]; ok {
			continue
		}
		out = append(out, id)
	}
	return out
}

// Set replaces the saved order with the supplied ids. Unknown ids
// (not in the camera registry) are silently dropped, mirroring the
// API contract: callers send their full desired order; the server is
// the validator. Duplicates are de-duplicated, keeping the first
// occurrence.
func (s *Store) Set(ids []string) ([]string, error) {
	next := dedupAndFilter(ids, s.cameraExist)

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.atomicWrite(next); err != nil {
		return nil, err
	}
	s.order = next
	out := make([]string, len(next))
	copy(out, next)
	return out, nil
}

// Forget removes camID from the saved order, writing the file atomically.
// No-op (no disk write) when camID is not present, so refresh cleanup
// over many never-ordered cams does not thrash the filesystem.
func (s *Store) Forget(camID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := -1
	for i, id := range s.order {
		if id == camID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil
	}
	next := make([]string, 0, len(s.order)-1)
	next = append(next, s.order[:idx]...)
	next = append(next, s.order[idx+1:]...)
	if err := s.atomicWrite(next); err != nil {
		return err
	}
	s.order = next
	return nil
}

// dedupAndFilter returns ids with duplicates removed (first occurrence
// wins) and unknown ids stripped when cameraExist is non-nil. A nil
// cameraExist is treated as "allow all" so the load path can keep an
// otherwise-valid file even if the registry is briefly unavailable.
func dedupAndFilter(ids []string, cameraExist CameraExistsFunc) []string {
	out := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		if cameraExist != nil && !cameraExist(id) {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// atomicWrite serialises the order list via temp + rename.
func (s *Store) atomicWrite(order []string) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cameraorder: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "camera_order-*.tmp")
	if err != nil {
		return fmt.Errorf("cameraorder: create temp: %w", err)
	}
	tmpPath := tmp.Name()

	// Make a nil order encode as an empty list rather than `null` so the
	// file stays readable when the order is fully cleared.
	payload := fileSchema{Order: order}
	if payload.Order == nil {
		payload.Order = []string{}
	}

	enc := yaml.NewEncoder(tmp)
	enc.SetIndent(2)
	if err := enc.Encode(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("cameraorder: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("cameraorder: close encoder: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("cameraorder: close temp: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("cameraorder: chmod temp: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("cameraorder: rename: %w", err)
	}
	return nil
}

// Package cameras implements the camera registry: a thread-safe, YAML-backed
// list of cameras sourced from Frigate's /api/config at startup and on every
// POST /api/cameras/refresh. The file is the crash-recovery snapshot —
// Frigate is the source of truth, the BFF falls back to the on-disk snapshot
// only when Frigate is unreachable on a subsequent start.
//
// Derivation rules from a Frigate cameras.<id> entry:
//
//   - Skip when enabled == false (info log).
//   - Name = Frigate's `name` field (downstream names.Store override layer
//     still applies in the API handler).
//   - StreamMain = live.streams["Main"]. If missing/empty, warn-log and fall
//     back to ffmpeg.inputs[0].path. If that is also empty, warn-log and
//     skip the camera.
//   - StreamSub  = live.streams["Sub"]. If missing/empty, leave as "".
//     Sub-quality requests for such cams will hit the existing "quality must
//     be main or sub" 400 path in the streaming handler.
//
// Capability flags (talk_back, ptz) live in internal/capabilities, not here.
//
// The on-disk slice is sorted by ID for determinism — Frigate's map
// iteration order is not stable.
package cameras

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/skua-app/skua/internal/frigate"
)

// CameraSpec is the per-camera record served to the API layer. Capability
// flags (talk_back, ptz) live in internal/capabilities and are merged in by
// the handler; they are deliberately absent here.
type CameraSpec struct {
	ID         string `yaml:"id"`
	Name       string `yaml:"name"`
	StreamMain string `yaml:"stream_main"`
	StreamSub  string `yaml:"stream_sub"`
}

// Diff is the result of a Refresh: the set of cam IDs that newly appeared
// in the snapshot and the set that disappeared. Both slices are sorted
// ascending and are never nil (empty slices, not nil, simplify JSON shape
// for the refresh endpoint).
type Diff struct {
	Added   []string
	Removed []string
}

// fileSchema is the on-disk YAML envelope, forward-compatible with sibling
// keys later (e.g. capabilities meta).
type fileSchema struct {
	Cameras []CameraSpec `yaml:"cameras"`
}

const frigateConfigTimeout = 10 * time.Second

// Store is the thread-safe camera registry.
type Store struct {
	path      string
	frigate   *frigate.Client
	mu        sync.RWMutex
	cameras   []CameraSpec
	onAdded   func(specs []CameraSpec)
	onRemoved func(camIDs []string)
}

// New loads or initialises the registry at path:
//
//  1. Read cameras.yaml from disk if present. Malformed YAML → fail-fast.
//  2. Ask Frigate for the current config with a 10s timeout.
//     - On success: derive the spec list, persist atomically, and replace
//     the in-memory slice.
//     - On error with no on-disk snapshot: fail-fast (E5: surface upstream
//     outage explicitly rather than serving an empty UI).
//     - On error with an on-disk snapshot: warn-log and proceed with the
//     cached snapshot.
//
// Hooks (SetOnAdded / SetOnRemoved) are NOT fired during New — no client is
// connected at boot. Only Refresh fires them.
func New(path string, frigateClient *frigate.Client) (*Store, error) {
	s := &Store{path: path, frigate: frigateClient}

	hasDisk := false
	if data, err := os.ReadFile(path); err == nil {
		var fs fileSchema
		if err := yaml.Unmarshal(data, &fs); err != nil {
			return nil, fmt.Errorf("parse %s: %w", path, err)
		}
		if fs.Cameras == nil {
			fs.Cameras = []CameraSpec{}
		}
		s.cameras = fs.Cameras
		hasDisk = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), frigateConfigTimeout)
	defer cancel()

	cfg, err := frigateClient.GetConfig(ctx)
	if err != nil {
		if !hasDisk {
			return nil, fmt.Errorf("frigate config unreachable and no on-disk snapshot at %s: %w", path, err)
		}
		slog.Warn("cameras: frigate config unreachable, using on-disk snapshot", "path", path, "count", len(s.cameras), "error", err)
		return s, nil
	}

	specs := deriveSpecs(cfg)
	if err := s.atomicWrite(specs); err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.cameras = specs
	s.mu.Unlock()
	return s, nil
}

// NewForTest builds a Store backed by the supplied specs without touching
// Frigate or the filesystem. Tests use this to construct a registry without
// standing up a fake Frigate.
func NewForTest(specs []CameraSpec) *Store {
	return &Store{cameras: cloneSpecs(specs)}
}

// NewForTestWithFrigate builds a Store backed by the supplied specs and a
// frigate.Client targeting an httptest fake. Tests use this to exercise
// Refresh against a fake Frigate /api/config without driving the full New
// startup flow (which involves persistence + fail-fast logic).
func NewForTestWithFrigate(path string, frigateClient *frigate.Client, specs []CameraSpec) *Store {
	return &Store{path: path, frigate: frigateClient, cameras: cloneSpecs(specs)}
}

// SetOnAdded registers a hook fired from Refresh with the specs of every
// camera that newly appeared. Called once from main.go; later calls
// overwrite with a warn log (no concurrency expected).
func (s *Store) SetOnAdded(cb func(specs []CameraSpec)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.onAdded != nil {
		slog.Warn("cameras: SetOnAdded called more than once, overwriting")
	}
	s.onAdded = cb
}

// SetOnRemoved registers a hook fired from Refresh with the IDs of every
// camera that disappeared. Called once from main.go; later calls overwrite
// with a warn log (no concurrency expected).
func (s *Store) SetOnRemoved(cb func(camIDs []string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.onRemoved != nil {
		slog.Warn("cameras: SetOnRemoved called more than once, overwriting")
	}
	s.onRemoved = cb
}

// Snapshot returns a defensive copy of the current registry.
func (s *Store) Snapshot() []CameraSpec {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSpecs(s.cameras)
}

// Find returns the spec for id and true, or the zero value and false.
func (s *Store) Find(id string) (CameraSpec, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, c := range s.cameras {
		if c.ID == id {
			return c, true
		}
	}
	return CameraSpec{}, false
}

// Refresh re-pulls Frigate /api/config, derives the spec list, computes the
// diff against the current snapshot, persists the new snapshot, swaps the
// in-memory slice, and fires the OnAdded / OnRemoved hooks (in that order).
// On any error from Frigate or persistence, the in-memory snapshot and the
// on-disk YAML are left untouched and no hooks fire.
func (s *Store) Refresh(ctx context.Context) (Diff, error) {
	cfg, err := s.frigate.GetConfig(ctx)
	if err != nil {
		return Diff{Added: []string{}, Removed: []string{}}, fmt.Errorf("cameras: refresh: %w", err)
	}
	next := deriveSpecs(cfg)

	s.mu.Lock()
	prev := s.cameras
	added, removed, addedSpecs := diffSpecs(prev, next)
	if err := s.atomicWrite(next); err != nil {
		s.mu.Unlock()
		return Diff{Added: []string{}, Removed: []string{}}, err
	}
	s.cameras = next
	onAdded := s.onAdded
	onRemoved := s.onRemoved
	s.mu.Unlock()

	if onAdded != nil && len(addedSpecs) > 0 {
		onAdded(addedSpecs)
	}
	if onRemoved != nil && len(removed) > 0 {
		onRemoved(removed)
	}
	return Diff{Added: added, Removed: removed}, nil
}

// diffSpecs returns the sorted IDs added and removed between prev and next,
// plus the full specs for the added cams (so the OnAdded hook can include
// the display name in its broadcast payload). next must already be sorted.
func diffSpecs(prev, next []CameraSpec) (added, removed []string, addedSpecs []CameraSpec) {
	prevByID := make(map[string]struct{}, len(prev))
	for _, c := range prev {
		prevByID[c.ID] = struct{}{}
	}
	nextByID := make(map[string]CameraSpec, len(next))
	for _, c := range next {
		nextByID[c.ID] = c
	}

	added = []string{}
	addedSpecs = []CameraSpec{}
	for _, c := range next {
		if _, ok := prevByID[c.ID]; !ok {
			added = append(added, c.ID)
			addedSpecs = append(addedSpecs, c)
		}
	}
	removed = []string{}
	for _, c := range prev {
		if _, ok := nextByID[c.ID]; !ok {
			removed = append(removed, c.ID)
		}
	}
	sort.Strings(added)
	sort.Strings(removed)
	sort.Slice(addedSpecs, func(i, j int) bool { return addedSpecs[i].ID < addedSpecs[j].ID })
	return
}

// deriveSpecs converts a Frigate config response into the sorted spec list
// the BFF serves. See package doc for the derivation rules.
func deriveSpecs(cfg *frigate.ConfigResponse) []CameraSpec {
	specs := make([]CameraSpec, 0, len(cfg.Cameras))
	for id, cam := range cfg.Cameras {
		if !cam.Enabled {
			slog.Info("cameras: skipping disabled camera", "id", id)
			continue
		}
		name := cam.Name
		if name == "" {
			name = id
		}

		main := cam.Live.Streams["Main"]
		if main == "" {
			fallback := ""
			if len(cam.FFmpeg.Inputs) > 0 {
				fallback = cam.FFmpeg.Inputs[0].Path
			}
			if fallback == "" {
				slog.Warn("cameras: no streams or inputs, skipping", "id", id)
				continue
			}
			slog.Warn("cameras: no live.streams.Main, falling back to first input path", "id", id)
			main = fallback
		}
		sub := cam.Live.Streams["Sub"]

		specs = append(specs, CameraSpec{
			ID:         id,
			Name:       name,
			StreamMain: main,
			StreamSub:  sub,
		})
	}
	sort.Slice(specs, func(i, j int) bool { return specs[i].ID < specs[j].ID })
	return specs
}

func cloneSpecs(in []CameraSpec) []CameraSpec {
	out := make([]CameraSpec, len(in))
	copy(out, in)
	return out
}

// atomicWrite serializes specs to disk via temp + rename. Error messages
// here are bare (no "cameras:" prefix) because main.go's diagnostic prefixes
// them once; keeping the prefix here as well would produce "cameras: cameras:
// create temp: ..." in the user-facing stderr output. The %w wrapping
// preserves the unwrap chain, so errors.Is(err, fs.ErrPermission) still
// matches an underlying EACCES on /data.
func (s *Store) atomicWrite(specs []CameraSpec) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "cameras-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()

	enc := yaml.NewEncoder(tmp)
	enc.SetIndent(2)
	payload := fileSchema{Cameras: specs}
	if err := enc.Encode(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close encoder: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod temp: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

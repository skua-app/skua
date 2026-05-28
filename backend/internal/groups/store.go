// Package groups implements the camera-group store: a thread-safe, YAML-backed
// store with a single-membership invariant (a camera belongs to at most one
// group). The only writer is our own /api/groups* endpoints — there is no
// hot-reload watcher.
package groups

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

const (
	nameMinLen = 1
	nameMaxLen = 30
)

// Error values returned by Create/Update so callers can map them to HTTP
// status codes and error codes.
var (
	ErrNameEmpty       = errors.New("name_empty")
	ErrNameTooLong     = errors.New("name_too_long")
	ErrNameDuplicate   = errors.New("name_duplicate")
	ErrCameraNotFound  = errors.New("camera_not_found")
	ErrDuplicateCamera = errors.New("duplicate_camera")
	ErrNotFound        = errors.New("not_found")
)

// Group is the on-disk and in-memory representation of a single camera group.
// camera_ids carries the cameras in this group (0 or 1 elements per camera
// across the whole store, enforced server-side).
type Group struct {
	ID        string   `yaml:"id" json:"id"`
	Name      string   `yaml:"name" json:"name"`
	CameraIDs []string `yaml:"camera_ids" json:"camera_ids"`
}

// fileSchema is the top-level YAML shape so the file is forward-compatible
// with additional sibling keys later.
type fileSchema struct {
	Groups []Group `yaml:"groups"`
}

// CameraExistsFunc is used to validate that a camera id corresponds to a
// known camera (currently config.FindCamera). Injected to keep the groups
// package free of a direct config dependency.
type CameraExistsFunc func(id string) bool

// Store is the thread-safe groups store, persisted as YAML.
type Store struct {
	path        string
	cameraExist CameraExistsFunc
	mu          sync.RWMutex
	groups      []Group
}

// New loads or initialises the groups file at path. Missing → empty store
// (file is not created until first write). Malformed YAML → error (fail-fast).
func New(path string, cameraExist CameraExistsFunc) (*Store, error) {
	s := &Store{path: path, cameraExist: cameraExist}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.groups = []Group{}
			return s, nil
		}
		return nil, fmt.Errorf("groups: read %s: %w", path, err)
	}

	var fs fileSchema
	if err := yaml.Unmarshal(data, &fs); err != nil {
		return nil, fmt.Errorf("groups: parse %s: %w", path, err)
	}
	if fs.Groups == nil {
		fs.Groups = []Group{}
	}
	for i := range fs.Groups {
		if fs.Groups[i].CameraIDs == nil {
			fs.Groups[i].CameraIDs = []string{}
		}
	}
	s.groups = fs.Groups
	return s, nil
}

// List returns a snapshot copy of all groups, in creation order.
func (s *Store) List() []Group {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Group, len(s.groups))
	for i, g := range s.groups {
		out[i] = cloneGroup(g)
	}
	return out
}

// Get returns the group with the given id and true, or the zero value and false.
func (s *Store) Get(id string) (Group, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, g := range s.groups {
		if g.ID == id {
			return cloneGroup(g), true
		}
	}
	return Group{}, false
}

// Create appends a new group with an empty camera list and persists.
func (s *Store) Create(name string) (Group, error) {
	name = strings.TrimSpace(name)
	if err := validateName(name); err != nil {
		return Group{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.nameTaken(name, "") {
		return Group{}, ErrNameDuplicate
	}

	g := Group{
		ID:        uuid.NewString(),
		Name:      name,
		CameraIDs: []string{},
	}
	next := append(cloneGroups(s.groups), g)
	if err := s.atomicWrite(next); err != nil {
		return Group{}, err
	}
	s.groups = next
	return cloneGroup(g), nil
}

// Update replaces name and/or camera_ids for the group with id. Any camera id
// added here is removed from other groups (single-membership invariant).
// Pass nil for fields that should be left unchanged.
func (s *Store) Update(id string, name *string, cameraIDs *[]string) (Group, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	for i, g := range s.groups {
		if g.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return Group{}, ErrNotFound
	}

	next := cloneGroups(s.groups)
	target := next[idx]

	if name != nil {
		trimmed := strings.TrimSpace(*name)
		if err := validateName(trimmed); err != nil {
			return Group{}, err
		}
		if s.nameTakenIn(next, trimmed, id) {
			return Group{}, ErrNameDuplicate
		}
		target.Name = trimmed
	}

	if cameraIDs != nil {
		cleaned, err := s.validateCameras(*cameraIDs)
		if err != nil {
			return Group{}, err
		}
		// Strip these cameras from every other group (single-membership).
		owned := make(map[string]struct{}, len(cleaned))
		for _, cid := range cleaned {
			owned[cid] = struct{}{}
		}
		for i := range next {
			if i == idx {
				continue
			}
			next[i].CameraIDs = filterOut(next[i].CameraIDs, owned)
		}
		target.CameraIDs = cleaned
	}

	next[idx] = target
	if err := s.atomicWrite(next); err != nil {
		return Group{}, err
	}
	s.groups = next
	return cloneGroup(target), nil
}

// Delete removes the group with id. Cameras stay (they simply lose their
// membership). Returns ErrNotFound if no such group.
func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	for i, g := range s.groups {
		if g.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ErrNotFound
	}
	next := make([]Group, 0, len(s.groups)-1)
	next = append(next, cloneGroups(s.groups[:idx])...)
	next = append(next, cloneGroups(s.groups[idx+1:])...)
	if err := s.atomicWrite(next); err != nil {
		return err
	}
	s.groups = next
	return nil
}

// GroupFor returns the id of the group containing cameraID, or "" if none.
func (s *Store) GroupFor(cameraID string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, g := range s.groups {
		for _, c := range g.CameraIDs {
			if c == cameraID {
				return g.ID
			}
		}
	}
	return ""
}

// Forget strips camID from every group's camera_ids and persists the result.
// No-op (no disk write) when camID is not a member of any group, so refresh
// cleanup over many never-grouped cams does not thrash the filesystem.
func (s *Store) Forget(camID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	changed := false
	for _, g := range s.groups {
		for _, c := range g.CameraIDs {
			if c == camID {
				changed = true
				break
			}
		}
		if changed {
			break
		}
	}
	if !changed {
		return nil
	}

	next := cloneGroups(s.groups)
	drop := map[string]struct{}{camID: {}}
	for i := range next {
		next[i].CameraIDs = filterOut(next[i].CameraIDs, drop)
	}
	if err := s.atomicWrite(next); err != nil {
		return err
	}
	s.groups = next
	return nil
}

// Exists reports whether a group with id is present.
func (s *Store) Exists(id string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, g := range s.groups {
		if g.ID == id {
			return true
		}
	}
	return false
}

func validateName(name string) error {
	if len(name) < nameMinLen {
		return ErrNameEmpty
	}
	if len([]rune(name)) > nameMaxLen {
		return ErrNameTooLong
	}
	return nil
}

func (s *Store) nameTaken(name, exceptID string) bool {
	return s.nameTakenIn(s.groups, name, exceptID)
}

func (s *Store) nameTakenIn(groups []Group, name, exceptID string) bool {
	lower := strings.ToLower(name)
	for _, g := range groups {
		if g.ID == exceptID {
			continue
		}
		if strings.ToLower(g.Name) == lower {
			return true
		}
	}
	return false
}

func (s *Store) validateCameras(ids []string) ([]string, error) {
	seen := make(map[string]struct{}, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, dup := seen[id]; dup {
			return nil, ErrDuplicateCamera
		}
		if s.cameraExist != nil && !s.cameraExist(id) {
			return nil, ErrCameraNotFound
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

func filterOut(ids []string, drop map[string]struct{}) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, skip := drop[id]; skip {
			continue
		}
		out = append(out, id)
	}
	return out
}

func cloneGroup(g Group) Group {
	cams := make([]string, len(g.CameraIDs))
	copy(cams, g.CameraIDs)
	return Group{ID: g.ID, Name: g.Name, CameraIDs: cams}
}

func cloneGroups(in []Group) []Group {
	out := make([]Group, len(in))
	for i, g := range in {
		out[i] = cloneGroup(g)
	}
	return out
}

// atomicWrite serializes groups to disk via temp + rename.
func (s *Store) atomicWrite(groups []Group) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("groups: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "groups-*.tmp")
	if err != nil {
		return fmt.Errorf("groups: create temp: %w", err)
	}
	tmpPath := tmp.Name()

	enc := yaml.NewEncoder(tmp)
	enc.SetIndent(2)
	payload := fileSchema{Groups: groups}
	if err := enc.Encode(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("groups: encode: %w", err)
	}
	if err := enc.Close(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("groups: close encoder: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("groups: close temp: %w", err)
	}
	if err := os.Chmod(tmpPath, 0o644); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("groups: chmod temp: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("groups: rename: %w", err)
	}
	return nil
}

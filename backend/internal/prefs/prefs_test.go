package prefs_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/skua-app/skua/internal/prefs"
)

func TestNew_MissingFile_ReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := store.Get()
	if p.GridMode != "eco" {
		t.Errorf("GridMode: got %q, want %q", p.GridMode, "eco")
	}
	if !p.MutedByDefault {
		t.Errorf("MutedByDefault: got false, want true")
	}
}

func TestNew_ExistingFile_ReturnsStoredValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")

	stored := prefs.Prefs{GridMode: "hd", MutedByDefault: false}
	data, err := json.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := prefs.New(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := store.Get()
	if p.GridMode != "hd" {
		t.Errorf("GridMode: got %q, want %q", p.GridMode, "hd")
	}
	if p.MutedByDefault {
		t.Errorf("MutedByDefault: got true, want false")
	}
}

func TestUpdate_ValidPartial_MergesCorrectly(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}

	updated, err := store.Update(map[string]any{"grid_mode": "hd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.GridMode != "hd" {
		t.Errorf("GridMode: got %q, want %q", updated.GridMode, "hd")
	}
	// MutedByDefault should retain default
	if !updated.MutedByDefault {
		t.Errorf("MutedByDefault: got false, want true (default)")
	}

	// Partial update of muted_by_default should not change grid_mode
	updated2, err := store.Update(map[string]any{"muted_by_default": false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated2.GridMode != "hd" {
		t.Errorf("GridMode after muted update: got %q, want %q", updated2.GridMode, "hd")
	}
	if updated2.MutedByDefault {
		t.Errorf("MutedByDefault: got true, want false")
	}
}

func TestUpdate_InvalidGridMode_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Update(map[string]any{"grid_mode": "invalid"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpdate_UnknownField_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Update(map[string]any{"nonexistent": "value"})
	if err == nil {
		t.Fatal("expected error for unknown field, got nil")
	}
}

func TestNew_MissingFile_ReturnsNewFieldDefaults(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	p := store.Get()
	if p.Accent != "cyan" {
		t.Errorf("Accent: got %q, want %q", p.Accent, "cyan")
	}
	if p.NameStyle != "below" {
		t.Errorf("NameStyle: got %q, want %q", p.NameStyle, "below")
	}
	if p.ShowTimestamp {
		t.Errorf("ShowTimestamp: got true, want false")
	}
	if p.DesktopColumns != 4 {
		t.Errorf("DesktopColumns: got %d, want 4", p.DesktopColumns)
	}
}

func TestUpdate_NewFields_ValidValues(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}

	updated, err := store.Update(map[string]any{
		"accent":          "violet",
		"name_style":      "overlay",
		"show_timestamp":  true,
		"desktop_columns": float64(3),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Accent != "violet" {
		t.Errorf("Accent: got %q, want %q", updated.Accent, "violet")
	}
	if updated.NameStyle != "overlay" {
		t.Errorf("NameStyle: got %q, want %q", updated.NameStyle, "overlay")
	}
	if !updated.ShowTimestamp {
		t.Errorf("ShowTimestamp: got false, want true")
	}
	if updated.DesktopColumns != 3 {
		t.Errorf("DesktopColumns: got %d, want 3", updated.DesktopColumns)
	}

	// Partial update must not reset previously set new fields.
	updated2, err := store.Update(map[string]any{"accent": "sage"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated2.Accent != "sage" {
		t.Errorf("Accent: got %q, want %q", updated2.Accent, "sage")
	}
	if updated2.NameStyle != "overlay" {
		t.Errorf("NameStyle after partial: got %q, want %q", updated2.NameStyle, "overlay")
	}
	if updated2.DesktopColumns != 3 {
		t.Errorf("DesktopColumns after partial: got %d, want 3", updated2.DesktopColumns)
	}
}

func TestUpdate_InvalidAccent_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Update(map[string]any{"accent": "crimson"})
	if err == nil {
		t.Fatal("expected error for invalid accent, got nil")
	}
}

func TestUpdate_InvalidNameStyle_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Update(map[string]any{"name_style": "left"})
	if err == nil {
		t.Fatal("expected error for invalid name_style, got nil")
	}
}

func TestUpdate_NameStyleOff_PersistsAcrossReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	store, err := prefs.New(path)
	if err != nil {
		t.Fatal(err)
	}

	updated, err := store.Update(map[string]any{"name_style": "off"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.NameStyle != "off" {
		t.Errorf("NameStyle: got %q, want %q", updated.NameStyle, "off")
	}

	// "off" must survive sanitize on load, not be reset to the default.
	reloaded, err := prefs.New(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := reloaded.Get().NameStyle; got != "off" {
		t.Errorf("NameStyle after reload: got %q, want %q", got, "off")
	}
}

func TestUpdate_InvalidDesktopColumns_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Update(map[string]any{"desktop_columns": float64(7)})
	if err == nil {
		t.Fatal("expected error for out-of-range desktop_columns, got nil")
	}
}

func TestUpdate_NonIntegerDesktopColumns_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = store.Update(map[string]any{"desktop_columns": float64(3.5)})
	if err == nil {
		t.Fatal("expected error for non-integer desktop_columns, got nil")
	}
}

func TestUpdate_GridFPS_ValidValuesAccepted(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Default is 1 Hz; bumping to 2 must round-trip, then back to 1.
	for _, want := range []int{2, 1} {
		updated, err := store.Update(map[string]any{"grid_fps": float64(want)})
		if err != nil {
			t.Fatalf("grid_fps=%d: unexpected error: %v", want, err)
		}
		if updated.GridFPS != want {
			t.Errorf("grid_fps=%d: GridFPS = %d, want %d", want, updated.GridFPS, want)
		}
	}
}

func TestUpdate_InvalidGridFPS_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(map[string]any{"grid_fps": float64(3)}); err == nil {
		t.Fatal("expected error for out-of-range grid_fps, got nil")
	}
	if _, err := store.Update(map[string]any{"grid_fps": float64(1.5)}); err == nil {
		t.Fatal("expected error for non-integer grid_fps, got nil")
	}
}

func TestNew_MissingFile_ReturnsTimelineZoomAnchorDefault(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := store.Get().TimelineZoomAnchor; got != "pointer" {
		t.Errorf("TimelineZoomAnchor: got %q, want default %q", got, "pointer")
	}
}

func TestUpdate_TimelineZoomAnchor_PersistsAcrossReload(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	store, err := prefs.New(path)
	if err != nil {
		t.Fatal(err)
	}

	updated, err := store.Update(map[string]any{"timeline_zoom_anchor": "playhead"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.TimelineZoomAnchor != "playhead" {
		t.Errorf("TimelineZoomAnchor: got %q, want %q", updated.TimelineZoomAnchor, "playhead")
	}

	reloaded, err := prefs.New(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.Get().TimelineZoomAnchor; got != "playhead" {
		t.Errorf("TimelineZoomAnchor after reload: got %q, want %q", got, "playhead")
	}
}

func TestUpdate_InvalidTimelineZoomAnchor_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Update(map[string]any{"timeline_zoom_anchor": "centre"}); err == nil {
		t.Fatal("expected error for invalid timeline_zoom_anchor, got nil")
	}
	if _, err := store.Update(map[string]any{"timeline_zoom_anchor": 1}); err == nil {
		t.Fatal("expected error for non-string timeline_zoom_anchor, got nil")
	}
}

func TestNew_InvalidFieldValues_SanitizedToDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")

	// Mix invalid fields with valid non-default fields. Sanitize must reset
	// the invalid ones to defaults while preserving the valid ones — a
	// field-scoped reset, not a wholesale defaults restore.
	raw := []byte(`{
		"grid_mode": "foo",
		"muted_by_default": false,
		"stream_quality": "ultra",
		"show_timestamp": true,
		"accent": "neon",
		"name_style": "sideways",
		"desktop_columns": 99,
		"mobile_columns": 7,
		"timeline_zoom_anchor": "sideways"
	}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	store, err := prefs.New(path)
	if err != nil {
		t.Fatalf("New must not error on invalid field values, got: %v", err)
	}
	p := store.Get()

	if p.GridMode != "eco" {
		t.Errorf("GridMode: got %q, want default %q", p.GridMode, "eco")
	}
	if p.StreamQuality != "main" {
		t.Errorf("StreamQuality: got %q, want default %q", p.StreamQuality, "main")
	}
	if p.Accent != "cyan" {
		t.Errorf("Accent: got %q, want default %q", p.Accent, "cyan")
	}
	if p.NameStyle != "below" {
		t.Errorf("NameStyle: got %q, want default %q", p.NameStyle, "below")
	}
	if p.DesktopColumns != 4 {
		t.Errorf("DesktopColumns: got %d, want default 4", p.DesktopColumns)
	}
	if p.MobileColumns != 1 {
		t.Errorf("MobileColumns: got %d, want default 1", p.MobileColumns)
	}
	if p.TimelineZoomAnchor != "pointer" {
		t.Errorf("TimelineZoomAnchor: got %q, want default %q", p.TimelineZoomAnchor, "pointer")
	}

	// Valid non-default bool fields must be preserved across sanitize.
	if p.MutedByDefault {
		t.Errorf("MutedByDefault: got true, want preserved false")
	}
	if !p.ShowTimestamp {
		t.Errorf("ShowTimestamp: got false, want preserved true")
	}
}

func TestNew_MissingFile_ReturnsGlanceMaxMomentsDefault(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := store.Get().GlanceMaxMoments; got != 20 {
		t.Errorf("GlanceMaxMoments: got %d, want default 20", got)
	}
}

func TestUpdate_GlanceMaxMoments_ValidValue(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}

	updated, err := store.Update(map[string]any{"glance_max_moments": float64(30)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.GlanceMaxMoments != 30 {
		t.Errorf("GlanceMaxMoments: got %d, want 30", updated.GlanceMaxMoments)
	}
}

func TestUpdate_GlanceMaxMoments_InvalidValueRejected(t *testing.T) {
	dir := t.TempDir()
	store, err := prefs.New(filepath.Join(dir, "prefs.json"))
	if err != nil {
		t.Fatal(err)
	}

	if _, err := store.Update(map[string]any{"glance_max_moments": float64(7)}); err == nil {
		t.Fatal("expected error for out-of-set glance_max_moments, got nil")
	}
	if _, err := store.Update(map[string]any{"glance_max_moments": float64(20.5)}); err == nil {
		t.Fatal("expected error for non-integer glance_max_moments, got nil")
	}
}

func TestNew_InvalidGlanceMaxMoments_SanitizedToDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	raw := []byte(`{"glance_max_moments": 7}`)
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := prefs.New(path)
	if err != nil {
		t.Fatalf("New must not error on invalid field values, got: %v", err)
	}
	if got := store.Get().GlanceMaxMoments; got != 20 {
		t.Errorf("GlanceMaxMoments: got %d, want default 20", got)
	}
}

func TestNew_MalformedJSON_ReturnsError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	if err := os.WriteFile(path, []byte(`{not json`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prefs.New(path); err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestUpdate_CreatesMissingDataDir(t *testing.T) {
	dir := t.TempDir()
	// Point the store at a path whose parent directory does not yet exist.
	// atomicWrite must create it via MkdirAll on first write.
	nested := filepath.Join(dir, "nested", "subdir")
	path := filepath.Join(nested, "prefs.json")

	store, err := prefs.New(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Update(map[string]any{"grid_mode": "hd"}); err != nil {
		t.Fatalf("Update on missing parent dir failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected prefs.json at %s after Update, got: %v", path, err)
	}
}

func TestUpdate_ConcurrentCalls_DoNotCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prefs.json")
	store, err := prefs.New(path)
	if err != nil {
		t.Fatal(err)
	}

	const n = 50
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			mode := "hd"
			if i%2 == 0 {
				mode = "eco"
			}
			_, _ = store.Update(map[string]any{"grid_mode": mode})
		}(i)
	}
	wg.Wait()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file after concurrent updates: %v", err)
	}
	var p prefs.Prefs
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatalf("file corrupted after concurrent updates: %v", err)
	}
	if p.GridMode != "hd" && p.GridMode != "eco" {
		t.Errorf("unexpected grid_mode %q after concurrent updates", p.GridMode)
	}
}

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

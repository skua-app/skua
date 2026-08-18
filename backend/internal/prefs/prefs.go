package prefs

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Prefs holds the shared household preferences persisted at PrefsPath.
// GridFilter is a pointer so the JSON value `null` round-trips as "no filter".
type Prefs struct {
	GridMode          string  `json:"grid_mode"`
	MutedByDefault    bool    `json:"muted_by_default"`
	StreamQuality     string  `json:"stream_quality"`
	ShowTelemetry     bool    `json:"show_telemetry"`
	Accent            string  `json:"accent"`
	NameStyle         string  `json:"name_style"`
	ShowTimestamp     bool    `json:"show_timestamp"`
	DesktopColumns    int     `json:"desktop_columns"`
	MobileColumns     int     `json:"mobile_columns"`
	GridFilter        *string `json:"grid_filter"`
	GlanceWindowHours int     `json:"glance_window_hours"`
	GlanceMaxMoments  int     `json:"glance_max_moments"`
	GridFPS           int     `json:"grid_fps"`
	// TimelineZoomAnchor selects what a zoom gesture on the recording timeline
	// holds in place: "pointer" keeps the time under the cursor fixed, "playhead"
	// keeps the timeline centred on the playhead. Named for zoom anchoring in
	// general, not for the wheel — the pinch gesture adopts the same preference.
	TimelineZoomAnchor string `json:"timeline_zoom_anchor"`
}

var defaults = Prefs{
	GridMode:          "eco",
	MutedByDefault:    true,
	StreamQuality:     "main",
	ShowTelemetry:     false,
	Accent:            "cyan",
	NameStyle:         "below",
	ShowTimestamp:     false,
	DesktopColumns:    4,
	MobileColumns:     1,
	GridFilter:        nil,
	GlanceWindowHours: 24,
	GlanceMaxMoments:  20,
	GridFPS:           1,
	// Cursor anchoring by default: the focus view's image zoom already anchors
	// on the pointer, so the timeline agreeing with it is the least surprising
	// behaviour. Centre-on-playhead stays available.
	TimelineZoomAnchor: "pointer",
}

var validGridModes = map[string]bool{"hd": true, "eco": true}
var validAccents = map[string]bool{"cyan": true, "sage": true, "amber": true, "violet": true}
var validNameStyles = map[string]bool{"below": true, "overlay": true, "off": true}
var validDesktopColumns = map[int]bool{2: true, 3: true, 4: true, 5: true}
var validMobileColumns = map[int]bool{1: true, 2: true}
var validGlanceWindowHours = map[int]bool{6: true, 12: true, 24: true, 48: true, 72: true}
var validGlanceMaxMoments = map[int]bool{10: true, 20: true, 30: true, 50: true}
var validGridFPS = map[int]bool{1: true, 2: true}
var validTimelineZoomAnchors = map[string]bool{"pointer": true, "playhead": true}

// valueList renders an accepted-value set for a validation error message.
// Derived from the set rather than restated, so adding a value cannot leave
// the message naming a stale set. Sorted for a stable message.
func valueList(valid map[string]bool) string {
	out := make([]string, 0, len(valid))
	for k := range valid {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, "/")
}

var knownFields = map[string]bool{
	"grid_mode":            true,
	"muted_by_default":     true,
	"stream_quality":       true,
	"show_telemetry":       true,
	"accent":               true,
	"name_style":           true,
	"show_timestamp":       true,
	"desktop_columns":      true,
	"mobile_columns":       true,
	"grid_filter":          true,
	"glance_window_hours":  true,
	"glance_max_moments":   true,
	"grid_fps":             true,
	"timeline_zoom_anchor": true,
}

// Store is a thread-safe, file-backed preferences store.
type Store struct {
	path string
	mu   sync.RWMutex
	cur  Prefs
}

// New loads prefs from path. If the file does not exist, defaults are used.
func New(path string) (*Store, error) {
	s := &Store{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			s.cur = defaults
			return s, nil
		}
		return nil, fmt.Errorf("prefs: read %s: %w", path, err)
	}
	s.cur = defaults
	if err := json.Unmarshal(data, &s.cur); err != nil {
		return nil, fmt.Errorf("prefs: parse %s: %w", path, err)
	}
	cleaned, reset := sanitize(s.cur)
	s.cur = cleaned
	if len(reset) > 0 {
		slog.Warn("prefs: reset invalid persisted fields to defaults", "path", path, "fields", reset)
	}
	return s, nil
}

// sanitize resets any field whose persisted value is not valid back to its
// default, returning the cleaned prefs and the names of reset fields. Bool
// fields and grid_filter (free-form *string) are never reset — only the
// constrained string/int fields with documented valid sets.
func sanitize(p Prefs) (Prefs, []string) {
	var reset []string
	if !validGridModes[p.GridMode] {
		p.GridMode = defaults.GridMode
		reset = append(reset, "grid_mode")
	}
	if p.StreamQuality != "main" && p.StreamQuality != "sub" {
		p.StreamQuality = defaults.StreamQuality
		reset = append(reset, "stream_quality")
	}
	if !validAccents[p.Accent] {
		p.Accent = defaults.Accent
		reset = append(reset, "accent")
	}
	if !validNameStyles[p.NameStyle] {
		p.NameStyle = defaults.NameStyle
		reset = append(reset, "name_style")
	}
	if !validDesktopColumns[p.DesktopColumns] {
		p.DesktopColumns = defaults.DesktopColumns
		reset = append(reset, "desktop_columns")
	}
	if !validMobileColumns[p.MobileColumns] {
		p.MobileColumns = defaults.MobileColumns
		reset = append(reset, "mobile_columns")
	}
	if !validGlanceWindowHours[p.GlanceWindowHours] {
		p.GlanceWindowHours = defaults.GlanceWindowHours
		reset = append(reset, "glance_window_hours")
	}
	if !validGlanceMaxMoments[p.GlanceMaxMoments] {
		p.GlanceMaxMoments = defaults.GlanceMaxMoments
		reset = append(reset, "glance_max_moments")
	}
	if !validGridFPS[p.GridFPS] {
		p.GridFPS = defaults.GridFPS
		reset = append(reset, "grid_fps")
	}
	if !validTimelineZoomAnchors[p.TimelineZoomAnchor] {
		p.TimelineZoomAnchor = defaults.TimelineZoomAnchor
		reset = append(reset, "timeline_zoom_anchor")
	}
	return p, reset
}

// Get returns the current preferences under a read lock.
func (s *Store) Get() Prefs {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

// Update merges partial into the current prefs, validates, writes atomically,
// and returns the updated value. Unknown fields or invalid values yield an error.
func (s *Store) Update(partial map[string]any) (Prefs, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for k := range partial {
		if !knownFields[k] {
			return Prefs{}, fmt.Errorf("unknown field %q", k)
		}
	}

	next := s.cur

	if v, ok := partial["grid_mode"]; ok {
		mode, ok := v.(string)
		if !ok {
			return Prefs{}, fmt.Errorf("grid_mode must be a string")
		}
		if !validGridModes[mode] {
			return Prefs{}, fmt.Errorf("grid_mode must be %q or %q, got %q", "hd", "eco", mode)
		}
		next.GridMode = mode
	}

	if v, ok := partial["muted_by_default"]; ok {
		b, ok := v.(bool)
		if !ok {
			return Prefs{}, fmt.Errorf("muted_by_default must be a bool")
		}
		next.MutedByDefault = b
	}

	if v, ok := partial["stream_quality"]; ok {
		q, ok := v.(string)
		if !ok {
			return Prefs{}, fmt.Errorf("stream_quality must be a string")
		}
		if q != "main" && q != "sub" {
			return Prefs{}, fmt.Errorf("stream_quality must be %q or %q, got %q", "main", "sub", q)
		}
		next.StreamQuality = q
	}

	if v, ok := partial["show_telemetry"]; ok {
		b, ok := v.(bool)
		if !ok {
			return Prefs{}, fmt.Errorf("show_telemetry must be a bool")
		}
		next.ShowTelemetry = b
	}

	if v, ok := partial["accent"]; ok {
		a, ok := v.(string)
		if !ok {
			return Prefs{}, fmt.Errorf("accent must be a string")
		}
		if !validAccents[a] {
			return Prefs{}, fmt.Errorf("accent must be one of cyan/sage/amber/violet, got %q", a)
		}
		next.Accent = a
	}

	if v, ok := partial["name_style"]; ok {
		ns, ok := v.(string)
		if !ok {
			return Prefs{}, fmt.Errorf("name_style must be a string")
		}
		if !validNameStyles[ns] {
			return Prefs{}, fmt.Errorf("name_style must be one of %s, got %q", valueList(validNameStyles), ns)
		}
		next.NameStyle = ns
	}

	if v, ok := partial["show_timestamp"]; ok {
		b, ok := v.(bool)
		if !ok {
			return Prefs{}, fmt.Errorf("show_timestamp must be a bool")
		}
		next.ShowTimestamp = b
	}

	if v, ok := partial["grid_filter"]; ok {
		switch typed := v.(type) {
		case nil:
			next.GridFilter = nil
		case string:
			s := typed
			next.GridFilter = &s
		default:
			return Prefs{}, fmt.Errorf("grid_filter must be a string or null")
		}
	}

	if v, ok := partial["desktop_columns"]; ok {
		// JSON numbers decode as float64 through map[string]any.
		f, ok := v.(float64)
		if !ok {
			return Prefs{}, fmt.Errorf("desktop_columns must be a number")
		}
		n := int(f)
		if float64(n) != f {
			return Prefs{}, fmt.Errorf("desktop_columns must be an integer, got %v", f)
		}
		if !validDesktopColumns[n] {
			return Prefs{}, fmt.Errorf("desktop_columns must be 2/3/4/5, got %d", n)
		}
		next.DesktopColumns = n
	}

	if v, ok := partial["mobile_columns"]; ok {
		f, ok := v.(float64)
		if !ok {
			return Prefs{}, fmt.Errorf("mobile_columns must be a number")
		}
		n := int(f)
		if float64(n) != f {
			return Prefs{}, fmt.Errorf("mobile_columns must be an integer, got %v", f)
		}
		if !validMobileColumns[n] {
			return Prefs{}, fmt.Errorf("mobile_columns must be 1/2, got %d", n)
		}
		next.MobileColumns = n
	}

	if v, ok := partial["glance_window_hours"]; ok {
		f, ok := v.(float64)
		if !ok {
			return Prefs{}, fmt.Errorf("glance_window_hours must be a number")
		}
		n := int(f)
		if float64(n) != f {
			return Prefs{}, fmt.Errorf("glance_window_hours must be an integer, got %v", f)
		}
		if !validGlanceWindowHours[n] {
			return Prefs{}, fmt.Errorf("glance_window_hours must be 6/12/24/48/72, got %d", n)
		}
		next.GlanceWindowHours = n
	}

	if v, ok := partial["glance_max_moments"]; ok {
		f, ok := v.(float64)
		if !ok {
			return Prefs{}, fmt.Errorf("glance_max_moments must be a number")
		}
		n := int(f)
		if float64(n) != f {
			return Prefs{}, fmt.Errorf("glance_max_moments must be an integer, got %v", f)
		}
		if !validGlanceMaxMoments[n] {
			return Prefs{}, fmt.Errorf("glance_max_moments must be 10/20/30/50, got %d", n)
		}
		next.GlanceMaxMoments = n
	}

	if v, ok := partial["grid_fps"]; ok {
		f, ok := v.(float64)
		if !ok {
			return Prefs{}, fmt.Errorf("grid_fps must be a number")
		}
		n := int(f)
		if float64(n) != f {
			return Prefs{}, fmt.Errorf("grid_fps must be an integer, got %v", f)
		}
		if !validGridFPS[n] {
			return Prefs{}, fmt.Errorf("grid_fps must be 1 or 2, got %d", n)
		}
		next.GridFPS = n
	}

	if v, ok := partial["timeline_zoom_anchor"]; ok {
		a, ok := v.(string)
		if !ok {
			return Prefs{}, fmt.Errorf("timeline_zoom_anchor must be a string")
		}
		if !validTimelineZoomAnchors[a] {
			return Prefs{}, fmt.Errorf("timeline_zoom_anchor must be one of %s, got %q", valueList(validTimelineZoomAnchors), a)
		}
		next.TimelineZoomAnchor = a
	}

	if err := s.atomicWrite(next); err != nil {
		return Prefs{}, err
	}
	s.cur = next
	return next, nil
}

func (s *Store) atomicWrite(p Prefs) error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("prefs: mkdir %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, "prefs-*.tmp")
	if err != nil {
		return fmt.Errorf("prefs: create temp: %w", err)
	}
	tmpPath := tmp.Name()

	if err := json.NewEncoder(tmp).Encode(p); err != nil {
		if cerr := tmp.Close(); cerr != nil {
			return fmt.Errorf("prefs: encode: %w; close temp: %v", err, cerr)
		}
		if rerr := os.Remove(tmpPath); rerr != nil {
			return fmt.Errorf("prefs: encode: %w; remove temp: %v", err, rerr)
		}
		return fmt.Errorf("prefs: encode: %w", err)
	}
	if err := tmp.Close(); err != nil {
		if rerr := os.Remove(tmpPath); rerr != nil {
			return fmt.Errorf("prefs: close temp: %w; remove temp: %v", err, rerr)
		}
		return fmt.Errorf("prefs: close temp: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		if rerr := os.Remove(tmpPath); rerr != nil {
			return fmt.Errorf("prefs: rename: %w; remove temp: %v", err, rerr)
		}
		return fmt.Errorf("prefs: rename: %w", err)
	}
	return nil
}

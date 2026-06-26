package events

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// audioEvent builds a FrigateEvent with data.type set, the only field
// ListAudioEvents discriminates on.
func audioEvent(id, label string, start float64, typ string) FrigateEvent {
	e := FrigateEvent{ID: id, Camera: "cam1", Label: label, StartTime: start}
	e.Data.Type = typ
	return e
}

func TestListAudioEvents_KeepsOnlyAudio(t *testing.T) {
	page := []FrigateEvent{
		audioEvent("obj-1", "person", 1779310000, "object"),
		audioEvent("aud-1", "speech", 1779310010, "audio"),
		audioEvent("obj-2", "car", 1779310020, "object"),
		audioEvent("aud-2", "bark", 1779310030, "audio"),
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(page)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil, 0)
	got, err := c.ListAudioEvents(context.Background(), AudioEventParams{})
	if err != nil {
		t.Fatalf("ListAudioEvents: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d events, want 2 (audio only)", len(got))
	}
	if got[0].ID != "aud-1" || got[1].ID != "aud-2" {
		t.Errorf("kept ids = [%s %s], want [aud-1 aud-2]", got[0].ID, got[1].ID)
	}
}

func TestListAudioEvents_BuildsParams(t *testing.T) {
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("[]"))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, nil, 0)
	_, err := c.ListAudioEvents(context.Background(), AudioEventParams{
		Cameras: []string{"cam1", "cam2"},
		After:   time.Unix(1779310000, 0),
		Before:  time.Unix(1779320000, 0),
		Limit:   500,
	})
	if err != nil {
		t.Fatalf("ListAudioEvents: %v", err)
	}
	if gotQuery.Get("cameras") != "cam1,cam2" {
		t.Errorf("cameras = %q", gotQuery.Get("cameras"))
	}
	if gotQuery.Get("after") != "1779310000" {
		t.Errorf("after = %q (must be unix seconds integer)", gotQuery.Get("after"))
	}
	if gotQuery.Get("before") != "1779320000" {
		t.Errorf("before = %q (must be unix seconds integer)", gotQuery.Get("before"))
	}
	if gotQuery.Get("limit") != "500" {
		t.Errorf("limit = %q", gotQuery.Get("limit"))
	}
	// Audio events may lack a snapshot; the lane never requests one.
	if _, ok := gotQuery["has_snapshot"]; ok {
		t.Errorf("has_snapshot must not be sent for audio events; got %q", gotQuery.Get("has_snapshot"))
	}
}

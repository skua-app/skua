package events

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func reviewPtrF(v float64) *float64 { return &v }

func TestReviewItemToMoment_BasicFields(t *testing.T) {
	r := FrigateReviewItem{
		ID:        "1779310000.123-abc",
		Camera:    "camA",
		StartTime: 1779310000,
		EndTime:   reviewPtrF(1779310060),
		Severity:  "alert",
	}
	r.Data.Objects = []string{"person"}
	r.Data.Detections = []string{"1779310005.0-aaa"}
	r.Data.Zones = []string{"ZoneA"}

	m := ReviewItemToMoment(r)

	if m.ID != "1779310000.123-abc" {
		t.Errorf("ID = %q", m.ID)
	}
	if m.CamID != "camA" {
		t.Errorf("CamID = %q", m.CamID)
	}
	if m.Severity != "alert" {
		t.Errorf("Severity = %q, want alert", m.Severity)
	}
	if m.StartedAt != "2026-05-20T20:46:40Z" {
		t.Errorf("StartedAt = %q", m.StartedAt)
	}
	if m.EndedAt == nil {
		t.Fatalf("EndedAt nil, want non-nil")
	}
	if *m.EndedAt != "2026-05-20T20:47:40Z" {
		t.Errorf("EndedAt = %q", *m.EndedAt)
	}
}

func TestReviewItemToMoment_EndedAtNilWhenEndTimeNullOrZero(t *testing.T) {
	cases := []struct {
		name    string
		endTime *float64
	}{
		{"end_time null (active segment)", nil},
		{"end_time zero (treat as unset)", reviewPtrF(0)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := FrigateReviewItem{
				ID:        "1779310000.0-x",
				Camera:    "camA",
				StartTime: 1779310000,
				EndTime:   tc.endTime,
				Severity:  "detection",
			}
			m := ReviewItemToMoment(r)
			if m.EndedAt != nil {
				t.Errorf("EndedAt = %q, want nil", *m.EndedAt)
			}
		})
	}
}

func TestReviewItemToMoment_KindsLabelsZonesMapping(t *testing.T) {
	r := FrigateReviewItem{
		ID:        "1779310000.0-x",
		Camera:    "camA",
		StartTime: 1779310000,
		Severity:  "alert",
	}
	// person → KindPerson; car → KindVehicle; second person dedups; raccoon → KindOther
	r.Data.Objects = []string{"person", "car", "person", "raccoon"}
	r.Data.Zones = []string{"ZoneA", "ZoneB"}

	m := ReviewItemToMoment(r)

	wantKinds := []Kind{KindPerson, KindVehicle, KindOther}
	if !reflect.DeepEqual(m.Kinds, wantKinds) {
		t.Errorf("Kinds = %v, want %v", m.Kinds, wantKinds)
	}
	// Labels: distinct raw labels, sorted ascending.
	wantLabels := []string{"car", "person", "raccoon"}
	if !reflect.DeepEqual(m.Labels, wantLabels) {
		t.Errorf("Labels = %v, want %v", m.Labels, wantLabels)
	}
	// Zones: copied as supplied (no reordering).
	wantZones := []string{"ZoneA", "ZoneB"}
	if !reflect.DeepEqual(m.Zones, wantZones) {
		t.Errorf("Zones = %v, want %v", m.Zones, wantZones)
	}
}

func TestReviewItemToMoment_ThumbPicksLatestDetectionByPrefix(t *testing.T) {
	r := FrigateReviewItem{
		ID:        "1779310000.0-x",
		Camera:    "camA",
		StartTime: 1779310000,
		Severity:  "detection",
	}
	// Three detections; "latest" wins by unix-seconds prefix.
	r.Data.Detections = []string{
		"1779310010.5-aaa",
		"1779310200.0-bbb", // latest
		"1779310100.0-ccc",
	}

	m := ReviewItemToMoment(r)

	if m.ThumbEventID != "1779310200.0-bbb" {
		t.Errorf("ThumbEventID = %q, want 1779310200.0-bbb (latest by prefix)", m.ThumbEventID)
	}
	wantDetections := []string{
		"1779310010.5-aaa",
		"1779310200.0-bbb",
		"1779310100.0-ccc",
	}
	if !reflect.DeepEqual(m.DetectionIDs, wantDetections) {
		t.Errorf("DetectionIDs = %v, want %v (input order preserved)", m.DetectionIDs, wantDetections)
	}
}

func TestReviewItemToMoment_EmptyDetectionsLeavesThumbEmpty(t *testing.T) {
	r := FrigateReviewItem{
		ID:        "1779310000.0-x",
		Camera:    "camA",
		StartTime: 1779310000,
		Severity:  "detection",
	}
	// No detections at all.
	m := ReviewItemToMoment(r)
	if m.ThumbEventID != "" {
		t.Errorf("ThumbEventID = %q, want empty", m.ThumbEventID)
	}
	// Contract: empty, never nil (so it marshals as [] not null).
	if m.DetectionIDs == nil || len(m.DetectionIDs) != 0 {
		t.Errorf("DetectionIDs = %v, want non-nil empty", m.DetectionIDs)
	}
}

func TestReviewItemToMoment_EmptyDataYieldsEmptyArraysNotNull(t *testing.T) {
	// All slice-bearing data fields empty (zero value): the resulting Moment's
	// array fields must be non-nil empty so the JSON contract is [] not null —
	// a null wedges the glance render (GlancePeek derefs these at render time).
	r := FrigateReviewItem{
		ID:        "1779310000.0-x",
		Camera:    "camA",
		StartTime: 1779310000,
		Severity:  "detection",
	}

	m := ReviewItemToMoment(r)

	if m.Kinds == nil || len(m.Kinds) != 0 {
		t.Errorf("Kinds = %v, want non-nil empty", m.Kinds)
	}
	if m.Labels == nil || len(m.Labels) != 0 {
		t.Errorf("Labels = %v, want non-nil empty", m.Labels)
	}
	if m.Zones == nil || len(m.Zones) != 0 {
		t.Errorf("Zones = %v, want non-nil empty", m.Zones)
	}
	if m.DetectionIDs == nil || len(m.DetectionIDs) != 0 {
		t.Errorf("DetectionIDs = %v, want non-nil empty", m.DetectionIDs)
	}

	// Lock the wire contract: empty slices marshal as [], not null.
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	got := string(b)
	for _, want := range []string{`"kinds":[]`, `"labels":[]`, `"zones":[]`, `"detection_ids":[]`} {
		if !strings.Contains(got, want) {
			t.Errorf("marshaled Moment missing %s; got %s", want, got)
		}
	}
}

func TestReviewItemToMoment_MalformedDetectionIDFallsToZero(t *testing.T) {
	// Two detections: one malformed (no dash), one well-formed; the
	// well-formed wins.
	r := FrigateReviewItem{
		ID:        "1779310000.0-x",
		Camera:    "camA",
		StartTime: 1779310000,
		Severity:  "detection",
	}
	r.Data.Detections = []string{"no-prefix-id-not-a-float", "1779310010.0-real"}
	// Note: "no" parses as NaN — guard ensures it falls below "real".
	m := ReviewItemToMoment(r)
	if m.ThumbEventID != "1779310010.0-real" {
		t.Errorf("ThumbEventID = %q, want 1779310010.0-real", m.ThumbEventID)
	}
}

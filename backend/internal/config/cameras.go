// Package config carries process-wide configuration. This file used to host
// the static camera roster as a Go slice; the slice was removed in E5 when
// the camera registry moved to internal/cameras.Store, which sources cameras
// from Frigate at startup. All that remains here is a type alias so existing
// imports keep compiling, plus a slice-lookup helper used by callers that
// already hold a Snapshot.
package config

import "github.com/skua-app/skua/internal/cameras"

// CameraSpec is the per-camera capability map. The canonical definition
// lives in internal/cameras; this alias preserves the older import path.
type CameraSpec = cameras.CameraSpec

// FindCamera returns the CameraSpec with the given ID and true, or the zero
// value and false if no camera with that ID exists.
func FindCamera(cams []CameraSpec, id string) (CameraSpec, bool) {
	for _, cam := range cams {
		if cam.ID == id {
			return cam, true
		}
	}
	return CameraSpec{}, false
}

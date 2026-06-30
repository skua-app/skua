package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"
)

// storageMount is one disk mount in the GET /api/storage response. The MiB
// figures are passed through from Frigate's stats.service.storage unchanged.
type storageMount struct {
	Path     string  `json:"path"`
	Type     string  `json:"type"`
	TotalMiB float64 `json:"total_mib"`
	UsedMiB  float64 `json:"used_mib"`
	FreeMiB  float64 `json:"free_mib"`
}

// storageCamera is one camera's recordings usage in the GET /api/storage
// response, sourced from Frigate's /api/recordings/storage. The MiB and
// MiB-per-hr figures are passed through unchanged; UsagePercent is the
// camera's share of the recordings disk (already normalized 0–100).
type storageCamera struct {
	ID                string  `json:"id"`
	UsageMiB          float64 `json:"usage_mib"`
	BandwidthMiBPerHr float64 `json:"bandwidth_mib_per_hr"`
	UsagePercent      float64 `json:"usage_percent"`
}

// storageResponse is the JSON shape for GET /api/storage. Both Mounts and
// Cameras are always present and never null; an empty/missing source yields
// []. A failed per-camera fetch still returns the mounts with Cameras: [].
type storageResponse struct {
	Mounts  []storageMount  `json:"mounts"`
	Cameras []storageCamera `json:"cameras"`
}

// handleStorage sources Frigate's stats.service.storage map and returns a
// normalized, sorted list of mounts. Paths under /media sort first, all
// other paths after; within each group paths sort ascending.
func (h *Handler) handleStorage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	stats, err := h.frigate.GetStats(ctx)
	if err != nil {
		writeError(w, h.logger, http.StatusBadGateway, "upstream_error", "could not load storage stats", err)
		return
	}

	mounts := make([]storageMount, 0, len(stats.Service.Storage))
	for path, m := range stats.Service.Storage {
		mounts = append(mounts, storageMount{
			Path:     path,
			Type:     m.MountType,
			TotalMiB: m.Total,
			UsedMiB:  m.Used,
			FreeMiB:  m.Free,
		})
	}
	sort.Slice(mounts, func(i, j int) bool {
		iMedia := strings.HasPrefix(mounts[i].Path, "/media")
		jMedia := strings.HasPrefix(mounts[j].Path, "/media")
		if iMedia != jMedia {
			return iMedia
		}
		return mounts[i].Path < mounts[j].Path
	})

	// Per-camera recordings usage is best-effort: a failure here must not
	// break the mounts view. Log and proceed with an empty cameras slice.
	cameras := make([]storageCamera, 0)
	camStorage, err := h.frigate.GetRecordingsStorage(ctx)
	if err != nil {
		h.logger.Warn("could not load per-camera recordings storage", "error", err)
	} else {
		for id, c := range camStorage {
			cameras = append(cameras, storageCamera{
				ID:                id,
				UsageMiB:          c.Usage,
				BandwidthMiBPerHr: c.Bandwidth,
				UsagePercent:      c.UsagePercent,
			})
		}
		sort.Slice(cameras, func(i, j int) bool {
			return cameras[i].UsageMiB > cameras[j].UsageMiB
		})
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(storageResponse{Mounts: mounts, Cameras: cameras}); err != nil {
		h.logger.Error("encode storage response", "error", err)
	}
}

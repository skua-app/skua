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

// storageResponse is the JSON shape for GET /api/storage. Mounts is always
// present and never null; an empty/missing service.storage yields [].
type storageResponse struct {
	Mounts []storageMount `json:"mounts"`
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

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(storageResponse{Mounts: mounts}); err != nil {
		h.logger.Error("encode storage response", "error", err)
	}
}

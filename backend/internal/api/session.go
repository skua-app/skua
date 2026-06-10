package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"
)

// deviceCookieName is the cookie carrying the opaque per-device id
// that the glance heartbeat uses to derive its "away" verdict. The
// value is server-minted (16 bytes of crypto/rand, hex-encoded) and
// is meaningful only to the session store — no PII, no identity.
const deviceCookieName = "skua_device"

// resolveDeviceID returns the device id from the request cookie,
// minting and setting a fresh one if absent or empty. The cookie is
// HttpOnly + SameSite=Lax with a one-year MaxAge; Secure is NOT set
// because LAN deployments routinely run on plain HTTP (the wider
// reverse-proxy story is BYO TLS — see CLAUDE.md §2). The cookie is
// written before the response body so the client always sees it on
// first contact.
func (h *Handler) resolveDeviceID(w http.ResponseWriter, r *http.Request) (string, error) {
	if c, err := r.Cookie(deviceCookieName); err == nil && c.Value != "" {
		return c.Value, nil
	}
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf[:])
	http.SetCookie(w, &http.Cookie{
		Name:     deviceCookieName,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   365 * 24 * 60 * 60,
	})
	return id, nil
}

// glanceHeartbeatResponse is the JSON envelope for POST
// /api/glance/heartbeat: the per-device away verdict the SPA uses to
// decide whether to auto-surface the "while you were away" sheet.
type glanceHeartbeatResponse struct {
	Away bool `json:"away"`
}

// handleGlanceHeartbeat serves POST /api/glance/heartbeat: records
// a fresh activity timestamp for the requesting device and reports
// whether this beat counts as a return from absence (no prior beat
// or previous beat older than AWAY_SESSION_GAP). The /api group's
// cross-site guard already enforces Sec-Fetch-Site on this mutating
// route.
func (h *Handler) handleGlanceHeartbeat(w http.ResponseWriter, r *http.Request) {
	id, err := h.resolveDeviceID(w, r)
	if err != nil {
		writeError(w, h.logger, http.StatusInternalServerError, "internal", "could not mint device id", err)
		return
	}
	away := false
	if h.session != nil {
		away = h.session.Touch(id, time.Now())
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err := json.NewEncoder(w).Encode(glanceHeartbeatResponse{Away: away}); err != nil {
		h.logger.Error("encode glance heartbeat response", "error", err)
	}
}

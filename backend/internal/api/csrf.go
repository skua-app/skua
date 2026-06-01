package api

import (
	"encoding/json"
	"net/http"
)

// crossSiteGuard rejects mutating cross-site requests using the
// Sec-Fetch-Site fetch-metadata header. Browsers attach this header to
// every request; page JS cannot forge it (Sec-Fetch-* is a forbidden
// header name per the Fetch spec). The point is origin hygiene against
// drive-by CSRF — e.g. a malicious page open in a household user's
// browser firing POST /api/runtime-config/restart and bouncing the BFF.
// It is NOT authentication: an attacker already on the LAN is still in
// scope as the SECURITY.md threat model states.
//
// Policy:
//   - safe methods (GET, HEAD, OPTIONS)            → pass through
//   - Sec-Fetch-Site: same-origin / same-site /
//     none (user-initiated nav, address bar, PWA)  → allow
//   - Sec-Fetch-Site: cross-site                   → 403 cross_site_blocked
//   - header absent                                → allow (fail-open)
//
// Fail-open on the absent case is deliberate: curl, scripts, the BFF's
// own healthcheck binary, and pre-2020 browsers do not send the header,
// and the in-browser drive-by CSRF vector only exists in browsers, which
// all set it. An attacker who controls a non-browser HTTP client already
// has direct LAN access — see SECURITY.md.
//
// Unknown future values are treated as allow (only the exact "cross-site"
// value is rejected) so a new spec value cannot accidentally lock out
// legitimate traffic.
func crossSiteGuard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error":   "cross_site_blocked",
				"message": "Cross-site mutating requests are not allowed.",
			})
			return
		}
		next.ServeHTTP(w, r)
	})
}

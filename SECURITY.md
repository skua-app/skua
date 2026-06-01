# Security Policy

## Supported versions

Skua is a small open-source project with a single active release
line. Only the latest tagged release is actively maintained. Fixes for
security issues land in the next tagged release after disclosure.

- Latest release (current tag on GHCR): supported.
- Older releases: not supported — please upgrade.

## Reporting a vulnerability

Do not file security issues as public GitHub issues, Discussions, or
pull requests. Use the private vulnerability reporting workflow at
<https://github.com/skua-app/skua/security/advisories/new>.

The maintainers aim to acknowledge a report within 7 days and to land
a fix within 30 days for high-severity issues. Lower-severity issues
may take longer. These targets are aspirational, not contractual —
Skua is a small-team project.

A useful report includes:

- Affected version (image tag or commit).
- Description of the vulnerability and its impact.
- Reproduction steps or a proof-of-concept.
- Any suggested mitigation.

Coordinated disclosure is appreciated. The maintainers will work with
the reporter on an embargo window before publishing the advisory and
the corresponding fix release.

## Threat model and scope

### What Skua assumes

- The BFF is deployed on a trusted local network.
- All clients that reach the BFF are trusted to view cameras and
  modify household preferences. There is no application-level
  authentication, by design.
- Frigate is treated as a trusted upstream. Credentials between
  Skua and Frigate's `:5000` REST port or `:1984` go2rtc port are
  not negotiated.
- The reverse proxy (if used) handles TLS termination and any
  authentication layer.

### What Skua does not protect against

- Anyone with network reach to the BFF can list cameras, view
  snapshots, view stored events, and change preferences. This also
  covers `/settings → Connection` (E7.1) — the in-app editor for the
  Frigate / go2rtc URLs and its Apply (restart now) action have no
  app-level auth, consistent with the LAN-only-by-design posture.
  Apply triggers a self-bounce of the BFF container via the restart
  policy, not a data-exfil path; a reverse proxy with auth in front
  of Skua gates this and every other endpoint identically. See the
  cross-site note below for what the BFF *does* block on its own.
- There is no rate limiting, no audit log, and no per-user isolation.
- The events list includes thumbnails of motion events without
  redaction.

### In-browser cross-site requests are blocked (origin hygiene)

The BFF rejects mutating requests (POST / PUT / PATCH / DELETE on
`/api/*`) that carry `Sec-Fetch-Site: cross-site`, returning a 403
with `{"error":"cross_site_blocked"}`. Browsers attach this fetch-
metadata header to every request and the Fetch spec forbids page JS
from setting it, so this is enough to stop a malicious external web
page open in a household user's browser from drive-by-firing endpoints
like `POST /api/cameras/refresh` or `POST /api/runtime-config/restart`
(the Apply / restart action of E7.1). Safe methods (GET / HEAD /
OPTIONS) are not guarded, and requests without `Sec-Fetch-Site` (curl,
scripts, the healthcheck binary) are allowed through — the in-browser
threat only exists from browsers, all of which set the header.

This is origin hygiene, not authentication. It does not protect
against an attacker who can reach the BFF directly (curl from the
LAN, a host on the same network, etc.) — that scenario is still
covered by the LAN-only posture above and the reverse-proxy auth
recommendation below.

### Out of scope by design

- Multi-tenant deployment. Skua is sized for a small household
  (2-4 users typically).
- Internet exposure without an external authentication layer. Putting
  Skua behind a public hostname without auth is unsafe; the
  maintainers will not accept patches that add a half-built auth layer
  to the BFF instead of leaning on a proper reverse proxy.

### Hardening recommendations

- Put Skua behind a reverse proxy that terminates HTTPS and adds
  an authentication layer (HTTP basic auth, OIDC, or similar). Common
  options: Nginx Proxy Manager, Caddy with authelia, Traefik with
  forward-auth.
- If remote access is needed, prefer a VPN (WireGuard, Tailscale) or
  a zero-trust tunnel (Cloudflare Access, Pomerium) over public
  exposure.
- Run Skua, Frigate, and go2rtc on an isolated VLAN if your
  router supports it.
- Keep the underlying Docker host and Frigate up to date.

## Out of scope for security reports

Theoretical issues that require an attacker already on the trusted
LAN are by definition out of scope — see "What Skua assumes"
above. Reports framed as "I can call `/api/prefs` without logging in"
will be closed with a pointer to this document.

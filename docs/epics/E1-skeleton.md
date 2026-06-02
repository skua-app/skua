# Epic E1 — Skeleton + Camera Grid

> **Historical record (shipped v0.1.0).** This spec was written against the original private homelab deployment. Infrastructure references — host IPs, VPN gateway addresses, network topology — reflect that environment and are no longer authoritative. See CLAUDE.md §5 for the current canonical placeholders and docs/setup/ for setup guidance.

> **Read `CLAUDE.md` first.** Everything in that file applies. This spec only
> describes the scope and acceptance for E1.

## Goal

Deliver a deployable Docker container that, when running on Unraid behind NPM
on `skua.example.com`, shows a responsive grid of all 9 cameras with live
snapshots and online/offline status. The site is installable as a PWA on
iPhone and desktop. **No live video yet** — that is E2.

## Scope

In:
- Repo skeleton (monorepo: `backend/`, `frontend/`, `docs/`, root configs)
- Multi-stage Dockerfile producing a `<30 MB` distroless image
- Go BFF with `/healthz`, `/api/cameras`, `/api/cameras/:id/snapshot.jpg`
- SvelteKit frontend with a single grid route
- Snapshot polling every 5 seconds (no SSE yet)
- PWA manifest, icons, service worker for shell caching
- iOS-specific install support (`apple-touch-icon`, splash, status-bar)
- `compose.yaml` for Dockge
- `.env.example` documenting every variable
- README with setup, dev, build, deploy
- Setup docs: NPM proxy host, Pi-hole DNS, go2rtc ICE candidates

Out:
- Live streaming (WebRTC, MSE, HLS) → E2
- Events list, SSE → E3
- Talk-back, push notifications → E4
- Authentication of any kind — never (trusted-network access model; see CLAUDE.md §2)
- Auto-detection of camera capabilities (use the static map below)

## Camera capability map (hardcoded for E1)

This map lives in `backend/internal/config/cameras.go`. In E4+ it may move to
a config file, but for now compile-time is fine.

```go
var Cameras = []CameraSpec{
    {ID: "cam1", Name: "Cam 1", StreamMain: "cam1_main",      StreamSub: "cam1_sub", TalkBack: false, PTZ: false, Audio: false},
    {ID: "cam2", Name: "Cam 2", StreamMain: "cam2_main_h264", StreamSub: "cam2_sub", TalkBack: false, PTZ: false, Audio: true},
    {ID: "cam3", Name: "Cam 3", StreamMain: "cam3_main",      StreamSub: "cam3_sub", TalkBack: false, PTZ: false, Audio: false},
    {ID: "cam4", Name: "Cam 4", StreamMain: "cam4_main_h264", StreamSub: "cam4_sub", TalkBack: false, PTZ: false, Audio: true},
    {ID: "cam5", Name: "Cam 5", StreamMain: "cam5_main_h264", StreamSub: "cam5_sub", TalkBack: true,  PTZ: false, Audio: true},
    {ID: "cam6", Name: "Cam 6", StreamMain: "cam6_main",      StreamSub: "cam6_sub", TalkBack: true,  PTZ: false, Audio: true},
    {ID: "cam7", Name: "Cam 7", StreamMain: "cam7_main",      StreamSub: "cam7_sub", TalkBack: true,  PTZ: false, Audio: true},
}
```

The actual camera IDs must match the keys used in the live Frigate config.
Confirm with `curl http://<frigate-host>:5000/api/config | jq '.cameras | keys'`
before finalizing this list. If the names differ, update both this file and
the code; do not silently rename.

## Backend specification

### Stack

- Go 1.23
- `github.com/go-chi/chi/v5` for routing
- `log/slog` for structured logging (JSON in production, text in dev)
- Standard library `net/http` for the server
- `embed` for shipping the SvelteKit build inside the binary

### Layout

```
backend/
├── cmd/server/main.go          # entry, graceful shutdown
├── internal/
│   ├── config/
│   │   ├── config.go           # env parsing with defaults and validation
│   │   └── cameras.go          # static capability map
│   ├── frigate/
│   │   ├── client.go           # REST client (only what E1 needs)
│   │   └── types.go
│   ├── api/
│   │   ├── router.go           # chi setup, middleware, mount static
│   │   ├── cameras.go          # GET /api/cameras, snapshot proxy
│   │   ├── health.go
│   │   └── errors.go           # writeError helper
│   ├── static/
│   │   └── embed.go            # //go:embed dist
│   └── log/
│       └── log.go              # slog setup
├── go.mod
├── go.sum
└── Makefile                    # build, test, lint, run
```

### Endpoints (E1)

- `GET /healthz` — returns `200 ok`. Plain text, no JSON.
- `GET /api/cameras` — returns `Camera[]` per the contract in `CLAUDE.md` §10.
- `GET /api/cameras/:id/snapshot.jpg` — proxies
  `http://frigate/api/<id>/latest.jpg` with a 3 s timeout. Sets
  `Cache-Control: no-store` (we want fresh).
- `GET /*` — serves the embedded SvelteKit build. SPA fallback to
  `index.html` for any path that does not match a static file.

### Online/offline detection (E1 minimal)

For E1, "online" is determined by polling `GET /api/stats` on Frigate every 15 s
and checking `cameras.<id>.camera_fps > 0`. A camera with `detection_enabled: false`
and `camera_fps: 0` is probed via `GET /api/<cam>/latest.jpg` as a fallback
(close body immediately; just check status 200). Cache results in memory;
all cameras are marked offline if `/api/stats` itself fails. Note: Frigate 0.17
returns 405 for HEAD on the snapshot endpoint — always use GET.

In E3 this is replaced by subscribing to the Frigate WebSocket for real-time
health events. Do not implement that here.

### Configuration

All config from environment variables. No flags, no config files. Validation
in `config.Load()` returns errors with field name and reason.

```
PORT=3200                                       (default 3200)
LOG_LEVEL=info                                  (debug|info|warn|error)
LOG_FORMAT=json                                 (json|text, default json)
FRIGATE_URL=http://<frigate-host>:5000            (required; use :5000 internal API, not :8971 UI)
GO2RTC_URL=http://<frigate-host>:1984             (required for E2, optional E1)
ONLINE_CHECK_INTERVAL=15s
HTTP_TIMEOUT=5s
SHUTDOWN_TIMEOUT=10s
```

### Logging

- One log line per request: method, path, status, duration_ms, remote_addr,
  user_agent (truncated to 120 chars).
- Errors include `error` field with full message and stack if panic.
- Never log full URLs containing query strings that might have tokens (none
  expected in E1, but enforce the habit).

### Tests

- `go test ./...` must pass.
- Unit-test the snapshot caching logic (pure function, no HTTP).
- Unit-test config parsing including error cases.
- HTTP handler smoke tests with `httptest.NewServer` and a fake Frigate.
- Coverage target for E1: 60% on `internal/`. Not gating, just a signal.

## Frontend specification

### Stack

- SvelteKit 2 with Svelte 5 runes
- TypeScript strict
- Tailwind 4
- `vite-plugin-pwa` for manifest and service worker
- `adapter-static`, SSR off (`prerender = false`, `ssr = false` globally —
  this is a SPA shipped from Go)

### Layout

```
frontend/
├── src/
│   ├── app.html
│   ├── app.css                         # tailwind imports + tiny resets
│   ├── routes/
│   │   ├── +layout.svelte              # shell, install prompt
│   │   ├── +layout.ts                  # ssr=false, prerender=false
│   │   └── +page.svelte                # grid
│   ├── lib/
│   │   ├── api.ts                      # typed BFF client, fetch wrapper
│   │   ├── stores/cameras.svelte.ts    # runes-based store with polling
│   │   ├── components/
│   │   │   ├── CameraTile.svelte
│   │   │   ├── OnlineDot.svelte
│   │   │   └── InstallPrompt.svelte
│   │   └── utils/platform.ts           # isIOS, isStandalone
│   └── service-worker.ts               # workbox-injected
├── static/
│   ├── manifest.webmanifest
│   ├── icons/
│   │   ├── icon-192.png
│   │   ├── icon-512.png
│   │   └── icon-maskable-512.png
│   └── apple-touch-icon.png            # 180x180
├── svelte.config.js
├── vite.config.ts
├── tailwind.config.js
├── postcss.config.js
├── tsconfig.json
├── eslint.config.js
└── package.json
```

### Grid view

- CSS Grid via Tailwind. Responsive breakpoints:
  - mobile (< 640px): 1 column
  - sm (640px+): 2 columns
  - lg (1024px+): 3 columns
  - xl (1280px+): 3 columns (cap; do not go to 4 — tiles become too small to be useful)
- Each tile: 16:9 snapshot, camera name overlay (bottom), online dot (top-right).
- Tile is a link to `/cam/[id]` (route exists but is a stub for E1: "Live view coming in E2").
- Snapshot refresh: each tile has its own interval. Stagger initial fetches
  by 200 ms to avoid simultaneous requests on load.
- Snapshots use a cache-busting query parameter `?t=<unix_seconds>` to defeat
  HTTP caching while still allowing the browser to display the previous
  image during the swap (no flicker).

### Online dot

- Green: online.
- Gray with red ring: offline.
- Unknown (initial state): outline only.
- Dot is `aria-label`'d with the status. The tile is also dimmed when offline.

### PWA manifest

```json
{
  "name": "Cams",
  "short_name": "Cams",
  "description": "Home cameras viewer",
  "start_url": "/",
  "scope": "/",
  "display": "standalone",
  "background_color": "#0a0a0a",
  "theme_color": "#0a0a0a",
  "orientation": "any",
  "icons": [
    { "src": "/icons/icon-192.png", "sizes": "192x192", "type": "image/png" },
    { "src": "/icons/icon-512.png", "sizes": "512x512", "type": "image/png" },
    { "src": "/icons/icon-maskable-512.png", "sizes": "512x512",
      "type": "image/png", "purpose": "maskable" }
  ]
}
```

### Service worker

- Workbox via `vite-plugin-pwa`, `injectManifest` strategy.
- Precache: app shell only (HTML, JS, CSS, icons).
- Runtime cache: **none** in E1. Snapshots and API responses are network-only.
- Skip waiting + clients claim on activation, with a one-time toast in the UI
  when an update is applied (`InstallPrompt.svelte` handles this too).

### iOS specifics

- `<meta name="apple-mobile-web-app-capable" content="yes">`
- `<meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">`
- `<meta name="apple-mobile-web-app-title" content="Cams">`
- `apple-touch-icon.png` referenced explicitly in `app.html`
- Detect standalone mode via `window.matchMedia('(display-mode: standalone)')
  .matches` and hide the install prompt when already installed.

### Install prompt

- Desktop / Android: listen for `beforeinstallprompt`, show a small banner.
- iOS: detect iOS + not standalone, show a one-line hint with the share-icon
  glyph and "Add to Home Screen" — once per session, dismissable.

## Docker

### Multi-stage Dockerfile

Three stages:

1. **node-build:** `node:22-alpine`, installs frontend deps, builds SvelteKit
   to `frontend/build/`.
2. **go-build:** `golang:1.23-alpine`. Copies the SvelteKit build into
   `backend/internal/static/dist/`. Runs `go build` with
   `-ldflags="-s -w" -trimpath`. Output: a static binary.
3. **runtime:** `gcr.io/distroless/static-debian12:nonroot`. Copies only the
   binary. `EXPOSE 3200`. `USER nonroot`. `ENTRYPOINT ["/server"]`.

Target final image size: under 30 MB.

### compose.yaml

```yaml
services:
  skua:
    image: ghcr.io/skua-app/skua:${TAG:-latest}
    container_name: skua
    restart: unless-stopped
    ports:
      - "3200:3200"
    environment:
      FRIGATE_URL: "http://<frigate-host>:5000"
      GO2RTC_URL: "http://<frigate-host>:1984"
      LOG_LEVEL: "info"
      LOG_FORMAT: "json"
    volumes:
      - ./data:/data
    healthcheck:
      test: ["CMD", "/server", "-healthcheck"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
    labels:
      - "com.centurylinklabs.watchtower.enable=false"
```

The `-healthcheck` flag is a small Go convention: when set, `main.go` issues
an HTTP GET to `localhost:$PORT/healthz` and exits 0/1. This avoids needing
`wget`/`curl` in the distroless image.

### .env.example

```
# Required
FRIGATE_URL=http://<frigate-host>:5000
GO2RTC_URL=http://<frigate-host>:1984

# Optional, with defaults
PORT=3200
LOG_LEVEL=info
LOG_FORMAT=json
ONLINE_CHECK_INTERVAL=15s
HTTP_TIMEOUT=5s
SHUTDOWN_TIMEOUT=10s
```

## Setup docs

Three short markdown files under `docs/setup/`. These document host-side
configuration that is **not** done by the app, but is required for it to work.

### `docs/setup/pihole-dns.md`

Steps to add `skua.example.com → <reverse-proxy>` on both Pi-hole instances.
Mention nebula-sync handles propagation if added on primary only, but
explicitly recommend adding to both for safety.

### `docs/setup/npm-proxy.md`

NPM proxy host config:
- Domain: `skua.example.com`
- Forward to: `<docker-host>:3200`, scheme `http`
- Block common exploits: on
- Websocket support: on (needed for E3)
- SSL: Let's Encrypt, DNS challenge via Cloudflare. Force SSL, HTTP/2,
  HSTS on.
- Custom Nginx config snippet:

```
client_max_body_size 8m;
proxy_buffering off;             # SSE, future WHEP
proxy_read_timeout 3600s;        # SSE long-poll
```

### `docs/setup/frigate-config.md`

Frigate `config.yml` additions for go2rtc ICE candidates. This is needed for
E2 but is documented in E1 because it must be applied at the same time as
Pi-hole/NPM setup (deploy is one operation).

```yaml
go2rtc:
  webrtc:
    candidates:
      - <frigate-host>:8555 # LAN address of the Frigate host
      - stun:8555           # STUN fallback (public STUN servers)
```

Note: UDP 8555 must be reachable from the client subnet to the Frigate host. If a VPN or tunnel gateway sits between them, ensure the gateway forwards UDP 8555 across the relevant interfaces.

## Acceptance criteria

E1 is done when **all** of these are true:

1. Running `docker compose up -d --build` from a clean checkout brings the
   stack up with no manual intervention.
2. Visiting `https://skua.example.com` from the VPN shows the grid with all
   configured cameras and live snapshots within 2 seconds of page load.
3. Stopping a camera (or pulling its network) makes its tile go offline within
   30 seconds; restoring it brings it back online within 30 seconds.
4. The site is installable on iPhone via "Add to Home Screen" and opens in
   standalone mode without a browser chrome.
5. The site is installable on Chrome desktop via the address bar install
   button.
6. Lighthouse PWA score ≥ 90, Performance ≥ 80 on the grid page.
7. Image size of the published GHCR image is under 30 MB.
8. `make check` passes (gofmt, golangci-lint, tsc, svelte-check, prettier,
   eslint).
9. README explains: prerequisites, dev workflow, build, deploy, troubleshooting.
10. `docs/setup/*.md` files exist with the host-side configuration.

## Deliverables checklist (for the implementer)

- [ ] Repo initialized with all listed files
- [ ] Backend builds and runs locally with `make run`
- [ ] Frontend dev server runs with `npm run dev` and proxies to BFF
- [ ] Multi-stage Dockerfile builds a working image
- [ ] `compose.yaml` deployable on Unraid via Dockge
- [ ] `.env.example` complete with comments
- [ ] All static assets present (icons, manifest, apple-touch-icon)
- [ ] `make check` passes
- [ ] All acceptance criteria verified manually on iPhone Safari and Chrome
      desktop with the actual production Frigate
- [ ] README written
- [ ] Three setup docs written
- [ ] Tagged release `v0.1.0` published to GHCR

## Notes for the implementer

- When in doubt about conventions, re-read `CLAUDE.md`.
- Do not implement features from E2/E3/E4 here, even if "it would be easy" —
  it always grows the PR and delays merge. Open the next epic instead.
- If you discover that the camera IDs in our static map do not match Frigate's
  actual config, **stop and ask** — do not silently rename or guess.
- If a hard requirement in `CLAUDE.md` conflicts with this spec, `CLAUDE.md`
  wins. Surface the conflict in the PR description.

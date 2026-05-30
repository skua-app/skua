# Skua — Project Context

> This file is the **single source of truth** for the Skua project.
> Claude Code reads this on every session. Architectural decisions made here
> are binding for all future work unless explicitly revised.

## 1. What this is

A self-hosted Progressive Web App (Skua) that replaces the official
Frigate PWA for day-to-day camera viewing. The official Frigate PWA has documented stability
issues on iOS Safari (MSE timeouts, slow first-load on the install) which make
it painful for small household use. This project delivers a leaner,
WebRTC-first viewer optimized for two specific user contexts:

1. iPhone PWA installed on home screen — primary use case
2. Desktop browser — secondary, mostly for monitoring grids

The project is **not** trying to replace Frigate's full UI (Explore, Settings,
admin tooling). Frigate's own UI remains for those tasks. This project is a
focused **live + events** client.

## 2. Users and access model

**Audience.** Small household, typically 2-4 users. iPhone-first with
occasional desktop access. The product is sized for that group, not for
multi-tenant or organisation-wide deployment.

**Access model.** Skua expects to run on a trusted local network.
There is no application-level login by design — the BFF exposes no auth
surface. The default deployment is LAN-only: the app is reachable at the
Docker host's address from inside the network. This is a deliberate scope
choice, not a missing feature.

**Remote access — bring your own.** Common patterns: put Skua behind
a reverse proxy with HTTP basic auth or OIDC, behind a VPN (WireGuard,
Tailscale, or similar), or behind a zero-trust tunnel (Cloudflare Access
or similar). Hardening recommendations and threat-model notes live in
SECURITY.md.

**PWA TLS note.** The iOS PWA install path requires HTTPS for the install
prompt and service-worker registration. On LAN-only deployments without
HTTPS, the app still loads in Safari but cannot be installed to the home
screen. Users who want install-to-home-screen need a real TLS cert via a
reverse proxy or tunnel.

## 3. Architecture (Concept B: BFF + PWA)

```
iPhone PWA / Desktop browser
       │  HTTPS (skua.example.com) — trusted network or reverse proxy
       ▼
Reverse proxy (<reverse-proxy>) — optional: TLS termination, websocket upgrade — required for PWA install on iOS
       │
       ▼
skua container (port 3200)
   ├─ skua-bff (Go) — REST, SSE, WHEP signaling proxy, JPEG tile resize
   │      ├─→ Frigate :5000 REST (cameras, events, snapshots, clips)
   │      ├─→ Frigate :5000 /ws  (events real-time, used in E3+)
   │      └─→ go2rtc :1984 (WHEP signaling only)
   │
   └─ skua (static SvelteKit build, embedded into Go binary)

Direct media path (WebRTC, low-latency live):
   PWA  ◄── UDP 8555 (ICE/SRTP) ──►  go2rtc on Frigate host
```

**What stays direct (not proxied through BFF):**

- WebRTC media (UDP 8555 between PWA and go2rtc). Proxying media through Go
  would defeat the latency advantage.
- Snapshot images are always proxied through BFF (single origin) — both the
  full-resolution passthrough and the BFF-resized ECO tiles.

## 4. Tech stack and versions

| Layer | Choice | Version | Notes |
|---|---|---|---|
| Backend language | Go | 1.23+ | Single static binary |
| HTTP router | go-chi/chi | v5 | |
| Image resize | golang.org/x/image/draw | latest | Bilinear, for ECO tile.jpg |
| Logging | log/slog | stdlib | Structured JSON |
| Frontend framework | SvelteKit | 2.x | adapter-static, SSR off |
| Frontend language | TypeScript | 5.x strict | |
| Component layer | Svelte | 5 (runes) | `$state`, `$derived`, `$effect` |
| CSS | Tailwind 4 + CSS variables | | Tailwind for layout, CSS vars for tokens |
| Build tool | Vite | 5.x | |
| PWA tooling | vite-plugin-pwa **generateSW** | latest | NOT injectManifest |
| Fonts | Geist (Latin only) + JetBrains Mono (Latin) | self-hosted | `/static/fonts/` |
| Container runtime | gcr.io/distroless/static-debian12:nonroot | | |
| Final image size | **~9 MB** | | |

**Frigate target version:** 0.17.1 (commit `416a9b7`). Bundled go2rtc 1.9.10.

**Frigate auth model in 0.17:**
- `:8971` — public-facing UI, nginx auth
- `:5000` — internal API, **no auth** even when `auth.enabled: true`
- `:1984` — go2rtc, no auth, CORS `*`
- BFF must use `:5000` and `:1984` only.

## 5. Infrastructure context

| Component | Address | Notes |
|---|---|---|
| Docker host | `<docker-host>` | Where the Skua container runs |
| Frigate | `<frigate-host>` | NVR, runs cameras + go2rtc |
| Frigate REST | `http://<frigate-host>:5000` | Use this, not :8971 |
| go2rtc API | `http://<frigate-host>:1984` | WHEP at `/api/webrtc?src=<name>&type=whep` |
| go2rtc media | `UDP <frigate-host>:8555` | Must be reachable from PWA clients (see §7 hard rule 3) |
| Reverse proxy | `<reverse-proxy>` | Optional. Required only for HTTPS / PWA install / remote access (e.g. Nginx Proxy Manager, Caddy, Traefik). |

**Compose stack & persistent data:** host-defined; see `compose.yaml`.
**GHCR image:** `ghcr.io/skua-app/skua:<tag>`

## 6. Camera inventory and capabilities

Cameras are discovered from Frigate's `/api/config` at startup and persisted
to `cameras.yaml` — there is no static roster in code. Audio capability is
detected at runtime from the WHEP track event, not from static config, so
swapping a camera for one with a different mic situation requires zero
edits. Per-camera quirks of a real deployment (e.g. SKUs missing ISAPI
talk-back) are captured in `docs/hikvision-no-web-sku.md` as a case study.

**go2rtc alias format** (in Frigate config):
```yaml
cam3_main_h264: ffmpeg:cam3_main#video=h264#hardware=vaapi#width=1920#height=1080#bitrate=3500k
cam5_main_h264: ffmpeg:cam5_main#video=h264#hardware=vaapi#width=1920#height=1080#bitrate=3500k#audio=opus
```

go2rtc starts ffmpeg **on-demand** (when a consumer connects) and kills it
when the last consumer disconnects. No idle CPU/GPU load from transcode.

**Camera config baseline** (applied in camera firmware):

| Setting | Main stream | Sub stream |
|---|---|---|
| Codec | H.265 (native, for recording) | H.264 Baseline |
| I-frame interval | 40 frames | 10 frames |
| Audio | enabled where available | disabled |

## 7. Streaming strategy

| View | Protocol | Source | Rationale |
|---|---|---|---|
| Single camera (focus) | WebRTC via WHEP | `*_main_h264` or `*_sub` | Sub-500 ms latency |
| Grid (HD mode) | JPEG snapshots 1 Hz | BFF `/api/cameras/<id>/snapshot.jpg` | Full-res passthrough |
| Grid (ECO mode, default) | JPEG snapshots 1 Hz | BFF `/api/cameras/<id>/tile.jpg` — 320px wide, q=60 | Minimum bandwidth |
| Event clip (modal) | MP4 via BFF (`/api/events/:id/clip.mp4`) | Frigate `/api/events/<id>/clip.mp4`, buffered + retagged in-process | E3.1+ |

**Hard rules:**

1. **Never auto-fallback by timeout.** If WebRTC fails within 3 s (watchdog),
   surface explicit error UI. No silent switching.
2. **iOS detection is not for codec selection.** Use `*_h264` aliases from
   `StreamMain` regardless of platform.
3. **ICE candidates** must cover every path between PWA clients and the
   go2rtc media port. At minimum: the LAN address of the Frigate host. If
   clients reach the host through a VPN or tunnel, add the VPN gateway
   address(es) clients use to traverse into the LAN. For NAT'd remote
   clients, add a STUN srflx entry. See `docs/setup/frigate-config.md` for
   the configuration block.
4. **Grid quality is persisted server-side** via `/api/prefs`. HD/ECO toggle
   in header, default ECO. No live video in grid — focus view is one tap away.
5. **Audio availability is runtime-detected**, not config. The WHEP track
   event determines if audio is present; the mute button activates accordingly.
6. **iOS autoplay requires:** static `muted` HTML attribute on `<video>` at
   mount (not a JS property), plus explicit `videoEl.play()` call after
   `srcObject` assignment. After `play()` resolves, apply real muted pref.
   Forcing `muted=true` before `play()` in the track handler prevents
   `NotAllowedError` when `muted_by_default: false`.

## 8. Repository layout

```
skua/
├── CLAUDE.md
├── README.md
├── compose.yaml
├── .env.example
├── Dockerfile
├── Makefile
├── .github/workflows/build.yml
├── design_handoff_security_cams/    ← design reference (READ-ONLY)
│   ├── README.md                    ← handoff spec, pixel-accurate
│   └── prototype/                   ← React/JSX prototypes (reference only)
├── docs/
│   ├── epics/
│   │   ├── E1-skeleton.md           ← done (v0.1.0)
│   │   ├── E2-streaming.md          ← done (v0.2.0)
│   │   └── E3-live-polish-events.md ← done (v0.3.0; patch tags v0.4.1-v0.4.3 appended)
│   └── setup/                       ← host-side notes (Frigate, NPM, Pi-hole DNS)
├── backend/
│   ├── cmd/server/main.go
│   └── internal/
│       ├── config/cameras.go        ← static camera spec (StreamMain, StreamSub, TalkBack, PTZ)
│       ├── api/                     ← HTTP handlers (incl. groups.go for /api/groups CRUD)
│       ├── events/                  ← Frigate /api/events client + clip pipeline
│       │   ├── events.go            ← List, ServeClip, FetchImage
│       │   ├── clipcache.go         ← per-event LRU (16 entries / 512 MiB)
│       │   └── hevc_retag.go        ← in-place hev1→hvc1 box-tag rewrite (go-mp4)
│       ├── sse/                     ← Frigate WS upstream + SSE fanout hub
│       ├── prefs/prefs.go           ← user prefs store (file-backed, atomic; incl. grid_filter)
│       ├── groups/store.go         ← camera-group store (YAML, atomic, single-membership)
│       ├── names/store.go          ← per-camera friendly-name overrides (YAML map[cam_id]name)
│       ├── cameras/store.go        ← camera registry (Frigate-sourced at startup, YAML snapshot; E5)
│       ├── capabilities/store.go   ← per-camera talk_back/ptz overrides (YAML; hand-edited until sprint C.1)
│       ├── streamoverrides/store.go ← per-camera go2rtc stream override map (YAML, atomic; merges in WHEP handler only, not /api/cameras)
│       ├── go2rtc/client.go        ← go2rtc REST client (only GetStreams today; shares the long-lived httpClient)
│       ├── frigate/                 ← REST client
│       ├── static/                  ← embed.FS for SvelteKit build
│       └── log/
└── frontend/
    ├── src/
    │   ├── app.css                  ← CSS tokens (:root), @font-face, body font
    │   ├── app.html                 ← theme-color, viewport-fit=cover
    │   ├── routes/
    │   │   ├── +layout.svelte       ← shell only: store inits, SW register, lifecycle wiring
    │   │   ├── +layout.ts           ← ssr=false, prerender=false
    │   │   ├── +error.svelte        ← Russian "Перезагрузить" card; catches bootstrap throws
    │   │   ├── +page.svelte         ← breakpoint switcher: MobileGrid / DesktopGrid
    │   │   ├── cam/[id]/+page.svelte ← breakpoint switcher: MobileFocus / DesktopFocus
    │   │   ├── events/+page.svelte  ← real events list (filters, infinite scroll, modal)
    │   │   └── settings/+page.svelte ← /settings IA shell: rail + scroll-spy (desktop), single-column (mobile); lazy-inits streamOverrides + go2rtcStreams stores
    │   └── lib/
    │       ├── api.ts               ← typed BFF client (includes EventItem types)
    │       ├── icons.ts             ← ICONS: Record<IconName, IconDef> (21 icons)
    │       ├── lifecycle.svelte.ts  ← onBackground/onForeground hub (iOS PWA snapshot teardown)
    │       ├── streams/whep.ts      ← WHEP client (runtime audio detection)
    │       ├── util/time.ts         ← relativeTimeRu, formatDurationRu
    │       ├── screens/
    │       │   ├── MobileGrid.svelte
    │       │   ├── DesktopGrid.svelte
    │       │   ├── MobileFocus.svelte
    │       │   └── DesktopFocus.svelte
    │       ├── stores/
    │       │   ├── cameras.svelte.ts
    │       │   ├── prefs.svelte.ts  ← includes gridFilter (persisted via /api/prefs)
    │       │   ├── config.svelte.ts ← /api/config (frigate_ui_url)
    │       │   ├── groups.svelte.ts ← /api/groups CRUD + cached list
    │       │   ├── streamOverrides.svelte.ts ← /api/stream-overrides CRUD; optimistic save + server-revert (E6)
    │       │   ├── go2rtcStreams.svelte.ts ← one-shot list of go2rtc aliases for /settings (E6)
    │       │   └── events-stream.svelte.ts ← SSE EventSource, ring cap 50, backoff reconnect
    │       ├── i18n/strings.ts
    │       └── components/
    │           ├── AppHeader.svelte ← layout shell header (mobile + desktop); mobile carries the group filter button
    │           ├── BottomSheet.svelte ← reusable slide-up sheet (Escape, body-scroll lock, safe-area inset)
    │           ├── CameraTile.svelte
    │           ├── EventCard.svelte ← 16:9 thumb + meta row (events list)
    │           ├── EventModal.svelte ← inline clip player + snapshot fallback + "Открыть в Frigate"
    │           ├── Icon.svelte
    │           ├── IconBtn.svelte
    │           ├── Segmented.svelte
    │           ├── Mono.svelte
    │           ├── OnlineDot.svelte
    │           ├── MobileTabBar.svelte
    │           ├── EmptyState.svelte
    │           ├── InstallPrompt.svelte
    │           ├── StreamError.svelte
    │           └── settings/    ← /settings IA: section components + per-row cards (E6)
    │               ├── AppearanceSection.svelte  ← 6 Segmented pref controls
    │               ├── CamerasSection.svelte     ← refresh button + status banner + CameraCard list
    │               ├── CameraCard.svelte         ← per-camera name editor + main/sub stream selects
    │               ├── GroupsSection.svelte      ← groups CRUD + confirm-delete dialog
    │               └── GroupCard.svelte          ← per-group view + inline edit mode
    └── static/fonts/                ← self-hosted Geist + JetBrains Mono (Latin only)
```

## 9. Design system

### CSS tokens (app.css :root)

```css
--bg: #0a0b0d
--surface: rgba(255,255,255,0.025)
--border: rgba(255,255,255,0.07)
--border-strong: rgba(255,255,255,0.12)
--text: #f5f6f7
--text-2: rgba(245,246,247,0.58)
--text-3: rgba(245,246,247,0.34)
--accent: oklch(0.78 0.10 200)   /* default cyan; changed via prefs */
--online: oklch(0.74 0.14 145)
--offline: oklch(0.55 0.06 30)
```

### Accent variants

```ts
export const ACCENT_VALUES = {
  cyan:   'oklch(0.78 0.10 200)',
  sage:   'oklch(0.78 0.07 150)',
  amber:  'oklch(0.78 0.10 75)',
  violet: 'oklch(0.75 0.10 290)',
}
```

Applying: `document.documentElement.style.setProperty('--accent', value)`
called from `prefsStore.applyAccent()` on load and on `setAccent()`.

### Responsive layout

Breakpoint: **900px** (`bind:innerWidth` in +page.svelte and cam/[id]/+page.svelte).

| Width | Grid screen | Focus screen |
|---|---|---|
| < 900px | MobileGrid | MobileFocus |
| ≥ 900px | DesktopGrid | DesktopFocus |

The `<video>` element lives in `cam/[id]/+page.svelte` and is passed to the
active focus screen as a **Svelte 5 snippet** (`{#snippet videoSnippet()}`).
This prevents video element recreation on breakpoint change — the stream
continues uninterrupted during window resize.

### Typography

- **Sans**: Geist (Latin-only subset, self-hosted). Weights 300/400/500/600/700.
  Russian UI text renders in system fallback (SF Pro / Roboto).
- **Mono**: JetBrains Mono (Latin-only). Weights 400/500/600.
  Used for: camera IDs, latency/bitrate numbers, timestamps, LIVE chip,
  technical labels. Never for Russian copy.
- font-feature-settings: `"ss01"`, `"cv11"` (tabular numbers).

### Styling rules

- **Tailwind** for layout utilities (flex, grid, padding, gap, overflow).
- **CSS variables** for all colors and decorative effects. No hex in components
  except `#000` and `rgba(0,0,0,0.X)` for "on top of video" chips.
- **Scoped `<style>` blocks** in components for anything using CSS vars,
  `color-mix()`, `backdrop-filter`, or transition-dependent decoration.
- **No inline `style={{}}` objects** anywhere.

## 10. Conventions (non-negotiable)

**Language:**
- All code, comments, identifiers, commit messages, log messages, README: English.
- UI strings: English baseline in `lib/i18n/strings.ts`. Russian translation kept as backup in `lib/i18n/strings.ru.ts` for a future runtime locale-switching PR. Inline UI literals in components are forbidden; every visible string must come through the `ui` / `eventKindLabels` / `streamErrorReasons` exports.

**Code quality:**
- Go: `gofmt` + `golangci-lint run` clean. No `interface{}` without reason.
- TypeScript: `strict: true`, no `any`, no `@ts-ignore`. Use `unknown`.
- Frontend: `prettier` + `eslint --max-warnings 0`. Tailwind class order enforced.
- Never silence linter with `//nolint`. Log unchecked errors at debug level.
- Run `make check` to completion before committing.

**Storage:**
- No `localStorage` or `sessionStorage`. Persist prefs to BFF via `/api/prefs`.
- IndexedDB acceptable for offline event thumbnails (E3+).
- No secrets in repo. `.env.example` lists all vars.

**Backend:**
- Every handler uses `writeError(w, status, msg, err)` helper.
- Every external call has explicit `context` timeout.
- Never `context.Background()` in handlers.
- `Body.Close()` deferred with debug log on error.

**Frontend:**
- Components < 200 LOC. Logic in `lib/`, presentation in `components/`.
- Svelte 5 runes only. No legacy `let`-based reactivity.
- Network calls only through `lib/api.ts`.

**Git:**
- Conventional Commits: `feat(scope):`, `fix(scope):`, `chore:`.
- Scope: `bff` / `frontend` / `compose` / `docs`.
- Each epic ends with a tagged release.

## 11. BFF API contract (stable)

Endpoint groups exposed by the BFF, with status:

- `cameras` (GET list, snapshot.jpg, tile.jpg; POST refresh) — stable
- `webrtc` (POST whep) — stable
- `prefs` (GET, PUT) — stable
- `config` (GET) — stable
- `events` (GET list, thumbnail.jpg, snapshot.jpg, clip.mp4 incl. HEAD and `?download=1`) — stable
- `stream` (GET SSE) — stable
- `groups` (GET, POST, PATCH, DELETE; YAML-backed) — stable
- `camera-names` (GET, PUT; YAML-backed) — stable
- `stream-overrides` (GET, PUT; YAML-backed, per-installation; merges into WHEP handler only, not /api/cameras) — stable
- `go2rtc` (GET streams list; pass-through to go2rtc /api/streams) — stable

See docs/api-contract.md for the full TypeScript-style definitions, error
shapes, storage paths, defaults, and the clip pipeline.

Camera list is sourced from Frigate at startup and persisted to cameras.yaml
— see E5 in §13. `POST /api/cameras/refresh` re-pulls Frigate's config,
broadcasts `camera.added` / `camera.removed` SSE events on diff, and cleans
up orphan refs in groups/names/capabilities. Frigate unreachable → 502 with
`{error:'frigate_unreachable',message:...}` and no mutation. SSE events
emitted by the BFF: `event.new`, `event.end`, `camera.online`,
`camera.offline`, `reconnected`, `camera.added` ({cam_id, name}),
`camera.removed` ({cam_id}).

## 12. Gotchas and known issues

- **Camera registry is loaded once at startup.** Frigate unreachable AND no
  cameras.yaml on disk → BFF fails fast (homelab expectation: surface the
  upstream outage explicitly rather than serving an empty UI).

- **The /data volume must be writable by uid 65532.** The runtime image is
  `gcr.io/distroless/static-debian12:nonroot` with `USER nonroot` (uid/gid
  65532). A root-owned bind mount makes the BFF fail on first write
  (cameras.yaml, prefs.json, the YAML stores) with the data dir left empty
  — Compose users need `chown -R 65532:65532 ./data` once or a Docker
  named volume. Documented in README Troubleshooting.

- **Stream override layer applies server-side only inside the WHEP handler.**
  `GET /api/cameras` still surfaces Frigate-truth main/sub names from
  cameras.yaml — the override merge happens at WHEP-negotiation time. This
  means a stream override never appears in the camera list shape, but it
  determines which go2rtc alias serves the next focus-view connect. The
  override store also participates in the `SetOnRemoved` orphan-cleanup
  chain alongside groups / names / capabilities.

### Streaming / WebRTC

- **Live video in grid was scoped out.** Replaced by HD/ECO snapshot toggle.
  The `GET /api/cameras/:id/stream.mp4` endpoint was built and then removed.
  If reviving, restore from git history.

- **iOS WebRTC + high-resolution streams.** iOS Safari rejects WebRTC streams
  that exceed the H.264 level advertised in SDP. go2rtc advertises
  `profile-level-id=42e01f` (Constrained Baseline 3.1) but delivers High
  profile streams if source is high-bitrate. Fix: VAAPI transcode with explicit
  `-level:v 4.1` and bitrate cap (3500k). Currently applied to all `_h264`
  aliases in Frigate config.

- **Hikvision sub-stream startup ≈ keyframe interval.** GOP=10 at 10fps →
  first MP4 fragment in 1-2s. Show loading state until `loadeddata` fires.

- **Sub-streams are audio-free** (disabled in camera firmware). Main streams
  carry audio (AAC from Hikvision, transcoded to Opus by go2rtc).

- **Audio codec in getStats()** comes from `inbound-rtp.codecId` → `report.get(codecId).mimeType`.
  The `codecId` field is absent for the first ~5 stats cycles after connection.
  This is normal — codec negotiation completes slightly after ICE.

- **go2rtc VAAPI transcode is on-demand.** ffmpeg starts when a consumer
  connects and stops when it disconnects. No idle CPU/GPU load.

- **Event clip <video> on iOS Safari needs three things from the BFF.**
  The `/api/events/:id/clip.mp4` pipeline exists in its current shape
  because of three independent constraints:
  1. HTTP Range responses (`206 Partial Content` with `Content-Length`),
     not chunked Transfer-Encoding.
  2. One upstream fetch per event, served from an in-process LRU — not
     one fetch per Range subrequest.
  3. HEVC sample-entry boxes retagged `hev1` → `hvc1` in place before
     caching.

  See docs/ios-clip-playback.md for the full reasoning and fix details.

- **Focus screen renders `/api/cameras/<id>/snapshot.jpg` as a poster
  layered behind the `<video>` element.** The poster mounts immediately
  with `opacity: 1` and fades to `opacity: 0` (250 ms) only when the
  native `playing` event fires on the `<video>` — at which point a real
  frame is on screen. The `<video>` itself is set to
  `background: transparent` so Safari does not draw its own black layer
  in front of the poster while frames are still in flight. The `emptied`
  event (fired by whep cleanup when it nulls `srcObject`) re-shows the
  poster instantly on pause / HQ↔LQ reconnect with no HTTP roundtrip,
  because the `<img>` stays mounted at `opacity: 0` rather than being
  unmounted. The two-stage spinner overlay (24 px SVG +
  "Подключение..." / "Буферизация...") sits above the poster and
  unmounts on `playing`. See `frontend/src/routes/cam/[id]/+page.svelte`
  for the snippet and styles; no edits to MobileFocus / DesktopFocus are
  required because both render the snippet inside their already-relative
  `.mf-video-frame` / `.df-video-frame` containers.

### Hikvision DS-2CD2443G2-IW (no-web SKU) — talk-back not feasible

The OEM no-web variant of the Hikvision DS-2CD2443G2-IW ships with the
standard ISAPI surface stripped (every ISAPI endpoint returns 404). ONVIF
AudioOutput is intact on the camera, but go2rtc 1.9.10's `onvif://` source
is discovery-only and does not publish a sendonly-audio track, so the
backchannel cannot be driven from go2rtc as shipped. Talk-back from a
browser PWA is architecturally blocked on these units until they are
replaced with retail DS-2CD2443G2-IW (which carry ISAPI) or a custom
go2rtc exec-source is built against ONVIF SOAP.

See docs/hikvision-no-web-sku.md for the full port-scan results, ISAPI
endpoint inventory, ONVIF probe output, and replacement options.

### iOS / PWA autoplay

- **Static `muted` HTML attribute is required** for autoplay on iOS Safari.
  `bind:muted={isMuted}` or setting `videoEl.muted = true` via JS before
  mount does NOT satisfy iOS autoplay policy — the attribute must be present
  in the HTML. Always write `<video muted playsinline autoplay>`.

- **Explicit `videoEl.play()` after `srcObject` assignment** is required in
  addition to the `autoplay` attribute, because Svelte mounts the element
  before `srcObject` is assigned. Pattern:
  ```ts
  videoEl.srcObject = stream
  videoEl.muted = true          // guarantee for autoplay policy
  videoEl.play().then(() => {
    videoEl.muted = initialMuted  // apply real pref after play starts
  })
  ```

- **`muted_by_default: false` + navigation from grid** caused audio to play
  despite muted UI. Fix: in `onAudioDetected(true)` callback, re-apply
  `videoEl.muted = isMuted` to override any intermediate state from
  play().then() timing.

- **Safe-area insets in PWA standalone mode.** `viewport-fit=cover` is set
  in app.html and `body` carries `padding-top: env(safe-area-inset-top)` +
  `padding-bottom: env(safe-area-inset-bottom)`, which transparently lifts
  AppHeader and the MobileTabBar below the notch / home indicator. The
  `cam/[id]` focus screen renders without the layout shell, so MobileFocus
  carries its own `.status-spacer` div for the top inset.

- **iOS PWA snapshot freezes the JS context — tear down before, re-arm
  after.** When the user backgrounds an installed PWA, iOS Safari takes a
  process snapshot rather than running pause-and-resume. Anything holding
  a live network handle when the snapshot fires (a WHEP `RTCPeerConnection`,
  an `EventSource`, a polling `setInterval`) is captured in whatever
  half-state it was in. On resume the page reads back a stale object and
  fails silently — black-on-video for WHEP, "disconnected" forever for SSE,
  occasionally the whole shell repaints white because some module-graph
  read paths through a broken reference. The mitigation lives in
  [`frontend/src/lib/lifecycle.svelte.ts`](frontend/src/lib/lifecycle.svelte.ts):
  it owns `visibilitychange` + `pagehide` + `pageshow` listeners and
  exposes `onBackground(fn)` / `onForeground(fn)` subscriptions. The
  layout subscribes the SSE store and cameras polling; `cam/[id]/+page.svelte`
  subscribes the WHEP `disconnect()`/`connect()` pair (respecting
  user-initiated pause). Also: if the page is restored with `pageshow.persisted=true`
  (bfcache) or hidden longer than 30 minutes, lifecycle does a hard
  `location.reload()` rather than trust the resumed JS — that's the
  "fresh shell" path the validation step exercises.

### Svelte 5 / SvelteKit

- **`injectManifest` strategy is broken with SvelteKit.** Use `generateSW`.

- **`workbox.navigateFallback: 'index.html'` is mandatory for SPA deep
  links.** `adapter-static` with `fallback: 'index.html'` plus
  `serviceWorker.register: false` means the Workbox SW is the only thing
  intercepting navigations once installed. Leaving `navigateFallback: null`
  makes the SW return nothing for paths it has no precache entry for —
  every route except `/` (`/cam/<id>`, `/events`, `/settings`). Visually
  the failure is a blank page on iOS PWA relaunch, because iOS resumes
  the last-visited URL when the app reopens from the home-screen icon,
  and that URL almost never matches a precached file. Always pair
  `navigateFallback: 'index.html'` with `navigateFallbackDenylist: [/^\/api\//]`
  so `/api/*` keeps reaching the BFF instead of being shadowed by the
  fallback. See `vite.config.ts` workbox block.

- **PWA SW activation: `registerType: 'prompt'`, never `'autoUpdate'`,
  and call `registerSW()` from `onMount` (not `$effect`).** vite-plugin-pwa's
  `'autoUpdate'` injects `skipWaiting + clientsClaim` so a newer SW takes
  over mid-load — but mid-load means in-flight ESM chunk requests can
  switch backing SWs between two precache manifests, hit a hash mismatch,
  silently throw, and blank the shell. `'prompt'` (default) emits a
  `registerSW.js` whose generated SW only responds to a `SKIP_WAITING`
  message we never send: the waiting SW patiently waits until every tab
  for the scope is closed, which on iOS PWA is exactly the kill-and-relaunch
  the user does between updates. Net effect: the household gets the new
  build on the next cold launch, with zero cache-miss-during-bootstrap
  risk. Putting `registerSW()` in `$effect` is also wrong — `$effect` can
  re-run, and stacking SW registrations is just asking for the same
  hash-mismatch failure mode from a different angle.

- **`bind:muted` removes the static `muted` attribute** from rendered HTML,
  breaking iOS autoplay. Never use `bind:muted`. Manage `videoEl.muted`
  manually via `$effect`.

- **Svelte 5 runes auto-track reactive reads in `$effect`.** If `connect(cam)`
  reads reactive state (e.g. `isMuted`), the effect re-runs on every mute
  toggle — causing spurious reconnects. Fix: wrap `connect(cam)` with
  `untrack(() => connect(cam))` in the connect effect.

- **`untrack` boundary rule.** Read every dependency the effect should
  re-run on **above** `untrack`; place the side-effect and any fast-changing
  reads (mute toggles, transient UI state) **inside** `untrack`. Wrapping the
  *entire* effect body in `untrack` (as the E2 fix above naively did) will
  silently kill route-param reactivity — the symptom is "URL changes but
  stream does not". Found while fixing the camera-switch bug in E3/A1.

- **Prefs-derived initial state needs a tracked `prefsSynced` gate.** Any
  effect whose body branches on a value that originated from prefsStore
  (e.g. `streamQuality`, `isMuted` in `cam/[id]/+page.svelte`) must read a
  reactive flag like `let prefsSynced = $state(false)` above its `untrack`
  call, and only flip it inside a separate effect once `prefsStore.loaded`
  is true. Without this gate, the connect effect runs once with the literal
  `$state(...)` defaults before `/api/prefs` resolves, and the user is
  stuck on the wrong stream/mute setting until they toggle manually. This
  was the root cause of E3.1 bug 1 (HQ stream forced on LQ users after
  grid → focus navigation on slower mobile networks).

- **Use `goto(url, { replaceState: true })` for sidebar/rail nav within a
  single conceptual screen.** In MobileFocus and DesktopFocus the
  "other cameras" rail/sidebar swaps the current `/cam/[id]` for another
  `/cam/[id2]` via intercepted click → `goto`. A native `<a href>` is kept
  on the element for accessibility / "open in new tab", but a normal
  navigation would push a history entry per hop, and one back-tap would
  walk through every camera the user touched before reaching the grid.
  With `replaceState: true`, the focus screen is treated as a single
  history slot regardless of how many cameras are visited. Fixed in E3.2.

- **Camera-group single-membership is enforced server-side, not in the
  editor.** The /settings group editor sends the full desired
  `camera_ids` for the target group on PATCH; the BFF strips those cameras
  from any other group atomically. Don't add "remove from old group" calls
  in the frontend — the move is one request, and `groupsStore.refresh()`
  after a successful PATCH is enough to surface the new state in every
  card.

- **`generics="T extends string"` on `<svelte:component>`** causes
  `svelte-eslint-parser` to fail with `no-undef`. Workaround: file-scoped
  eslint disable in `Segmented.svelte` only.

### Go / Backend

- **`writeTimeout` kills long-running responses.** For streaming endpoints,
  use `http.ResponseController(w).SetWriteDeadline(time.Time{})`.
- **`httputil.ReverseProxy` buffers by default.** Set `FlushInterval = -1`
  for any proxy that streams data.
- **WebSocket reconnect on Frigate restart.** BFF must reconnect with
  exponential backoff, capped at 30s.

### Fonts

- **Geist has no Cyrillic glyphs.** Russian UI text falls back to system fonts
  (SF Pro on iOS/macOS, Roboto on Android). This is accepted — Geist is
  used for Latin/digits only. JetBrains Mono is also Latin-only.

### Mocks

- `frontend/src/lib/mocks/` was removed in E3/D8 once `/api/events` landed.
  Reintroduce only behind a clear `// TODO(api):` marker if you ever need to
  stub a future endpoint; `grep -rn "TODO(api)" frontend/src` must stay
  empty between epics.

### Cosmetic backlog (focus view)

These known issues are recorded but not yet fixed:
- **Page scrolls on some devices** even though flex layout should fill viewport.
  Culprit suspected: combined py-4 from layout + flex height calculation.

## 13. Epic roadmap

| Epic | Goal | Status | Tag |
|---|---|---|---|
| **E1** | Skeleton, camera grid, PWA installable | **DONE** | v0.1.0 |
| **E2** | WebRTC focus view + HD/ECO grid + prefs | **DONE** | v0.2.0 |
| **Design** | Full UI redesign (4 screens, design tokens, components) | **DONE** | — |
| **E3** | Live UX polish + events list, modal, SSE | **DONE** | v0.3.0 |
| **E3.1** | Mobile UX polish + inline event clips | **DONE** | v0.3.1 |
| **E3.2** | Mobile UX round 2: merged header, replaceState nav, tab bar on focus | **DONE** | v0.3.2 |
| **E3.3** | Camera groups with in-app editor | **DONE** | v0.4.0 |
| **E3.4** | iOS clip playback fix (LRU cache + HEAD + hev1→hvc1 retag) and mobile header de-dup | **DONE** | v0.4.3 |
| **E3.5** | Per-camera friendly names persisted to YAML; settings editor | **DONE** | v0.5.0 |
| **patch** | iOS PWA white/black-screen-on-relaunch fix (SW navigateFallback, deferred SW activation, lifecycle teardown, error boundary) | **DONE** | v0.5.1 |
| **patch** | Focus screen: snapshot poster + two-stage spinner replace black-frame flash on connect | **DONE** | v0.5.2 |
| **patch** | Focus spinner: dark pill backdrop + blur so "Подключение…" / "Буферизация…" stay readable over bright snapshots | **DONE** | v0.5.3 |
| **E5** | Dynamic camera discovery from Frigate (refresh + orphan cleanup) | **DONE** | v0.6.0 |
| **patch** | E6 sprint A — backend: per-camera go2rtc stream override store + endpoints (no UI) | **DONE** | v0.6.1 |
| **E6** | Per-camera go2rtc stream override editor in /settings; /settings IA redesign (desktop rail + scroll-spy, mobile single-column) | **DONE** | v0.7.0 |
| **Public Launch** | Rebrand to Skua, desensitize, English baseline, public-grade docs, MIT, public migration | in progress | v0.8.0 |
| **patch** | header domain + app-name rebrand residue fix, manifest color sync | **DONE** | v0.8.1 |
| **patch** | focus LQ fix for cameras without a Sub stream (per-camera effective quality, greyed-out LQ + hint) + DesktopFocus real stream-name labels | **DONE** | v0.8.2 |
| E7+ | PTZ, semantic search, multi-user prefs | unscheduled | |

- BFF exposes: `/api/config`, `/api/cameras` (now carrying `groups[]`),
  `/api/prefs` (now including `grid_filter`), `/api/events` + per-event
  `thumbnail.jpg`/`snapshot.jpg`/`clip.mp4` (with `?download=1` variant),
  `/api/stream` (SSE), and the full `/api/groups` CRUD with structured
  snake_case error codes.
- Groups live in YAML at `$GROUPS_CONFIG_PATH` (default `/data/groups.yaml`),
  enforce a single-membership invariant server-side, and persist the last
  selected group filter via `prefs.grid_filter` so reopens land on the same
  view.
- Mobile shell: AppHeader is sticky with safe-area-inset-top, HD/ECO is
  merged into its title row (route-gated on `/`), and the group filter
  opens a slide-up `BottomSheet` from the same row. `MobileTabBar` is now
  present on `/cam/[id]` as well, so the user always has bottom navigation.
- Focus screens: navigation between cameras via the rail/sidebar uses
  `goto(url, { replaceState: true })` so a single back-tap from focus
  always lands on the grid, regardless of how many cameras were hopped
  through.
- EventModal plays clips inline (`<video>` against `/api/events/:id/clip.mp4`)
  with a "Скачать видео" button hitting the `?download=1` variant; falls
  back to the snapshot image when `has_clip === false`.
- `lib/mocks/` is gone.
- v0.6.0 — dynamic camera discovery (E5): startup pull from Frigate
  `/api/config`, persistent `cameras.yaml`, `POST /api/cameras/refresh`,
  SSE `camera.added`/`camera.removed`, `capabilities.yaml` override layer
  (hand-edited until C.1).

Detailed notes on the latest deployed state in docs/roadmap-notes.md.

## 14. How to work in this repo

- Read this file first. Then read the active epic spec.
- Read CLAUDE.md first, then the docs/ file referenced by the section you are working in.
- Implement only what the epic specifies. No preemptive features.
- When in doubt about a convention, this file wins.
- Run `make check` before committing.
- Test WebRTC on actual iPhone Safari in standalone (PWA) mode — iOS behavior
  differs significantly from desktop.
- The user has Go installed locally on Mac but no Docker. Build verification
  happens in GHA CI; smoke tests happen on Unraid against pulled images.
- design_handoff_security_cams/ is READ-ONLY reference material. Never modify it.

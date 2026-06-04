# Changelog

All notable changes to Skua are documented in this file.

The format is based on [Keep a Changelog 1.1.0](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

> **Note on early history.** Versions before 0.8.0 shipped under the
> project's private development name. The functionality described in
> each entry is unchanged; only the project name, package paths, and
> infrastructure references differ between the historical record and
> the public release. The 0.8.0 entry covers the rename and the
> surrounding open-source readiness work.

## [Unreleased]

### Added

- Server-side moments grouping endpoint `GET /api/moments` that collapses
  recent Frigate events into per-camera "moments" (time-clusters with a
  5-minute gap). Read-only and stateless: no persistence, no seen-state,
  not yet surfaced in the UI. Phase 1 of the glance feature.
- Server-side household seen-state for the glance feature. `GET /api/glance`
  returns the unseen "while you were away" moments plus an unseen count
  composed from the Phase 1 grouping, and `POST /api/glance/ack` advances
  the stored `last_seen` monotonically. Backed by a new dedicated state
  file at `GLANCE_STATE_PATH` (default `/data/glance.json`); single
  household timestamp, no per-user state. Internal, not yet surfaced in
  the UI. Phase 2 of the glance feature.

### Changed

- Container restart-policy requirement for runtime reconfiguration is
  now stated up front. The first-run wizard Save and Settings →
  Connection Apply both restart Skua by exiting the process and rely
  on the container restart policy (`restart: unless-stopped` or
  `always`) to bring the BFF back; with no restart policy a Save or
  Apply leaves Skua stopped. README Quick start and Configuration
  sections call out the dependency explicitly, the Connection editor
  shows a persistent note above the actions, and the setup wizard
  shows the same line under its Save and start button. No runtime
  change.

## [0.11.1] — 2026-06-03

### Changed

- App icons and favicon updated to the bracket mark. Covers the PWA
  manifest icons (`icon-192.png`, `icon-512.png`, `icon-maskable-512.png`),
  the iOS home-screen icon (`apple-touch-icon.png`), and the browser
  favicon (`favicon.ico`). File names and paths are unchanged.

### Added

- Event modal: graceful fallback when an event clip can't be decoded on
  the current device (most often high-resolution HEVC on budget Android
  SoCs). On any `<video>` `error` event the modal swaps the player for
  the event snapshot and shows a short note pointing at the existing
  Download and Open in Frigate buttons. iOS and desktop playback paths
  are unchanged. The clip-codec limitation is documented in the README
  Limitations and Troubleshooting sections.

### Fixed

- "Cameras" back button on the focus view now reliably returns to the
  grid when the app is opened directly on a camera (PWA cold start /
  restored URL). The previous implementation called `history.back()`
  when `window.history.length > 1`, which on a restored standalone PWA
  session was a silent no-op because the history stack had no in-app
  grid entry behind it. The button now always navigates to the grid.
- Android Chrome no longer fires pull-to-refresh or the overscroll glow
  when scrolling the grid and events list. The root scroll context
  now uses `overscroll-behavior-y: contain` instead of `none`, which
  blocks the browser default action without affecting legitimate
  scrolling momentum.
- Desktop focus view no longer stretches the video horizontally when the
  browser window is shortened vertically, and the controls row no longer
  detaches from the frame leaving a floating gap between them on tall
  narrow windows. The video frame and controls are now grouped into one
  centered player block sized to the largest 16:9 box that fits both the
  available width and the available height (after reserving room for the
  controls bar). HQ (main, 16:9) stays correctly proportioned at any
  window size while the anamorphic LQ (sub, 3:4) stream continues to be
  stretched to 16:9 via `object-fit: fill` as before.

## [0.11.0] — 2026-06-02

### Added

- CI now runs the full quality gate (Go `vet`, `golangci-lint`, race
  tests; frontend type-check, lint, format check) on pull requests and
  pushes, not just on release-tag builds.

### Changed

- **Breaking:** the `SNAPSHOT_CACHE_TTL` environment variable is renamed
  to `ONLINE_CHECK_INTERVAL`. The default (`15s`) and behaviour are
  unchanged: the value has always been the camera online-status poll
  interval (how often the BFF hits Frigate `/api/stats`), never a
  snapshot cache. There is no backward-compatibility alias — operators
  who set `SNAPSHOT_CACHE_TTL` in their environment must rename it.
- All BFF error responses now use a single envelope shape,
  `{error: <code>, message: <text>}`, where `error` is always a stable
  snake_case machine code and `message` is always human-readable text.
  Previously several endpoints returned a bare `{error: <message>}` with
  the human string in the code slot. Relevant to anyone integrating
  with the API directly; the front-end already used the new shape.
- Internal cleanup with no user-facing behaviour change: HTTP timeout
  handling unified on per-call contexts (no long-lived `Client.Timeout`
  outside the ephemeral upstream-probe client), the camera
  online-status poller gained a construct-then-`Start(ctx)` lifecycle,
  and the SSE upstream picked up test coverage for its frame-handling
  paths.

### Fixed

- Event clip playback on iOS is more robust. A clip fetch is no longer
  capped by the shared 5-second HTTP timeout that was making large or
  slow clips fail; the per-call 30-second context deadline is now the
  real bound. The many parallel Range requests iOS issues for a single
  clip also collapse into one shared upstream fetch on a cold miss, so
  opening a fresh clip no longer stampedes Frigate.
- A corrupt or hand-edited `prefs.json` no longer prevents startup.
  Invalid individual fields are reset to their defaults and the rest of
  the file is preserved.
- The event-detail modal traps keyboard focus while open
  (accessibility).
- Desktop no longer briefly flashes the mobile layout on first paint.

### Security

- The BFF rejects cross-site requests to mutating endpoints
  (`POST` / `PUT` / `PATCH` / `DELETE`) using the browser's
  `Sec-Fetch-Site` signal, returning `403 cross_site_blocked`. This
  closes an in-browser drive-by vector where a page open in another tab
  on the same network could trigger state changes (notably the
  Connection editor's Apply restart) without the operator acting. Safe
  methods and requests without the header pass through. This is origin
  hygiene, not authentication — the LAN-only, no-app-login model is
  unchanged.

## [0.10.0] — 2026-05-31

### Added

- In-app Connection editor in `/settings`: edit the Frigate, go2rtc, and
  Frigate UI URLs from inside the app. The form has explicit Test, Save,
  and Apply (restart now) actions. Save writes `/data/config.yaml`;
  Apply triggers a clean process exit so the container restart policy
  boots the new overlay. Env-locked fields render read-only with a hint
  and are stripped from PUT bodies server-side.
- `GET` / `PUT /api/runtime-config` returning `{effective, overlay,
  locked}` and persisting the overlay file.
- `POST /api/runtime-config/test` runs the same upstream probes the
  first-run wizard uses (Frigate `GetStats`, go2rtc `GetStreams`).
- `POST /api/runtime-config/restart` returns `202 {status:"restarting"}`
  and triggers the BFF's shutdown-select restart branch.
- `internal/probe` package: shared ephemeral upstream-probe logic
  (`Frigate`, `Go2RTC`, `ValidateURL`, `Report`) so the setup wizard and
  the new runtime-config editor reuse a single implementation.

### Changed

- The setup wizard's test-connect endpoint now delegates to
  `internal/probe`. External JSON shape and behaviour are unchanged.
- `main.go`'s shutdown select gained a second branch watching a restart
  channel so the runtime-config editor's Apply funnels into the same
  graceful `srv.Shutdown` as the SIGINT/SIGTERM path. The signal-driven
  shutdown path is byte-identical in behaviour.

## [0.9.0] — 2026-05-31

### Added

- First-run setup wizard: when `FRIGATE_URL` is not set in the
  environment and no overlay file exists, Skua serves an interactive
  setup page on its normal port instead of exiting. The wizard
  test-connects to Frigate and go2rtc before saving and persists the
  entered URLs to `/data/config.yaml`. After a successful save the
  process exits cleanly so the container restart policy boots into
  the configured app. The same wizard is also served when the
  configured Frigate URL came from the overlay file and Frigate is
  unreachable at startup; the form is prefilled and a banner
  explains the failure.
- Runtime config overlay (`internal/runtimeconfig`) backed by
  `/data/config.yaml` with `frigate_url`, `frigate_ui_url`, and
  `go2rtc_url` keys. Path is configurable via `RUNTIME_CONFIG_PATH`.
- `POST /api/setup/test` and `POST /api/setup/save` endpoints on
  the setup-mode server. Both are stripped of env-locked fields
  server-side so a tampered payload cannot pollute the overlay file.

### Changed

- `FRIGATE_URL` is no longer required at startup. When the effective
  Frigate URL is empty after merging env and the overlay file, the
  BFF enters setup mode instead of failing with a config error.
  Env variables continue to win over the overlay file, so existing
  env-driven deployments are unaffected.
- When `cameras.New` reports Frigate as unreachable and the URL is
  editable (came from the overlay file, not env), Skua now serves
  the setup wizard prefilled with the current URLs and an error
  banner — replacing the informational emergency page in that one
  case. The env-locked case and the `/data` not-writable case keep
  the existing emergency page.

## [0.8.4] — 2026-05-30

### Added

- Emergency setup page: when the BFF can't start because the `/data`
  volume isn't writable by the container's non-root uid (65532), or
  because Frigate is unreachable on the very first start with no
  cached camera snapshot, Skua now serves a self-contained styled
  page on its normal port explaining the problem and the fix,
  instead of exiting with the browser left at connection-refused.
  The page auto-reloads into the app once the issue is fixed and the
  container is restarted.

### Fixed

- Startup error when `/data` is not writable now prints an actionable
  diagnostic naming the required uid/gid (65532) and the two
  remedies (`chown -R 65532:65532 <dir>` or a named Docker volume),
  and no longer repeats the `cameras:` prefix.

## [0.8.3] — 2026-05-30

### Changed

- Documented that the container runs as the distroless non-root uid
  (65532) and that the host `./data` directory must be writable by it; a
  root-owned bind mount makes the BFF fail on first write with the data
  folder left empty. Added a Troubleshooting subsection to `README.md`, a
  note to the Configuration section, and an inline comment in
  `compose.yaml`. No runtime change.

## [0.8.2] — 2026-05-29

### Fixed

- Focus-view LQ on cameras without a Sub stream: quality is now resolved
  per-camera (a Sub-less camera always uses Main), the LQ segment is
  greyed out with a hint, and opening such a camera while the global
  pref is `sub` no longer triggers a `StreamError` from the WHEP `400 no
  stream configured for quality`. The global LQ choice is left untouched
  so Sub-capable cameras keep it.
- DesktopFocus stream-name labels show the camera's real go2rtc stream
  name (Frigate-truth Main/Sub) instead of a fabricated `*_h264` string.

## [0.8.1] — 2026-05-28

### Changed

- PWA manifest `theme_color` / `background_color` aligned to `#0a0b0d`,
  matching `--bg` in `app.css` and the `app.html` `theme-color` meta.

### Fixed

- Header no longer shows a placeholder domain — desktop shows the
  wordmark only, mobile shows the live host.
- iOS home-screen install and the web manifest now name the app "Skua"
  instead of "Cams".

## [0.8.0] — 2026-05-28

### Added

- `LICENSE` (MIT, Copyright (c) 2026 Skua contributors).
- `CONTRIBUTING.md` with conventions distilled from the contributor
  conventions documentation.
- `SECURITY.md` with the threat model and hardening recommendations
  referenced from the README.
- `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1, enforcement contact
  via GitHub private security advisories).
- `.github/ISSUE_TEMPLATE/` with bug-report and feature-request GitHub
  Forms plus a `config.yml` that disables blank issues and redirects
  questions to Discussions.
- `frontend/src/lib/i18n/strings.ru.ts` as a Russian-translation
  backup, kept for a future runtime locale-switching PR but not
  imported by any code path today.
- This `CHANGELOG.md`.

### Changed

- Renamed the project from its private development name to Skua.
  Go module path is now `github.com/skua-app/skua`; frontend
  package name is `skua`; container image is
  `ghcr.io/skua-app/skua`.
- UI strings translated to English as the baseline; 31 inline-Russian
  literals were extracted from components into `lib/i18n/strings.ts`
  keys; `Intl.DateTimeFormat` calls switched from `ru-RU` to the
  runtime default locale; the `app.html` `lang` attribute switched to
  `en`.
- Backend user-facing error messages and code comments translated to
  English.
- README rewritten end-to-end as a public-grade landing document with
  a security callout above the fold, a feature list, requirements, a
  five-minute Docker Compose quick start, a full environment-variable
  table aligned to `.env.example`, a build-from-source section,
  troubleshooting, and forward-references to `LICENSE` /
  `CONTRIBUTING.md` / `SECURITY.md` / `CODE_OF_CONDUCT.md`.
- Access-model documentation reframed from "VPN-only by network" to
  "LAN-only by default with the user's reverse-proxy or VPN as an
  optional remote-access layer". The architecture documentation
  rewritten; `docs/setup/frigate-config.md` rewritten with LAN-default
  ICE candidates and an optional remote-access block;
  `docs/setup/npm-proxy.md` and `docs/setup/pihole-dns.md` re-headed
  as optional examples.
- Historical epic specs (`docs/epics/E1-skeleton.md`,
  `E2-streaming.md`, `E3-live-polish-events.md`) gained a
  historical-record header note and had infrastructure literals (`wg0`,
  `awg0`, specific VPN IP literals) replaced with generic placeholders.
- Private homelab fingerprints removed from machine-read config:
  `.env.example` and `compose.yaml` now use `frigate:5000` /
  `frigate:1984` as the canonical Frigate URL examples; documentation
  prose uses `<frigate-host>` / `<docker-host>` / `<reverse-proxy>`
  placeholders.

### Removed

- All references to the project's private development name, the
  maintainer's personal domain, specific homelab IPs, owner
  identities, and the maintainer's seven-camera inventory from code
  and shipped docs (`design_handoff_security_cams/` retained as a
  read-only reference is unchanged).

## [0.7.1] — 2026-05-26

### Changed

- Documentation-tree statements in the architecture documentation's
  repo-layout section reconciled with the actual filesystem.
- Per-camera card hint rhythm in `/settings` consolidated for visual
  consistency.

### Removed

- Dead i18n keys that no component referenced.

## [0.7.0] — 2026-05-26

### Added

- Per-camera `go2rtc` stream-source selectors in `/settings` with
  optimistic save and server-revert on `stream_not_found` /
  `go2rtc_unreachable`.
- `/settings` page reorganised into three sections — Appearance,
  Cameras, Groups.
- `lib/components/settings/` subfolder split the page into
  `AppearanceSection`, `CamerasSection`, `CameraCard`,
  `GroupsSection`, `GroupCard`.
- `streamOverridesStore` and `go2rtcStreamsStore` (lazy-inited on
  `/settings` mount only, not in the shell).

### Changed

- On desktop (≥900px), `/settings` is a two-column layout with a
  sticky left rail of section anchors driven by an
  `IntersectionObserver`-backed scroll-spy; mobile keeps a single
  column.

## [0.6.1] — 2026-05-26

### Added

- `internal/streamoverrides` store (YAML-backed, atomic write,
  auto-prune on empty entry).
- `internal/go2rtc` REST client (`GetStreams`).
- `GET /api/go2rtc/streams`, `GET /api/stream-overrides`, `PUT
  /api/stream-overrides/{cam_id}` with full validation (invalid body,
  unknown camera, unknown stream against the live `go2rtc` list,
  `go2rtc` unreachable).
- Override store joins the orphan-cleanup chain on `POST
  /api/cameras/refresh`.

### Changed

- WHEP handler merges the override layer (`override.Main` over
  `cam.StreamMain`, same for Sub) and returns `400 no stream
  configured for quality` when the resolved stream name is empty.

## [0.6.0] — 2026-05-25

### Added

- `POST /api/cameras/refresh` re-pulls the roster on demand, returns
  the `{added, removed}` diff, broadcasts `camera.added` /
  `camera.removed` SSE events, and orphan-cleans dependent stores
  (groups, names, capabilities).
- `capabilities.yaml` override layer for per-camera `talk_back` / `ptz`
  flags (read-only via API for now, hand-edited on the host).

### Changed

- Removed the static `config.Cameras` slice; the camera roster is
  pulled from Frigate `/api/config` at startup and persisted to
  `cameras.yaml`.
- BFF falls back to the persisted `cameras.yaml` snapshot when Frigate
  is unreachable at startup; if neither is available, the BFF fails
  fast.

## [0.5.3] — 2026-05-25

### Changed

- The connecting / buffering text on the focus screen now sits inside
  a dark blurred pill (`rgba(0, 0, 0, 0.5)` + `backdrop-filter:
  blur(10px)`, matching the existing `.telemetry-pill` / `.ts-chip`
  style), so it stays readable over bright snapshots.

## [0.5.2] — 2026-05-25

### Added

- Snapshot poster layered behind the `<video>` (the `<video>`
  background is transparent) that fades out on the native `playing`
  event; the poster stays in the DOM at `opacity: 0` so pause/resume
  and HQ↔LQ reconnects re-show it instantly.
- Two-stage spinner overlay reads "Connecting…" during signalling and
  switches to "Buffering…" once `streamState === 'connected'` but
  before the first frame paints.

### Changed

- Focus mount no longer shows a black frame between WHEP negotiation
  and the first decoded frame.

## [0.5.1] — 2026-05-25

### Added

- `lib/lifecycle.svelte.ts` hub that tears down the WHEP
  `RTCPeerConnection`, the SSE `EventSource`, and the cameras polling
  interval on `pagehide` / `visibilitychange:hidden`, and re-arms them
  on resume; a 30-minute-absence guard forces a hard reload rather
  than trust the resumed JS context.
- `routes/+error.svelte` so a bootstrap-time throw renders a
  recoverable error card instead of a blank shell.

### Changed

- Service-worker activation switched from `'autoUpdate'` to
  `'prompt'`; the new SW waits until every tab for the scope is closed
  (i.e. the next cold launch on iOS PWA) instead of skipping-waiting
  mid-load.

### Fixed

- White/black screen on iOS PWA relaunch via `workbox.navigateFallback:
  'index.html'` plus `navigateFallbackDenylist: [/^\/api\//]` so
  deep-link navigations resolve against the precached shell while
  `/api/*` keeps reaching the BFF.

## [0.5.0] — 2026-05-22

### Added

- Per-camera display-name overrides persisted to `camera_names.yaml`,
  edited inline in `/settings`, merged into `/api/cameras` as `.name`.
- `CAMERA_NAMES_CONFIG_PATH` environment variable (default
  `/data/camera_names.yaml`).

### Changed

- Cameras without an override fall back to the Frigate-sourced default
  rather than the historical static config map.

## [0.4.3] — 2026-05-22

### Changed

- In-page `<h1>` hidden on mobile for `/events` and `/settings`
  because the sticky `AppHeader` already shows the page title. Kept on
  desktop where the header carries only small nav links.

## [0.4.2] — 2026-05-22

### Added

- In-place `hev1` → `hvc1` sample-entry box-tag rewrite (via `go-mp4`)
  inside the clip pipeline so iOS Safari decodes HEVC clips in
  `<video>`. See `docs/ios-clip-playback.md`.

## [0.4.1] — 2026-05-22

### Added

- Per-event MP4 LRU cache (16 entries / 512 MiB) so one upstream
  Frigate fetch serves all Range subrequests for a clip.
- `HEAD /api/events/:id/clip.mp4` with the same headers as `GET`,
  required by iOS Safari before issuing the first Range request.

## [0.4.0] — 2026-05-21

### Added

- Camera groups with an in-app editor at `/settings`. Groups are
  YAML-backed at `$GROUPS_CONFIG_PATH` (default `/data/groups.yaml`),
  enforce a single-membership invariant server-side, and persist the
  last-selected group filter via `prefs.grid_filter` so reopens land
  on the same view.
- Full `/api/groups` CRUD with structured snake_case error codes.
- Mobile group filter opens a slide-up `BottomSheet` from the merged
  `AppHeader` title row.

## [0.3.2] — 2026-05-21

### Added

- `MobileTabBar` now appears on `/cam/[id]`, so the user always has
  bottom navigation.

### Changed

- `AppHeader` merged with the page title row on mobile, HD/ECO toggle
  moved into the title row (route-gated on `/`).
- Focus screens navigate between cameras via `goto(url, {
  replaceState: true })` so a single back-tap from focus always lands
  on the grid, regardless of how many cameras were hopped through.

## [0.3.1] — 2026-05-21

### Added

- Inline `<video>` clip playback in `EventModal` against
  `/api/events/:id/clip.mp4`, with a snapshot fallback when
  `has_clip === false` and a "Download video" button hitting the
  `?download=1` variant.

### Changed

- `prefsStore.loaded` gate prevents the focus connect effect from
  running with `$state(...)` defaults before `/api/prefs` resolves
  (fixes HQ stream forced on LQ users after grid → focus navigation on
  slower mobile networks).

## [0.3.0] — 2026-05-21

### Added

- `/events` route — real events list with cam, kind, and group
  filters, infinite scroll, real-time SSE updates, `EventCard`
  thumb-and-meta layout.
- `EventModal` for per-event detail (snapshot, clip placeholder at
  this tag, metadata).
- `GET /api/events` listing endpoint, per-event `thumbnail.jpg` /
  `snapshot.jpg` / `clip.mp4` (full clip pipeline lands later in the
  0.4.x line), `GET /api/stream` SSE hub.

### Removed

- `lib/mocks/` — replaced by the real `/api/events` shape.

## [0.2.0] — 2026-05-18

### Added

- Single-camera focus view via WHEP through `go2rtc`, sub-500 ms
  latency, runtime audio detection via the WHEP track event.
- HD/ECO snapshot modes for the grid; ECO renders 320px-wide JPEG
  tiles resized server-side via `golang.org/x/image/draw` at quality
  60.
- `POST /api/webrtc/whep` proxy endpoint and `GO2RTC_URL` environment
  variable.
- Server-side user preferences via `GET` / `PUT /api/prefs`
  (file-backed, atomic write).

### Changed

- ICE candidate strategy verified across LAN, VPN-tunnelled, and STUN
  srflx paths (configuration block in `docs/setup/frigate-config.md`).

## [0.1.0] — 2026-04-30

### Added

- Go BFF (`chi` router, `log/slog`) and SvelteKit + Svelte 5 frontend
  embedded into the binary via `embed.FS`.
- Camera grid with 1 Hz JPEG snapshots passthrough at
  `/api/cameras/:id/snapshot.jpg`.
- PWA manifest, service worker via `vite-plugin-pwa` `generateSW`,
  iOS / Android home-screen install.
- `Dockerfile` producing a single-binary distroless image around 9 MB.
- GitHub Actions workflow `build.yml` that publishes to GHCR on tag
  push.

## Comparison links

[Unreleased]: https://github.com/skua-app/skua/compare/v0.11.1...HEAD
[0.11.1]: https://github.com/skua-app/skua/releases/tag/v0.11.1
[0.11.0]: https://github.com/skua-app/skua/releases/tag/v0.11.0
[0.10.0]: https://github.com/skua-app/skua/releases/tag/v0.10.0
[0.9.0]: https://github.com/skua-app/skua/releases/tag/v0.9.0
[0.8.4]: https://github.com/skua-app/skua/releases/tag/v0.8.4
[0.8.3]: https://github.com/skua-app/skua/releases/tag/v0.8.3
[0.8.2]: https://github.com/skua-app/skua/releases/tag/v0.8.2
[0.8.1]: https://github.com/skua-app/skua/releases/tag/v0.8.1
[0.8.0]: https://github.com/skua-app/skua/releases/tag/v0.8.0

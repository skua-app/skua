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

Added
- Recording timeline. Scrub a camera's recorded history on a 1-hour window with an instant low-res preview as you drag, then release (or press play) to watch the full-resolution recording from any point. Reach it from a camera's focus view with the new History button, or from any event with "See on timeline", which opens the timeline centred on the moment the event began. On a device that can't decode the camera's recording codec (e.g. H.265 without a hardware decoder), the timeline falls back to a preview/scrub-only mode — the scrubber and low-res preview keep working and an honest hint explains why full-resolution playback is unavailable. The player also gained 10-second skip-back/forward buttons and a speed chip that cycles playback through 0.5×, 1×, 2×, and 4×, plus press-and-hold rewind / fast-forward buttons that rush the preview through the recording (the rate ramps up the longer you hold) with a VHS-style on-frame badge, releasing to settle full-resolution playback where you land. A thin activity lane along the top of the scrubber now marks Frigate's review segments — alerts in amber, detections in cyan — at their wall-clock spots, following the playhead as you pan and zoom so you can see at a glance when something happened. A second thin lane marks the moments Frigate's audio detection heard a sound, in green, and it stays hidden when there's no audio activity in view.
- A small colour legend under the recording timeline scrubber (desktop only) that labels the lane colours — Alert, Detection, Audio, and Recorded — so you can tell at a glance what each band means. It stays hidden on phones to keep the player on one screen.
- A "LIVE" control on the recording timeline scrubber that marks the live edge on the track and moves with it as you pan and zoom (parking just inside the right end when the live edge scrolls off-screen). Tapping it opens the camera's live view. It no longer tries to seek the recording to the live edge, where there is no footage yet.
- Recording-coverage lane on the timeline scrubber. The scrubber now shades the spans where footage was actually recorded as a neutral grey fill and leaves real recording gaps as empty track, so on a motion-only camera you can see at a glance which parts of the window have footage and which don't, while a continuous-record camera shows a fully filled bar. The coverage follows the playhead as you pan and zoom. This replaces the old faint per-hour density wash with real sub-hour recorded/gap coverage sourced from Frigate's per-segment recordings.
- Recording timeline BFF passthrough. The backend now reverse-proxies Frigate's recording VOD endpoint (HLS fMP4 playlists and segments) and the per-camera recordings-summary JSON through `/api/cameras/{id}/vod/{start}/{end}/*` and `/api/cameras/{id}/recordings-summary`. Single-camera scope, codec-agnostic, no transcode — Range is forwarded verbatim, segment bytes are cached as immutable, and playlists are not cached. Phase 1 of the recording timeline epic; no frontend yet.
- Each camera chip in the Events filters now shows a small green or grey status dot, so you can tell at a glance which cameras are online while filtering.
- Event types now show as icons. The type filter chips, the mobile event rows, and the desktop event cards carry a small glyph for person, vehicle, animal, or other, so the kind is recognisable at a glance.

Changed
- The recording timeline now opens on a 1-hour scrubber window instead of 3 hours, so on entry the cursor sits about 30 minutes before live with the right edge at live. You can still zoom out to a wider span.
- The small uppercase section labels on Events and the camera grid now use the regular Geist typeface instead of the monospace one, which reads much better at that size. This covers the GROUP/CAMERA/TYPE filter labels, the TODAY/YESTERDAY day dividers, and the grid's room/group headers, and the Events filter block now sits clearly apart from the day list so the TYPE label and the first day divider no longer blur together.
- The moments bell now appears only on the Cameras grid, on phone and desktop. It no longer sits in the header on Events, Settings, or a camera's focus view, where it was just clutter.
- The moment and event detail modals no longer pile their buttons into a wrapping row. Close is now an X over the top-right corner of the video, the way modals usually close, and "See on timeline" has become the filled primary action that spans the row, with Download (and, for a live moment, Open live) sitting beside it as small icon buttons. The separate "Open in Frigate" button is gone, since the app now has its own recording timeline reached through "See on timeline".
- The moment and event detail modals now have a proper header bar. The camera name reads as a clear title with the camera id as a small subtitle below it, and the close button moved off the video into that header. The camera name no longer shows up twice, since the duplicate row under the video is gone.
- The Events page header no longer shows the camera online and offline count. That summary belongs on the camera grid, where it stays, not on Events.

Fixed
- Recording timeline: the scrub preview no longer fails to load once the playhead sits on a fractional second.
- Recording timeline: cam5 (H.264) windows whose audio track flaps no longer hang playback. The player now falls back to video-only for that segment and keeps audio everywhere else; a muted-speaker glyph marks segments with no audio.
- Recording timeline on desktop: the player now fits one screen with no page scroll, the video fills the height above the controls instead of being capped, and the controls read as one centered bar instead of spreading to the screen edges. Mobile is unchanged.
- Recording timeline on desktop: the player column now accounts for the global app header height, so the scrubber no longer drops below the fold and the page no longer scrolls.
- The connecting and error overlays in the camera focus view now stay fixed and centered when you zoom the live view. Before, the "Connecting…"/"Buffering" pill and the stream-error card were inside the zoomable layer, so pinch or wheel zoom scaled and dragged them along with the video; the retry button could end up off screen. They now sit in a fixed layer over the feed frame like the telemetry panel does.
- The About screen no longer shows its tagline twice. The line "A calmer front-end for Frigate" now appears only once, in the identity card.
- A quick downward flick on the moments sheet now closes it instead of springing back. Before, only a slow drag that pulled the sheet far enough would dismiss it; a fast short flick snapped back. A slow short drag still settles back to its detent.

Removed
- Dropped the unused event-review deep-link endpoint (GET /api/events/{id}/review) and its frontend helper. It only ever fed the modal's old "Open in Frigate" button, which is gone now that events open on the app's own recording timeline.

## [0.13.0] — 2026-06-16

Added
- Drag-to-reorder cameras. Settings > Cameras lists every camera in one column with a drag handle; reorder by dragging and the new order saves right away. The order is shared across the household and shows up in the grid (phone and desktop) and in the focus view's camera switcher. Newly discovered cameras are appended so they never disappear from the list.
- A "Grid frame rate" setting in Appearance to switch the camera grid between 1 Hz (default) and 2 Hz. 2 Hz refreshes the tiles twice a second for a smoother grid, at roughly double the snapshot bandwidth. The setting is shared across the household.
- A Download button in the moment view that saves the full-resolution clip for that moment.

Changed
- Settings > Cameras has been reworked from inline editor cards into a single-column list. Editing a camera's friendly name and its per-camera main/sub stream overrides now happens in a focused modal opened from the row.
- The "while you were away" glance now uses Frigate's own review segments instead of grouping raw events itself, so each moment matches a single review segment and carries its severity (alert or detection) and zones. The grouping now matches what Frigate shows.
- The moment view now plays the full-resolution recording with audio for the whole moment, instead of stitching together the individual event clips.
- "Open in Frigate" from a moment or an event now opens the Frigate review timeline for that activity, instead of the Explore tab.

Fixed
- The glance sheet no longer flashes and then vanishes right after the app starts.
- The glance sheet can now be dragged to dismiss from anywhere on it, not just the handle. When the moment list is scrolled, dragging down scrolls the list first and pulls the sheet once it reaches the top, the way an iOS sheet behaves.
- The camera you're watching now stays in the focus filmstrip on phones and is highlighted, matching the desktop layout.
- Camera thumbnails in Settings no longer show an old frame after you drag a camera to a new position.
- The back gesture inside Settings now returns to the Settings list instead of leaving for another tab.
- The focus play/pause button is no longer highlighted while the stream is playing; it lights up only when the stream is paused.

Removed
- The GET /api/moments endpoint has been removed; the glance feed at GET /api/glance is now sourced from Frigate review segments. The moment payload changed shape as part of this: the representative-event fields and the inline event list are gone, replaced by the review id, severity, zones, detection ids, and a thumbnail detection id.

## [0.12.1] — 2026-06-14

Changed
- The events filters now fit on tighter single-line rows on both phone and desktop, so more of the events list is visible on first paint.
- The online/offline status now sits inline beside the page title on the mobile header and on the desktop Cameras and Events pages, instead of on its own second line. The status line is hidden on Settings.
- The Download button in the event clip modal and the Open Live button in the moment modal are now compact icon buttons; the labelled "Open in Frigate" picks up a leading link icon.
- The focus view's fullscreen toggle now uses conventional outward diagonal arrows for "enter fullscreen" and swaps to inward arrows when you're already in fullscreen.

## [0.12.0] — 2026-06-14

Added
- A refreshed interface across the whole app — grid, camera focus view, events, settings, and the "while you were away" sheet — on both phone and desktop, with a cleaner layout and clearer controls.
- Light, dark, and auto themes. Auto follows your device's system setting. Your choice is remembered per device, so switching theme on your phone doesn't change it on other household devices.
- Picture-in-picture button in the camera view on desktop and Android. (iPhone is excluded because Safari can't do picture-in-picture on a live stream.)
- Pinch-to-zoom on the live camera and on event/moment clips: pinch on touch, scroll-wheel zoom on desktop, drag to pan, double-tap to reset.
- Freeze-frame on pause: pausing the live view now holds the current frame instead of dropping back to an older thumbnail.
- A "while you were away" digest. On opening the app you get a sheet of recent activity grouped into per-camera "moments" (nearby detections clustered together), with an unseen count on a header bell you can reopen any time. Opening a moment marks just that one as seen; "mark all seen" clears the rest. Just dismissing the sheet never marks anything seen. You can set how far back it looks (6–72 hours) and how many moments it shows, in Settings → Appearance.
- The running app version is now shown in Settings → About.

Fixed
- Large event clips (long or high-quality recordings) now play instead of failing. The size limit is raised to 256 MB by default and can be changed with the CLIP_MAX_MIB setting.
- The camera focus view now shows a recent frame immediately when you open it, instead of an older one, and switches cameras faster.
- Various interface polish on phone: the selected camera in the focus filmstrip is no longer clipped, the camera filter closes cleanly, and zoom/pan gestures feel smoother.
- The pop-up sheets keep their title and handle in place while the list scrolls, and can be closed with a button, a swipe down, tapping outside, or Escape.

Changed
- The installed app (PWA) now opens instantly even offline and no longer shows a blank screen after an update — it refreshes itself to the latest version on next launch, and shows your camera grid's last images while reconnecting.
- When the app can't reach your Skua server it now shows a clear, retryable message instead of hanging or going blank, and recovers on its own once the server is back.
- The app feels more native on phones: page-zoom, accidental text selection, the iOS long-press menu, and tap highlights are turned off (text fields still let you select and edit).
- The "while you were away" sheet now rests at two positions — peek at the bottom or full height — instead of three.
- "Mark all seen" replaces the old "Clear": seen moments stay in the list marked as seen instead of disappearing, so you can still review them.

Reconfiguration note
- Saving the first-run setup or changing the connection in Settings restarts Skua by exiting the process, so the container needs a restart policy (restart: unless-stopped or always) to come back up. This is now called out in the setup screens and the README.

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

[Unreleased]: https://github.com/skua-app/skua/compare/v0.12.1...HEAD
[0.12.1]: https://github.com/skua-app/skua/compare/v0.12.0...v0.12.1
[0.12.0]: https://github.com/skua-app/skua/compare/v0.11.1...v0.12.0
[0.11.1]: https://github.com/skua-app/skua/releases/tag/v0.11.1
[0.11.0]: https://github.com/skua-app/skua/releases/tag/v0.11.0
[0.10.0]: https://github.com/skua-app/skua/releases/tag/v0.10.0
[0.9.0]: https://github.com/skua-app/skua/releases/tag/v0.9.0
[0.8.4]: https://github.com/skua-app/skua/releases/tag/v0.8.4
[0.8.3]: https://github.com/skua-app/skua/releases/tag/v0.8.3
[0.8.2]: https://github.com/skua-app/skua/releases/tag/v0.8.2
[0.8.1]: https://github.com/skua-app/skua/releases/tag/v0.8.1
[0.8.0]: https://github.com/skua-app/skua/releases/tag/v0.8.0

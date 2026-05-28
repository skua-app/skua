# Roadmap notes

Narrative notes attached to the epic roadmap: current deployed state, next
active epic, and what comes after the latest shipped tag. CLAUDE.md §13
holds the epic table and short status bullets; this file holds the prose
context.

## Public Launch series (v0.8.0)

The Public Launch series prepares the project for open-source publication
under `github.com/skua-app/skua`. Goal: ship a single squashed public
repo under MIT, with English as the baseline UI locale (ru kept as a soft
backup), public-grade docs, real screenshots, a CHANGELOG covering the
v0.1.0..v0.8.0 arc, a multi-arch GHA release workflow, and a public GHCR
package. Positioning is LAN-only by default — the README leads with
"trusted local network, no app-level auth" as a deliberate scope choice.
Remote-access patterns (reverse proxy with auth, VPN, zero-trust tunnel)
are presented as the user's responsibility, with hardening
recommendations in SECURITY.md.

- [x] L1 Project namespace booking
- [x] L2 Desensitize and rename to Skua
- [x] L3 Translate UI strings to English, ru kept as backup
- [x] L4 Rewrite VPN/LAN positioning in CLAUDE.md and docs/setup
- [x] L5 Public-grade README
- [x] L6 Legal and community meta (LICENSE MIT, CONTRIBUTING, SECURITY, CODE_OF_CONDUCT, issue templates)
- [x] L8 CHANGELOG.md covering v0.1.0..v0.8.0
- [x] L9 GHA release workflow with multi-arch and auto release notes
- [ ] L10 GHCR public visibility flip
- [x] L11 Squash and migrate to public repo, tag v0.8.0
- [x] L7 Real screenshots in docs/screenshots/ (deferred — synthetic Frigate rig built post-migration)
- [ ] L12 Announce (r/selfhosted, r/homelab, Frigate Discord, Show HN)

**Current deployed state:** v0.7.0 is the shipped tag. The household app
is feature-complete for live + events + groups + per-camera names + the
dynamic-camera-discovery loop, and per-camera go2rtc stream overrides can
now be edited from /settings. v0.4.0
landed camera groups (E3.3); v0.4.1 added a per-event MP4 LRU cache and
HEAD on the clip route; v0.4.2 added the in-place `hev1`→`hvc1` retag
so iOS Safari will decode the HEVC clip in `<video>`; v0.4.3 hid the
in-page `<h1>` on mobile for /events and /settings (AppHeader already
shows the page title, so the on-page heading was a visual duplicate —
kept on desktop where the header only has small nav links); v0.5.0
adds per-camera friendly-name overrides persisted in
`camera_names.yaml` (set in /settings, merged into `/api/cameras` as
`.name`, falls back to the static `config.Cameras` default when no
override is set); v0.5.1 patches the iOS PWA white/black-screen-on-relaunch
bug by setting `workbox.navigateFallback: 'index.html'` so deep-link
navigations resolve against the precached shell, switching SW activation
from `autoUpdate` to `'prompt'` so a new SW waits for the next cold start
instead of skipping-waiting mid-load, adding a `lib/lifecycle.svelte.ts`
hub that tears down the WHEP PeerConnection, the SSE EventSource, and the
cameras polling interval on `pagehide`/`visibilitychange:hidden` and
re-arms them on resume (with a 30-minute-absence hard-reload guard), and
adding `routes/+error.svelte` so a bootstrap-time throw renders a Russian
"Перезагрузить" card instead of a blank screen. v0.5.2 replaces the
black-then-video flash on focus mount with a snapshot poster layered
behind the `<video>` (the `<video>` is set to `background: transparent`
so it shows through until frames arrive) and a two-stage spinner
overlay that reads "Подключение..." while WHEP negotiates and
"Буферизация..." once `streamState === 'connected'` but before the
first frame paints. The poster stays in the DOM at `opacity: 0` after
the native `playing` event fires, so pause/resume and HQ↔LQ reconnects
re-show it with no HTTP roundtrip. v0.5.3 wraps the spinner and stage
text in a dark pill (`rgba(0, 0, 0, 0.5)` + `backdrop-filter: blur(10px)`,
matching the existing `.telemetry-pill` / `.ts-chip` style) so the
overlay stays readable when the snapshot underneath is bright.

**v0.6.0 ships E5 — Dynamic camera discovery from Frigate.** The static
`config.Cameras` slice is gone. The BFF now pulls the roster from
Frigate `/api/config` at startup and persists it to `cameras.yaml`,
falling back to that snapshot when Frigate is unreachable on a
subsequent boot. `POST /api/cameras/refresh` re-pulls the roster on
demand, returns the `{added, removed}` diff, broadcasts SSE
`camera.added` / `camera.removed`, and orphan-cleans the dependent
stores (groups, names, capabilities). Per-camera `talk_back` / `ptz`
flags moved to a separate `capabilities.yaml` override layer — read-only
via the API for now, hand-edited on the host. The full epic narrative
(architecture, sprint breakdown, deferred work) lives at
`docs/epics/E5-dynamic-camera-discovery.md`.

**v0.6.1 ships E6 sprint A — backend-only.** New
`internal/streamoverrides` package: thread-safe YAML-backed store keyed
by `cam_id`, mirroring the capabilities/names file-on-disk semantics,
with a `Set` that auto-removes the entry when both fields end up empty
after trim. New `internal/go2rtc` REST client (one method, `GetStreams`)
shares the long-lived `httpClient` with the rest of the BFF. Three new
endpoints land: `GET /api/go2rtc/streams` pass-through, `GET
/api/stream-overrides` (envelope-wrapped map), `PUT
/api/stream-overrides/{cam_id}` with full validation chain (invalid
body, unknown camera, unknown stream against the live go2rtc list,
go2rtc unreachable). The WHEP handler grows an override merge layer
that picks `override.Main` over `cam.StreamMain` (and same for Sub),
adds an `override` boolean to the debug log line, and returns a 400
`no stream configured for quality` when the final stream name is empty.
The override store joins the existing `SetOnRemoved` orphan-cleanup
chain in `main.go` so dropped cameras get their override row Forgot
alongside groups / names / capabilities. The editor itself was
deliberately deferred to v0.7.0 so the user-visible surface ships as a
single coherent diff.

**v0.7.0 ships E6 sprint B — the editor + a /settings IA redesign.**
The /settings page is reorganised into three sections — Внешний вид
(the existing 6 Segmented appearance prefs), Камеры (the
Refresh-from-Frigate button, the per-camera friendly-name editor, and
the new main/sub go2rtc stream selectors merged into a per-camera
card), and Группы (the existing groups CRUD). On mobile (<900px) the
three sections stack in a single column with no rail. On desktop
(≥900px) the page is a two-column grid: a sticky 180px left rail of
section anchor links with an IntersectionObserver-driven scroll-spy
active state, and a 720px max-width main column. The per-camera card
auto-saves on dropdown change with optimistic local update and
server-revert on `stream_not_found` / `go2rtc_unreachable`, surfacing
the BFF's Russian error message inline below the failed control for
4 s. The go2rtc streams list is fetched once on /settings mount and
cached in a new `go2rtcStreamsStore`; if go2rtc was unreachable at
that moment, a section-level error banner renders and every card's
selects are disabled while the name editor (independent code path)
keeps working. A new component subfolder `lib/components/settings/`
hosts the five split components (`AppearanceSection`, `CamerasSection`,
`CameraCard`, `GroupsSection`, `GroupCard`); the page itself stays
well under 200 LOC after extraction. Two new lazy-inited stores —
`streamOverridesStore` (mirrors the `groupsStore` CRUD shape) and
`go2rtcStreamsStore` (one-shot list cache) — are inited on /settings
mount only, not in `+layout.svelte`, so non-settings sessions don't
pay the shell-mount cost.

**After v0.7.0.** The most likely near-term follow-up is a small
`v0.7.x` patch that adds a `/settings` editor for `capabilities.yaml`
once a real talk-back consumer reappears (currently all SKUs in the
household either lack a microphone path or ship without ISAPI — see
`docs/hikvision-no-web-sku.md`). Beyond that, E7+ in §13 covers PTZ,
semantic search, and multi-user prefs, none of which have a near-term
trigger.

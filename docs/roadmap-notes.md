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
- [x] L10 GHCR public visibility flip
- [x] L11 Squash and migrate to public repo, tag v0.8.0
- [x] L7 Real screenshots in docs/screenshots/ (deferred — synthetic Frigate rig built post-migration)
- [ ] L12 Announce (r/selfhosted, r/homelab, Frigate Discord, Show HN) — in progress (Frigate Discord first; see LAUNCH.md)

**Current deployed state:** v0.10.0 is the shipped tag. The household app
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

**v0.8.x — Public Launch and first-run hardening.** v0.8.0 was the
Public Launch tag itself: rebrand from the private development name
to Skua, desensitize the repo (no real hosts, ports, or domains in
source), English baseline for UI strings with `strings.ru.ts` kept
as a backup for a future runtime locale-switching PR, MIT license,
`CONTRIBUTING.md` / `SECURITY.md` / `CODE_OF_CONDUCT.md` / issue
templates, the v0.1.0..v0.8.0 CHANGELOG, a public squash-migration
to `github.com/skua-app/skua`, and a multi-arch GHA release workflow
publishing to public GHCR. v0.8.1 cleaned up rebrand residue in the
header (no placeholder domain — desktop wordmark only, mobile shows
the live host) and aligned the PWA manifest `theme_color` /
`background_color` to `#0a0b0d`. v0.8.2 fixed focus-view LQ on
cameras without a Sub stream: quality now resolves per-camera (a
Sub-less camera always uses Main), the LQ segment is greyed out with
a hint, and DesktopFocus labels show the real go2rtc stream name
instead of a fabricated `*_h264` string. v0.8.3 documented the
distroless non-root uid (65532) requirement on `/data` in the README
Troubleshooting + Configuration sections and as an inline comment in
`compose.yaml` — no runtime change. v0.8.4 was a launch-hardening
patch prompted by the first public issue (#1): a fresh install
failed because the host `./data` bind mount was owned by `root` and
the container uid 65532 could not write to it, leaving the data
folder empty and the browser at connection-refused. Two changes
shipped together — the startup stderr block for an unwritable
`/data` became an actionable diagnostic naming the required uid/gid
and the two concrete remedies (`chown -R 65532:65532 <dir>` or a
named Docker volume), and a new `internal/emergency` package serves
a self-contained styled setup page on the normal port instead of
letting `main.go` exit on the two startup blockers (unwritable
`/data` via `fs.ErrPermission`, or Frigate unreachable on the very
first start with no `cameras.yaml` snapshot to fall back on). The
page polls `/healthz` every three seconds and auto-reloads into the
real app once the operator applies the fix and runs
`docker compose restart skua` — emergency `/healthz` returns 503 so
the reload only fires when the normal server returns 200. Only the
`camerasStore` blocker routes into emergency mode; the other stores
keep `os.Exit(1)`.

**What's next.** The only open Public Launch item is L12 announce (in progress) —
Frigate Discord, then r/selfhosted, then r/homelab, then Show HN —
which is copywriting, not code, sequenced warm-to-cold (see
`LAUNCH.md`). PTZ / semantic search / multi-user (E8+) remain
unscheduled with no near-term trigger.

## E7 — First-run setup wizard and runtime config overlay (v0.9.0)

v0.9.0 removed the `FRIGATE_URL`-required hard fatal at startup and
made Skua self-configuring in the browser. Three packages cooperate:

- `internal/runtimeconfig` is a thread-safe, YAML-backed store over
  a single overlay file (default `/data/config.yaml`), modelled on
  the `internal/streamoverrides` house style — flat envelope,
  atomic temp+rename write, fail-fast on malformed YAML, no file
  created until the first save.
- `internal/config` reads the overlay in `Load()` and resolves each
  URL with **env > file** precedence, then exposes provenance flags
  (`FrigateURLFromEnv`, `Go2RTCURLFromEnv`, `FrigateUIURLFromEnv`)
  and a `NeedsSetup` boolean. The prior "FRIGATE_URL is required"
  fatal is gone; `LOG_LEVEL` / `LOG_FORMAT` validation stays fatal.
- `internal/setup` is a server-rendered Go wizard that evolves the
  `internal/emergency` page pattern — same tokens, same brackets,
  same `/healthz`-poll-and-reload script — plus an interactive
  panel with **Test connection** (`POST /api/setup/test`,
  ephemeral probes against `frigate.GetStats` and
  `go2rtc.GetStreams`, never persists) and **Save and start**
  (`POST /api/setup/save`, validates with `url.Parse`, persists via
  the runtimeconfig store, then closes a restart channel that
  `Serve` selects on to shut down cleanly). Env-set fields render
  read-only with a "set via X environment variable" hint and are
  stripped from save POSTs server-side so a tampered payload can't
  pollute the overlay file.

Reconfiguration is **restart-based, not hot-reload.** After a
successful save, `setup.Serve` returns and `main.go` exits 0; the
container's `restart: unless-stopped` policy brings the BFF back
against the new file. This is the same shape the emergency page
already relied on, and it keeps the boot path linear — no client
re-wiring, no SSE drain.

The wizard has two entry modes:

- **Initial** — `cfg.NeedsSetup == true`. Empty fields, no banner.
  Replaces the prior `os.Exit(1)` on missing `FRIGATE_URL`.
- **Unreachable** — `cameras.New` returned a frigate-unreachable
  error **and** `!cfg.FrigateURLFromEnv`. The wizard is prefilled
  with the current overlay values and shows an error banner. This
  is the **Q5=B unreachable-wizard rule**: only serve the editable
  wizard when the operator can actually fix the URL from the
  browser. When the URL is env-locked, the prior informational
  `internal/emergency` `ReasonFrigateUnreachable` page is served
  instead, because env will win at next boot regardless of what
  the wizard could save. The `fs.ErrPermission` data-not-writable
  path also keeps the informational emergency page — a read-only
  data dir means the wizard couldn't persist anything anyway.

Caveat carried in CLAUDE.md §12: on an already-installed PWA, the
cached service worker can shadow the unreachable-wizard at the app
origin. The wizard is reliable on a fresh browser / first run,
which is the target deployment case.

E7.1 — an in-app `/settings` URL editor — landed in v0.10.0; see
the next section.

## E7.1 — In-app Connection editor (v0.10.0)

v0.10.0 added a `/settings → Connection` section that reads and
writes the same `internal/runtimeconfig` overlay file the first-run
wizard uses. Reconfiguration stays restart-based, not hot-reload:
swapping `frigateClient` / `eventsClient` / the SSE hub / the events
LRU mid-process would leak open connections and risk subscriber
drift, so a clean exit + container bounce is the only path. The UI
makes that explicit by splitting Save and Apply.

The form has three actions:

- **Test connection** runs ephemeral probes against the entered URLs
  via `POST /api/runtime-config/test`. Never persists.
- **Save** writes the overlay via `PUT /api/runtime-config`. Cheap:
  the effective in-memory URLs don't change until the next start,
  and a `Saved. Restart to apply.` hint is rendered.
- **Apply (restart now)** is a separate destructive control. It
  opens an inline confirm (matching the `GroupCard` confirm-delete
  pattern, no `window.confirm`), then calls
  `POST /api/runtime-config/restart`. That endpoint closes a
  restart channel in `main.go`; the shutdown-select sees it, runs
  the same graceful `srv.Shutdown` as SIGTERM, and exits 0. The
  page switches to a Restarting… state with a `/healthz` poller
  that reloads on 200; a ~30s fallback note suggests
  `docker compose restart skua` if the bounce takes longer.

Three implementation choices worth recording:

- **Probe extraction.** The setup wizard's test-connect logic moved
  into `internal/probe` (`Frigate`, `Go2RTC`, `ValidateURL`,
  `Report`). Both surfaces now share one implementation; the wizard
  delegates to `probe.Frigate(...)` / `probe.Go2RTC(...)` and the
  external JSON shape is unchanged so no client coupling regressed.
- **Restart wiring.** `main.go` builds a buffered restart channel +
  `sync.Once` closer **before** `api.NewHandler`. The closer is
  threaded into the handler via a new `RuntimeConfigDeps` struct
  (kept the constructor signature readable instead of seven trailing
  positional args). The shutdown block became a `select` over
  `quit` and `restart`; both branches funnel into the same
  `srv.Shutdown(ctx)`, and only the restart branch calls
  `os.Exit(0)`. The signal-driven path is byte-identical in
  behaviour, which keeps `docker compose restart skua` and the
  Docker SIGTERM lifecycle untouched.
- **Env precedence preserved.** `Env > file` resolution stays exactly
  as v0.9.0 left it. The new endpoints expose the same `*FromEnv`
  provenance the wizard already used: env-locked fields render
  read-only in the SPA **and** are stripped from PUT bodies
  server-side. The SPA could be bypassed, but env wins at next boot
  anyway, so storing the operator's attempt in the overlay would
  only confuse a future reader.

Caveat carried in CLAUDE.md §12: the iPhone PWA still cold-relaunches
after Apply because the SW shell cache survives the restart bounce —
the polling reload picks it up. Installed-PWA users see exactly the
same recovery as a normal container restart.

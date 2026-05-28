# Epic E3 — Live UX Polish + Events

> **Historical record (shipped v0.3.0).** This spec was written against the original private homelab deployment. Infrastructure references — host IPs, VPN gateway addresses, network topology — reflect that environment and are no longer authoritative. See CLAUDE.md §5 for the current canonical placeholders and docs/setup/ for setup guidance.

> **Read `CLAUDE.md` first.** Everything in that file applies. This spec only
> describes the scope and acceptance for E3.

## Goal

Bring the live-viewing experience to the quality that justifies this project
existing alongside the native Frigate UI: stable focus view, persistent app
shell, no mute glitches, instant camera switching, and an events list with
snapshot previews. Recording playback is **explicitly out of scope** —
Frigate's own UI handles that, and we deep-link to it.

## Scope

In:
- App shell rework: header and tab bar live in `+layout.svelte`, visible on
  all routes except `/cam/[id]`
- Focus view fixes: camera switching, mute persistence, LIVE chip removal,
  timestamp overlay toggle
- Desktop focus controls cleanup: deduplicate snapshot and fullscreen buttons,
  replace snapshot and telemetry icons
- Desktop focus sidebar: live `tile.jpg` previews of other cameras (1 Hz, ECO)
- `/archive` route removed entirely (deleted from filesystem, nav, i18n)
- Activity rail removed from DesktopGrid
- BFF events API: list + per-event thumbnail and snapshot proxies
- BFF `GET /api/config` for runtime config (Frigate UI base URL)
- BFF SSE hub: one upstream WS to Frigate, fanout to N PWA clients
- `/events` route: real list with cam + label filters, infinite scroll,
  snapshot previews, event detail modal with "Open in Frigate" deep-link
- Replacement of `MOCK_EVENTS` in DesktopGrid (the section that stays —
  Recent events in focus sidebar) with real data
- Deletion of `lib/mocks/events.ts` and all `TODO(api):` comments
- Tagged release `v0.3.0`

Out:
- HLS / MP4 clip playback (Frigate UI does this) → never
- IndexedDB offline cache for thumbnails → deferred to backlog
- Talk-back, push notifications → E4
- Mobile focus sidebar previews (no screen real estate; not asked for)
- PTZ controls (capability map keeps the field but UI stays disabled)
- Authentication of any kind — never (trusted-network access model; see CLAUDE.md §2)

## Sprint plan

E3 is delivered in four PRs, each independently shippable. Run `make check`
to completion before each commit.

| Sprint | Theme | Files touched | Risk |
|---|---|---|---|
| A | Stream polish (focus view bugs) | `routes/cam/[id]/+page.svelte`, `streams/whep.ts`, focus screens, `prefs/prefs.go` | Medium — WHEP regression risk |
| B | Desktop controls + icons | `lib/icons.ts`, `screens/DesktopFocus.svelte` | Low |
| C | Shell rework + `/archive` removal | `+layout.svelte`, all four screen components, `routes/archive/`, `i18n/strings.ts` | High — touches every route |
| D | Events backend + frontend | `backend/internal/api/events.go`, `backend/internal/sse/`, `routes/events/`, `lib/api.ts`, delete `lib/mocks/` | Medium — new surface area |

**Recommended order:** B → A → C → D. Rationale: B is mechanical and
visible. A stabilises the file C will later restructure (`+page.svelte` in
`/cam/[id]`). C is the most invasive — best done on a stable A. D is
self-contained new surface.

---

## Sprint A — Stream polish

### A1. Camera switch reconnect (bug 3)

**Symptom:** From `/cam/cam7`, tapping cam6 preview in sidebar updates URL,
header title, and sidebar grid — but the main video continues showing cam7's
stream. No new WHEP POST is issued.

**Root cause:** the `$effect` that calls `startWhep` is wrapped in
`untrack(() => connect(cam))` (from E2 fix for the mute reconnect bug, see
CLAUDE.md §12). `untrack` swallowed not only the transient reads (like
`isMuted`) but also `cam.id` itself, so the effect no longer re-runs when
the route param changes.

**Fix in `routes/cam/[id]/+page.svelte`:**

```ts
$effect(() => {
  // Tracked reads — must be OUTSIDE untrack so the effect re-runs on change.
  const id = cam.id
  const quality = prefsStore.streamQuality

  return untrack(() => {
    if (!videoEl) return
    const ac = new AbortController()
    startWhep({
      camId: id,
      videoEl,
      quality,
      signal: ac.signal,
      getMuted: () => prefsStore.mutedByDefault,  // see A2
      onStats, onState, onAudioDetected
    })
    return () => ac.abort()
  })
})
```

**Rule of thumb (add to CLAUDE.md §12):** in Svelte 5 effects, place all
reactive reads above the `untrack` boundary; place the side-effect and any
fast-changing reactive reads (toggles, transient UI state) inside.

### A2. Mute glitch on grid→focus navigation (bug 10)

**Symptom:** with `muted_by_default: false`, audio plays after navigating
from grid to focus, despite the mute button showing muted state. Tapping
mute restores correct behaviour.

**Root cause:** `startWhep` captures `initialMuted` by value at call time.
By the time `play().then()` resolves, `prefsStore.mutedByDefault` may have
been re-read elsewhere into a different value, but the closure still holds
the original. Race between `play().then(() => muted = initialMuted)` and
`onAudioDetected(true)`'s own muted re-application.

**Fix in `lib/streams/whep.ts`:**

Replace the `initialMuted: boolean` parameter with `getMuted: () => boolean`.
Call it at both points where muted state is applied:

```ts
videoEl.muted = true                    // satisfy iOS autoplay policy
await videoEl.play()
videoEl.muted = getMuted()              // apply current pref AFTER play

// inside onAudioDetected wrapper:
function handleAudioDetected(hasAudio: boolean) {
  if (hasAudio) videoEl.muted = getMuted()
  onAudioDetected(hasAudio)
}
```

This eliminates the stale closure entirely — the live pref value is read at
every application point.

### A3. LIVE chip removal (bug 9)

Remove the LIVE pill overlay from `MobileFocus.svelte` and
`DesktopFocus.svelte`. Status is already expressed by `OnlineDot` in the
meta row beneath the video. Delete the corresponding CSS in both files.

### A4. Timestamp overlay toggle (bug 4)

`prefs.show_timestamp` already exists and currently controls the timestamp
chip on grid tiles. Extend its semantics: it also controls the timestamp
overlay shown in the top-right of the focus video.

**Backend:** no change to `Prefs` struct — same field.

**Frontend:**
- `MobileFocus.svelte`, `DesktopFocus.svelte`: wrap the timestamp chip in
  `{#if prefsStore.showTimestamp}`
- `routes/settings/+page.svelte`: update the label for the existing
  `showTimestamp` toggle to reflect that it now applies to grid AND focus.
  New label in `i18n/strings.ts`: `"Время на тайлах и видео"`.

### A5. DesktopFocus sidebar live previews (bug 2)

**Current state:** DesktopFocus sidebar shows other cameras as static images
(presumably `snapshot.jpg` fetched once on mount).

**Target state:** each sidebar preview polls `tile.jpg` at 1 Hz, same as
ECO-mode grid tiles. The grid mode pref does NOT apply here — always ECO.
Rationale: focus view is bandwidth-sensitive (main video is the budget),
and 6 small tiles refreshing 1×/s at 320px wide is a known-cheap pattern.

**Implementation:** extract polling logic from `CameraTile.svelte` into a
small composable (`lib/streams/poll-tile.ts`) returning an updating image URL.
Alternatively, reuse `CameraTile.svelte` directly with a `forceMode='eco'`
prop. The second is simpler and avoids divergence.

Tapping a sidebar preview navigates to `/cam/<other_id>`. With A1 fixed,
this triggers a clean WHEP reconnect for the new camera.

### Acceptance — Sprint A

- [ ] From `/cam/cam7`, tapping cam6 in sidebar → main video shows cam6
      within 2 s, new WHEP POST visible in DevTools Network
- [ ] With `muted_by_default: false`, navigating grid → cam7 → audio is
      audible immediately; with `true`, audio is muted and mute button
      reflects state
- [ ] Toggling mute on focus persists for the session; navigating back to
      grid and into a different camera respects the latest pref
- [ ] LIVE pill no longer present on focus video on either breakpoint
- [ ] `show_timestamp = false` in settings → no timestamp on focus video AND
      no timestamp on grid tiles; `true` → both visible
- [ ] DesktopFocus sidebar tiles update every ~1 s, network shows
      `GET /api/cameras/<id>/tile.jpg` requests
- [ ] Sidebar tile tap on cam_other → URL changes, video reconnects, sidebar
      reflows so cam_other is no longer in the "other cameras" list

---

## Sprint B — Desktop controls + icons

### B1. Replace snapshot icon (bug 5)

Current `snapshot` icon path produces visual artefacts (per user report —
"глючная"). Replace with a cleaner camera shape, lucide-react style:

```ts
// lib/icons.ts
snapshot: [
  'M14.5 4h-5L7 7H4a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V9a2 2 0 0 0-2-2h-3l-2.5-3z',
  'M12 17a4 4 0 1 0 0-8 4 4 0 0 0 0 8z'
],
```

### B2. Replace telemetry icon (bug 6)

Currently appears to use `more` (three dots horizontal) for the telemetry
toggle — semantically wrong. Add a new `activity` icon (ECG zigzag) and
swap the call site:

```ts
// lib/icons.ts — add new entry
activity: 'M3 12h4l3-9 4 18 3-9h4',
```

Update the telemetry toggle in `DesktopFocus.svelte` (and `MobileFocus.svelte`
if it has the same affordance) to reference `activity` instead of `more`.

### B3. Deduplicate snapshot button (bug 7)

DesktopFocus currently has two snapshot affordances:
- snapshot button in the top-right cluster (above the video)
- snapshot + "download" buttons in the inline controls row beneath the video

Both invoke the same canvas-grab logic. Keep only **one** snapshot button in
the inline controls row. Delete:
- the top-right cluster snapshot button
- the "download" button (a duplicate of snapshot under a different label)

### B4. Deduplicate fullscreen button (bug 8)

DesktopFocus currently has:
- fullscreen chip in the "Stream details rail" (mono technical strip
  beneath the video)
- fullscreen button in the inline controls row

Keep only the inline controls row button. Delete the chip from the stream
details rail.

### Acceptance — Sprint B

- [ ] `snapshot` icon renders cleanly at 20 px and 24 px on Safari and Chrome
- [ ] `activity` icon is used wherever the telemetry toggle is rendered;
      `more` icon no longer appears in focus view
- [ ] DesktopFocus has exactly one snapshot button (in inline controls row)
      and one fullscreen button (same row)
- [ ] Snapshot button still produces a downloaded JPEG with correct filename
      (`<cam_id>_<iso_timestamp>.jpg`)

---

## Sprint C — Shell rework + `/archive` removal

### C1. Move header and tab bar to `+layout.svelte`

**Current state:** each grid screen (`MobileGrid`, `DesktopGrid`) renders
its own header. `MobileTabBar` is rendered only inside `MobileGrid`. Result:
`/events` and `/settings` have no header and no tab bar; `/cam/[id]` has
its own focus-specific header.

**Target state:** `+layout.svelte` renders the app shell (header + bottom
tab bar on mobile, header + top nav on desktop). Screen components render
only their content. Focus route (`/cam/[id]`) hides the layout chrome.

**Implementation:**

```svelte
<!-- routes/+layout.svelte -->
<script lang="ts">
  import { page } from '$app/state'
  import AppHeader from '$lib/components/AppHeader.svelte'
  import MobileTabBar from '$lib/components/MobileTabBar.svelte'

  let { children } = $props()
  let width = $state(0)
  const isDesktop = $derived(width >= 900)
  const isFocus = $derived(page.route.id === '/cam/[id]')
</script>

<svelte:window bind:innerWidth={width} />

{#if !isFocus}
  <AppHeader {isDesktop} />
{/if}

{@render children()}

{#if !isFocus && !isDesktop}
  <MobileTabBar />
{/if}
```

`AppHeader.svelte` is new — extracted from the headers currently embedded in
`MobileGrid` and `DesktopGrid`. Pass `isDesktop` to choose layout (mobile:
logo + online dot; desktop: logo + top nav + status + HD/ECO segmented +
refresh).

The HD/ECO segmented and refresh control belong in the header (desktop only)
because they are global — moving them up keeps them accessible from `/events`
without duplicating into every screen.

Screen components (`MobileGrid`, `DesktopGrid`, `MobileFocus`, `DesktopFocus`,
plus `/events`, `/settings`) lose their header markup and start with their
own content directly.

**Safe area:** `AppHeader` carries `padding-top: env(safe-area-inset-top)`.
The `.status-spacer` div currently in `MobileGrid` moves into the header or
is deleted (the header itself absorbs the inset).

### C2. Delete `/archive` route

```bash
rm -r frontend/src/routes/archive
```

Remove from `i18n/strings.ts`:

```diff
- archive: 'Архив',
```

Remove the "Архив" tab from `MobileTabBar.svelte` (4 → 3 tabs: Камеры /
События / Настройки).

Remove the "Архив" entry from the desktop top-nav in the new `AppHeader.svelte`.

The `history` icon stays in `icons.ts` for now (unused, but cheap to keep —
may be revived for a "last seen" affordance later).

### C3. Remove Activity rail from DesktopGrid

The "Активность · сегодня" 6-card grid section at the bottom of DesktopGrid
is deleted. Reasoning: events live on `/events`, the rail was a mock-driven
duplicate of that, and live cameras are the point of this screen.

The "Recent events" section in **focus** sidebars stays — it's contextual
(events of *this* camera).

### Acceptance — Sprint C

- [ ] Header visible on `/`, `/events`, `/settings` on both mobile and desktop
- [ ] Mobile tab bar visible on `/`, `/events`, `/settings`; absent on
      `/cam/[id]`
- [ ] Header absent on `/cam/[id]` (focus has its own back-button header)
- [ ] `/archive` returns 404; no "Архив" pill in nav anywhere; no
      `ui.archive` string referenced anywhere (`grep -rn "ui.archive"
      frontend/src` returns empty)
- [ ] DesktopGrid no longer renders the Activity rail; recent events in
      focus sidebars still render (using mock data until Sprint D)
- [ ] PWA install + iOS safe-area still correct (header padding-top from
      `env(safe-area-inset-top)`)

---

## Sprint D — Events backend + frontend

### D1. Runtime config endpoint

**New env var (in `.env.example`):**

```
# Public URL of the Frigate UI, used for "Open in Frigate" deep-links from
# the events list. Must be reachable from the user's device (i.e. a LAN
# address is fine — the PWA itself is LAN-only / behind the user's auth layer).
FRIGATE_UI_URL=http://<frigate-host>:5000
```

**New endpoint:** `GET /api/config` →

```ts
type AppConfig = {
  frigate_ui_url: string
}
```

The endpoint is unauthenticated, returns the same fields for everyone, no
secrets ever go here. Fetched once on app boot, cached in a Svelte store
(`lib/stores/config.svelte.ts`).

### D2. Events list endpoint

**New endpoint:** `GET /api/events`

Query parameters:
| Param | Type | Default | Notes |
|---|---|---|---|
| `camera` | string (cam_id) | — | optional, repeatable for OR filter |
| `label` | string | — | optional, repeatable for OR filter |
| `before` | ISO 8601 | now | cursor — return events `started_at < before` |
| `limit` | int | 50 | max 200 |

Response:

```ts
type EventsResponse = {
  items: EventItem[]
  next_before: string | null  // ISO of last item's started_at; null if items.length < limit
}

type EventItem = {
  id: string
  cam_id: string
  started_at: string      // ISO 8601 UTC
  ended_at: string | null // null if event still in progress
  duration_seconds: number | null
  label: string           // raw Frigate label (person, car, dog, ...)
  kind: EventKind         // normalised: person | vehicle | animal | other
  score: number | null    // top_score from Frigate
  has_snapshot: boolean
  has_clip: boolean       // informational only; we never serve the clip
}

type EventKind = 'person' | 'vehicle' | 'animal' | 'other'
```

**Label → kind normalisation** (server-side, in `backend/internal/events/`):

```go
var labelKind = map[string]EventKind{
  "person":     KindPerson,
  "car":        KindVehicle,
  "truck":      KindVehicle,
  "bus":        KindVehicle,
  "motorcycle": KindVehicle,
  "bicycle":    KindVehicle,
  "dog":        KindAnimal,
  "cat":        KindAnimal,
  "bird":       KindAnimal,
  // ... extend as needed
}
// Anything unmapped → KindOther.
```

The "motion" kind from the design mock disappears — Frigate's `motion`
events are noisy and not in the same `/api/events` endpoint upstream. If a
"motion" channel proves necessary later, it gets its own endpoint.

**Upstream call:** proxy `GET http://<frigate-host>:5000/api/events?` with
mapped query params. Frigate's params: `cameras`, `labels`, `before`,
`limit`, `has_snapshot=1`. Set `has_snapshot=1` always — we only show
events with snapshots.

**Context timeout:** 5 s for the upstream call.

### D3. Snapshot and thumbnail proxies

**New endpoints:**
- `GET /api/events/:id/thumbnail.jpg` → proxies `http://<frigate-host>:5000/api/events/<id>/thumbnail.jpg`
- `GET /api/events/:id/snapshot.jpg` → proxies `http://<frigate-host>:5000/api/events/<id>/snapshot.jpg`

Both respond with `Cache-Control: public, max-age=31536000, immutable`.
Event snapshots and thumbnails are write-once — once an event is recorded,
its preview never changes. Long cache is safe and dramatically reduces
both BFF load and Frigate hits during scrolling.

`ETag`: pass through from Frigate if present; otherwise `id` itself is a
sufficient validator.

### D4. SSE hub

**New endpoint:** `GET /api/stream` (Server-Sent Events)

Events emitted:
| event | data |
|---|---|
| `event.new` | full `EventItem` (started but not ended) |
| `event.end` | `{id, ended_at, duration_seconds}` |
| `camera.online` | `{cam_id}` |
| `camera.offline` | `{cam_id}` |

**Topology** (decision per discussion): one upstream WebSocket BFF → Frigate
`ws://<frigate-host>:5000/ws`, fanout to N SSE clients. Implemented as:

```
backend/internal/sse/
  hub.go         // subscribers map, broadcast()
  upstream.go    // Frigate WS client with exponential backoff (cap 30s)
  handler.go     // /api/stream HTTP handler
```

**HTTP wiring rules** (from CLAUDE.md §12):
- `http.ResponseController(w).SetWriteDeadline(time.Time{})` on the SSE handler
- `Cache-Control: no-store`, `Content-Type: text/event-stream`, `X-Accel-Buffering: no`
- Per-client send buffer is bounded (e.g. 32 events); slow clients are dropped

**Reconnect:** upstream WS reconnects with exponential backoff (1s, 2s, 4s,
8s, 16s, 30s capped). On reconnect, broadcast a synthetic
`event: reconnected` so clients can refresh their lists if needed.

### D5. Frontend events list

**New file: `routes/events/+page.svelte`** (replaces the EmptyState stub)

Layout:
- Filter chips at top: `Все · Cam 1 · Cam 2 · ... · Cam 7` (camera filter,
  multi-select), and below: `Все · Люди · Машины · Животные` (kind filter,
  multi-select)
- List of event cards, each:
  - `snapshot.jpg` (16:9 thumbnail of the full snapshot via BFF, lazy-loaded)
  - cam name + kind + time (relative: "5 мин назад") + duration
  - score (mono, right-aligned)
- Infinite scroll: when last card crosses 80% viewport → fetch next page
  with `?before=<last_item.started_at>`
- SSE subscription: `event.new` matching current filters → prepend to list

**Tap behaviour:** opens event detail modal (D6).

### D6. Event detail modal

**New component: `lib/components/EventModal.svelte`**

Triggered by tap on any event card or on a "Recent events" row in focus
sidebar. Contents:
- Full-size `snapshot.jpg` (via BFF; lazy-loaded)
- Metadata strip: cam name, label, kind, score, started_at (absolute,
  local time), duration
- Two actions:
  - Close
  - "Открыть в Frigate" — `<a href="${config.frigate_ui_url}/events/${id}" target="_blank">`

**Deep-link format:** to be verified against Frigate 0.17 at implementation
time. The 0.17 Explore UI changed paths; if `/events/<id>` no longer works,
fall back to `${frigate_ui_url}/explore?event_id=<id>`, and if that also
doesn't deep-link to a specific event, fall back to
`${frigate_ui_url}/?camera=<cam_id>`. Final choice committed in PR.

### D7. SSE store

**New file: `lib/stores/events-stream.svelte.ts`**

Long-lived `EventSource` opened once on app boot in `+layout.svelte`,
exposes the latest events via a Svelte store. Both the `/events` list and
the focus-sidebar "Recent events" subscribe to the same store.

Reconnect logic: `EventSource` auto-reconnects; on `error` event with
`readyState === EventSource.CLOSED`, manually reopen with the same
exponential backoff used on the server side (1s → 30s cap).

### D8. Replace `MOCK_EVENTS` and clean up

- `MobileFocus.svelte`, `DesktopFocus.svelte`: replace the `MOCK_EVENTS`
  import with a derived store filtered by current `cam.id`, limit 6,
  ordered by `started_at` desc. Use thumbnail (not snapshot) here —
  smaller and denser.
- Delete `frontend/src/lib/mocks/events.ts` and `frontend/src/lib/mocks/README.md`
- Verify: `grep -rn "TODO(api)" frontend/src/` returns no matches
- Verify: `grep -rn "MOCK_EVENTS" frontend/src/` returns no matches

### Acceptance — Sprint D

- [ ] `curl http://localhost:3200/api/config` returns
      `{"frigate_ui_url":"http://<frigate-host>:5000"}`
- [ ] `curl 'http://localhost:3200/api/events?limit=10'` returns 10 most
      recent events with full schema (D2)
- [ ] `curl 'http://localhost:3200/api/events?camera=cam7&kind=person&limit=5'`
      filters correctly
- [ ] `curl 'http://localhost:3200/api/events?before=2026-05-19T22:00:00Z&limit=5'`
      paginates backwards
- [ ] `GET /api/events/<real_id>/thumbnail.jpg` returns 200, image, with
      `Cache-Control: ... immutable`; second call hits browser cache
- [ ] Same for `/snapshot.jpg`
- [ ] `curl -N http://localhost:3200/api/stream` streams `event.new` lines
      when a real detection fires upstream
- [ ] BFF survives Frigate restart: SSE reconnects within ≤32 s, no client
      crash
- [ ] `/events` route shows real list, filters work (cam + kind, multi-select),
      infinite scroll fires at ~80% viewport
- [ ] Event card tap → modal with full snapshot + "Открыть в Frigate" link
      that opens `http://<frigate-host>:5000/...` in a new tab
- [ ] Focus sidebar "Recent events" populated from real API, thumbnails (not
      snapshots) used, max 6 items, this-cam-only
- [ ] `grep -rn "TODO(api)\|MOCK_EVENTS" frontend/src/` is empty
- [ ] `lib/mocks/` directory deleted

---

## Release

Tag: `v0.3.0`

Release notes (English):
- Fix: focus view reconnects WebRTC when switching camera from sidebar
- Fix: mute state persists across grid → focus navigation
- Fix: header and bottom tab bar present on all routes except focus
- New: live tile previews of other cameras in desktop focus sidebar
- New: timestamp overlay on focus video respects existing
  `show_timestamp` preference
- New: events list at `/events` with snapshot previews, cam/kind filters,
  infinite scroll, real-time updates via SSE
- New: event detail modal with deep-link to Frigate UI for clip playback
- Removed: `/archive` route (Frigate UI handles recording browsing)
- Removed: Activity rail from desktop grid (events live on /events)
- Removed: duplicate snapshot and fullscreen buttons in desktop focus
- Changed: LIVE pill removed from focus video; online status remains in
  meta row

## Updates to CLAUDE.md (apply at end of D)

### §4 / §11 — add config endpoint and Prefs note

`§11` API contract section grows:

```ts
// === E3 (stable) ===

// GET /api/config → AppConfig (frigate_ui_url)
// GET /api/events?camera=&label=&before=&limit= → EventsResponse
// GET /api/events/:id/thumbnail.jpg → image, immutable cache
// GET /api/events/:id/snapshot.jpg → image, immutable cache
// GET /api/stream (SSE: event.new, event.end, camera.online, camera.offline)
```

Prefs comment for `show_timestamp` is updated:

```ts
show_timestamp: boolean   // grid tiles AND focus video overlay
```

### §12 — append Svelte 5 untrack rule

> **`untrack` boundary rule for reactive effects.** Read all dependencies
> of an effect **above** `untrack`; place the side-effect and fast-changing
> reads (mute toggles, transient UI state) **inside** `untrack`. Wrapping
> the entire effect body in `untrack` (as was done in E2 to fix mute
> reconnects) will silently break route-param reactivity — the symptom is
> "URL changes but stream does not".

### §13 — roadmap

| Epic | Goal | Status | Tag |
|---|---|---|---|
| **E1** | Skeleton, grid, PWA | DONE | v0.1.0 |
| **E2** | WebRTC focus + HD/ECO grid + prefs | DONE | v0.2.0 |
| **Design** | Full UI redesign | DONE | — |
| **E3** | Live UX polish + Events | DONE | v0.3.0 |
| E4 | Talk-back (cam5/6/7), VAPID push, install UX | planned | v0.4.0 |
| E5+ | PTZ, semantic search, multi-user prefs | unscheduled | |

### §12 — remove from cosmetic backlog (now fixed)

> ~~- **Tab bar absent on /events, /archive, /settings on mobile.**~~
> Fixed in E3 (C1). Header and tab bar now in `+layout.svelte`.

### Cleanup of `/archive` references

Search and remove from `CLAUDE.md`:
- `/archive` in §8 repo layout tree
- "Архив" / `ui.archive` mentions if any

---

## Subsequent fixes (post-v0.3.0)

After the v0.3.0 release that closed the E3 scope above, the events
pipeline picked up four patch tags as iOS-specific issues surfaced in
real use. None are big enough to deserve their own epic doc; details
live in CLAUDE.md §12 (gotchas) and §11 (clip.mp4 contract).

- **v0.4.1** — BFF clip cache + HEAD. `<video>` issues 10–20 parallel
  Range subrequests per source; the original buffer-on-every-request
  path saturated the BFF. Per-event LRU (16 entries / 512 MiB) keyed
  by Frigate event id; HEAD on the clip route shares the GET handler.
  See `backend/internal/events/clipcache.go`.
- **v0.4.2** — In-place `hev1`→`hvc1` MP4 sample-entry retag. iOS
  Safari refuses to decode HEVC tagged `hev1`; Frigate writes `hev1`
  natively. 4-byte patch per video sample entry, no re-encode. Uses
  `github.com/abema/go-mp4`. See `backend/internal/events/hevc_retag.go`.
- **v0.4.3** — Frontend polish: hid the duplicate `<h1>` on mobile
  for `/events` and `/settings` (AppHeader already shows the page
  title; kept on desktop where the header only has small nav links).

For the full commit list: `git log --oneline v0.3.0..HEAD`.

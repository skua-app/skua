# BFF API contract

This is the canonical BFF contract for Skua. Currency: matches the latest
release tag in CHANGELOG.md. This file holds the full TypeScript-style
endpoint definitions and error shapes.

**Error envelope.** All BFF error responses use a single shape:

```json
{ "error": "<snake_case_code>", "message": "<human-readable>" }
```

`error` is always a machine-readable snake_case code and `message` is always
human text suitable for direct UI display. The generic `writeError` helper
emits a small set of common codes — `bad_request`, `not_found`,
`upstream_error`, `upstream_timeout`, `internal` — and individual feature
endpoints add their own codes (documented in the per-endpoint sections
below, e.g. `name_duplicate` for groups, `frigate_url_invalid` for runtime
config).

```ts
// === Health (stable) ===

// GET /healthz
//   NOT under /api — it is registered at the root, above the /api group, so
//   it carries no cross-site guard and no JSON envelope.
//   → 200, Content-Type: text/plain, body "ok". No auth, no dependencies
//   checked: it reports that THIS process is serving, not that Frigate or
//   go2rtc are reachable.
//
//   The 503 case comes from a different server. When a startup blocker fires
//   (fs.ErrPermission on /data, or first start with no cameras.yaml AND
//   Frigate unreachable) the BFF serves internal/emergency on the same port
//   instead, and that server answers /healthz — and every /api/* path — with
//   503. The emergency page polls /healthz every 3 s and reloads on a 200, so
//   only a restart into a healthy BFF clears it. A container healthcheck
//   pointed here therefore reports the emergency state as unhealthy, which is
//   the intent.

// === E1/E2 (stable) ===

// GET /api/cameras
// The response is ordered: see "Camera order (v0.13.0)" below. cam_ids
// present in the household-shared saved order appear first in that
// order; cam_ids not yet in the saved order are appended in registry
// order so newly discovered cameras never vanish.
type Camera = {
  id: string
  name: string         // user-supplied override (see /api/camera-names) else the Frigate-sourced default from the camera registry (cameras.yaml)
  online: boolean
  snapshot_url: string
  capabilities: {
    talk_back: boolean
    ptz: boolean
    // NOTE: audio is NOT in capabilities — detected at runtime from WHEP stream
  }
  streams: {
    main: string   // go2rtc stream name from cameras.<id>.live.streams.Main; the alias name is arbitrary (user-chosen, no required pattern)
    sub: string    // go2rtc stream name from cameras.<id>.live.streams.Sub; "" when the camera has no Sub stream (focus-view LQ unavailable, v0.8.2)
  }
  groups: string[]  // group ids this camera belongs to; 0 or 1 element (single-membership)
}

// GET /api/cameras/:id/snapshot.jpg
// → image/jpeg, full-res passthrough from Frigate. Cache-Control: no-store.
// Grid HD mode source.

// GET /api/cameras/:id/tile.jpg
// → image/jpeg, 320px wide, JPEG q=60, bilinear resize. Cache-Control: no-store.
// Grid ECO mode source.

// POST /api/webrtc/:cam_id/whep?quality=main|sub
// Body: SDP offer (Content-Type: application/sdp)
// Response: SDP answer. The handler relays go2rtc's upstream status
// verbatim (passthrough via w.WriteHeader(resp.StatusCode)) — go2rtc
// :1984 returns 200 on success, so 200 is what clients see in practice.
// quality defaults to "main". Invalid quality → 400.
// Upstream timeout 10s → 504. Other upstream error → 502.

// GET  /api/prefs
// PUT  /api/prefs  (partial update accepted, unknown fields → 400)
type Prefs = {
  grid_mode: 'hd' | 'eco'
  muted_by_default: boolean
  stream_quality: 'main' | 'sub'
  show_telemetry: boolean
  accent: 'cyan' | 'sage' | 'amber' | 'violet'
  name_style: 'below' | 'overlay' | 'off'
  show_timestamp: boolean   // grid tiles AND focus video overlay (E3)
  desktop_columns: 2 | 3 | 4 | 5
  mobile_columns: 1 | 2
  grid_filter: string | null  // last-selected group id, null = "Все" (E3.3)
  glance_window_hours: 6 | 12 | 24 | 48 | 72  // "while you were away" lookback
  glance_max_moments: 10 | 20 | 30 | 50       // cap on glance peek output moments
  grid_fps: 1 | 2                             // grid tile refresh rate, Hz
  timeline_mode: 'follow' | 'fixed'           // recording-timeline interaction
                                              // model: 'follow' pins the
                                              // playhead to the centre and
                                              // moves the track past it,
                                              // 'fixed' holds the track still
                                              // and moves the playhead along it
}
// Stored at /data/prefs.json. Atomic write (tmp + rename).
// Defaults: eco / true / main / false / cyan / below / false / 4 / 1 / null / 24 / 20 / 1 / follow

// === E3 (stable) ===

// GET /api/config → AppConfig
// Sourced from FRIGATE_UI_URL env (defaults to FRIGATE_URL when unset).
// Public payload; no secrets here, ever.
type AppConfig = {
  frigate_ui_url: string   // base URL for "Open in Frigate" deep-links
}

// GET /api/storage → StorageInfo
// mounts is sourced from Frigate's GET /api/stats service.storage map;
// cameras is sourced from Frigate's GET /api/recordings/storage. Both arrays
// are always present, never null; an empty/missing source yields [].
// The *_mib figures are mebibytes (1024-based), passed through from Frigate
// unchanged; mount `type` carries Frigate's mount_type.
// Mount ordering: paths under /media first, all other paths after; within
// each group sorted by path ascending (puts recordings/clips/db on top,
// /tmp/cache + /dev/shm at the bottom).
// Camera ordering: sorted by usage_mib descending (heaviest first).
//   usage_mib            — camera's recordings on disk, MiB
//   bandwidth_mib_per_hr — recording write rate, MiB/hr
//   usage_percent        — camera's share of the recordings disk, 0–100
//   id                   — the Frigate camera key, verbatim
// Error policy: if the mounts source (stats) fails → 502
// { error: "upstream_error", message }. If only the per-camera source
// (recordings/storage) fails, the BFF logs a warning and returns the mounts
// with cameras: [] (HTTP 200) — the per-camera block must not break the
// mounts view.
type StorageInfo = {
  mounts: StorageMount[]
  cameras: StorageCamera[]
}
type StorageMount = {
  path: string       // mount path as Frigate reports it
  type: string       // Frigate's mount_type
  total_mib: number  // MiB
  used_mib: number   // MiB
  free_mib: number   // MiB
}
type StorageCamera = {
  id: string                   // Frigate camera key, verbatim
  usage_mib: number            // MiB
  bandwidth_mib_per_hr: number // MiB/hr
  usage_percent: number        // share of recordings disk, 0–100
}

// GET /api/events?camera=&label=&before=&limit=
//   camera, label: repeatable OR-filters (also accept comma-separated values)
//   before:        ISO 8601; BFF translates to unix-seconds for Frigate
//   limit:         default 50, max 200
// has_snapshot=1 is always set upstream — events without snapshots are never surfaced.
//
// Pagination granularity (accepted limitation): the `before` cursor and the
// `next_before` response field are second-precision. Frigate's events API
// accepts only integer unix-seconds, so the BFF rounds the ISO 8601 cursor to
// whole seconds before forwarding. As a result, if two events share the exact
// same started_at second across a page boundary, one of them could be skipped
// or duplicated between pages. The API can't do better given Frigate's
// granularity, and at typical household event rates this collision is
// negligible.
type EventsResponse = {
  items: EventItem[]
  next_before: string | null  // started_at of last item, or null at end-of-history
}
type EventItem = {
  id: string
  cam_id: string
  started_at: string      // ISO 8601 UTC
  ended_at: string | null // null while in progress
  duration_seconds: number | null
  label: string           // raw Frigate label (person, car, dog, ...)
  kind: EventKind         // server-normalised; see backend/internal/events.KindFor
  score: number | null    // data.top_score (fallback data.score); top-level top_score is always null in 0.17
  has_snapshot: boolean
  has_clip: boolean       // gates inline clip playback (EventModal); falls back to snapshot when false
}
type EventKind = 'person' | 'vehicle' | 'animal' | 'other'

// GET /api/events/:id/thumbnail.jpg
// GET /api/events/:id/snapshot.jpg
// → image/jpeg; Cache-Control: public, max-age=31536000, immutable
// ETag passed through from Frigate, else derived from event id.

// GET  /api/events/:id/clip.mp4
// HEAD /api/events/:id/clip.mp4              (http.ServeContent skips the body)
// GET  /api/events/:id/clip.mp4?download=1   (Content-Disposition: attachment)
//
// Pipeline on a cache miss:
//   1. Fetch upstream clip from Frigate, bounded by a 30 s per-call
//      context deadline (the events http.Client no longer sets
//      Client.Timeout, so the per-call context is the real ceiling).
//   2. Buffer into memory with a per-clip cap of 64 MiB (default 30 s @
//      3500 kbps ≈ 13 MiB; >64 MiB → 502 with "exceeds limit" in the log).
//   3. Rewrite every `hev1` HEVC sample-entry tag to `hvc1` in place — 4
//      bytes per video sample entry, no re-encode, same length. Required
//      for iOS Safari <video>; desktop browsers don't care.
//   4. Insert the retagged bytes into an in-process LRU keyed by event id
//      (16 entries / 512 MiB total; oversize entries are served but not
//      cached). Frigate event ids are immutable (clips are write-once),
//      so no TTL — old entries fall out by LRU only.
//   5. Serve via http.ServeContent: Accept-Ranges, 206 Partial Content,
//      Content-Length, ETag = event id, Cache-Control: public, max-age=
//      31536000, immutable. Inline by default; ?download=1 swaps the
//      Content-Disposition to attachment with filename "frigate-<id>.mp4".
//
// Cache hits skip steps 1–4 and go straight to ServeContent against the
// stored []byte. Why the cache is mandatory, not optional: <video> issues
// 10–20 parallel Range subrequests per source — re-fetching ~35 MiB of
// upstream per range saturates the BFF, individual ranges hit the 30 s
// ceiling or get cancelled, and the player reports a MediaError. See
// backend/internal/events/{clipcache.go,hevc_retag.go} for the
// implementations.

// GET /api/stream  (Server-Sent Events)
// Events emitted to clients:
//   event.new       — full EventItem
//   event.end       — { id, ended_at, duration_seconds }
//   camera.online   — { cam_id }
//   camera.offline  — { cam_id }
//   reconnected     — empty payload; sent on every upstream WS reconnect (not initial)
// Topology: one persistent WS BFF → Frigate /ws, fanout to N clients with
// bounded per-client buffers (cap 32); slow consumers are dropped.

// === E3.3 (stable) ===

// Env: GROUPS_CONFIG_PATH (default /data/groups.yaml). See .env.example
// for host-side mapping options.

// GET    /api/groups          → Group[]
// POST   /api/groups          → { name: string }                                   → Group (201)
// PATCH  /api/groups/:id      → { name?: string, camera_ids?: string[] }          → Group
// DELETE /api/groups/:id      → 204
type Group = {
  id: string         // uuid v4, server-generated
  name: string       // 1..30 chars after trim; unique case-insensitive
  camera_ids: string[]  // each must exist in the camera registry (sourced from Frigate, persisted to cameras.yaml); no duplicates
}

// Single-membership invariant: a camera belongs to at most one group at a
// time. On PATCH the backend reconciles by stripping the given camera ids
// from every other group atomically. Frontend always sends the desired full
// membership for the target group — no separate "remove from old group" call.

// Storage: YAML at $GROUPS_CONFIG_PATH (default /data/groups.yaml; the file
// is auto-created on first write). Malformed YAML at startup → fail-fast.

// Validation errors return 400 with a structured body. The error code is
// snake_case and stable; message is English and meant for direct UI display.
type GroupErrorBody = {
  error:
    | 'name_empty'
    | 'name_too_long'
    | 'name_duplicate'
    | 'camera_not_found'
    | 'duplicate_camera'
    | 'not_found'
    | 'invalid_body'
    | 'empty_patch'
    | 'missing_id'
    | 'internal'
  message: string  // English, ready to display
}
// Frontend wraps this in GroupApiError (extends Error, carries `.code`) so
// editor forms can switch on .code for inline placement.

// === Per-camera friendly names (v0.5.0) ===

// Env: CAMERA_NAMES_CONFIG_PATH (default /data/camera_names.yaml).

// GET /api/camera-names → { names: { [cam_id]: string } }
//   Returns ONLY cameras with an explicit override. Cameras absent from the
//   map use their Frigate-sourced default from the camera registry
//   (cameras.yaml) — which is already merged into
//   /api/cameras under .name, so the UI rarely needs to call this endpoint
//   directly (settings reads .name from camerasStore).

// PUT /api/camera-names/{cam_id}  body: { name: string }
//   Sets the override for cam_id. Trimmed; rune-length max 30.
//   An empty/whitespace name CLEARS the override; the camera reverts to its
//   config default on the next /api/cameras fetch. Returns:
//     { cam_id: string, name: string }  // name = merged result after the write
//
// Storage: YAML map under top-level key `names:`. File auto-created on first
// write; missing file at startup is fine (empty store). Malformed YAML on
// load → fail-fast.
//
// Validation errors return 400 with a structured body { error, message }.
// Error codes: 'camera_not_found' | 'name_too_long' | 'invalid_body' |
// 'missing_id' | 'internal'. Frontend wraps in CameraNameApiError.

// === E5 (sprint B — backend) ===

// POST /api/cameras/refresh
//   Re-pulls Frigate /api/config, persists the new snapshot to cameras.yaml,
//   broadcasts SSE camera.added / camera.removed for the diff, and cleans
//   up orphan refs in groups.yaml / camera_names.yaml / capabilities.yaml
//   (each store implements Forget(cam_id) — no-op when absent, atomic write
//   when present). Body is ignored. Returns 200 with the sorted diff:
type RefreshDiff = {
  added: string[]    // sorted ascending; empty array, not null
  removed: string[]  // sorted ascending; empty array, not null
}
//
// Frigate unreachable → 502 with the standard envelope, no mutation, no
// SSE events:
type RefreshErrorBody = {
  error: 'frigate_unreachable' | 'internal'
  message: string  // English, ready to display
}

// SSE event additions (broadcast on /api/stream):
//   camera.added   — { cam_id: string, name: string }  // name = merged
//                    (override if any, otherwise Frigate name)
//   camera.removed — { cam_id: string }
// These fire exclusively from the refresh endpoint; startup does NOT emit
// them (no client is connected yet). camera.online / camera.offline keep
// their existing semantics (per-camera live status from the WS upstream).

// capabilities.yaml schema (hand-edited on the host in sprint B; no API):
//
//   capabilities:
//     cam5: { talk_back: true, ptz: false }
//     cam6: { talk_back: true }
//
// Cameras absent from the file get the zero value {talk_back:false, ptz:false}.
// Storage path: $CAPABILITIES_CONFIG_PATH (default /data/capabilities.yaml).
// Missing file → empty store (fine). Malformed YAML on load → fail-fast.

// === E6 (sprint A — backend) ===
//
// Per-camera go2rtc stream-name overrides. The override layer applies ONLY
// inside the WHEP handler — GET /api/cameras keeps surfacing the Frigate-truth
// stream names from cameras.yaml. Override storage is per-installation
// (parallel to capabilities.yaml), not per-user, so it lives outside prefs.
// On POST /api/cameras/refresh the OnRemoved hook chain Forgets cleared cam
// ids in stream_overrides.yaml alongside groups / camera_names / capabilities.

// GET /api/go2rtc/streams
// → string[]  // sorted ascending; pass-through of go2rtc /api/streams keys
//   Used by the sprint-B editor to populate per-camera selector options.
//   go2rtc unreachable → 502 with the standard StreamOverrideErrorBody envelope.

// GET /api/stream-overrides
// → { overrides: { [cam_id]: { main: string, sub: string } } }
//   Only entries where at least one of main/sub is non-empty are present;
//   the empty map serialises as {}, not null. Mirrors /api/camera-names.

// PUT /api/stream-overrides/{cam_id}  body: { main: string, sub: string }
//   Both fields are REQUIRED (a missing field is invalid_body, not "fall
//   through"). Empty strings are allowed; sending both fields blank clears
//   the entry. Each non-empty value must exist in /api/go2rtc/streams or
//   the request is rejected with stream_not_found before persistence.
//   Returns the saved Override: { main: string, sub: string }.
//
// Storage: YAML map under top-level key `stream_overrides:`. File auto-
// created on first non-empty write; missing file at startup is fine
// (empty store). Malformed YAML on load → fail-fast.
//
// Validation errors return 4xx/5xx with a structured body { error, message }.
// Error codes are snake_case and stable; messages are English and ready for
// direct UI display.
type StreamOverrideErrorBody = {
  error:
    | 'invalid_body'        // missing field, malformed JSON
    | 'camera_not_found'    // cam_id absent from the registry → 404
    | 'stream_not_found'    // value not in /api/go2rtc/streams → 400
    | 'go2rtc_unreachable'  // upstream listing failed → 502
    | 'missing_id'          // empty {cam_id} path param
    | 'internal'
  message: string  // English, ready to display
}

// Env: STREAM_OVERRIDES_CONFIG_PATH (default /data/stream_overrides.yaml).
// See .env.example for host-side mapping options.

// === E7.1 — runtime config (stable) ===
//
// The Frigate / go2rtc / Frigate-UI URLs are configurable at runtime
// through the in-app /settings → Connection editor. The editor reads
// and writes the same overlay file as the first-run setup wizard
// (/data/config.yaml via internal/runtimeconfig), with env > file
// precedence: env-sourced values are "locked" (rendered read-only in
// the SPA AND stripped from PUT bodies server-side, so a tampered
// client cannot persist a stale value into the overlay — the env
// value would win at next boot anyway). Reconfiguration is
// restart-based: a successful Apply closes the main shutdown-select
// restart channel, the BFF runs the same graceful srv.Shutdown as
// SIGTERM, exits 0, and the container restart policy brings it back
// up against the new overlay. No hot-reload of frigateClient /
// eventsClient / SSE hub / events LRU.

type RuntimeConfigURLs = {
  frigate_url: string
  frigate_ui_url: string
  go2rtc_url: string
}

type RuntimeConfigLocked = {
  frigate_url: boolean      // true → set via FRIGATE_URL env, read-only in UI, stripped from PUT
  frigate_ui_url: boolean   // true → set via FRIGATE_UI_URL env
  go2rtc_url: boolean       // true → set via GO2RTC_URL env
}

type RuntimeConfigResponse = {
  effective: RuntimeConfigURLs   // what the running process is actually using
  overlay:   RuntimeConfigURLs   // raw values from /data/config.yaml (empty strings when no overlay)
  locked:    RuntimeConfigLocked // env-provenance flags
}

// GET /api/runtime-config → RuntimeConfigResponse

// PUT /api/runtime-config
//   body: RuntimeConfigURLs   (all three fields; env-locked entries are
//                              ignored server-side and replaced with the
//                              current effective value before persistence)
//   → RuntimeConfigResponse   (effective is unchanged until the next start;
//                              overlay reflects the new on-disk values)
//   Validation: probe.ValidateURL on every non-empty field
//   (parseable, http/https scheme, host present). frigate_url is required
//   after env-lock stripping; go2rtc_url and frigate_ui_url are optional.
//
//   Persistence: atomic write of /data/config.yaml via
//   internal/runtimeconfig.Store.Save. fs.ErrPermission → 500
//   data_not_writable (uid/gid 65532 chown hint in the message).

// POST /api/runtime-config/test
//   body: { frigate_url: string, go2rtc_url: string }
//     (Frigate-UI URL is not probed — it's a browser-side deep-link target.)
//   → ProbeReport
//   Frigate calls GetStats; go2rtc calls GetStreams; each with a 3 s
//   per-call context timeout. Empty go2rtc_url is "skipped" (Skipped:true,
//   OK:false, Error:""). Empty frigate_url is a hard error.
type ProbeResult = {
  ok: boolean
  skipped?: boolean   // only emitted for go2rtc when the caller passes ""
  error?: string      // short message stripped of nested net wrapping
}
type ProbeReport = {
  frigate: ProbeResult
  go2rtc:  ProbeResult
}

// POST /api/runtime-config/restart
//   → 202 { status: "restarting" }
//   Closes the main shutdown-select restart channel; the process exits 0
//   via the normal graceful shutdown path and the container restart
//   policy boots the new overlay file. There is no "in-place reload"
//   path — swapping clients mid-process would leak open connections and
//   risk cache / subscriber drift.

// Structured error body { error, message }; codes are snake_case and stable.
type RuntimeConfigErrorBody = {
  error:
    | 'invalid_body'           // malformed JSON or missing field
    | 'frigate_url_required'   // empty after env-lock stripping
    | 'frigate_url_invalid'    // probe.ValidateURL failed
    | 'go2rtc_url_invalid'     // probe.ValidateURL failed
    | 'frigate_ui_url_invalid' // probe.ValidateURL failed
    | 'data_not_writable'      // fs.ErrPermission on overlay write (chown 65532)
    | 'internal'
  message: string  // English, ready to display
}

// First-run setup wizard (server-rendered, no SPA route): the
// internal/setup server exposes POST /api/setup/test and POST /api/setup/save
// with the same ProbeReport / overlay-write shape. Served only when the
// BFF has no effective Frigate URL (env+overlay both empty); a successful
// save triggers the same clean exit + container-restart as
// /api/runtime-config/restart. Not part of the steady-state SPA surface.

// === Glance — Moment shape (internal) ===
//
// "Moments" surface recent Frigate activity for the glance "while you
// were away" UI. Each Moment is a per-camera review segment sourced
// directly from Frigate's /api/review endpoint — Frigate already
// performs the cross-event clustering server-side, so the BFF reshapes
// one review item into one Moment with no client-side grouping.
//
// The previous BFF-internal grouping endpoint (GET /api/moments) and
// its representative-event ranking have been removed in favour of
// Frigate-native review segments. Replaced fields from that contract:
//   - event_count, representative_event_id, representative_has_clip,
//     events[]  → removed
// New fields:
//   - id           (Frigate review id; stable for seen-state keying)
//   - severity     ("alert" | "detection")
//   - zones        (zone names from Frigate config)
//   - detection_ids (tracked-object event ids in the segment)
//   - thumb_event_id (the detection id whose snapshot the UI should
//                     use as the moment thumbnail — the latest
//                     tracked-object event in the segment by
//                     unix-seconds prefix; empty when detections is
//                     empty)

type Moment = {
  id: string                      // Frigate review id, stable per segment
  cam_id: string                  // Frigate camera id
  started_at: string              // ISO 8601 UTC
  ended_at: string | null         // null while the segment is still active
  severity: 'alert' | 'detection'
  kinds: EventKind[]              // distinct, stable encounter order, deduped
  labels: string[]                // distinct raw labels, sorted ascending
  zones: string[]                 // zone names from Frigate config, as supplied
  detection_ids: string[]         // tracked-object event ids inside the segment
  thumb_event_id: string          // empty when detection_ids is empty
}

// === Glance — household seen-state ===
//
// Household seen-state for the glance feature. The store persists a
// scope-keyed SET of viewed ids plus a seen_through watermark to a
// dedicated state file (env GLANCE_STATE_PATH, default
// /data/glance.json) and exposes endpoints that compose Frigate's
// review segments (see Moment above) with that set.
//
// The seen-state is keyed on the review-sourced moment id (m.id): a
// moment is "seen" iff its id appears in the household scope's set OR
// its started_at is at or before the household seen_through
// watermark. The set is scope-keyed under a single v1 scope
// ("household") shared by every client on the LAN-only deployment;
// the `scope` field is reserved for a future per-user split and
// clients must omit it (or send "household") in v1. Set entries are
// pruned to a 30-day retention window on load and after every write
// so the file stays bounded.
//
// Wire-compat note: POST /api/glance/seen still accepts the body
// shape { event_ids: string[], scope?: string }. The field is named
// `event_ids` for backwards compatibility with the original glance
// rollout; the store is id-agnostic, so clients send moment ids
// (m.id) under this key.

// GET /api/glance?hours=&max=
//   hours: optional positive integer (default 24, clamped to 1..168 =
//          1 hour through 7 days). Controls how far back the "while you
//          were away" window extends.
//   max:   optional positive integer (default 20, clamped to 1..200).
//          Caps the number of REAL moments returned.
//   The BFF makes a single GET /api/review call against Frigate with
//   `after = now - hours` (unix seconds) and an over-fetched
//   `limit = min(max * 4, 200)`. Moments with no tracked-object
//   detections (empty motion-only segments) are omitted, so the
//   upstream page is over-fetched to let the output `max` count real
//   moments. Frigate returns review segments newest-first; the BFF
//   preserves that order and truncates the surviving moments to `max`.
//   `unseen_count` counts only the moments actually returned. The window is purely
//   time-based: there is no `cleared_at`-style clamp, and moments
//   already at-or-before the household `seen_through` watermark stay
//   in the response (they render with `seen: true`, not dropped).
//   Each moment carries a `seen` boolean that is true iff its `id`
//   is in the household seen-set OR its `started_at` is at or before
//   the household `seen_through` watermark. `unseen_count` is the
//   count of moments whose `seen` is false. There is no pagination
//   cursor on this endpoint; clients re-fetch from the top.
//
// Response: Cache-Control: no-store.

type GlanceResponse = {
  unseen_count: number     // moments where seen === false, surfaced for badge UI
  moments: GlanceMoment[]  // all surviving moments, both seen and unseen
}
type GlanceMoment = Moment & { seen: boolean }

// GET /api/glance/{id}/clip.mp4
//   Resolves the Frigate review id `{id}` to the segment's
//   [start_time, end_time] window (same pad/clamp rule as the
//   preview endpoint) and serves Frigate's full-resolution
//   /api/{camera}/start/{start}/end/{end}/clip.mp4 through the same
//   buffered + hev1→hvc1 retag pipeline as
//   GET /api/events/{id}/clip.mp4. Real-time playback with audio,
//   intended for the moment-detail "Open clip" affordance — the
//   counterpart to the scrub-quality preview endpoint below.
//
//   The clip path uses the per-event clip timeout (eventsClipTimeout,
//   30s) — Frigate reassembles the time-range clip on demand and is
//   roughly as expensive as a per-event clip, so the 10s preview
//   timeout would be too tight here.
//
//   Cache + single-flight are keyed on the review id, so repeat
//   opens of the same moment share one upstream fetch. The ETag and
//   immutable Cache-Control assume the cached window is stable; an
//   active (still-growing) review served before its segment
//   finalises may keep serving a slightly-short window until LRU
//   evicts the entry. Acceptable for the household glance UI.
//
//   With ?download=1 the response is served as an attachment with
//   filename "frigate-{id}.mp4"; without the flag it stays inline.
//   The download and the inline player share the same buffered bytes
//   (cache + single-flight are keyed on the review id) — only the
//   Content-Disposition differs per request. Same convention as
//   GET /api/events/{id}/clip.mp4?download=1.
//
//   No HEAD route: the buffered pipeline does not have a useful HEAD
//   path, and the glance UI only ever issues GETs.
//
//   Errors map the same way as the preview path:
//     - invalid id format     → 404 not_found
//     - upstream review 404   → 404 not_found
//     - upstream review or
//       clip context deadline → 504 upstream_timeout
//     - other upstream / transport failure → 502 upstream_error

// GET  /api/glance/{id}/preview.mp4
// HEAD /api/glance/{id}/preview.mp4
//   Resolves the Frigate review id `{id}` to the segment's
//   [start_time, end_time] window, then reverse-proxies Frigate's
//   /api/{camera}/start/{start}/end/{end}/preview.mp4 inline. The
//   window is padded by 4 seconds on each side (start clamped to
//   >= 0); an active segment with null end_time uses now as the
//   upper bound before padding.
//
//   The BFF forwards the incoming Range header verbatim and passes
//   the upstream status code, Content-Type, Content-Length,
//   Content-Range, Accept-Ranges, and ETag through unchanged.
//   Content-Disposition is rewritten to "inline" — Frigate sends
//   "attachment", which would block in-page <video> playback.
//   Cache-Control: private, max-age=86400.
//
//   The body is streamed, NOT buffered: preview MP4s from Frigate
//   are already iOS-friendly and Range-native, so this path skips
//   the buffer + hev1→hvc1 retag the full event clip pipeline
//   needs. This is intentionally a separate, lighter path from
//   GET /api/events/{id}/clip.mp4 — preview.mp4 is low-res
//   scrub-quality video for the moment-detail card; full-res clips
//   continue to live behind the event clip endpoint.
//
//   Errors:
//     - invalid id format → 404 not_found
//     - upstream review 404 → 404 not_found
//     - upstream review timeout (ctx deadline) → 504 upstream_timeout
//     - any other upstream / transport failure → 502 upstream_error
//   Statuses from Frigate's preview endpoint (e.g. 416 Range Not
//   Satisfiable) are passed through verbatim.

// POST /api/glance/seen
//   body: { event_ids: string[], scope?: string }
//   Marks each id as seen in the requested scope, refreshing the
//   timestamp on idempotent re-marks. `scope` defaults to
//   "household" when absent or empty; the field is accepted on the
//   wire for forward-compat with a future per-user split but v1
//   has no per-user identity and always operates on the household
//   set. Malformed body / missing event_ids / non-array event_ids
//   → 400 bad_request. Empty event_ids array → 204 with no write.
//   Returns 204 No Content on success; 500 internal on persistence
//   failure.

type GlanceSeenRequest = {
  event_ids: string[]
  scope?: string   // defaults to "household"; reserved for a future per-user split
}

// POST /api/glance/seen-all
//   body: { scope?: string } (or empty body — both accepted)
//   Advances the scope's `seen_through` watermark to the server's
//   current time. Subsequent GET /api/glance responses still surface
//   every moment in the window, but moments whose newest event start
//   sits at or before the watermark render with `seen: true` (they
//   are NOT removed from the list). `scope` defaults to "household"
//   when absent or empty. Returns 204 No Content on success; 500
//   internal on persistence failure. The /api cross-site guard
//   covers this mutating route.

type GlanceSeenAllRequest = {
  scope?: string   // defaults to "household"; reserved for a future per-user split
}

// POST /api/glance/heartbeat
//   No body. Per-device away detection: the server consults the
//   `skua_device` cookie (httpOnly + SameSite=Lax + Path=/, MaxAge
//   ≈ 1 year; minted server-side from 16 bytes of crypto/rand on
//   first contact when absent) to identify the calling device, then
//   stamps that device's last activity at "now" in an in-memory
//   store. The response is:
//
//     { "away": boolean }
//
//   where `away` is true iff the device has no prior activity OR
//   its previous heartbeat is older than `AWAY_SESSION_GAP`
//   (default 30m, env-tunable). Clients use `away` to decide
//   whether to auto-surface the glance "while you were away" sheet
//   on visibility/route changes; the unseen badge count keeps
//   coming from GET /api/glance. The store is in-memory only —
//   a server restart reports every device as away on its next
//   beat, which is acceptable for a household glance feature.
//   Response: Cache-Control: no-store. The /api cross-site guard
//   covers this mutating route.

type GlanceHeartbeatResponse = {
  away: boolean
}

// Storage: JSON at $GLANCE_STATE_PATH (default /data/glance.json;
// the file is auto-created on the first non-empty MarkSeen or
// MarkAllSeen). Shape:
//   {
//     "<scope>": {
//       "seen": { "<event_id>": <seen_at_unix_seconds>, ... },
//       "seen_through": <unix_seconds>   // omitted when zero
//     }, ...
//   }
// A missing file means an empty store; a corrupt file, the older
// scope→{id:ts} shape, the pre-Model-B { "last_seen": ... } shape,
// and the pre-rename { "cleared_at": <unix> } shape are all logged
// and start the store empty — best-effort recency state must not
// block startup, and there is no automatic migration off the legacy
// watermark tag. Pruning drops seen entries older than the 30-day
// retention window and only removes a scope when both its seen set
// is empty AND its seen_through watermark is zero — a mark-all-seen
// must survive an empty seen set across restarts.
//
// Note: `seen` is keyed on the review-sourced moment id (m.id),
// which is stable per Frigate review segment for the segment's
// lifetime. A moment marked seen stays seen as long as the segment
// is still in the response window. If Frigate finalises a still-
// active segment and reissues it with a stable id, the seen flag
// continues to apply; if a segment ages out of the configured
// retention window, its id naturally falls out of both the response
// and the pruned seen set.

// === Camera order (v0.13.0) ===
//
// Household-shared display order for the camera list. The order is the
// user's preference layer; the API handler applies it on top of the
// registry from internal/cameras when answering GET /api/cameras, so
// the grid (mobile + desktop) and the Settings → Cameras list both
// follow the saved order. Reordering is performed by the Settings →
// Cameras drag-and-drop list and persisted via PUT /api/camera-order.
//
// Ordering guarantee for GET /api/cameras:
//   - cam_ids present in the saved order appear first, in saved order.
//   - cam_ids not yet present in the saved order are appended in the
//     registry order so newly discovered cameras never vanish from
//     the list (a fresh camera shows up at the end on next refresh).
//   - cam_ids in the saved order that are absent from the registry
//     are skipped — orphan cleanup on POST /api/cameras/refresh runs
//     Forget(cam_id) alongside groups / camera_names / capabilities /
//     stream_overrides.

// Env: CAMERA_ORDER_CONFIG_PATH (default /data/camera_order.yaml).

// GET /api/camera-order
// → { order: string[] }
//   Returns the effective saved order (after server-side dedup and
//   filtering of any ids that are no longer in the registry). Empty
//   array, never null, on a fresh store.

// PUT /api/camera-order  body: { order: string[] }
//   Replaces the saved order with the supplied list. Duplicates are
//   de-duplicated server-side (first occurrence wins); unknown ids
//   (not in the camera registry) are silently dropped — the contract
//   is that the client sends its full desired order and the server
//   validates and normalises. No partial-update mode and no per-id
//   route. Cross-site guarded like other mutating /api routes.
//   Returns: { order: string[] }  // the effective saved order
//
// Storage: YAML list under top-level key `order:`. File auto-created
// on first non-empty write; missing file at startup is fine (empty
// store). Malformed YAML on load → fail-fast. The on-disk file is
// also filtered against the live registry on load so a stale snapshot
// from before a refresh never re-introduces a ghost camera.
//
// Validation errors return 4xx/5xx with the standard { error, message }
// envelope. Error codes are snake_case and stable:
type CameraOrderErrorBody = {
  error: 'invalid_body' | 'internal'
  message: string  // English, ready to display
}

// === Recording timeline (v0.14.0) ===
//
// Shipped in v0.14.0 and extended since; the group covers the whole
// /cam/{id}/timeline screen. Nine routes: the VOD ladder, the recordings
// summary, the three activity/coverage lanes (review, audio-events,
// recordings) and the four preview routes (two list, two static file).
//
// SHARED VALIDATION. Every route in this group validates in the same order,
// entirely before any upstream call, and returns the same three codes:
//   1. {id} against validUpstreamID          → 400 invalid_id
//   2. {id} present in the camera registry   → 404 not_found
//   3. where the route takes start/end as QUERY parameters: both must be
//      INTEGER unix seconds (validUnixSeconds + strconv.ParseInt, so no
//      fractional, signed, or empty value passes) and end > start
//                                            → 400 invalid_range
// The /vod/ route embeds start/end in the PATH instead and enforces only
// digit-only / non-empty there — see its own note. A fractional second is
// the easy mistake to make from the client side: the FE floors both slots in
// timelineMasterURL / timelineVideoOnlyURL precisely because the BFF rejects
// a fractional path.
//
// Thin BFF passthrough for Frigate's recording VOD endpoint plus the
// per-camera recordings summary. Single camera; codec-agnostic; no
// transcode, no retag, no buffering — the BFF only validates camera
// id / time slots / filename, forwards Range verbatim, and streams
// the upstream body. Frigate emits an HLS fMP4 ladder
// (master.m3u8 → index-*.m3u8 → init-*.mp4 + seg-*.m4s) under
// /vod/{cam}/start/{start}/end/{end}/. Every URI inside the playlists
// is relative, so the path-embedded {start}/{end} survive relative
// resolution and clients fetch every child playlist / segment back
// through the same /api/cameras/{id}/vod/{start}/{end}/ prefix
// without any playlist-body rewriting in the BFF.

// GET  /api/cameras/{id}/vod/{start}/{end}/*
// HEAD /api/cameras/{id}/vod/{start}/{end}/*
//   {id}    camera id (must be present in the camera registry).
//   {start},{end}  integer unix seconds — passed verbatim into the
//     upstream URL. The BFF only enforces digit-only / non-empty so
//     the upstream URL stays parseable; Frigate decides whether the
//     range is valid against its recording retention.
//   {*}     the relative playlist or segment filename: master.m3u8,
//     index-*.m3u8, init-*.mp4, or seg-*.m4s. Restricted to
//     [A-Za-z0-9._-] with `.` / `..` and any `/` or `\` explicitly
//     rejected — the catch-all slot is the SSRF / path-traversal
//     surface and is the only validation gate between the client and
//     the upstream URL composition.
//
//   The BFF forwards the incoming Range header verbatim and passes
//   the upstream status code, Content-Type, Content-Length,
//   Content-Range, Accept-Ranges, and ETag through unchanged. HEAD
//   skips the body copy.
//
//   Cache-Control by suffix:
//     - .m3u8                → "no-store" (playlist mutates while
//                              the window includes now)
//     - .mp4 (init) / .m4s   → "public, max-age=31536000, immutable"
//                              (segment bytes are write-once for a
//                              given time range)
//   Content-Disposition is not touched — Frigate's VOD does not send
//   attachment for these.
//
//   Errors:
//     - invalid camera id                → 400 invalid_id
//     - camera not in registry           → 404 not_found
//     - non-numeric start or end         → 400 invalid_range
//     - malformed {*} (traversal, slash,
//       non-allowed bytes)               → 404 not_found
//     - context deadline                 → 504 upstream_timeout
//     - other upstream / transport       → 502 upstream_error
//   Frigate's VOD endpoint may return 416 / 404 / etc. directly;
//   those statuses are passed through verbatim.
//
//   THE 503 A REIMPLEMENTER WILL HIT. Frigate serves /vod/ through
//   nginx-vod, which refuses to splice adjacent recording segments whose
//   AUDIO-TRACK COUNTS DIFFER — which happens whenever a camera's RTSP
//   audio flaps mid-window. The combined video+audio rendition
//   (master.m3u8) then 503s for that window while the video-only
//   rendition (index-v1.m3u8) serves 200 for the very same
//   [start,end]. The BFF passes the 503 through unchanged and does not
//   retry: it cannot know which rendition the client wants. The client
//   owns the fallback — on a 503 it reloads the same range from
//   index-v1.m3u8 and marks the chunk audio-less (see
//   timelineVideoOnlyURL and degradeActiveToVideoOnly on the FE). So a
//   503 from this route is NOT necessarily "no recording": treat it as
//   "not in this rendition" and retry video-only before surfacing an
//   error, or windows that play fine will be reported as missing.

// GET /api/cameras/{id}/recordings-summary[?timezone=]
//   Verbatim pass-through of Frigate's
//   /api/{cam}/recordings/summary. The timezone query is forwarded
//   only when non-empty (Frigate uses it to bucket entries by the
//   caller's local day). The response shape is owned upstream and
//   not yet typed by the BFF — clients consume it as opaque JSON.
//   Content-Type is copied through from upstream (fallback
//   "application/json"); the body is streamed verbatim.
//
//   Errors mirror the VOD path: invalid_id (400), not_found (404),
//   upstream_timeout (504), upstream_error (502).

// GET /api/cameras/{id}/review?start=&end=
//   The windowed list of Frigate review segments (grouped alert / detection
//   activity) overlapping [start,end), reshaped into the lean shape the
//   recording-timeline scrubber renders as a thin activity lane along the top
//   of the bar. The BFF queries Frigate's /api/review with cameras={id},
//   after=start-1800 (a fixed 30-min lookback — Frigate filters review items
//   on start_time, so the lookback catches a segment that started just before
//   the window but overlaps it; a rare, very-long active segment that started
//   more than 30 min before the left edge may still be missed there),
//   before=end, and limit=500, then re-emits an array, one entry per segment:
//
//     [ { "id": <string>, "severity": <string>,
//         "start": <float64>, "end": <float64|null> }, ... ]
//
//   severity is Frigate's value passed through ("alert" | "detection"). start
//   is the segment's wall-clock start (unix seconds, subseconds preserved).
//   end is null while the segment is still active (Frigate's end_time null) —
//   the FE draws an active segment out to the live edge.
//   {id}    camera id (must be present in the camera registry).
//   start,end  integer unix seconds, end > start.
//   Cache-Control: no-store.
//
//   Errors:
//     - invalid camera id                → 400 invalid_id
//     - camera not in registry           → 404 not_found
//     - missing / non-numeric start|end,
//       or end <= start                  → 400 invalid_range
//     - context deadline                 → 504 upstream_timeout
//     - any other upstream failure       → 502 upstream_error

// GET /api/cameras/{id}/audio-events?start=&end=
//   The windowed list of Frigate audio-detection events overlapping
//   [start,end), reshaped into the lean shape the recording-timeline scrubber
//   renders as a thin activity lane directly under the review lane. The source
//   is Frigate's /api/events filtered to data.type=="audio" (the same endpoint
//   the events list uses; the BFF drops the object events). The BFF queries
//   /api/events with cameras={id}, after=start-300 (a fixed 5-min lookback —
//   Frigate filters events on start_time, so the lookback catches an audio
//   event that started just before the window but overlaps it; audio events are
//   short, well under a couple of minutes), before=end, and limit=500, then
//   re-emits an array, one entry per audio event:
//
//     [ { "id": <string>, "label": <string>,
//         "start": <float64>, "end": <float64|null> }, ... ]
//
//   label is the raw Frigate audio class ("speech", "bark", ...). start is the
//   event's wall-clock start (unix seconds, subseconds preserved). end is null
//   while the event is still active (Frigate's end_time null) — the FE draws an
//   active marker out to the live edge.
//   {id}    camera id (must be present in the camera registry).
//   start,end  integer unix seconds, end > start.
//   Cache-Control: no-store.
//
//   Limit caveat: /api/events has no audio-only upstream filter, so limit=500
//   caps the raw mixed object+audio list BEFORE the type filter — in a very
//   busy window object events could crowd out audio, acceptable at household
//   rates and bounded by the time window.
//
//   Errors (same table as /review):
//     - invalid camera id                → 400 invalid_id
//     - camera not in registry           → 404 not_found
//     - missing / non-numeric start|end,
//       or end <= start                  → 400 invalid_range
//     - context deadline                 → 504 upstream_timeout
//     - any other upstream failure       → 502 upstream_error

// GET /api/cameras/{id}/recordings?start=&end=
//   The windowed recording-COVERAGE lane for the recording-timeline scrubber:
//   the spans of recorded footage overlapping [start,end), rendered as a
//   neutral recorded fill with the gaps between spans left as empty track. The
//   BFF reads Frigate's per-segment /api/{cam}/recordings (the raw ~10s
//   recording segments) with after=start-60 (a fixed 60-s lookback — Frigate
//   filters segments on start_time, so the lookback catches a segment that
//   started just before the window's left edge and straddles it) and
//   before=end, then sorts the segments ascending and coalesces contiguous
//   ones — bridging gaps up to COVERAGE_GAP_MERGE (5 s) so sub-second boundary
//   jitter does not fragment continuous recording — into a small set of
//   coverage ranges:
//
//     [ { "start": <float64>, "end": <float64> }, ... ]
//
//   sorted ascending. Recorded ranges are the entries; gaps are the spans
//   BETWEEN them. start/end are wall-clock unix seconds (subseconds preserved).
//   {id}    camera id (must be present in the camera registry).
//   start,end  integer unix seconds, end > start.
//   Cache-Control: no-store.
//
//   Errors (same table as /review):
//     - invalid camera id                → 400 invalid_id
//     - camera not in registry           → 404 not_found
//     - missing / non-numeric start|end,
//       or end <= start                  → 400 invalid_range
//     - context deadline                 → 504 upstream_timeout
//     - any other upstream failure       → 502 upstream_error
//   A non-2xx upstream status is passed through with an empty JSON array (like
//   /preview-clips) so the FE degrades to no coverage.

// GET /api/cameras/{id}/preview-frame-list?start=&end=
//   The chronological list of preview FRAMES Frigate has rendered for
//   [start,end), each rewritten to the BFF /preview-frame proxy. The
//   frame analogue of /preview-clips. The scrubber uses it to seek the open
//   (in-progress) current hour frame-by-frame before that hour's preview
//   mp4 clip has been assembled. The BFF reads Frigate's preview-frames
//   list (/api/preview/{cam}/start/{start}/end/{end}/frames) and re-emits
//   a reduced array, one entry per frame:
//
//     [ { "ts": <float64>, "src": <string> }, ... ]
//
//   ts is the frame's wall-clock timestamp (unix seconds, parsed from the
//   substring after the filename's last '-' so hyphenated camera ids
//   parse). src is REWRITTEN to the BFF route
//   "/api/cameras/{id}/preview-frame/{file}" — the client never reaches
//   Frigate's /api/preview path directly. Entries whose filename fails the
//   traversal whitelist (validPreviewFrame) or whose timestamp does not
//   parse are dropped. Upstream order (chronological) is preserved.
//   {id}    camera id (must be present in the camera registry).
//   start,end  integer unix seconds, end > start.
//   Cache-Control: no-store (the open hour's frame list grows).
//
//   FRAMES VERSUS THE ASSEMBLED MP4, AND THE HOUR BOUNDARY. Frigate keeps
//   only the CURRENT hour as individual webp frames. When that hour ends it
//   assembles the hour's preview mp4, publishes it to /preview-clips — and
//   DELETES the frames. The two sources are therefore never both valid for
//   the same hour, and the crossover is driven by the wall clock, not by
//   anything in a response.
//
//   The consequence for any client: a fetched frame list is identified by
//   TWO things, not one — the span it covers AND the clock hour it was
//   fetched in. Cached on the span alone it outlives its footage the instant
//   the clock crosses the top of the hour, and every later scrub in that span
//   requests webps Frigate has already deleted (404s from
//   /preview-frame/{file}, and a scrub surface that silently stops painting).
//   A client must re-derive "is the tail hour still open?" from now on each
//   scrub and switch to the newly assembled clip when it is not; see
//   tailHourOpen / previewTailKey in frontend/src/lib/timeline.ts. This
//   endpoint is stateless and will happily serve a list for a closed hour —
//   it does not, and cannot, tell the client the frames are about to vanish.
//
//   Errors:
//     - invalid camera id                → 400 invalid_id
//     - camera not in registry           → 404 not_found
//     - missing / non-numeric start|end,
//       or end <= start                  → 400 invalid_range
//     - context deadline                 → 504 upstream_timeout
//     - other upstream / transport, or
//       undecodable upstream body        → 502 upstream_error
//   A non-2xx upstream status is passed through with an empty JSON array
//   so the FE degrades to "no frames".

// GET /api/cameras/{id}/preview-clips?start=&end=
//   The list of real preview CLIPS Frigate has for [start,end) — the same
//   source Frigate's own History timeline scrubs against (distinct from
//   the degenerate single preview.mp4 concat). The BFF reads Frigate's
//   clips list (/api/preview/{cam}/start/{start}/end/{end}) and re-emits a
//   reduced array, one entry per clip:
//
//     [ { "start": <float64>, "end": <float64>, "src": <string> }, ... ]
//
//   src is REWRITTEN to the BFF route
//   "/api/cameras/{id}/preview-clip/{file}" — the client never reaches
//   Frigate's /clips/previews path directly. Entries whose filename fails
//   the traversal whitelist (validPreviewClip) are dropped. Ordering is
//   preserved from upstream.
//   {id}    camera id (must be present in the camera registry).
//   start,end  integer unix seconds, end > start.
//   Cache-Control: no-store (the current hour's clip list grows).
//
//   Errors:
//     - invalid camera id                → 400 invalid_id
//     - camera not in registry           → 404 not_found
//     - missing / non-numeric start|end,
//       or end <= start                  → 400 invalid_range
//     - context deadline                 → 504 upstream_timeout
//     - other upstream / transport, or
//       undecodable upstream body        → 502 upstream_error
//   A non-2xx upstream status is passed through with an empty JSON array
//   so the FE degrades to "no preview".

// GET  /api/cameras/{id}/preview-clip/{file}
// HEAD /api/cameras/{id}/preview-clip/{file}
//   Thin passthrough for an individual static Frigate preview clip file
//   (/clips/previews/{cam}/{file}). {file} is the clip name
//   "{firstTs}-{lastTs}.mp4" (digits, dots, hyphens, .mp4 suffix only),
//   gated by the traversal whitelist — the SSRF surface for the static
//   file route.
//   {id}    camera id (must be present in the camera registry).
//
//   Range is forwarded verbatim; the upstream status code, Content-Type,
//   Content-Length, Content-Range, Accept-Ranges, and ETag pass through;
//   HEAD skips the body. Content-Disposition is dropped (Frigate sends
//   attachment; dropping it keeps the clip playing inline in a
//   <video src>). Cache-Control: no-store (the current hour's clip is
//   still being written).
//
//   Errors:
//     - invalid camera id                → 400 invalid_id
//     - camera not in registry           → 404 not_found
//     - malformed {file} (traversal,
//       slash, bad bytes, wrong suffix)  → 404 not_found
//     - context deadline                 → 504 upstream_timeout
//     - other upstream / transport       → 502 upstream_error
//   Frigate's static endpoint may return 416 / 404 / etc. directly; those
//   statuses are passed through verbatim.

// GET  /api/cameras/{id}/preview-frame/{file}
// HEAD /api/cameras/{id}/preview-frame/{file}
//   Thin passthrough for a single static Frigate preview FRAME image
//   (/api/preview/{file}/thumbnail.webp → image/webp). Used to scrub the
//   open (in-progress) current hour by webp frame before that hour's mp4
//   preview clip has been assembled. {file} is the frame name
//   "preview_{cam}-{unix.frac}.webp" (must start "preview_", end ".webp",
//   bytes in [A-Za-z0-9._-] only), gated by the traversal whitelist
//   (validPreviewFrame) — the SSRF surface for the static frame route. The
//   camera is NOT a separate upstream path segment; the filename already
//   encodes it, so {id} is used only for auth / registry consistency with
//   the other camera-scoped routes.
//   {id}    camera id (must be present in the camera registry).
//
//   Range is forwarded verbatim; the upstream status code, Content-Type,
//   Content-Length, Content-Range, Accept-Ranges, and ETag pass through;
//   HEAD skips the body. Content-Disposition is dropped (renders inline).
//   Cache-Control: public, max-age=31536000, immutable — a given frame
//   file is content-addressed by its timestamp and never changes (UNLIKE
//   the no-store clip / list routes).
//
//   Errors:
//     - invalid camera id                → 400 invalid_id
//     - camera not in registry           → 404 not_found
//     - malformed {file} (traversal,
//       slash, bad bytes, wrong
//       prefix/suffix)                   → 404 not_found
//     - context deadline                 → 504 upstream_timeout
//     - other upstream / transport       → 502 upstream_error
//   Frigate's static endpoint may return 416 / 404 / etc. directly; those
//   statuses are passed through verbatim.
```

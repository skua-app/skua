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
// === E1/E2 (stable) ===

// GET /api/cameras
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
  name_style: 'below' | 'overlay'
  show_timestamp: boolean   // grid tiles AND focus video overlay (E3)
  desktop_columns: 2 | 3 | 4 | 5
  mobile_columns: 1 | 2
  grid_filter: string | null  // last-selected group id, null = "Все" (E3.3)
}
// Stored at /data/prefs.json. Atomic write (tmp + rename).
// Defaults: eco / true / main / false / cyan / below / false / 4 / 1 / null

// === E3 (stable) ===

// GET /api/config → AppConfig
// Sourced from FRIGATE_UI_URL env (defaults to FRIGATE_URL when unset).
// Public payload; no secrets here, ever.
type AppConfig = {
  frigate_ui_url: string   // base URL for "Open in Frigate" deep-links
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
```

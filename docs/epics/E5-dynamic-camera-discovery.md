# Epic E5 — Dynamic Camera Discovery

**Status: DONE** — tagged v0.6.0

The household app no longer carries a hardcoded camera list. Adding or
removing a camera in Frigate now propagates to the BFF (and every
connected client) without a code change or container restart. This file
captures the shape of the system for a future maintainer; CLAUDE.md
holds the day-to-day rules.

---

## Goal

Replace the static `config.Cameras` slice with a registry that is
sourced from Frigate at startup, refreshed on demand, persisted across
restarts, and capable of broadcasting roster changes to every open
client over the existing SSE channel.

## Scope

In scope:

- Startup pull of cameras from Frigate `/api/config`, with a
  fail-fast-or-fall-back policy depending on whether a local snapshot
  exists.
- Per-camera capability flags (`talk_back`, `ptz`) carried in a
  separate YAML override file — Frigate does not expose them.
- Manual refresh endpoint, settings-page button, and SSE diff events
  for cross-client sync.
- Orphan cleanup of `groups.yaml` / `camera_names.yaml` /
  `capabilities.yaml` when a camera disappears from Frigate.

Out of scope (kept for a future patch):

- Editor UI for `capabilities.yaml`. Today the file is hand-edited on
  the host; the household has no active talk-back consumer (see
  `docs/hikvision-no-web-sku.md`).
- Auto-refresh on a timer. Roster changes are user-driven (rare); the
  manual button + SSE fan-out is enough to keep clients consistent.
- Focus-screen recovery when the currently-viewed camera is removed.
  The existing WHEP error UI already surfaces the lost stream; the user
  navigates away manually.

## Architecture

**`backend/internal/cameras.Store`** is the single source of truth for
the runtime roster. `New(path, frigateClient)` is called once at
startup: it reads `cameras.yaml` if present (malformed YAML → fail
fast), then calls Frigate `/api/config` with a 10s timeout. On success
the response is derived into a sorted `[]CameraSpec`, persisted, and
swapped under a write lock. On failure with no on-disk snapshot the
process exits — surfacing the upstream outage explicitly beats serving
an empty UI in a homelab. On failure with a snapshot, the BFF logs a
warning and proceeds against the cached snapshot. `Snapshot()` returns
defensive copies; `Find(id)` is a convenience for the API layer.

**`backend/internal/capabilities.Store`** is a passive override map of
`{cam_id: {talk_back, ptz}}` persisted in `capabilities.yaml`. There is
no `Set`: the file is hand-edited until an editor UI lands. `Get`
returns the zero value for cameras with no entry; `Forget` is invoked
from the refresh cleanup path and is a no-op disk-write when the entry
does not exist (so refreshes over many never-overridden cams stay
quiet).

**Refresh flow.** `POST /api/cameras/refresh` calls
`Store.Refresh(ctx)`, which re-pulls Frigate, derives the new spec
list, diffs it against the current snapshot, persists, and swaps. Two
hooks then fire on the refresh goroutine: `OnAdded(specs)` broadcasts
SSE `camera.added` (with the merged display name) for each new
camera; `OnRemoved(ids)` broadcasts `camera.removed` *first*, then
calls `Forget(id)` on each dependent store (groups → names →
capabilities). Broadcasting before mutation lets connected clients
refetch with the camera still nominally present in their cached state
— the dependent-store mutation arrives by the time the refetch
resolves. Forget errors are logged but never propagated to the API
caller; the refresh has already succeeded by the time cleanup runs, and
the next refresh will retry the cleanup as part of its own diff.

**Hooks are dependency-injected**, not direct imports.
`cameras.Store` knows nothing about `groups.Store`, `names.Store`, or
`capabilities.Store` — `main.go` wires the callbacks after every store
is constructed. This preserves the existing `CameraExistsFunc`
layering (groups and names already accept it as an injected function).

**SSE event surface** is unchanged at the transport level. The hub in
`backend/internal/sse` already fans `Broadcast(name, payload)` out to
every subscriber; the refresh handler simply added two more event
names. On the frontend, `lib/stores/events-stream.svelte.ts`
subscribes to `camera.added` and `camera.removed` and triggers a
parallel refetch of `camerasStore`, `groupsStore`, and
`fetchCameraNames()`. The payloads are not inspected — refetching the
full set is cheap and idempotent, and it cannot desync if a single
event is missed.

**Frontend refresh path.** `/settings` carries a "Камеры" section with
a single "Обновить из Frigate" button. The handler POSTs the refresh
endpoint, formats the `{added, removed}` diff inline ("Добавлено: x,
y. Удалено: z." with empty halves dropped), and clears the status
after 5s. The user's own refresh also explicitly refetches
`camerasStore` and `groupsStore` for immediate UI consistency — the
SSE event arrives too, but the refetches are idempotent and we want
the user-visible state to settle before the round-trip completes.

## Sprints

- **Sprint A** (commit `2d21298`) — backend plumbing. `cameras.Store`
  with startup load + fallback. `config.Cameras` removed,
  `config.CameraSpec` aliased to `cameras.CameraSpec`. No API surface
  change.
- **Sprint B** (commit `12fa902`) — `capabilities.Store`, `Refresh`
  method with `OnAdded` / `OnRemoved` hooks, `POST
  /api/cameras/refresh`, `Forget(camID)` on `groups.Store` and
  `names.Store`, SSE `camera.added` / `camera.removed` events.
  `TalkBack` / `PTZ` dropped from `CameraSpec` — the handler now
  merges from `capabilities.Store`. No UI change.
- **Sprint C** (this commit) — `/settings` "Камеры" section, the
  `refreshCameras` API client, SSE-driven cross-client refetch in
  `events-stream`. Tag v0.6.0.

## Deferred

- **Capabilities editor UI.** Will land as a `v0.6.x` patch when a
  talk-back-capable camera reappears in the inventory. The store and
  API are intentionally read-only today so the deferral doesn't carry
  a write-path to maintain.
- **Focus-screen removed-camera handling.** Existing WHEP error UI is
  enough until someone tries to live-watch a camera at the exact
  moment another household member deletes it from Frigate.

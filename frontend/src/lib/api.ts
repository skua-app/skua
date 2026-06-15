export type Camera = {
  id: string
  name: string
  online: boolean
  snapshot_url: string
  capabilities: {
    talk_back: boolean
    ptz: boolean
  }
  streams: {
    main: string
    sub?: string
  }
  groups: string[]
}

export type Accent = 'cyan' | 'sage' | 'amber' | 'violet'
export type NameStyle = 'below' | 'overlay'
export type DesktopColumns = 2 | 3 | 4 | 5
export type MobileColumns = 1 | 2
export type GlanceWindowHours = 6 | 12 | 24 | 48 | 72
export type GlanceMaxMoments = 10 | 20 | 30 | 50

export type Prefs = {
  grid_mode: 'hd' | 'eco'
  muted_by_default: boolean
  stream_quality: 'main' | 'sub'
  show_telemetry: boolean
  accent: Accent
  name_style: NameStyle
  show_timestamp: boolean
  desktop_columns: DesktopColumns
  mobile_columns: MobileColumns
  grid_filter: string | null
  glance_window_hours: GlanceWindowHours
  glance_max_moments: GlanceMaxMoments
}

export const ACCENT_VALUES: Record<Accent, string> = {
  cyan: 'oklch(0.78 0.10 200)',
  sage: 'oklch(0.78 0.07 150)',
  amber: 'oklch(0.78 0.10 75)',
  violet: 'oklch(0.75 0.10 290)'
}

// EventKind mirrors backend/internal/events.Kind exactly. 'motion' is not
// a valid kind — Frigate's `motion` events collapse into `other` on the BFF.
export type EventKind = 'person' | 'vehicle' | 'animal' | 'other'

// EventItem mirrors backend/internal/events.Item. started_at is RFC3339 UTC.
// ended_at and duration_seconds are null while an event is in progress.
export type EventItem = {
  id: string
  cam_id: string
  started_at: string
  ended_at: string | null
  duration_seconds: number | null
  label: string
  kind: EventKind
  score: number | null
  has_snapshot: boolean
  has_clip: boolean
}

export type EventsResponse = {
  items: EventItem[]
  next_before: string | null
}

export type AppConfig = {
  frigate_ui_url: string
  version: string
}

export type EventsQuery = {
  cameras?: string[]
  labels?: string[]
  before?: string // ISO 8601; BFF translates to unix-seconds for Frigate
  limit?: number
}

// Typed error surface for the read path. Lets the connection overlay pick
// a user-facing message and lets callers retry intelligently rather than
// guessing from a string.
export type ApiErrorKind = 'offline' | 'timeout' | 'server' | 'unknown'
export class ApiError extends Error {
  kind: ApiErrorKind
  status?: number
  constructor(kind: ApiErrorKind, message: string, status?: number) {
    super(message)
    this.name = 'ApiError'
    this.kind = kind
    this.status = status
  }
}

const API_TIMEOUT_MS = 10_000

async function apiFetch<T>(path: string): Promise<T> {
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), API_TIMEOUT_MS)
  let res: Response
  try {
    res = await fetch(path, { signal: ctrl.signal })
  } catch (e) {
    // AbortController firing surfaces as DOMException 'AbortError'. Any
    // network/DNS/connection failure surfaces as a TypeError in fetch.
    if (e instanceof DOMException && e.name === 'AbortError') {
      throw new ApiError('timeout', `${path}: request timed out`)
    }
    if (e instanceof TypeError) {
      throw new ApiError('offline', `${path}: network unreachable`)
    }
    throw new ApiError('unknown', `${path}: ${e instanceof Error ? e.message : String(e)}`)
  } finally {
    clearTimeout(timer)
  }
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`
    try {
      const body = (await res.json()) as { error?: string; message?: string }
      if (body && typeof body.message === 'string' && body.message) {
        message = body.message
      }
    } catch {
      // non-JSON or empty body: keep the status-text fallback
    }
    throw new ApiError('server', `${path}: ${message}`, res.status)
  }
  return res.json() as Promise<T>
}

export async function fetchCameras(): Promise<Camera[]> {
  return apiFetch<Camera[]>('/api/cameras')
}

export async function fetchPrefs(): Promise<Prefs> {
  return apiFetch<Prefs>('/api/prefs')
}

export async function updatePrefs(partial: Partial<Prefs>): Promise<Prefs> {
  const res = await fetch('/api/prefs', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(partial)
  })
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`
    try {
      const body = (await res.json()) as { error?: string; message?: string }
      if (body && typeof body.message === 'string' && body.message) {
        message = body.message
      }
    } catch {
      // non-JSON or empty body: keep the status-text fallback
    }
    throw new Error(`/api/prefs: ${message}`)
  }
  return res.json() as Promise<Prefs>
}

export async function fetchConfig(): Promise<AppConfig> {
  return apiFetch<AppConfig>('/api/config')
}

export async function fetchEvents(q: EventsQuery = {}): Promise<EventsResponse> {
  const params = new URLSearchParams()
  if (q.cameras) for (const c of q.cameras) params.append('camera', c)
  if (q.labels) for (const l of q.labels) params.append('label', l)
  if (q.before) params.set('before', q.before)
  if (q.limit !== undefined) params.set('limit', String(q.limit))
  const qs = params.toString()
  return apiFetch<EventsResponse>(`/api/events${qs ? `?${qs}` : ''}`)
}

export function eventThumbnailURL(id: string): string {
  return `/api/events/${encodeURIComponent(id)}/thumbnail.jpg`
}

export function eventSnapshotURL(id: string): string {
  return `/api/events/${encodeURIComponent(id)}/snapshot.jpg`
}

export function eventClipURL(id: string, download = false): string {
  const base = `/api/events/${encodeURIComponent(id)}/clip.mp4`
  return download ? `${base}?download=1` : base
}

// momentPreviewURL returns the BFF route that resolves a moment id
// (Frigate review id) to its preview.mp4 — a low-res scrub-quality
// clip covering the whole review window, served inline with Range
// passthrough. Distinct from eventClipURL: the moment preview is one
// clip per review segment; the event clip is one clip per tracked
// detection.
export function momentPreviewURL(id: string): string {
  return `/api/glance/${encodeURIComponent(id)}/preview.mp4`
}

// Moment mirrors backend/internal/events.Moment: one Frigate review
// segment, surfaced as the glance "while you were away" unit. id is
// the Frigate review id (stable, suitable as a seen-state key).
// started_at is RFC3339 UTC; ended_at is null while the segment is
// still active. detection_ids lists the tracked-object event ids
// inside the segment, and thumb_event_id is the detection whose
// snapshot the UI should use as the moment thumbnail (empty when the
// segment has no detections yet).
export type Moment = {
  id: string
  cam_id: string
  started_at: string
  ended_at: string | null
  severity: 'alert' | 'detection'
  kinds: EventKind[]
  labels: string[]
  zones: string[]
  detection_ids: string[]
  thumb_event_id: string
}

// GlanceMoment is a Moment plus a per-moment seen flag, keyed against
// the household seen-set on the BFF. A moment flips to seen as soon as
// its id appears in the set.
export type GlanceMoment = Moment & { seen: boolean }

// GlanceResponse is the envelope of GET /api/glance: the count of
// unseen moments (surfaced to the badge) and the moment list itself
// (newest moment first), each carrying its own seen flag.
export type GlanceResponse = {
  unseen_count: number
  moments: GlanceMoment[]
}

export async function fetchGlance(hours?: number, max?: number): Promise<GlanceResponse> {
  const params = new URLSearchParams()
  if (hours !== undefined) params.set('hours', String(hours))
  if (max !== undefined) params.set('max', String(max))
  const qs = params.toString()
  return apiFetch<GlanceResponse>(`/api/glance${qs ? `?${qs}` : ''}`)
}

// markAllGlanceSeen advances the household watermark non-destructively:
// every currently-listed moment flips to seen=true on the next GET, but
// nothing is dropped. scope is optional and defaults to "household" on
// the server; with no scope the body is omitted entirely. Returns 204
// with no body — callers should not parse a payload.
export async function markAllGlanceSeen(scope?: string): Promise<void> {
  const init: RequestInit = { method: 'POST' }
  if (scope !== undefined) {
    init.headers = { 'Content-Type': 'application/json' }
    init.body = JSON.stringify({ scope })
  }
  const res = await fetch('/api/glance/seen-all', init)
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`
    try {
      const errBody = (await res.json()) as { error?: string; message?: string }
      if (errBody && typeof errBody.message === 'string' && errBody.message) {
        message = errBody.message
      }
    } catch {
      // non-JSON or empty body: keep the status-text fallback
    }
    throw new Error(`/api/glance/seen-all: ${message}`)
  }
}

// glanceHeartbeat pings the per-device session so the server can tell
// active devices from away ones. The BFF mints an httpOnly skua_device
// cookie on first call; same-origin fetch carries it automatically.
// Returns the server's away verdict for this device.
export async function glanceHeartbeat(): Promise<boolean> {
  const res = await fetch('/api/glance/heartbeat', { method: 'POST' })
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`
    try {
      const errBody = (await res.json()) as { error?: string; message?: string }
      if (errBody && typeof errBody.message === 'string' && errBody.message) {
        message = errBody.message
      }
    } catch {
      // non-JSON or empty body: keep the status-text fallback
    }
    throw new Error(`/api/glance/heartbeat: ${message}`)
  }
  const body = (await res.json()) as { away: boolean }
  return body.away
}

// markGlanceSeen records that the listed event ids have been viewed.
// scope is optional and defaults to "household" on the server; v1 has
// no per-user identity so the frontend omits it. The endpoint returns
// 204 with no body — callers should not parse a payload.
export async function markGlanceSeen(eventIds: string[], scope?: string): Promise<void> {
  const body: { event_ids: string[]; scope?: string } = { event_ids: eventIds }
  if (scope !== undefined) body.scope = scope
  const res = await fetch('/api/glance/seen', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body)
  })
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`
    try {
      const errBody = (await res.json()) as { error?: string; message?: string }
      if (errBody && typeof errBody.message === 'string' && errBody.message) {
        message = errBody.message
      }
    } catch {
      // non-JSON or empty body: keep the status-text fallback
    }
    throw new Error(`/api/glance/seen: ${message}`)
  }
}

// Camera groups (E3.3). Server-side single-membership: when a camera is added
// to a group, it is removed from any other group atomically.
export type Group = {
  id: string
  name: string
  camera_ids: string[]
}

export type GroupErrorCode =
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

export type GroupErrorBody = {
  error: GroupErrorCode
  message: string
}

// GroupApiError carries the parsed server error so callers can switch on
// `.code` for inline validation messages.
export class GroupApiError extends Error {
  code: GroupErrorCode
  status: number
  constructor(body: GroupErrorBody, status: number) {
    super(body.message)
    this.code = body.error
    this.status = status
  }
}

async function parseGroupError(res: Response): Promise<GroupApiError> {
  let body: GroupErrorBody
  try {
    body = (await res.json()) as GroupErrorBody
  } catch {
    body = { error: 'internal', message: `${res.status} ${res.statusText}` }
  }
  return new GroupApiError(body, res.status)
}

export async function fetchGroups(): Promise<Group[]> {
  return apiFetch<Group[]>('/api/groups')
}

export async function createGroup(name: string): Promise<Group> {
  const res = await fetch('/api/groups', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name })
  })
  if (!res.ok) throw await parseGroupError(res)
  return res.json() as Promise<Group>
}

export async function updateGroup(
  id: string,
  patch: { name?: string; camera_ids?: string[] }
): Promise<Group> {
  const res = await fetch(`/api/groups/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch)
  })
  if (!res.ok) throw await parseGroupError(res)
  return res.json() as Promise<Group>
}

export async function deleteGroup(id: string): Promise<void> {
  const res = await fetch(`/api/groups/${encodeURIComponent(id)}`, { method: 'DELETE' })
  if (!res.ok) throw await parseGroupError(res)
}

// Per-camera friendly-name overrides. The store keeps only cameras that
// have a user-supplied override; cameras with no entry use the config
// default exposed via /api/cameras. Setting an empty name clears the
// override and the camera reverts to its config default.
export type CameraNamesMap = { [camId: string]: string }
export type CameraNameUpdate = {
  cam_id: string
  name: string // merged result (override if set, else config default)
}

export type CameraNameErrorCode =
  | 'camera_not_found'
  | 'name_too_long'
  | 'invalid_body'
  | 'missing_id'
  | 'internal'

export type CameraNameErrorBody = {
  error: CameraNameErrorCode
  message: string
}

export class CameraNameApiError extends Error {
  code: CameraNameErrorCode
  status: number
  constructor(body: CameraNameErrorBody, status: number) {
    super(body.message)
    this.code = body.error
    this.status = status
  }
}

async function parseCameraNameError(res: Response): Promise<CameraNameApiError> {
  let body: CameraNameErrorBody
  try {
    body = (await res.json()) as CameraNameErrorBody
  } catch {
    body = { error: 'internal', message: `${res.status} ${res.statusText}` }
  }
  return new CameraNameApiError(body, res.status)
}

export async function fetchCameraNames(): Promise<CameraNamesMap> {
  const res = await apiFetch<{ names: CameraNamesMap }>('/api/camera-names')
  return res.names ?? {}
}

export async function setCameraName(camId: string, name: string): Promise<CameraNameUpdate> {
  const res = await fetch(`/api/camera-names/${encodeURIComponent(camId)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ name })
  })
  if (!res.ok) throw await parseCameraNameError(res)
  return res.json() as Promise<CameraNameUpdate>
}

// Dynamic camera discovery (E5). POST /api/cameras/refresh re-pulls Frigate
// /api/config, persists the new snapshot, and broadcasts SSE camera.added /
// camera.removed for the diff. The response carries the sorted diff; both
// arrays are always present (possibly empty), never null.
export type RefreshDiff = {
  added: string[]
  removed: string[]
}

export type RefreshErrorCode = 'frigate_unreachable' | 'internal'

export type RefreshErrorBody = {
  error: RefreshErrorCode
  message: string
}

export class RefreshApiError extends Error {
  code: RefreshErrorCode
  status: number
  constructor(body: RefreshErrorBody, status: number) {
    super(body.message)
    this.code = body.error
    this.status = status
  }
}

async function parseRefreshError(res: Response): Promise<RefreshApiError> {
  let body: RefreshErrorBody
  try {
    body = (await res.json()) as RefreshErrorBody
  } catch {
    body = { error: 'internal', message: `${res.status} ${res.statusText}` }
  }
  return new RefreshApiError(body, res.status)
}

export async function refreshCameras(): Promise<RefreshDiff> {
  const res = await fetch('/api/cameras/refresh', { method: 'POST' })
  if (!res.ok) throw await parseRefreshError(res)
  return res.json() as Promise<RefreshDiff>
}

// Household-shared camera display order (v0.13.0). PUT replaces the saved
// order with the supplied list; the server de-duplicates and silently
// drops unknown ids. The response carries the effective saved order so
// the caller doesn't need to re-fetch /api/camera-order to read it.
export type CameraOrderResponse = {
  order: string[]
}

export async function fetchCameraOrder(): Promise<string[]> {
  const res = await apiFetch<CameraOrderResponse>('/api/camera-order')
  return res.order ?? []
}

export async function setCameraOrder(order: string[]): Promise<string[]> {
  const res = await fetch('/api/camera-order', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ order })
  })
  if (!res.ok) {
    let message = `${res.status} ${res.statusText}`
    try {
      const body = (await res.json()) as { message?: string }
      if (body.message) message = body.message
    } catch {
      // Ignore: keep the status fallback above.
    }
    throw new Error(message)
  }
  const body = (await res.json()) as CameraOrderResponse
  return body.order ?? []
}

// Per-camera go2rtc stream overrides (E6). Applied server-side inside the
// WHEP handler only — GET /api/cameras still surfaces Frigate-truth stream
// names. An entry is present in the map only when at least one of main/sub
// is non-empty; setting both empty clears the entry.
export type Override = {
  main: string
  sub: string
}

export type StreamOverridesMap = { [camId: string]: Override }

export type StreamOverrideErrorCode =
  | 'invalid_body'
  | 'camera_not_found'
  | 'stream_not_found'
  | 'go2rtc_unreachable'
  | 'missing_id'
  | 'internal'

export type StreamOverrideErrorBody = {
  error: StreamOverrideErrorCode
  message: string
}

export class StreamOverrideApiError extends Error {
  code: StreamOverrideErrorCode
  status: number
  constructor(body: StreamOverrideErrorBody, status: number) {
    super(body.message)
    this.code = body.error
    this.status = status
  }
}

async function parseStreamOverrideError(res: Response): Promise<StreamOverrideApiError> {
  let body: StreamOverrideErrorBody
  try {
    body = (await res.json()) as StreamOverrideErrorBody
  } catch {
    body = { error: 'internal', message: `${res.status} ${res.statusText}` }
  }
  return new StreamOverrideApiError(body, res.status)
}

// fetchGo2RTCStreams returns the sorted list of go2rtc stream aliases.
// The BFF passes through go2rtc's /api/streams keys directly; the response
// body IS the array, not wrapped in an envelope.
export async function fetchGo2RTCStreams(): Promise<string[]> {
  const res = await fetch('/api/go2rtc/streams')
  if (!res.ok) throw await parseStreamOverrideError(res)
  return res.json() as Promise<string[]>
}

export async function fetchStreamOverrides(): Promise<StreamOverridesMap> {
  const res = await fetch('/api/stream-overrides')
  if (!res.ok) throw await parseStreamOverrideError(res)
  const body = (await res.json()) as { overrides: StreamOverridesMap | null }
  return body.overrides ?? {}
}

// setStreamOverride PUTs the override pair for camId. Both fields are
// always sent — the BFF requires them. Sending both empty clears the entry
// (the response carries the saved Override, which is {main:"", sub:""} on
// the clear path).
export async function setStreamOverride(
  camId: string,
  main: string,
  sub: string
): Promise<Override> {
  const res = await fetch(`/api/stream-overrides/${encodeURIComponent(camId)}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ main, sub })
  })
  if (!res.ok) throw await parseStreamOverrideError(res)
  return res.json() as Promise<Override>
}

// Runtime config editor (E7.1). The /settings → Connection section reads
// effective + overlay + locked from GET, writes via PUT, probes via /test,
// and triggers a process-level restart via /restart so the container's
// restart policy boots the new overlay file.
export type RuntimeConfigURLs = {
  frigate_url: string
  frigate_ui_url: string
  go2rtc_url: string
}

export type RuntimeConfigLocked = {
  frigate_url: boolean
  frigate_ui_url: boolean
  go2rtc_url: boolean
}

export type RuntimeConfig = {
  effective: RuntimeConfigURLs
  overlay: RuntimeConfigURLs
  locked: RuntimeConfigLocked
}

export type ProbeResult = {
  ok: boolean
  skipped?: boolean
  error?: string
}

export type ProbeReport = {
  frigate: ProbeResult
  go2rtc: ProbeResult
}

export type RuntimeConfigErrorCode =
  | 'invalid_body'
  | 'frigate_url_required'
  | 'frigate_url_invalid'
  | 'go2rtc_url_invalid'
  | 'frigate_ui_url_invalid'
  | 'data_not_writable'
  | 'internal'

export type RuntimeConfigErrorBody = {
  error: RuntimeConfigErrorCode
  message: string
}

export class RuntimeConfigApiError extends Error {
  code: RuntimeConfigErrorCode
  status: number
  constructor(body: RuntimeConfigErrorBody, status: number) {
    super(body.message)
    this.code = body.error
    this.status = status
  }
}

async function parseRuntimeConfigError(res: Response): Promise<RuntimeConfigApiError> {
  let body: RuntimeConfigErrorBody
  try {
    body = (await res.json()) as RuntimeConfigErrorBody
  } catch {
    body = { error: 'internal', message: `${res.status} ${res.statusText}` }
  }
  return new RuntimeConfigApiError(body, res.status)
}

export async function fetchRuntimeConfig(): Promise<RuntimeConfig> {
  const res = await fetch('/api/runtime-config')
  if (!res.ok) throw await parseRuntimeConfigError(res)
  return res.json() as Promise<RuntimeConfig>
}

export async function testRuntimeConfig(
  frigate_url: string,
  go2rtc_url: string
): Promise<ProbeReport> {
  const res = await fetch('/api/runtime-config/test', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ frigate_url, go2rtc_url })
  })
  if (!res.ok) throw await parseRuntimeConfigError(res)
  return res.json() as Promise<ProbeReport>
}

export async function saveRuntimeConfig(values: RuntimeConfigURLs): Promise<RuntimeConfig> {
  const res = await fetch('/api/runtime-config', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(values)
  })
  if (!res.ok) throw await parseRuntimeConfigError(res)
  return res.json() as Promise<RuntimeConfig>
}

export async function restartRuntimeConfig(): Promise<void> {
  const res = await fetch('/api/runtime-config/restart', { method: 'POST' })
  if (!res.ok) throw await parseRuntimeConfigError(res)
}

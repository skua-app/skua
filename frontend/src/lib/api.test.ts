import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  CameraNameApiError,
  GroupApiError,
  RefreshApiError,
  RuntimeConfigApiError,
  StreamOverrideApiError,
  createGroup,
  fetchCameras,
  fetchGlance,
  fetchPrefs,
  fetchRecordingsSummary,
  fetchStreamOverrides,
  markGlanceSeen,
  refreshCameras,
  restartRuntimeConfig,
  saveRuntimeConfig,
  setCameraName,
  setStreamOverride,
  timelineMasterURL,
  updateGroup,
  updatePrefs,
  type Camera,
  type GlanceResponse,
  type Prefs,
  type RecordingsSummary
} from './api'

function mockFetchOnce(response: Response): void {
  const fn = vi.fn<typeof fetch>().mockResolvedValueOnce(response)
  vi.stubGlobal('fetch', fn)
}

function jsonResponse(body: unknown, init: ResponseInit = {}): Response {
  return new Response(JSON.stringify(body), {
    ...init,
    headers: { 'Content-Type': 'application/json', ...(init.headers ?? {}) }
  })
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('success paths', () => {
  it('fetchCameras returns the parsed array', async () => {
    const cameras: Camera[] = [
      {
        id: 'cam1',
        name: 'Front',
        online: true,
        snapshot_url: '/api/cameras/cam1/snapshot.jpg',
        capabilities: { talk_back: false, ptz: false },
        streams: { main: 'cam1_main' },
        groups: []
      }
    ]
    mockFetchOnce(jsonResponse(cameras))
    await expect(fetchCameras()).resolves.toEqual(cameras)
  })

  it('updatePrefs returns the parsed Prefs', async () => {
    const prefs: Prefs = {
      grid_mode: 'eco',
      muted_by_default: true,
      stream_quality: 'sub',
      show_telemetry: false,
      accent: 'cyan',
      name_style: 'below',
      show_timestamp: true,
      desktop_columns: 3,
      mobile_columns: 2,
      grid_filter: null,
      glance_window_hours: 24,
      glance_max_moments: 20,
      grid_fps: 1
    }
    mockFetchOnce(jsonResponse(prefs))
    await expect(updatePrefs({ grid_mode: 'eco' })).resolves.toEqual(prefs)
  })
})

describe('GroupApiError', () => {
  it('createGroup throws GroupApiError on a structured error body', async () => {
    mockFetchOnce(
      jsonResponse(
        { error: 'name_duplicate', message: 'A group with that name already exists' },
        { status: 409, statusText: 'Conflict' }
      )
    )
    const thrown = await createGroup('Front').catch((e: unknown) => e)
    expect(thrown).toBeInstanceOf(GroupApiError)
    const err = thrown as GroupApiError
    expect(err.code).toBe('name_duplicate')
    expect(err.status).toBe(409)
    expect(err.message).toBe('A group with that name already exists')
  })

  it('updateGroup falls back to code internal with the status-text message on a non-JSON body', async () => {
    mockFetchOnce(new Response('not json', { status: 502, statusText: 'Bad Gateway' }))
    const thrown = await updateGroup('g1', { name: 'x' }).catch((e: unknown) => e)
    expect(thrown).toBeInstanceOf(GroupApiError)
    const err = thrown as GroupApiError
    expect(err.code).toBe('internal')
    expect(err.status).toBe(502)
    expect(err.message).toBe('502 Bad Gateway')
  })
})

describe('CameraNameApiError', () => {
  it('setCameraName throws CameraNameApiError on a structured error body', async () => {
    mockFetchOnce(
      jsonResponse(
        { error: 'name_too_long', message: 'Name too long' },
        { status: 400, statusText: 'Bad Request' }
      )
    )
    const thrown = await setCameraName('cam1', 'x'.repeat(200)).catch((e: unknown) => e)
    expect(thrown).toBeInstanceOf(CameraNameApiError)
    const err = thrown as CameraNameApiError
    expect(err.code).toBe('name_too_long')
    expect(err.status).toBe(400)
    expect(err.message).toBe('Name too long')
  })

  it('setCameraName falls back to code internal with the status-text message on an empty body', async () => {
    mockFetchOnce(new Response('', { status: 500, statusText: 'Internal Server Error' }))
    const thrown = await setCameraName('cam1', 'whatever').catch((e: unknown) => e)
    expect(thrown).toBeInstanceOf(CameraNameApiError)
    const err = thrown as CameraNameApiError
    expect(err.code).toBe('internal')
    expect(err.status).toBe(500)
    expect(err.message).toBe('500 Internal Server Error')
  })
})

describe('RefreshApiError', () => {
  it('refreshCameras throws RefreshApiError on a structured error body', async () => {
    mockFetchOnce(
      jsonResponse(
        { error: 'frigate_unreachable', message: 'Frigate is unreachable' },
        { status: 502, statusText: 'Bad Gateway' }
      )
    )
    const thrown = await refreshCameras().catch((e: unknown) => e)
    expect(thrown).toBeInstanceOf(RefreshApiError)
    const err = thrown as RefreshApiError
    expect(err.code).toBe('frigate_unreachable')
    expect(err.status).toBe(502)
    expect(err.message).toBe('Frigate is unreachable')
  })
})

describe('StreamOverrideApiError', () => {
  it('setStreamOverride throws StreamOverrideApiError on a structured error body', async () => {
    mockFetchOnce(
      jsonResponse(
        { error: 'stream_not_found', message: 'go2rtc has no such alias' },
        { status: 400, statusText: 'Bad Request' }
      )
    )
    const thrown = await setStreamOverride('cam1', 'cam1_main', '').catch((e: unknown) => e)
    expect(thrown).toBeInstanceOf(StreamOverrideApiError)
    const err = thrown as StreamOverrideApiError
    expect(err.code).toBe('stream_not_found')
    expect(err.status).toBe(400)
    expect(err.message).toBe('go2rtc has no such alias')
  })

  it('fetchStreamOverrides falls back to code internal on a non-JSON body', async () => {
    mockFetchOnce(new Response('boom', { status: 502, statusText: 'Bad Gateway' }))
    const thrown = await fetchStreamOverrides().catch((e: unknown) => e)
    expect(thrown).toBeInstanceOf(StreamOverrideApiError)
    const err = thrown as StreamOverrideApiError
    expect(err.code).toBe('internal')
    expect(err.status).toBe(502)
    expect(err.message).toBe('502 Bad Gateway')
  })
})

describe('RuntimeConfigApiError', () => {
  it('saveRuntimeConfig throws RuntimeConfigApiError on a structured error body', async () => {
    mockFetchOnce(
      jsonResponse(
        { error: 'frigate_url_invalid', message: 'frigate_url is not a valid URL' },
        { status: 400, statusText: 'Bad Request' }
      )
    )
    const thrown = await saveRuntimeConfig({
      frigate_url: 'nope',
      frigate_ui_url: '',
      go2rtc_url: ''
    }).catch((e: unknown) => e)
    expect(thrown).toBeInstanceOf(RuntimeConfigApiError)
    const err = thrown as RuntimeConfigApiError
    expect(err.code).toBe('frigate_url_invalid')
    expect(err.status).toBe(400)
    expect(err.message).toBe('frigate_url is not a valid URL')
  })

  it('restartRuntimeConfig falls back to code internal on a non-JSON body', async () => {
    mockFetchOnce(new Response('', { status: 500, statusText: 'Internal Server Error' }))
    const thrown = await restartRuntimeConfig().catch((e: unknown) => e)
    expect(thrown).toBeInstanceOf(RuntimeConfigApiError)
    const err = thrown as RuntimeConfigApiError
    expect(err.code).toBe('internal')
    expect(err.status).toBe(500)
    expect(err.message).toBe('500 Internal Server Error')
  })
})

describe('glance', () => {
  it('fetchGlance parses the envelope', async () => {
    const body: GlanceResponse = {
      unseen_count: 1,
      moments: [
        {
          id: 'rev-1',
          cam_id: 'camA',
          started_at: '2026-06-04T22:05:00Z',
          ended_at: '2026-06-04T22:05:30Z',
          severity: 'detection',
          kinds: ['person'],
          labels: ['person'],
          zones: [],
          detection_ids: ['1779310005.0-aaa'],
          thumb_event_id: '1779310005.0-aaa',
          seen: false
        }
      ]
    }
    mockFetchOnce(jsonResponse(body))
    await expect(fetchGlance()).resolves.toEqual(body)
  })

  it('markGlanceSeen posts the event id list and resolves on 204', async () => {
    const fn = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(new Response(null, { status: 204, statusText: 'No Content' }))
    vi.stubGlobal('fetch', fn)
    await expect(markGlanceSeen(['evt-1', 'evt-2'])).resolves.toBeUndefined()
    expect(fn).toHaveBeenCalledTimes(1)
    const [url, init] = fn.mock.calls[0]!
    expect(url).toBe('/api/glance/seen')
    expect(init?.method).toBe('POST')
    expect(JSON.parse(String(init?.body))).toEqual({ event_ids: ['evt-1', 'evt-2'] })
  })

  it('markGlanceSeen includes scope only when provided', async () => {
    const fn = vi.fn<typeof fetch>().mockResolvedValueOnce(new Response(null, { status: 204 }))
    vi.stubGlobal('fetch', fn)
    await markGlanceSeen(['evt-1'], 'household')
    const [, init] = fn.mock.calls[0]!
    expect(JSON.parse(String(init?.body))).toEqual({ event_ids: ['evt-1'], scope: 'household' })
  })

  it('markGlanceSeen surfaces the JSON error message on a non-ok response', async () => {
    mockFetchOnce(
      jsonResponse(
        { error: 'bad_request', message: 'event_ids must be non-empty' },
        { status: 400, statusText: 'Bad Request' }
      )
    )
    const thrown = await markGlanceSeen([]).catch((e: unknown) => e)
    expect(thrown).toBeInstanceOf(Error)
    expect((thrown as Error).message).toContain('event_ids must be non-empty')
  })

  it('markGlanceSeen falls back to the status text when the error body is not JSON', async () => {
    mockFetchOnce(new Response('not json', { status: 502, statusText: 'Bad Gateway' }))
    const thrown = await markGlanceSeen(['evt-1']).catch((e: unknown) => e)
    expect(thrown).toBeInstanceOf(Error)
    expect((thrown as Error).message).toContain('502 Bad Gateway')
  })
})

describe('recordings timeline', () => {
  it('fetchRecordingsSummary parses the captured summary verbatim', async () => {
    const body: RecordingsSummary = [
      {
        day: '2026-06-17',
        events: 12,
        hours: [
          { hour: '14', events: 5, motion: 8, objects: 3, duration: 2480 },
          { hour: '13', events: 7, motion: 9, objects: 4, duration: 3600 }
        ]
      }
    ]
    mockFetchOnce(jsonResponse(body))
    await expect(fetchRecordingsSummary('cam8')).resolves.toEqual(body)
  })

  it('fetchRecordingsSummary forwards the timezone query when supplied', async () => {
    const fn = vi.fn<typeof fetch>().mockResolvedValueOnce(jsonResponse([]))
    vi.stubGlobal('fetch', fn)
    await fetchRecordingsSummary('cam8', 'Europe/Riga')
    const [url] = fn.mock.calls[0]!
    expect(url).toBe('/api/cameras/cam8/recordings-summary?timezone=Europe%2FRiga')
  })

  it('fetchRecordingsSummary omits the timezone query when not supplied', async () => {
    const fn = vi.fn<typeof fetch>().mockResolvedValueOnce(jsonResponse([]))
    vi.stubGlobal('fetch', fn)
    await fetchRecordingsSummary('cam8')
    const [url] = fn.mock.calls[0]!
    expect(url).toBe('/api/cameras/cam8/recordings-summary')
  })

  it('fetchRecordingsSummary surfaces the JSON error message on a non-ok response', async () => {
    mockFetchOnce(
      jsonResponse(
        { error: 'upstream_error', message: 'frigate VOD unreachable' },
        { status: 502, statusText: 'Bad Gateway' }
      )
    )
    const thrown = await fetchRecordingsSummary('cam8').catch((e: unknown) => e)
    expect(thrown).toBeInstanceOf(Error)
    expect((thrown as Error).message).toContain('frigate VOD unreachable')
  })

  it('timelineMasterURL builds the BFF VOD master path', () => {
    expect(timelineMasterURL('cam8', 1718635200, 1718638800)).toBe(
      '/api/cameras/cam8/vod/1718635200/1718638800/master.m3u8'
    )
  })
})

describe('apiFetch error path (via fetchPrefs)', () => {
  it('includes the JSON error message in the thrown Error', async () => {
    mockFetchOnce(
      jsonResponse(
        { error: 'internal', message: 'prefs file is locked' },
        { status: 503, statusText: 'Service Unavailable' }
      )
    )
    const thrown = await fetchPrefs().catch((e: unknown) => e)
    expect(thrown).toBeInstanceOf(Error)
    expect((thrown as Error).message).toContain('prefs file is locked')
  })

  it('falls back to the status text when the error body is not JSON', async () => {
    mockFetchOnce(new Response('not json', { status: 503, statusText: 'Service Unavailable' }))
    const thrown = await fetchPrefs().catch((e: unknown) => e)
    expect(thrown).toBeInstanceOf(Error)
    expect((thrown as Error).message).toContain('503 Service Unavailable')
  })
})

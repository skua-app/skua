import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { isLoopback, parseAnswerCandidates, startWhep, type WhepOpts } from './whep'

// The vitest environment is node: RTCPeerConnection, HTMLVideoElement and
// fetch have types here but no runtime. Everything below is a hand-rolled
// stub installed with vi.stubGlobal, the idiom api.test.ts already uses.

const OFFER_SDP = 'v=0\r\no=- 1 1 IN IP4 0.0.0.0\r\ns=-\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n'

const HOST_CANDIDATE = 'a=candidate:1 1 udp 2130706431 192.168.1.10 8555 typ host'
const LOOPBACK_CANDIDATE = 'a=candidate:2 1 udp 2130706431 127.0.0.1 8555 typ host'

function answerWith(candidateLines: string[]): string {
  return ['v=0', 'o=- 1 1 IN IP4 0.0.0.0', 's=-', ...candidateLines].join('\r\n')
}

type StatsEntry = Record<string, unknown>
// A Map already has the forEach and get that the module uses of RTCStatsReport.
type StatsReport = Map<string, StatsEntry>

let peers: FakePeerConnection[] = []
let nextOfferSdp = OFFER_SDP

class FakePeerConnection extends EventTarget {
  connectionState: RTCPeerConnectionState = 'new'
  localDescription: { type: string; sdp: string } | null = null
  remoteSdp: string | null = null
  readonly transceivers: string[] = []
  closeCount = 0
  readonly statsQueue: StatsReport[] = []
  statsRejection: Error | null = null
  statsCalls = 0
  private lastStats: StatsReport = new Map()
  private readonly offerSdp = nextOfferSdp

  constructor() {
    super()
    peers.push(this)
  }

  addTransceiver(kind: string): void {
    this.transceivers.push(kind)
  }

  createOffer(): Promise<{ type: string; sdp: string }> {
    return Promise.resolve({ type: 'offer', sdp: this.offerSdp })
  }

  setLocalDescription(description: { type: string; sdp: string }): Promise<void> {
    this.localDescription = description
    return Promise.resolve()
  }

  setRemoteDescription(description: { type: string; sdp: string }): Promise<void> {
    this.remoteSdp = description.sdp
    return Promise.resolve()
  }

  // Each tick consumes one queued report; once the queue runs dry the last
  // report repeats, so a test only queues the ticks it actually asserts on.
  getStats(): Promise<StatsReport> {
    this.statsCalls += 1
    if (this.statsRejection !== null) return Promise.reject(this.statsRejection)
    const next = this.statsQueue.shift()
    if (next !== undefined) this.lastStats = next
    return Promise.resolve(this.lastStats)
  }

  close(): void {
    this.closeCount += 1
  }

  // Drives the module's listener the way a browser does: state first, then
  // the event. close() deliberately does not dispatch.
  emitState(state: RTCPeerConnectionState): void {
    this.connectionState = state
    this.dispatchEvent(new Event('connectionstatechange'))
  }
}

class FakeMediaStream {
  private readonly audioTracks: unknown[]

  constructor(audioTrackCount = 0) {
    this.audioTracks = Array.from({ length: audioTrackCount }, () => ({}))
  }

  getAudioTracks(): unknown[] {
    return this.audioTracks
  }
}

// RTCTrackEvent carries `streams`, which a bare Event does not, so extend
// Event rather than constructing one.
class FakeTrackEvent extends Event {
  readonly streams: readonly FakeMediaStream[]

  constructor(streams: readonly FakeMediaStream[]) {
    super('track')
    this.streams = streams
  }
}

class FakeVideoElement {
  playsInline = false
  muted = false
  srcObject: FakeMediaStream | null = null
  videoWidth = 0
  videoHeight = 0
  playCalls = 0
  private settle: { resolve: () => void; reject: (err: unknown) => void } | null = null

  play(): Promise<void> {
    this.playCalls += 1
    return new Promise<void>((resolve, reject) => {
      this.settle = { resolve, reject }
    })
  }

  resolvePlay(): void {
    this.settle?.resolve()
  }

  rejectPlay(err: unknown): void {
    this.settle?.reject(err)
  }
}

function currentPeer(): FakePeerConnection {
  const pc = peers[peers.length - 1]
  if (pc === undefined) throw new Error('no RTCPeerConnection was constructed')
  return pc
}

function stubFetchAnswer(sdp: string, init: ResponseInit = {}) {
  const mock = vi
    .fn<typeof fetch>()
    .mockImplementation(() => Promise.resolve(new Response(sdp, init)))
  vi.stubGlobal('fetch', mock)
  return mock
}

function stubFetchRejection(err: unknown) {
  const mock = vi.fn<typeof fetch>().mockImplementation(() => Promise.reject(err))
  vi.stubGlobal('fetch', mock)
  return mock
}

type StartOverrides = {
  camId?: string
  quality?: 'main' | 'sub'
  getMuted?: () => boolean
}

function buildOpts(overrides: StartOverrides = {}) {
  const video = new FakeVideoElement()
  const controller = new AbortController()
  const onStateChange = vi.fn<WhepOpts['onStateChange']>()
  const onStats = vi.fn<WhepOpts['onStats']>()
  const onAudioDetected = vi.fn<WhepOpts['onAudioDetected']>()
  const opts: WhepOpts = {
    camId: overrides.camId ?? 'cam1',
    // The single point where the stubs meet the real DOM types.
    videoEl: video as unknown as HTMLVideoElement,
    quality: overrides.quality,
    signal: controller.signal,
    getMuted: overrides.getMuted ?? (() => false),
    onStateChange,
    onStats,
    onAudioDetected
  }
  return { opts, video, controller, onStateChange, onStats, onAudioDetected }
}

async function start(overrides: StartOverrides = {}) {
  const built = buildOpts(overrides)
  const handle = await startWhep(built.opts)
  return { ...built, handle, pc: currentPeer() }
}

beforeEach(() => {
  vi.useFakeTimers()
  peers = []
  nextOfferSdp = OFFER_SDP
  vi.stubGlobal('RTCPeerConnection', FakePeerConnection)
  stubFetchAnswer(answerWith([HOST_CANDIDATE]))
})

afterEach(() => {
  vi.useRealTimers()
  vi.unstubAllGlobals()
})

describe('parseAnswerCandidates', () => {
  it('joins address and port in the order the lines appear', () => {
    const sdp = answerWith([
      'a=candidate:1 1 udp 2130706431 10.0.0.5 9000 typ host',
      'a=candidate:2 1 udp 2130706431 10.0.0.6 9001 typ host'
    ])
    expect(parseAnswerCandidates(sdp)).toEqual(['10.0.0.5:9000', '10.0.0.6:9001'])
  })

  it('collapses duplicate entries', () => {
    const sdp = answerWith([HOST_CANDIDATE, HOST_CANDIDATE])
    expect(parseAnswerCandidates(sdp)).toEqual(['192.168.1.10:8555'])
  })

  it('ignores lines that are not a=candidate', () => {
    const sdp = answerWith(['a=mid:0', 'a=sendrecv', HOST_CANDIDATE, 'a=end-of-candidates'])
    expect(parseAnswerCandidates(sdp)).toEqual(['192.168.1.10:8555'])
  })

  it('accepts both CRLF and LF line endings', () => {
    const lines = ['v=0', HOST_CANDIDATE, LOOPBACK_CANDIDATE]
    const expected = ['192.168.1.10:8555', '127.0.0.1:8555']
    expect(parseAnswerCandidates(lines.join('\r\n'))).toEqual(expected)
    expect(parseAnswerCandidates(lines.join('\n'))).toEqual(expected)
  })

  it('skips a candidate line with too few fields rather than yielding a partial entry', () => {
    const sdp = answerWith(['a=candidate:1 1 udp 2130706431', HOST_CANDIDATE])
    expect(parseAnswerCandidates(sdp)).toEqual(['192.168.1.10:8555'])
  })

  it('passes IPv6 and mDNS .local addresses through untouched', () => {
    const sdp = answerWith([
      'a=candidate:1 1 udp 2130706431 fe80::1cf2:3b4a 8555 typ host',
      'a=candidate:2 1 udp 2130706431 9b1d4e7a-2c3f.local 8556 typ host'
    ])
    expect(parseAnswerCandidates(sdp)).toEqual(['fe80::1cf2:3b4a:8555', '9b1d4e7a-2c3f.local:8556'])
  })
})

describe('isLoopback', () => {
  it('treats any address in 127.0.0.0/8 as loopback', () => {
    expect(isLoopback('127.0.0.1:8555')).toBe(true)
    expect(isLoopback('127.44.9.2:8555')).toBe(true)
  })

  it('treats ::1 as loopback, bare and bracketed', () => {
    expect(isLoopback('::1:8555')).toBe(true)
    expect(isLoopback('[::1]:8555')).toBe(true)
  })

  it('does not treat a LAN address as loopback', () => {
    expect(isLoopback('192.168.1.10:8555')).toBe(false)
  })

  it('does not treat an mDNS .local name as loopback', () => {
    expect(isLoopback('9b1d4e7a-2c3f.local:8555')).toBe(false)
  })
})

describe('negotiation request', () => {
  it('posts the offer to the camera WHEP endpoint, defaulting to quality=main', async () => {
    const fetchMock = stubFetchAnswer(answerWith([HOST_CANDIDATE]))
    await start({ camId: 'driveway' })
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/webrtc/driveway/whep?quality=main',
      expect.objectContaining({
        method: 'POST',
        headers: { 'Content-Type': 'application/sdp' },
        body: OFFER_SDP
      })
    )
  })

  it('carries quality=sub when the option is given', async () => {
    const fetchMock = stubFetchAnswer(answerWith([HOST_CANDIDATE]))
    await start({ camId: 'cam1', quality: 'sub' })
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/webrtc/cam1/whep?quality=sub',
      expect.objectContaining({ method: 'POST' })
    )
  })
})

describe('connection lifecycle', () => {
  it('emits connected, sets handle.state and cancels the watchdog', async () => {
    const { handle, pc, onStateChange } = await start()

    pc.emitState('connected')

    expect(onStateChange).toHaveBeenCalledWith('connected')
    expect(handle.state).toBe('connected')

    // Past the 3 s watchdog: it must have been cleared, so nothing fails.
    await vi.advanceTimersByTimeAsync(3500)
    expect(onStateChange).not.toHaveBeenCalledWith('failed', expect.anything(), expect.anything())
    expect(handle.state).toBe('connected')
  })
})

describe('failure classification', () => {
  it('reports ice_no_candidates when the watchdog expires on an answer with no candidates', async () => {
    stubFetchAnswer(answerWith([]))
    const { handle, onStateChange } = await start()

    await vi.advanceTimersByTimeAsync(3000)

    expect(onStateChange).toHaveBeenCalledWith('failed', 'ice_no_candidates', { candidates: [] })
    expect(handle.state).toBe('closed')
  })

  it('reports ice_no_candidates on loopback-only candidates and still lists them', async () => {
    stubFetchAnswer(answerWith([LOOPBACK_CANDIDATE]))
    const { onStateChange } = await start()

    await vi.advanceTimersByTimeAsync(3000)

    expect(onStateChange).toHaveBeenCalledWith('failed', 'ice_no_candidates', {
      candidates: ['127.0.0.1:8555']
    })
  })

  it('reports ice_unreachable when at least one candidate is routable', async () => {
    stubFetchAnswer(answerWith([LOOPBACK_CANDIDATE, HOST_CANDIDATE]))
    const { onStateChange } = await start()

    await vi.advanceTimersByTimeAsync(3000)

    expect(onStateChange).toHaveBeenCalledWith('failed', 'ice_unreachable', {
      candidates: ['127.0.0.1:8555', '192.168.1.10:8555']
    })
  })

  it('classifies a failed state before connected the same way, with candidate detail', async () => {
    stubFetchAnswer(answerWith([HOST_CANDIDATE]))
    const { pc, onStateChange } = await start()

    pc.emitState('failed')

    expect(onStateChange).toHaveBeenCalledWith('failed', 'ice_unreachable', {
      candidates: ['192.168.1.10:8555']
    })
  })

  it('reports ice_failed with no detail once the stream had reached connected', async () => {
    stubFetchAnswer(answerWith([HOST_CANDIDATE]))
    const { pc, onStateChange } = await start()

    pc.emitState('connected')
    pc.emitState('failed')

    expect(onStateChange).toHaveBeenCalledWith('failed', 'ice_failed', undefined)
  })

  it('reports timeout when the fetch rejects as a TimeoutError', async () => {
    stubFetchRejection(new DOMException('The operation was aborted', 'TimeoutError'))
    const { handle, onStateChange } = await start()

    expect(onStateChange).toHaveBeenCalledWith('failed', 'timeout')
    expect(handle.state).toBe('closed')
  })

  it('reports network for any other fetch rejection', async () => {
    stubFetchRejection(new TypeError('fetch failed'))
    const { onStateChange } = await start()

    expect(onStateChange).toHaveBeenCalledWith('failed', 'network')
  })

  it('reports negotiation_failed on a non-ok response', async () => {
    stubFetchAnswer('no stream', { status: 502 })
    const { onStateChange } = await start()

    expect(onStateChange).toHaveBeenCalledWith('failed', 'negotiation_failed')
  })

  it('reports unknown when the local description carries no SDP', async () => {
    nextOfferSdp = ''
    const fetchMock = stubFetchAnswer(answerWith([HOST_CANDIDATE]))
    const { onStateChange } = await start()

    expect(onStateChange).toHaveBeenCalledWith('failed', 'unknown')
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

describe('teardown', () => {
  it('emits closed once on abort and nothing further afterwards', async () => {
    const { pc, controller, onStateChange } = await start()

    controller.abort()
    pc.emitState('failed')

    expect(onStateChange).toHaveBeenCalledTimes(1)
    expect(onStateChange).toHaveBeenCalledWith('closed')
  })

  it('clears the watchdog on close(), so advancing past 3 s reports no failure', async () => {
    const { handle, onStateChange } = await start()
    expect(vi.getTimerCount()).toBe(1)

    handle.close()
    expect(vi.getTimerCount()).toBe(0)

    await vi.advanceTimersByTimeAsync(3500)
    expect(onStateChange).not.toHaveBeenCalled()
  })

  it('clears the stats interval on close(), so no further tick is even gathered', async () => {
    const { handle, pc, onStateChange, onStats } = await start()

    pc.emitState('connected')
    await vi.advanceTimersByTimeAsync(1000)
    const statsCallsWhileOpen = pc.statsCalls
    expect(statsCallsWhileOpen).toBeGreaterThan(0)
    onStateChange.mockClear()
    onStats.mockClear()

    handle.close()
    expect(vi.getTimerCount()).toBe(0)

    await vi.advanceTimersByTimeAsync(3500)

    // Not merely suppressed by the `closed` guard — getStats stops being called.
    expect(pc.statsCalls).toBe(statsCallsWhileOpen)
    expect(onStats).not.toHaveBeenCalled()
    expect(onStateChange).not.toHaveBeenCalled()
    expect(pc.closeCount).toBeGreaterThan(0)
  })
})

describe('track handling', () => {
  it('reports no audio before negotiation starts', async () => {
    const { opts, onAudioDetected } = buildOpts()
    const fetchMock = stubFetchAnswer(answerWith([HOST_CANDIDATE]))

    const pending = startWhep(opts)

    expect(onAudioDetected).toHaveBeenCalledWith(false)
    expect(fetchMock).not.toHaveBeenCalled()

    await pending
  })

  it('assigns the stream to srcObject and plays it', async () => {
    const { pc, video } = await start()
    const stream = new FakeMediaStream()

    pc.dispatchEvent(new FakeTrackEvent([stream]))

    expect(video.srcObject).toBe(stream)
    expect(video.playCalls).toBe(1)
    expect(video.muted).toBe(true)
  })

  it('applies the muted pref read at play resolution, not the value at start', async () => {
    let muted = true
    const { pc, video } = await start({ getMuted: () => muted })

    pc.dispatchEvent(new FakeTrackEvent([new FakeMediaStream()]))
    expect(video.muted).toBe(true)

    // The pref flips between startWhep() and play() resolving; the live read
    // in the .then must win over the value that was current at start.
    muted = false
    video.resolvePlay()
    await vi.advanceTimersByTimeAsync(0)

    expect(video.muted).toBe(false)
  })

  it('swallows a rejected play()', async () => {
    const { pc, video } = await start()

    pc.dispatchEvent(new FakeTrackEvent([new FakeMediaStream()]))
    video.rejectPlay(new DOMException('NotAllowedError', 'NotAllowedError'))
    await vi.advanceTimersByTimeAsync(0)

    expect(video.muted).toBe(true)
  })

  it('reports audio when the stream carries an audio track', async () => {
    const { pc, onAudioDetected } = await start()

    pc.dispatchEvent(new FakeTrackEvent([new FakeMediaStream(1)]))

    expect(onAudioDetected).toHaveBeenLastCalledWith(true)
  })
})

describe('stats aggregation', () => {
  function videoStat(extra: StatsEntry): StatsEntry {
    return { type: 'inbound-rtp', kind: 'video', ...extra }
  }

  it('reports a null bitrate on the first tick and exact kbps on the second', async () => {
    const { pc, onStats } = await start()
    pc.statsQueue.push(
      new Map([['v', videoStat({ bytesReceived: 100_000 })]]),
      new Map([['v', videoStat({ bytesReceived: 225_000 })]])
    )

    pc.emitState('connected')

    await vi.advanceTimersByTimeAsync(1000)
    expect(onStats).toHaveBeenLastCalledWith(expect.objectContaining({ bitrateKbps: null }))

    // 125 000 bytes over exactly 1 s of faked time = 1000 kbps.
    await vi.advanceTimersByTimeAsync(1000)
    expect(onStats).toHaveBeenLastCalledWith(expect.objectContaining({ bitrateKbps: 1000 }))
  })

  it('reads round-trip time only from a succeeded candidate-pair, in whole ms', async () => {
    const { pc, onStats } = await start()
    pc.statsQueue.push(
      new Map([['p', { type: 'candidate-pair', state: 'in-progress', currentRoundTripTime: 0.5 }]]),
      new Map([['p', { type: 'candidate-pair', state: 'succeeded', currentRoundTripTime: 0.0234 }]])
    )

    pc.emitState('connected')

    await vi.advanceTimersByTimeAsync(1000)
    expect(onStats).toHaveBeenLastCalledWith(expect.objectContaining({ latencyMs: null }))

    await vi.advanceTimersByTimeAsync(1000)
    expect(onStats).toHaveBeenLastCalledWith(expect.objectContaining({ latencyMs: 23 }))
  })

  it('takes codec labels from the referenced codec entry, video and audio apart', async () => {
    const { pc, onStats } = await start()
    pc.statsQueue.push(
      new Map<string, StatsEntry>([
        ['v', videoStat({ bytesReceived: 1000, codecId: 'cv' })],
        ['a', { type: 'inbound-rtp', kind: 'audio', codecId: 'ca' }],
        ['cv', { type: 'codec', mimeType: 'video/H264' }],
        ['ca', { type: 'codec', mimeType: 'audio/opus' }]
      ])
    )

    pc.emitState('connected')
    await vi.advanceTimersByTimeAsync(1000)

    expect(onStats).toHaveBeenLastCalledWith(
      expect.objectContaining({ videoCodec: 'H264', audioCodec: 'opus' })
    )
  })

  it('keeps the resolved codec labels when a later report omits codec information', async () => {
    const { pc, onStats } = await start()
    pc.statsQueue.push(
      new Map<string, StatsEntry>([
        ['v', videoStat({ bytesReceived: 1000, codecId: 'cv' })],
        ['cv', { type: 'codec', mimeType: 'video/H264' }]
      ]),
      new Map<string, StatsEntry>([['v', videoStat({ bytesReceived: 2000 })]])
    )

    pc.emitState('connected')
    await vi.advanceTimersByTimeAsync(2000)

    expect(onStats).toHaveBeenLastCalledWith(expect.objectContaining({ videoCodec: 'H264' }))
  })

  it('leaves resolution null at zero dimensions and joins them with U+00D7 otherwise', async () => {
    const { pc, video, onStats } = await start()
    pc.statsQueue.push(new Map([['v', videoStat({ bytesReceived: 1000 })]]))

    pc.emitState('connected')
    await vi.advanceTimersByTimeAsync(1000)
    expect(onStats).toHaveBeenLastCalledWith(expect.objectContaining({ resolution: null }))

    video.videoWidth = 1920
    video.videoHeight = 1080
    await vi.advanceTimersByTimeAsync(1000)
    expect(onStats).toHaveBeenLastCalledWith(expect.objectContaining({ resolution: '1920×1080' }))
  })

  it('rounds fps when present and leaves it null when the stat omits it', async () => {
    const { pc, onStats } = await start()
    pc.statsQueue.push(
      new Map([['v', videoStat({ bytesReceived: 1000, framesPerSecond: 14.6 })]]),
      new Map([['v', videoStat({ bytesReceived: 2000 })]])
    )

    pc.emitState('connected')

    await vi.advanceTimersByTimeAsync(1000)
    expect(onStats).toHaveBeenLastCalledWith(expect.objectContaining({ fps: 15 }))

    await vi.advanceTimersByTimeAsync(1000)
    expect(onStats).toHaveBeenLastCalledWith(expect.objectContaining({ fps: null }))
  })

  it('swallows a getStats rejection without emitting stats', async () => {
    const { pc, onStats } = await start()
    pc.statsRejection = new Error('stats unavailable')

    pc.emitState('connected')
    await vi.advanceTimersByTimeAsync(2000)

    expect(onStats).not.toHaveBeenCalled()
  })
})

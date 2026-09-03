export type WhepHandle = {
  videoEl: HTMLVideoElement
  state: 'connecting' | 'connected' | 'failed' | 'closed'
  stats: {
    latencyMs: number | null
    bitrateKbps: number | null
    videoCodec: string | null
    audioCodec: string | null
    resolution: string | null
    fps: number | null
  }
  close(): void
}

export type WhepFailureDetail = {
  candidates?: string[]
}

export type WhepOpts = {
  camId: string
  videoEl: HTMLVideoElement
  quality?: 'main' | 'sub'
  signal: AbortSignal
  getMuted: () => boolean
  onStateChange: (state: WhepHandle['state'], reason?: string, detail?: WhepFailureDetail) => void
  onStats: (stats: WhepHandle['stats']) => void
  onAudioDetected: (hasAudio: boolean) => void
}

// go2rtc answers non-trickle: the SDP answer already carries every candidate
// it intends to offer, so the answer alone tells us whether the browser was
// given anywhere reachable to send media.
export function parseAnswerCandidates(sdp: string): string[] {
  const out: string[] = []
  const seen = new Set<string>()
  for (const line of sdp.split(/\r\n|\r|\n/)) {
    if (!line.startsWith('a=candidate:')) continue
    const parts = line.slice('a=candidate:'.length).trim().split(/\s+/)
    const address = parts[4]
    const port = parts[5]
    if (!address || !port) continue
    const entry = `${address}:${port}`
    if (seen.has(entry)) continue
    seen.add(entry)
    out.push(entry)
  }
  return out
}

export function isLoopback(entry: string): boolean {
  const address = entry.slice(0, entry.lastIndexOf(':'))
  return address === '::1' || address === '[::1]' || /^127\./.test(address)
}

export async function startWhep(opts: WhepOpts): Promise<WhepHandle> {
  const { camId, videoEl, signal, getMuted, onStateChange, onStats, onAudioDetected } = opts
  const quality = opts.quality ?? 'main'

  videoEl.playsInline = true
  videoEl.muted = true

  const pc = new RTCPeerConnection({ iceServers: [] })

  pc.addTransceiver('video', { direction: 'recvonly' })
  pc.addTransceiver('audio', { direction: 'recvonly' })

  onAudioDetected(false)

  const handle: WhepHandle = {
    videoEl,
    state: 'connecting',
    stats: {
      latencyMs: null,
      bitrateKbps: null,
      videoCodec: null,
      audioCodec: null,
      resolution: null,
      fps: null
    },
    close() {
      cleanup()
    }
  }

  let statsInterval: ReturnType<typeof setInterval> | null = null
  let watchdogTimer: ReturnType<typeof setTimeout> | null = null
  let prevBytes = 0
  let prevBytesTime = 0
  let videoCodec: string | null = null
  let audioCodec: string | null = null
  let closed = false
  let answerCandidates: string[] = []
  let hasUsableCandidates = false

  function cleanup() {
    closed = true
    if (statsInterval !== null) {
      clearInterval(statsInterval)
      statsInterval = null
    }
    if (watchdogTimer !== null) {
      clearTimeout(watchdogTimer)
      watchdogTimer = null
    }
    videoEl.srcObject = null
    pc.close()
    handle.state = 'closed'
  }

  signal.addEventListener('abort', () => {
    if (!closed) {
      cleanup()
      onStateChange('closed')
    }
  })

  pc.addEventListener('track', (ev) => {
    const stream = ev.streams[0]
    if (stream && videoEl.srcObject !== stream) {
      videoEl.srcObject = stream
      videoEl.muted = true // satisfies autoplay policy for play() call
      const playPromise = videoEl.play()
      if (playPromise !== undefined) {
        playPromise
          .then(() => {
            // Read pref live (not via stale closure) so toggles applied
            // between startWhep() and play() resolution take effect.
            videoEl.muted = getMuted()
          })
          .catch((err: unknown) => {
            console.debug('[whep] video.play() rejected:', err)
          })
      }
    }
    const audioTracks = stream?.getAudioTracks() ?? []
    const hasAudio = audioTracks.length > 0
    if (hasAudio) videoEl.muted = getMuted()
    onAudioDetected(hasAudio)
  })

  pc.addEventListener('connectionstatechange', () => {
    if (closed) return
    const state = pc.connectionState
    if (state === 'connected') {
      if (watchdogTimer !== null) {
        clearTimeout(watchdogTimer)
        watchdogTimer = null
      }
      handle.state = 'connected'
      onStateChange('connected')

      statsInterval = setInterval(() => {
        pc.getStats()
          .then((report) => {
            if (closed) return
            let latencyMs: number | null = null
            let bitrateKbps: number | null = null
            let fps: number | null = null

            report.forEach((s) => {
              if (
                s.type === 'candidate-pair' &&
                s.state === 'succeeded' &&
                s.currentRoundTripTime !== undefined
              ) {
                latencyMs = Math.round((s.currentRoundTripTime as number) * 1000)
              }
              if (s.type === 'inbound-rtp' && s.kind === 'video') {
                const bytes = s.bytesReceived as number
                const now = Date.now()
                if (prevBytesTime > 0) {
                  const dt = (now - prevBytesTime) / 1000
                  bitrateKbps = dt > 0 ? Math.round(((bytes - prevBytes) * 8) / dt / 1000) : null
                }
                prevBytes = bytes
                prevBytesTime = now
                const framesPerSecond = (s as { framesPerSecond?: number }).framesPerSecond
                if (typeof framesPerSecond === 'number') {
                  fps = Math.round(framesPerSecond)
                }
              }
            })

            let newVideoCodec: string | null = videoCodec
            let newAudioCodec: string | null = audioCodec

            report.forEach((s) => {
              if (s.type !== 'inbound-rtp') return
              const kind = s.kind as 'video' | 'audio' | undefined
              if (kind !== 'video' && kind !== 'audio') return
              const codecId = s.codecId as string | undefined
              if (!codecId) return
              const codecStat = report.get(codecId)
              if (!codecStat) return
              const mime = (codecStat as { mimeType?: string }).mimeType
              if (!mime) return
              const label = mime.includes('/') ? (mime.split('/')[1] ?? null) : mime
              if (kind === 'video') newVideoCodec = label
              else newAudioCodec = label
            })

            videoCodec = newVideoCodec
            audioCodec = newAudioCodec

            const w = videoEl.videoWidth
            const h = videoEl.videoHeight
            const resolution = w > 0 && h > 0 ? `${w}×${h}` : null

            handle.stats = { latencyMs, bitrateKbps, videoCodec, audioCodec, resolution, fps }
            onStats(handle.stats)
          })
          .catch(() => {
            // stats not available — ignore
          })
      }, 1000)
    } else if (state === 'failed') {
      // 'ice_failed' means only one thing: a stream that was playing and then
      // dropped. A stream that never reached 'connected' is an establishment
      // failure, classified from what go2rtc advertised in the answer.
      const established = handle.state === 'connected'
      const reason = established
        ? 'ice_failed'
        : hasUsableCandidates
          ? 'ice_unreachable'
          : 'ice_no_candidates'
      handle.state = 'failed'
      onStateChange('failed', reason, established ? undefined : { candidates: answerCandidates })
      cleanup()
    } else if (state === 'closed') {
      if (!closed) {
        handle.state = 'closed'
        onStateChange('closed')
        cleanup()
      }
    }
  })

  const offer = await pc.createOffer()
  await pc.setLocalDescription(offer)

  const sdpOffer = pc.localDescription?.sdp
  if (!sdpOffer) {
    cleanup()
    onStateChange('failed', 'unknown')
    return handle
  }

  let res: Response
  try {
    res = await fetch(`/api/webrtc/${camId}/whep?quality=${quality}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/sdp' },
      body: sdpOffer,
      signal: AbortSignal.any([signal, AbortSignal.timeout(10_000)])
    })
  } catch (err) {
    if (!closed) {
      // Only the 10 s AbortSignal.timeout rejects as TimeoutError; an abort
      // from the caller-supplied signal is teardown and is handled by the
      // signal's own listener, which sets `closed` before we get here.
      const timedOut = err instanceof DOMException && err.name === 'TimeoutError'
      handle.state = 'failed'
      onStateChange('failed', timedOut ? 'timeout' : 'network')
      cleanup()
    }
    return handle
  }

  if (!res.ok) {
    handle.state = 'failed'
    onStateChange('failed', 'negotiation_failed')
    cleanup()
    return handle
  }

  const sdpAnswer = await res.text()
  answerCandidates = parseAnswerCandidates(sdpAnswer)
  hasUsableCandidates = answerCandidates.some((c) => !isLoopback(c))
  await pc.setRemoteDescription({ type: 'answer', sdp: sdpAnswer })

  watchdogTimer = setTimeout(() => {
    if (closed) return
    if (pc.connectionState !== 'connected') {
      handle.state = 'failed'
      onStateChange('failed', hasUsableCandidates ? 'ice_unreachable' : 'ice_no_candidates', {
        candidates: answerCandidates
      })
      cleanup()
    }
  }, 3000)

  return handle
}

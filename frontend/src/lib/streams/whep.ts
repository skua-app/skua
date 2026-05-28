export type WhepHandle = {
  videoEl: HTMLVideoElement
  state: 'connecting' | 'connected' | 'failed' | 'closed'
  stats: {
    latencyMs: number | null
    bitrateKbps: number | null
    videoCodec: string | null
    audioCodec: string | null
    resolution: string | null
  }
  close(): void
}

export type WhepOpts = {
  camId: string
  videoEl: HTMLVideoElement
  quality?: 'main' | 'sub'
  signal: AbortSignal
  getMuted: () => boolean
  onStateChange: (state: WhepHandle['state'], reason?: string) => void
  onStats: (stats: WhepHandle['stats']) => void
  onAudioDetected: (hasAudio: boolean) => void
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
      resolution: null
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

            handle.stats = { latencyMs, bitrateKbps, videoCodec, audioCodec, resolution }
            onStats(handle.stats)
          })
          .catch(() => {
            // stats not available — ignore
          })
      }, 1000)
    } else if (state === 'failed') {
      const reason = handle.state === 'connected' ? 'ice_failed' : 'unknown'
      handle.state = 'failed'
      onStateChange('failed', reason)
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
  } catch {
    if (!closed) {
      handle.state = 'failed'
      onStateChange('failed', 'network')
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
  await pc.setRemoteDescription({ type: 'answer', sdp: sdpAnswer })

  watchdogTimer = setTimeout(() => {
    if (closed) return
    if (pc.connectionState !== 'connected') {
      handle.state = 'failed'
      onStateChange('failed', 'timeout')
      cleanup()
    }
  }, 3000)

  return handle
}

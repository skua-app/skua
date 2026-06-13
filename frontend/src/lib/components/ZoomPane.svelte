<script lang="ts">
  import type { Snippet } from 'svelte'

  type Props = {
    children: Snippet
    maxScale?: number
    // When this value changes, the pane animates back to scale 1, tx 0, ty 0.
    // Used to reset zoom on camera change / event change so a new media
    // doesn't open partially zoomed off-screen.
    resetKey?: unknown
  }

  let { children, maxScale = 4, resetKey }: Props = $props()

  let frameEl: HTMLDivElement | undefined = $state()
  let scale = $state(1)
  let tx = $state(0)
  let ty = $state(0)
  let pinching = $state(false)
  let reduceMotion = $state(false)

  // Frame size (in CSS px). Kept in sync via ResizeObserver so rotation /
  // breakpoint changes keep the pan-clamp bounds accurate.
  let frameW = $state(0)
  let frameH = $state(0)

  type PInfo = { x: number; y: number }
  const pointers = new Map<number, PInfo>()

  // Gesture session captures, set on pinch/pan start.
  let initScale = 1
  let initTx = 0
  let initTy = 0
  let initDist = 0
  let initMidX = 0
  let initMidY = 0
  let panStartX = 0
  let panStartY = 0
  let lastTapTime = 0

  function clampN(v: number, lo: number, hi: number) {
    return Math.max(lo, Math.min(hi, v))
  }
  function maxPan(s: number, size: number) {
    return Math.max(0, ((s - 1) * size) / 2)
  }
  function clampTranslate() {
    const mx = maxPan(scale, frameW)
    const my = maxPan(scale, frameH)
    tx = clampN(tx, -mx, mx)
    ty = clampN(ty, -my, my)
  }
  function frameLocal(clientX: number, clientY: number) {
    if (!frameEl) return { x: 0, y: 0 }
    const r = frameEl.getBoundingClientRect()
    return { x: clientX - r.left, y: clientY - r.top }
  }
  function resetAll() {
    scale = 1
    tx = 0
    ty = 0
  }

  $effect(() => {
    if (typeof window === 'undefined') return
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    reduceMotion = mq.matches
    const on = () => (reduceMotion = mq.matches)
    mq.addEventListener('change', on)
    return () => mq.removeEventListener('change', on)
  })

  $effect(() => {
    if (!frameEl) return
    const el = frameEl
    const ro = new ResizeObserver(() => {
      frameW = el.clientWidth
      frameH = el.clientHeight
      clampTranslate()
    })
    ro.observe(el)
    return () => ro.disconnect()
  })

  // Reset whenever the resetKey changes (camera id, event id, …).
  $effect(() => {
    void resetKey
    resetAll()
  })

  function onPointerDown(e: PointerEvent) {
    const loc = frameLocal(e.clientX, e.clientY)
    pointers.set(e.pointerId, loc)

    if (pointers.size === 2) {
      const [a, b] = [...pointers.values()]
      initDist = Math.hypot(b!.x - a!.x, b!.y - a!.y) || 1
      initMidX = (a!.x + b!.x) / 2
      initMidY = (a!.y + b!.y) / 2
      initScale = scale
      initTx = tx
      initTy = ty
      pinching = true
      // Capture every active pointer so the underlying media doesn't get
      // stray taps mid-pinch.
      pointers.forEach((_, id) => {
        try {
          frameEl?.setPointerCapture(id)
        } catch {
          // older browsers may reject if pointer is already released; benign.
        }
      })
      e.preventDefault()
      return
    }

    // Single pointer. Double-tap reset only when zoomed in — at scale 1
    // double-tap belongs to the underlying native controls.
    const now = performance.now()
    if (now - lastTapTime < 300 && scale > 1) {
      resetAll()
      lastTapTime = 0
      e.preventDefault()
      return
    }
    lastTapTime = now

    if (scale > 1) {
      panStartX = loc.x
      panStartY = loc.y
      initTx = tx
      initTy = ty
      try {
        frameEl?.setPointerCapture(e.pointerId)
      } catch {
        // benign
      }
      e.preventDefault()
    }
    // else: scale === 1 — let the event pass through to native controls /
    // child handlers. We don't capture and don't preventDefault.
  }

  function onPointerMove(e: PointerEvent) {
    if (!pointers.has(e.pointerId)) return
    const loc = frameLocal(e.clientX, e.clientY)
    pointers.set(e.pointerId, loc)

    if (pinching && pointers.size >= 2) {
      const [a, b] = [...pointers.values()]
      const curDist = Math.hypot(b!.x - a!.x, b!.y - a!.y) || 1
      const midX = (a!.x + b!.x) / 2
      const midY = (a!.y + b!.y) / 2
      const newScale = clampN((initScale * curDist) / initDist, 1, maxScale)
      const ratio = newScale / (initScale || 1)
      const cx = frameW / 2
      const cy = frameH / 2
      // Zoom-about-midpoint with midpoint drift = pan during pinch. With
      // transform-origin centred, the screen coord of a content point p is
      // screen = center + scale*p + translate, so to keep the content point
      // that was under (initMidX, initMidY) under (midX, midY):
      let newTx = midX - cx - ratio * (initMidX - cx - initTx)
      let newTy = midY - cy - ratio * (initMidY - cy - initTy)
      const mx = maxPan(newScale, frameW)
      const my = maxPan(newScale, frameH)
      newTx = clampN(newTx, -mx, mx)
      newTy = clampN(newTy, -my, my)
      scale = newScale
      tx = newTx
      ty = newTy
      e.preventDefault()
      return
    }

    if (pointers.size === 1 && scale > 1) {
      const dx = loc.x - panStartX
      const dy = loc.y - panStartY
      const mx = maxPan(scale, frameW)
      const my = maxPan(scale, frameH)
      tx = clampN(initTx + dx, -mx, mx)
      ty = clampN(initTy + dy, -my, my)
      e.preventDefault()
    }
  }

  function onPointerUp(e: PointerEvent) {
    pointers.delete(e.pointerId)
    try {
      frameEl?.releasePointerCapture(e.pointerId)
    } catch {
      // benign
    }
    if (pointers.size < 2) pinching = false
    // If one pointer remains after the other lifted (e.g. one finger of a
    // pinch released), re-baseline pan so the next move is continuous.
    if (pointers.size === 1) {
      const [p] = [...pointers.values()]
      panStartX = p!.x
      panStartY = p!.y
      initTx = tx
      initTy = ty
    }
  }

  function onWheel(e: WheelEvent) {
    e.preventDefault()
    const loc = frameLocal(e.clientX, e.clientY)
    const next = clampN(scale * Math.exp(-e.deltaY * 0.001), 1, maxScale)
    if (next === scale) return
    const cx = frameW / 2
    const cy = frameH / 2
    const ratio = next / scale
    let newTx = loc.x - cx - ratio * (loc.x - cx - tx)
    let newTy = loc.y - cy - ratio * (loc.y - cy - ty)
    if (next === 1) {
      newTx = 0
      newTy = 0
    } else {
      const mx = maxPan(next, frameW)
      const my = maxPan(next, frameH)
      newTx = clampN(newTx, -mx, mx)
      newTy = clampN(newTy, -my, my)
    }
    scale = next
    tx = newTx
    ty = newTy
  }

  function onDblClick(e: MouseEvent) {
    if (scale > 1) {
      e.preventDefault()
      resetAll()
    }
  }

  // While we own the gesture (pinching or zoomed) the frame consumes
  // touches. Otherwise let single-touch reach native video controls.
  const touchAction = $derived(pinching || scale > 1 ? 'none' : 'auto')
  const transitionStyle = $derived(reduceMotion ? 'none' : '')
  const transform = $derived(`translate(${tx}px, ${ty}px) scale(${scale})`)
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div
  class="zp-frame"
  bind:this={frameEl}
  style:touch-action={touchAction}
  onpointerdown={onPointerDown}
  onpointermove={onPointerMove}
  onpointerup={onPointerUp}
  onpointercancel={onPointerUp}
  onwheel={onWheel}
  ondblclick={onDblClick}
>
  <div class="zp-inner" style:transform style:transition={transitionStyle}>
    {@render children()}
  </div>
</div>

<style>
  .zp-frame {
    position: relative;
    width: 100%;
    height: 100%;
    overflow: hidden;
  }
  .zp-inner {
    position: absolute;
    inset: 0;
    transform-origin: center center;
    will-change: transform;
    transition: transform 0.2s ease;
  }
</style>

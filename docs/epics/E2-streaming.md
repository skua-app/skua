# Epic E2 — Live Streaming

> **Historical record (shipped v0.2.0).** This spec was written against the original private homelab deployment. Infrastructure references — host IPs, VPN gateway addresses, network topology — reflect that environment and are no longer authoritative. See CLAUDE.md §5 for the current canonical placeholders and docs/setup/ for setup guidance.

**Status: DONE** — tagged v0.2.0

All acceptance criteria verified on production. See CLAUDE.md for the
current architectural state. This file is preserved for historical reference.

---

## What shipped

- `POST /api/webrtc/:cam_id/whep?quality=main|sub` — WHEP signaling proxy
- `GET /api/cameras/:id/tile.jpg` — BFF-resized 320px ECO tile
- `GET /api/prefs`, `PUT /api/prefs` — persistent user prefs
- `/cam/[id]` focus view with WebRTC, controls, telemetry
- HD/ECO header toggle on grid, persisted server-side
- Grid at 1 Hz polling (HD = snapshot.jpg, ECO = tile.jpg)
- Runtime audio detection from WHEP stream tracks
- VAAPI transcode for all camera main streams (H.265→H.264 1080p)

## Key decisions made during E2

- **Live video in grid was dropped.** HD/ECO snapshot toggle replaced it.
- **`capabilities.audio` removed from API.** Audio presence detected at runtime
  from WHEP `ontrack` event — no static config needed.
- **iOS autoplay:** static `muted` HTML attribute required; explicit `play()`
  after `srcObject`; force `muted=true` before `play()`, then apply real pref
  in `.then()`.
- **go2rtc ICE candidates** verified: LAN, VPN gateway addresses, public WAN, STUN srflx (configuration documented in docs/setup/frigate-config.md).
- **VAAPI alias format:** `ffmpeg:cam_main#video=h264#hardware=vaapi#width=1920#height=1080#bitrate=3500k[#audio=opus]`
  go2rtc starts ffmpeg on-demand (no idle load).
- **Telemetry RTT** from `candidate-pair.currentRoundTripTime`, not `inbound-rtp`.
- **Hikvision camera config baseline:** main H.265 20fps (recording), sub H.264
  Baseline I=10 (WebRTC/grid), audio disabled on sub.

## Cosmetic items deferred

These were identified but not fixed before tagging:

- Focus view scrolls on some devices (py-4 + flex height interaction)
- MobileTabBar absent on /events, /archive, /settings routes on mobile
- Safe-area-inset on MobileGrid status-spacer (handled via env())

These live in the cosmetic backlog and can be picked up in any future session.

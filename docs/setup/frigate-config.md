# Frigate Config for Skua

Skua talks to your existing Frigate 0.17.1 / go2rtc 1.9.10 install. Two
pieces of Frigate-side configuration matter for the focus view: the go2rtc
**stream sources** each camera exposes (always), and the WebRTC **ICE
candidates** (only in some network topologies). This page covers both.

## go2rtc stream sources for the focus view

Skua's focus view plays a single camera live over WebRTC (WHEP). It reads
the stream names to use from each camera's `cameras.<id>.live.streams.Main`
and `cameras.<id>.live.streams.Sub` entries in the Frigate config. Those
values are go2rtc stream names — the underlying go2rtc alias can be called
anything you like; Skua simply serves whatever name `live.streams.Main` and
`.Sub` point at. There is no required naming convention.

### Main stream must be H.264

The Main stream backs the focus view, and iOS Safari's WebRTC stack does
not play H.265 (HEVC). Camera main streams are commonly H.265 — that is the
native codec most cameras record in — so you typically need a dedicated
H.264 go2rtc alias for the focus view and point `live.streams.Main` at it.

### Keep it within the H.264 level iOS will accept

iOS Safari also rejects a WebRTC stream that exceeds the H.264 level
advertised in the SDP. go2rtc advertises Constrained Baseline 3.1
(`profile-level-id=42e01f`), but it will pass a High-profile stream straight
through when the source is high-bitrate, and iOS then refuses it. The
symptom is the focus view staying on the snapshot poster while the 3-second
watchdog fires. The fix is to transcode the alias with an explicit
`-level:v 4.1` and a bitrate cap (around `3500k`).

### Resolution

Aim for roughly 1080p on the focus stream. For cameras above 2K, downscale
the H.264 focus alias to 1080p — higher resolutions cause lag and
negotiation failures on iOS.

### Audio (optional)

If you want audio in the focus view, prefer `opus` on the alias; go2rtc
transcodes the camera's AAC to Opus for you. Audio is detected at runtime,
per camera, from the WHEP track event — not from this config — so adding or
removing audio on the alias is all it takes, and Skua's mute button follows
whatever the track actually carries.

### Sub stream (optional)

`cameras.<id>.live.streams.Sub` is optional. When set, it powers the LQ
option in the focus view. If a camera has no Sub stream, LQ is simply
unavailable for that camera and Skua uses Main only — that is expected
behaviour, not an error.

### Example alias

Below is one go2rtc alias as an example — `cam3_main_h264` is just an
example name, call it whatever you like — defined under `go2rtc.streams` in
Frigate's config and then referenced from the camera's `live.streams.Main`:

```yaml
go2rtc:
  streams:
    # Example name — call it whatever you like.
    cam3_main_h264: ffmpeg:cam3_main#video=h264#hardware=vaapi#width=1920#height=1080#bitrate=3500k
    # Add #audio=opus if you want audio in the focus view:
    cam5_main_h264: ffmpeg:cam5_main#video=h264#hardware=vaapi#width=1920#height=1080#bitrate=3500k#audio=opus

cameras:
  cam3:
    live:
      streams:
        Main: cam3_main_h264   # H.264 alias above — backs the focus view
        Sub: cam3_sub          # optional — powers the LQ option
```

The `width=1920#height=1080` keeps the alias at 1080p and `bitrate=3500k`
caps the bitrate; pin the H.264 level to `4.1` on the transcode as described
above so iOS will accept the stream. go2rtc starts the transcode ffmpeg on
demand — only while a viewer is connected — and stops it when the last
viewer disconnects, so there is no idle CPU/GPU cost.

## ICE candidates

This is required only when the default ICE candidates advertised by go2rtc
don't cover the path between PWA clients and the Frigate host. That
typically happens when Frigate runs in a Docker bridge network instead of
host networking, or when clients connect from a different subnet, a VPN,
or a remote tunnel. On a pure LAN deployment with `network_mode: host` and
clients on the same subnet, the defaults usually work — try it first, and
only add the block below if WebRTC fails to negotiate (focus view stays on
the snapshot poster, watchdog fires at 3 s).

### Minimal LAN-only example

Add or merge the `go2rtc.webrtc` block in Frigate's `config.yml`:

```yaml
go2rtc:
  webrtc:
    candidates:
      - <frigate-host>:8555 # LAN address of the Frigate host
      - stun:8555           # STUN fallback (public STUN servers)
```

### Adding remote-access candidates (optional)

If clients reach the Frigate host through a VPN or tunnel, append the
gateway address(es) clients route through into the LAN. Use the address
inside the LAN that VPN-connected clients see as their next hop — not the
client's own VPN IP.

```yaml
go2rtc:
  webrtc:
    candidates:
      - <frigate-host>:8555  # LAN address of the Frigate host
      - <vpn-gateway-ip>:8555 # replace with the address inside the LAN that VPN-connected clients route through
      - stun:8555            # STUN fallback (public STUN servers)
```

### Why these candidates

WebRTC ICE needs reachable candidate addresses for the client to connect
to go2rtc's UDP media port. Without explicit candidates, go2rtc may
advertise only addresses that are not reachable from where the client
actually sits.

| Candidate | Who it helps |
|---|---|
| LAN address | clients on the same network as the Frigate host |
| STUN srflx  | clients behind NAT (uncommon for LAN-only deployments) |

### Verify

After applying the config and restarting Frigate:

```bash
curl -s http://<frigate-host>:1984/api/config | jq '.webrtc'
# Expected: shows the candidates array above
```

If your VPN or tunnel gateway has a firewall, ensure UDP 8555 is forwarded
between client subnets and the Frigate host.

# Frigate Config — go2rtc ICE Candidates

This is required only when the default ICE candidates advertised by go2rtc
don't cover the path between PWA clients and the Frigate host. That
typically happens when Frigate runs in a Docker bridge network instead of
host networking, or when clients connect from a different subnet, a VPN,
or a remote tunnel. On a pure LAN deployment with `network_mode: host` and
clients on the same subnet, the defaults usually work — try it first, and
only add the block below if WebRTC fails to negotiate (focus view stays on
the snapshot poster, watchdog fires at 3 s).

## Minimal LAN-only example

Add or merge the `go2rtc.webrtc` block in Frigate's `config.yml`:

```yaml
go2rtc:
  webrtc:
    candidates:
      - <frigate-host>:8555 # LAN address of the Frigate host
      - stun:8555           # STUN fallback (public STUN servers)
```

## Adding remote-access candidates (optional)

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

## Why these candidates

WebRTC ICE needs reachable candidate addresses for the client to connect
to go2rtc's UDP media port. Without explicit candidates, go2rtc may
advertise only addresses that are not reachable from where the client
actually sits.

| Candidate | Who it helps |
|---|---|
| LAN address | clients on the same network as the Frigate host |
| STUN srflx  | clients behind NAT (uncommon for LAN-only deployments) |

## Verify

After applying the config and restarting Frigate:

```bash
curl -s http://<frigate-host>:1984/api/config | jq '.webrtc'
# Expected: shows the candidates array above
```

If your VPN or tunnel gateway has a firewall, ensure UDP 8555 is forwarded
between client subnets and the Frigate host.

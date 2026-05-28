# Optional: Nginx Proxy Manager — Reverse Proxy Setup

This is one example of putting Skua behind a reverse proxy for HTTPS,
PWA install support, and optional remote access. The default LAN-only
deployment doesn't require a reverse proxy. Any reverse proxy works —
Nginx Proxy Manager, Caddy, Traefik, plain nginx. The settings below are
NPM-specific but transfer directly.

## Proxy Host Settings

| Field | Value |
|---|---|
| Domain names | `skua.example.com` |
| Scheme | `http` |
| Forward hostname/IP | `<docker-host>` |
| Forward port | `3200` |
| Block common exploits | ✅ |
| Websocket support | ✅ (needed for E3 SSE upgrades) |

## SSL Tab

| Field | Value |
|---|---|
| Certificate | Let's Encrypt |
| Challenge | DNS — Cloudflare API |
| Force SSL | ✅ |
| HTTP/2 support | ✅ |
| HSTS Enabled | ✅ |

## Advanced Tab — Custom Nginx config

```nginx
client_max_body_size 8m;
proxy_buffering off;        # required for SSE (E3) and future WHEP (E2)
proxy_read_timeout 3600s;   # SSE long-poll connections
```

## Verify

After saving, from a client that can reach the reverse proxy:

```bash
curl -I https://skua.example.com/healthz
# Expected: HTTP/2 200
```

# Skua

A self-hosted companion app for Frigate NVR. Fast live viewing and an events feed that groups detections into moments, on your phone or your desktop.

Skua is a focused live-and-events client for [Frigate](https://frigate.video). It's for people who run Frigate at home and want a quick, low-latency way to check their cameras day to day, without giving up Frigate's own UI for admin, Explore, the recording browser, and settings. It has no accounts and no built-in login, so you decide how it's reached: plenty of people run it on the LAN only, others put it behind their own reverse proxy with auth, a VPN, or a tunnel for remote access. It isn't a multi-tenant product and it isn't a full Frigate UI replacement.

## What Skua is

Skua does a few things and tries to do them well:

- **Live focus view.** One camera over WebRTC (WHEP), typically under 500 ms of latency. Pinch and wheel to zoom.
- **Camera grid.** JPEG snapshot tiles at 1 Hz, in full-resolution HD or a bandwidth-saving ECO mode that's resized on the server. No heavy live grid to stall the first paint.
- **Events as moments.** Nearby detections collapse into a single moment with a representative frame, so you read "the courier came by" instead of scrolling twelve near-identical rows. More on this below.
- **Recording timeline.** Scrub back through a camera's recordings and play past footage inline, reached straight from the focus view.

It installs to an iPhone, Android, or desktop home screen as a PWA. The installed app opens instantly (the shell is precached) and tells you clearly when the server is unreachable instead of hanging.

On why it feels quick: the live view uses WebRTC for sub-500 ms latency, the grid uses light snapshot tiles instead of a live MSE grid, and the installed app cold-opens from cache. On iOS in particular this sidesteps the MSE timeouts and slow first paint that make Frigate's own grid painful to check from a home-screen icon. Frigate's UI stays the right tool for everything else.

## Security and access model

> **Skua has no application-level login. By design.**
>
> It doesn't ship accounts or its own auth, so you control how it's reached. Out of the box it's meant for a trusted network, at the Docker host's address. For remote access, put it behind your own layer: a reverse proxy with auth, a VPN, or a zero-trust tunnel. Plenty of people run it that way. What you shouldn't do is expose it raw to the internet with nothing in front.
>
> This is a scope decision, not a missing feature. Threat model and hardening notes live in [SECURITY.md](SECURITY.md).

Mutating API routes also carry a `Sec-Fetch-Site` cross-site guard. That's origin hygiene against in-browser drive-by requests, not authentication.

## Moments

Frigate gives you a stream of detections. Left raw, that's a long flat list where the same event shows up as a dozen near-identical rows: one person walking past the drive can be ten entries a minute apart.

A moment is Skua's answer to that. Detections close together on the same camera (within a 5-minute gap) collapse into one moment with a representative frame. The flat list becomes a short, readable one: "someone at the front door, 4:12pm", not forty rows around 4:12pm.

When you open the app after time away, Skua shows a "while you were away" sheet: recent moments listed newest first, each tagged with its camera, the unseen ones marked, and a filter to switch between all moments and unseen only. The header carries a bell with the unseen count you can reopen anytime. Opening a moment marks only that one seen. A mark-all action clears the rest. Dismissing the sheet marks nothing. Seen-state is shared across the household, so if someone else already looked, you're not both chasing the same alert. There are no accounts.

How far back the digest looks (6 to 72 hours) and how many moments it shows live in **Settings → Appearance**.

## Features

- Live single-camera focus over WHEP, typically under 500 ms latency, with pinch and wheel zoom.
- Multi-camera grid with HD and ECO snapshot modes (1 Hz JPEG tiles, server-side resize for ECO).
- Events grouped into moments, with a "while you were away" digest on open (all/unseen filter) and a header bell showing the unseen count. Seen-state is household-shared; lookback is configurable.
- Recording timeline per camera: scrub and play back past footage inline, from the focus view.
- Real-time events list with camera, type, and group filters, plus infinite scroll.
- Inline event clip playback that works on iOS Safari (HEVC `hev1`->`hvc1` retag plus a Range-aware LRU cache).
- Storage usage screen (**Settings -> System**): disk usage per mount Frigate reports, plus a per-camera breakdown of how much of the recordings disk each camera uses, color-coded, heaviest first.
- Picture-in-Picture on desktop browsers and Android Chrome.
- Pausing the live view or a clip freezes the last frame instead of showing a black box.
- Light, dark, and auto themes, remembered per device.
- Camera groups with an in-app editor (single-membership, YAML-backed).
- Per-camera friendly names, persisted server-side.
- Per-camera go2rtc stream-source overrides, editable from Settings.
- Dynamic camera discovery from Frigate (refresh on demand, with SSE `camera.added` / `camera.removed`).
- Audio detected at runtime per camera, not from static config.
- Server-side preferences (no `localStorage`), synced across every device in the household.
- Single static Go binary, ~9 MB distroless image, multi-arch (amd64 + arm64).

## Requirements

- Frigate 0.17.1 (commit `416a9b7` verified). Skua talks to
  Frigate's internal API on port `:5000` (no auth) and to go2rtc on
  port `:1984`. The authed UI port `:8971` is not used.
- go2rtc 1.9.10 (ships with Frigate 0.17).
- Docker and Docker Compose v2 on the host (Linux, x86_64 or aarch64).
- A trusted LAN where the Docker host is reachable from the client
  devices that will use Skua.
- Each camera needs an H.264 go2rtc stream for the focus view (iOS
  Safari WebRTC does not play H.265), pointed at by `live.streams.Main`
  in the Frigate config. The go2rtc alias name is your choice, not a
  required pattern (see
  [docs/setup/frigate-config.md](docs/setup/frigate-config.md) for the
  stream-source and ICE-candidates setup).
- WebRTC live view requires iOS 17.4+ / Safari 17.4 or newer on Apple
  devices (the WHEP signalling uses `AbortSignal.any`, available from
  that version). Older iOS can load the app and use snapshot tiles but
  cannot establish the live focus stream.
- Optional: a reverse proxy with HTTPS if you want iPhone
  home-screen install (PWA install requires TLS).

## Quick start

### Run with Docker Compose

1. Create a directory for Skua and a `compose.yaml`:

```yaml
services:
  skua:
    image: ghcr.io/skua-app/skua:latest
    container_name: skua
    restart: unless-stopped
    ports:
      - "3200:3200"
    volumes:
      - ./data:/data
```

Keep the `restart: unless-stopped` line — `restart: always` works
too. Runtime reconfiguration (the first-run wizard Save and
**Settings → Connection → Apply**) restarts Skua by exiting the
process and relies on the container restart policy to bring it
back. Without one (`restart: no`, `on-failure`, or no restart key
at all), a Save or Apply leaves Skua stopped until you start the
container again.

2. Start the container:

```bash
   docker compose up -d
```

3. Open `http://<docker-host>:3200` from a device on the same network.
   On first run Skua shows a setup wizard. Enter your Frigate URL (and
   optionally the go2rtc URL and a public Frigate UI URL), click
   **Test connection** to confirm Skua can reach them, then **Save and
   start**. Skua writes the values to `/data/config.yaml` and restarts
   into the app; the grid populates within a second or two as cameras
   come online.

   Use the host or IP that resolves to your Frigate container — if
   Frigate runs in the same Docker network, `frigate` is enough
   (`http://frigate:5000` for the API, `http://frigate:1984` for
   go2rtc).

If the grid is empty or all cameras show offline, check the
Troubleshooting section below.

### Configure with environment variables instead (optional)

If you prefer declarative config in your compose file (for GitOps or
reproducible deployments), set the URLs as environment variables and
skip the wizard:

```yaml
environment:
  FRIGATE_URL: "http://frigate:5000"
  GO2RTC_URL: "http://frigate:1984"
```

Environment variables always win over the wizard's `/data/config.yaml`
overlay. A value set in `FRIGATE_URL` cannot be changed from the wizard —
the field renders read-only with a hint. To switch back to
wizard-driven config, remove the env var and restart the container.

### With HTTPS and a friendly hostname (optional)

If you want iPhone home-screen install, a real TLS cert, or a friendly
URL like `skua.example.com` instead of an IP, put Skua behind
a reverse proxy. See
[docs/setup/npm-proxy.md](docs/setup/npm-proxy.md) for an example with
Nginx Proxy Manager and
[docs/setup/pihole-dns.md](docs/setup/pihole-dns.md) for a local-DNS
example with Pi-hole. Any reverse proxy works.

## Configuration

Skua reads its configuration from environment variables. Full
descriptions live in [.env.example](.env.example). The table below is
the canonical reference.

After first run, the Frigate / go2rtc / Frigate UI URLs can also be
edited from **Settings → Connection** in the app. The form has separate
Test, Save, and Apply (restart now) actions; Save updates
`/data/config.yaml` and Apply triggers a container restart so the new
values take effect. Env-set fields render read-only with a hint —
environment variables always win over the on-disk overlay at next boot.
Apply restarts Skua by exiting the process, so the container needs a
restart policy (`restart: unless-stopped` or `always`) to come back
automatically — without one, Apply leaves Skua stopped.

| Variable                       | Default                       | Required | Notes                                                                                                                                                                                                                                                                                                    |
| ------------------------------ | ----------------------------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `FRIGATE_URL`                  | —                             | No       | Frigate internal API base URL. Use port 5000 (no auth), not `:8971` (the authed UI port). When unset, Skua serves the first-run setup wizard and persists the entered value to `/data/config.yaml`.                                                                                                      |
| `FRIGATE_UI_URL`               | falls back to `FRIGATE_URL`   | No       | Public URL of the Frigate UI, used for "Open in Frigate" deep-links from the events list. Must be reachable from the user's device. Also settable via the wizard.                                                                                                                                        |
| `GO2RTC_URL`                   | —                             | No       | go2rtc REST API for WHEP signaling. Required for WebRTC live view; BFF starts without it. Also settable via the wizard.                                                                                                                                                                                  |
| `PORT`                         | `3200`                        | No       | Port the BFF listens on.                                                                                                                                                                                                                                                                                 |
| `LOG_LEVEL`                    | `info`                        | No       | `debug` / `info` / `warn` / `error`.                                                                                                                                                                                                                                                                     |
| `LOG_FORMAT`                   | `json`                        | No       | `json` / `text`.                                                                                                                                                                                                                                                                                         |
| `ONLINE_CHECK_INTERVAL`        | `15s`                         | No       | How often the BFF polls Frigate `/api/stats` to refresh camera online status.                                                                                                                                                                                                                            |
| `HTTP_TIMEOUT`                 | `5s`                          | No       | Default timeout for outbound HTTP calls.                                                                                                                                                                                                                                                                 |
| `SHUTDOWN_TIMEOUT`             | `10s`                         | No       | Graceful-shutdown grace period.                                                                                                                                                                                                                                                                          |
| `WHEP_TIMEOUT`                 | `10s`                         | No       | Upstream timeout for go2rtc WHEP signalling.                                                                                                                                                                                                                                                             |
| `PREFS_PATH`                   | `/data/prefs.json`            | No       | File-backed user prefs store.                                                                                                                                                                                                                                                                            |
| `GROUPS_CONFIG_PATH`           | `/data/groups.yaml`           | No       | Camera-group YAML; auto-created on first `POST /api/groups`.                                                                                                                                                                                                                                             |
| `CAMERA_NAMES_CONFIG_PATH`     | `/data/camera_names.yaml`     | No       | Per-camera display name overrides; auto-created on first `PUT /api/camera-names/{cam_id}`. Cameras without an entry fall back to the Frigate-sourced name from `cameras.yaml`.                                                                                                                           |
| `CAMERAS_CONFIG_PATH`          | `/data/cameras.yaml`          | No       | Camera registry persisted from Frigate config; auto-created at first startup when Frigate is reachable. Used as a fallback snapshot when Frigate is unreachable on subsequent starts.                                                                                                                    |
| `CAPABILITIES_CONFIG_PATH`     | `/data/capabilities.yaml`     | No       | Per-camera `talk_back` / `ptz` overrides — not exposed by Frigate's config, hand-edited on the host until a future editor lands.                                                                                                                                                                         |
| `STREAM_OVERRIDES_CONFIG_PATH` | `/data/stream_overrides.yaml` | No       | Per-camera go2rtc stream-name overrides; applied only inside the WHEP handler. `GET /api/cameras` still surfaces Frigate-truth stream names.                                                                                                                                                             |
| `CAMERA_ORDER_CONFIG_PATH`     | `/data/camera_order.yaml`     | No       | Household-shared camera display order (v0.13.0). YAML list of cam_ids; auto-created on first `PUT /api/camera-order`. Applied to `GET /api/cameras` so the grid and Settings → Cameras follow the saved order; cams not yet in the file are appended in registry order.                                  |
| `RUNTIME_CONFIG_PATH`          | `/data/config.yaml`           | No       | Runtime config overlay written by the first-run wizard. Stores `frigate_url`, `frigate_ui_url`, `go2rtc_url`. Env vars above always win over this file.                                                                                                                                                  |
| `GLANCE_STATE_PATH`            | `/data/glance.json`           | No       | Household seen-state for the glance feature; JSON file holding the set of seen event ids plus a `seen_through` watermark shared across the household. Auto-created on the first `POST /api/glance/seen` or `POST /api/glance/seen-all`; missing or corrupt file means never-seen.                        |
| `AWAY_SESSION_GAP`             | `30m`                         | No       | How long a device may be inactive before the glance sheet treats the next visit as a return ("while you were away"). The BFF tracks per-device activity in memory via `POST /api/glance/heartbeat`, keyed on an httpOnly `skua_device` cookie.                                                           |
| `CLIP_MAX_MIB`                 | `256`                         | No       | Maximum per-event clip buffer size in MiB. The BFF reads each event clip into memory to serve Range requests for iOS Safari's `<video>` element; clips larger than this return 502. Raise it if long high-bitrate moments fail to play, lower it to tighten the RAM ceiling. Must be a positive integer. |

All `_PATH` variables are container-side paths. The default
`./data:/data` volume mount in the Quick Start compose maps them all
under a single host directory. That host directory must be writable by
the container's non-root uid (65532) — see the
[Container won't start / data folder stays empty](#container-wont-start--data-folder-stays-empty)
troubleshooting note below.

## Build from source

### Backend (Go)

```bash
cd backend
go run ./cmd/server     # runs against FRIGATE_URL from env
make check              # gofmt + go vet + golangci-lint + go test -race
```

Go 1.25 or newer required.

### Frontend (SvelteKit)

```bash
cd frontend
npm install
npm run dev             # Vite dev server on :5173, proxies /api/* to :3200
npm run build           # produces build/, embedded into the Go binary
npm run check           # svelte-check + tsc
```

Node 20 or newer required.

### Full Docker build

```bash
docker build -t skua:local .
docker run --rm -e FRIGATE_URL=http://<frigate-host>:5000 -p 3200:3200 skua:local
```

The Dockerfile produces a distroless single-binary image around 9 MB.
(No `--restart` policy is set here, so a wizard Save or
**Settings → Connection → Apply** will not auto-restart this container —
expected for a throwaway smoke test.)

## Limitations

- No built-in authentication or remote access. You bring your own layer for remote use: a reverse proxy with auth, a VPN, or a tunnel. See Security and access model above.
- Two-way audio (talk-back) isn't supported yet.
- The grid is JPEG snapshots, not live video, by design. The focus view is one tap away. A live grid was prototyped and removed (kept in git history).
- The live focus view needs an H.264 go2rtc stream per camera. iOS Safari rejects High-profile streams advertised as Baseline, so the alias must be a real Baseline or Main H.264 (see [docs/setup/frigate-config.md](docs/setup/frigate-config.md)).
- Event clips are served in the camera's native codec and resolution, often HEVC at 1440p or higher. Apple devices decode that in hardware. Many Android devices, especially budget SoCs, cap HEVC hardware decode at 1080p, so a high-resolution clip may not play inline there. The snapshot, Download, and Open in Frigate still work. There's no server-side transcoding, which keeps the image small.
- One Frigate instance per Skua. Multi-Frigate aggregation isn't supported.
- Built for small households, a handful of users. Not a multi-tenant product.

## Troubleshooting

### Container won't start / data folder stays empty

The Skua image is built on `gcr.io/distroless/static-debian12:nonroot` and
runs as the `nonroot` user (uid/gid 65532). A freshly created host `./data`
directory is usually owned by `root`, which means the container cannot
write to it — the BFF fails on its first write (`cameras.yaml`,
`prefs.json`, and the other YAML stores) and the data folder is left
empty, so the UI never comes up. The failure shows up in `docker logs
skua` as a `permission denied` write error.

Skua also detects this case at startup and serves a styled setup page on
its normal port instead of leaving the browser at connection-refused —
visit the Skua URL and you'll see the problem and the fix described
inline. The page polls `/healthz` in the background and auto-reloads
into the real app once you apply one of the fixes below and run `docker
compose restart skua`.

Two ways to fix it:

- **Use a Docker named volume** — let Docker manage ownership for you:

  ```yaml
  volumes:
    - skua-data:/data

  volumes:
    skua-data:
  ```

- **Keep the bind mount and `chown` the host directory once** before
  starting the container:

  ```bash
  mkdir -p ./data
  sudo chown -R 65532:65532 ./data
  docker compose up -d
  ```

`chmod 777 ./data` also works but is broader than necessary and not
recommended on a host you care about — prefer one of the two options
above.

### Grid shows no cameras / all offline

- Verify `FRIGATE_URL` is reachable from inside the Skua
  container:

  ```bash
  docker exec skua wget -qO- http://<frigate-host>:5000/api/stats
  ```

- Check the BFF logs: `docker logs skua`.
- If Frigate was unreachable on the very first start — before any
  camera snapshot has been cached to `cameras.yaml` — Skua serves one
  of two pages depending on how the URL was configured:
  - **URL came from the wizard / overlay file** (no `FRIGATE_URL` env
    var): the setup wizard renders prefilled with the current URL and
    an explanatory banner. Fix the URL in the browser, test, save, and
    Skua restarts into the working config.
  - **URL came from `FRIGATE_URL` env var**: an informational emergency
    page describes the issue and the fix is in the env file. Browser
    editing is not offered because env wins over the overlay file at
    next boot — restart the container after correcting the value.
    The same fail-fast log line is still emitted; the camera registry
    is loaded once at startup.

### PWA not installable on iPhone

- The site must be served over HTTPS. iOS Safari does not offer
  install on plain HTTP.
- It must be opened in Safari directly (not in Chrome or Firefox on
  iOS — they cannot install PWAs).
- The install banner only appears on fresh Safari sessions. Try a
  private tab to verify the manifest is being served.

### Snapshot tile shows a stale image

- Snapshots are served with `Cache-Control: no-store`. A stale frame
  indicates a Frigate or network issue rather than caching.
- Tiles refresh once per second on the client.

### Event clip won't play on Android

Skua does not transcode clips — they are served from Frigate in the
camera's native codec and resolution, typically HEVC at 1440p or
higher. Apple devices decode this in hardware, but many Android SoCs
cap HEVC hardware decode at 1080p and the clip then fails to start
inside the event modal. When that happens the modal falls back to the
event snapshot and points at the Download and Open in Frigate buttons,
which both work regardless of decoder support. There is no server-side
re-encode planned (it would defeat the small distroless single-binary
image).

### Service worker not updating

- Skua's service worker is auto-updating (`skipWaiting` +
  `clientsClaim`). A new version activates on the next launch and the
  page refreshes once so the new assets take effect. On iOS PWA this
  means updates land on the next cold launch from the home-screen icon,
  without waiting for every tab to close.

## Documentation

- [docs/api-contract.md](docs/api-contract.md) — BFF REST/SSE contract
  with TypeScript-style definitions.
- [docs/ios-clip-playback.md](docs/ios-clip-playback.md) — the three
  iOS Safari constraints on event clip playback.
- [docs/hikvision-no-web-sku.md](docs/hikvision-no-web-sku.md) —
  known-issue case study for the no-web Hikvision SKU.
- [docs/setup/](docs/setup/) — optional host-side setup (reverse
  proxy, local DNS, ICE candidates).

## Tech stack

Go 1.25 BFF (chi router, `log/slog`, single static binary), with a
SvelteKit 2 + Svelte 5 (runes) + Tailwind 4 frontend embedded into the
binary. Live video is WebRTC over WHEP through go2rtc, with no media
transcoding inside Skua. Grid tiles are JPEG snapshots resized
server-side with `golang.org/x/image/draw`. The PWA is built with
`vite-plugin-pwa` (`generateSW`, `autoUpdate` with `skipWaiting` and
`clientsClaim`), so the newest service worker activates on the next
launch and the page refreshes once to pick up the new assets. The
runtime image is distroless, around 9 MB, and multi-arch (amd64 +
arm64).

## Contributing

- License: MIT — see [LICENSE](LICENSE).
- Bug reports, feature requests, and pull requests welcome. Please
  read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a PR.
- Security issues: see [SECURITY.md](SECURITY.md) for responsible
  disclosure.
- Community conduct: [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md).

## Acknowledgments

Skua stands on top of [Frigate](https://frigate.video) and
[go2rtc](https://github.com/AlexxIT/go2rtc) — without those projects
this one would not exist. The frontend is built with
[SvelteKit](https://kit.svelte.dev) and
[Tailwind CSS](https://tailwindcss.com). Display fonts are
[Geist](https://vercel.com/font) and
[JetBrains Mono](https://www.jetbrains.com/lp/mono/). The interface
icons are drawn from [Lucide](https://lucide.dev), used under the ISC
license.
</content>
</invoke>

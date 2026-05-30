# Epic E7 — First-run setup wizard and runtime config overlay

**Status: DONE** — tagged v0.9.0

Skua no longer requires `FRIGATE_URL` in the environment to start.
When the env var is unset and no overlay file is present, the BFF
boots into a browser-based setup wizard at its normal port; the
operator enters the URLs, tests the connection, saves, and Skua
restarts into the configured app. This file captures the shape of
the system for a future maintainer; CLAUDE.md holds the day-to-day
rules.

---

## Goal

Lower the on-ramp from "set FRIGATE_URL in compose, then start the
container" to "start the container, open the browser, fill the form."
Avoid splitting the boot path into two unrelated code stacks (a setup
mode and a normal mode running side by side), and avoid a hot-reload
shape that would require every BFF client and the SSE hub to be
rebuilt mid-process.

## Scope

In scope:

- A runtime overlay file under `/data/config.yaml` for
  `frigate_url`, `frigate_ui_url`, `go2rtc_url`.
- `internal/config.Load` resolves each URL with **env > file**
  precedence and carries provenance flags + a `NeedsSetup` boolean.
  The prior "FRIGATE_URL is required" fatal is gone; the
  `LOG_LEVEL` / `LOG_FORMAT` fatal validation stays.
- A server-rendered Go wizard on the BFF's normal port. Two modes:
  initial (no values) and unreachable (prefilled + error banner).
  Restart-based reconfiguration — a successful save persists the
  file and exits the process so the container restart policy boots
  the fresh config.
- `POST /api/setup/test` (ephemeral probes against
  `frigate.GetStats` and `go2rtc.GetStreams`, never persists) and
  `POST /api/setup/save` (validates, persists, signals restart).
- Env-locked fields render read-only in the wizard and are stripped
  from save payloads server-side.

Out of scope (kept for a future patch):

- Editing the URLs from inside the SPA without a container restart
  (E7.1, deferred).
- Auto-discovery of Frigate on the local network. Deliberate: keeps
  the wizard predictable and avoids a service-discovery dependency.
- Persisting non-URL prefs (port, log level, etc.) through the
  wizard. Those stay env-only.

## Architecture

### `internal/runtimeconfig`

A thread-safe, YAML-backed store over a single overlay file
(default `/data/config.yaml`), modelled on `internal/streamoverrides`:
flat envelope, atomic temp+rename write (`MkdirAll 0o755`,
`SetIndent(2)`, `chmod 0o644`), fail-fast on malformed YAML, no
file created until the first `Save`. URL trimming is not this
layer's job — values are stored as written; `internal/config`
normalises them after merging with env.

### `internal/config`

`Load()` reads `RUNTIME_CONFIG_PATH` (default `/data/config.yaml`)
via `runtimeconfig.New`, then resolves each URL as
`firstNonEmpty(env, file)`. Trailing slashes are trimmed after the
merge. `FrigateUIURL` falls back to `FrigateURL` when neither env
nor file supplies it, matching the prior behaviour. Provenance
flags are set strictly on env presence (`os.Getenv(key) != ""`),
so file-supplied values report `*FromEnv == false`. `NeedsSetup`
is set when the resolved `FrigateURL` is empty.

### `internal/setup`

A small `http.Handler` that serves the wizard page on `GET /` (and
every non-API path, so SPA-deep-link reloads land on the wizard
during setup), 503 on `/healthz` (so the page's poll script only
reloads after the normal server replaces this handler with one
that answers 200), and three API endpoints under `/api/setup/`:

- `POST /api/setup/test` — ephemeral test-connect. Builds a
  `frigate.NewClient(url, ~3s)` and calls `GetStats`, then a
  `go2rtc.New(url, &http.Client{Timeout: ~3s})` and calls
  `GetStreams`. Returns `{frigate:{ok|skipped|error}, go2rtc:{...}}`.
  Never persists.
- `POST /api/setup/save` — validates the payload (non-empty
  `frigate_url`, scheme + host parses), drops env-locked fields
  server-side, persists via `runtimeconfig.Store.Save`, then
  closes a `restart` channel.
- `GET /api/setup/*` and any other `/api/*` — 503 JSON with
  `error: "setup_required"`.

`Serve(port, store, opts)` binds an `http.Server` and selects on
the restart channel + the listener error channel. On a restart
signal, `Shutdown` runs against a 5s context and `Serve` returns
nil so `main.go` can `os.Exit(0)`. Bind errors return non-nil and
`main.go` exits 1.

The page template evolves `internal/emergency/page.html`: same
tokens, same brackets, same pulse dot, same `/healthz`-poll-and-
reload script, plus a vanilla-JS panel with inputs, two buttons
(Test connection, Save and start), and a per-target result row.
Locked fields render as a read-only `.code` block with a "set via
X environment variable" hint and are excluded from the POST.

### `main.go` boot decision tree

After `config.Load()`:

1. Construct `runtimeconfig.Store` from `cfg.RuntimeConfigPath`.
   Failure (malformed YAML) → stderr + exit 1.
2. If `cfg.NeedsSetup` → `setup.Serve(ModeInitial, prefill=current
   overlay, locked=provenance, errMsg="")`. On return, exit 0.
3. Else, normal boot. On `cameras.New` error:
   - `fs.ErrPermission` → `emergency.Serve(ReasonDataNotWritable)`,
     exit 1 (unchanged).
   - `!cfg.FrigateURLFromEnv` → `setup.Serve(ModeUnreachable,
     prefill=current cfg URLs, locked=provenance,
     errMsg="Cannot reach Frigate at ...")`. On return, exit 0.
   - else (env-locked) → `emergency.Serve(ReasonFrigateUnreachable)`,
     exit 1 (unchanged).

The Q5=B unreachable-wizard rule: only serve the editable wizard
when the operator can actually fix the URL from the browser. When
the URL is env-locked, the informational emergency page is served
instead, because env will win at next boot regardless of what the
wizard could save.

## Why restart-based and not hot-reload

Hot-reload would mean rebuilding `frigateClient`, `eventsClient`,
`go2rtcClient`, the SSE upstream + hub, the events LRU cache, and
the HTTP router, then atomically swapping them in front of any
open client. Every one of those pieces holds state — open
connections, in-flight requests, cached blobs, subscriber sets —
and any of them re-wiring half-cleanly is a long tail of subtle
bugs. The restart-based shape gives a single clean boot path,
matches what the emergency page already relied on, and is cheap:
Docker restart policies bounce the container in well under a
second.

## Reading order for a future maintainer

1. CLAUDE.md §2 (access model, runtime config note), §5 (data
   files), §12 (E7 gotcha), §13 (epic row).
2. `backend/internal/runtimeconfig/store.go` —  shape of the
   overlay file.
3. `backend/internal/config/config.go` — `Load()` resolution and
   provenance.
4. `backend/internal/setup/server.go` — handler, endpoints,
   restart signal.
5. `backend/internal/setup/page.html` — the rendered wizard.
6. `backend/cmd/server/main.go` — boot decision tree wiring it all
   together.

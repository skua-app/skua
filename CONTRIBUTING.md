# Contributing to Skua

Thanks for considering a contribution. This document covers how to
file issues, propose changes, set up a development environment, and the
conventions the project follows.

## Code of Conduct

Everyone interacting with the project — in issues, discussions, pull
requests, and any other project space — is expected to follow the
[Code of Conduct](CODE_OF_CONDUCT.md).

## Reporting bugs

Bug reports are most useful when they let a maintainer reproduce the
problem without back-and-forth. A useful report includes:

- Skua version (image tag or commit).
- Frigate version.
- Browser plus OS, and iOS/Android version if relevant.
- Steps to reproduce.
- What you expected to happen versus what actually happened.
- BFF logs (`docker logs skua`) trimmed to the relevant window.
- Browser DevTools console errors, if any.

Please use the bug-report issue template — it asks for these
explicitly.

## Suggesting features

Open a GitHub Discussion in the Ideas category first to gauge fit:
<https://github.com/skua-app/skua/discussions>. Promote to a
tracked issue only after the design discussion converges.

Skua is intentionally a focused live + events viewer for Frigate,
not a Frigate UI replacement. Features that move it toward "Frigate UI
v2" — admin tooling, history scrubbing, the recording browser,
Explore — will be politely declined. Read the README's overview and
the "what Skua is / isn't" framing before proposing anything
substantial.

## Security issues

Do not file security issues as public GitHub issues. Use the private
vulnerability reporting workflow at
<https://github.com/skua-app/skua/security/advisories/new>.
Details and threat-model context live in [SECURITY.md](SECURITY.md).

## Development setup

The full breakdown is in the README's "Build from source" section. The
short version:

- Go 1.25 or newer for the backend.
- Node 20 or newer for the frontend.
- The BFF listens on `:3200`. The Vite dev server runs on `:5173` and
  proxies `/api/*` to the BFF.

```bash
cd backend && go run ./cmd/server     # BFF on :3200
cd frontend && npm install && npm run dev   # Vite on :5173
```

`make check` from the repo root runs the same set of checks CI runs:
`gofmt`, `go vet`, `golangci-lint`, `go test -race`, plus
`svelte-check`, `tsc`, `prettier`, and `eslint`. It must pass before
you open a PR.

## Conventions

- **Language.** English everywhere — code, comments, identifiers,
  commit messages, log messages, docs. UI strings have an English
  baseline in `frontend/src/lib/i18n/strings.ts`. The Russian backup
  file `strings.ru.ts` is not imported by the app and should not be
  edited unless you are intentionally translating a key for a future
  runtime locale-switching change.
- **Go.** `gofmt` and `golangci-lint` clean. No `interface{}` without a
  concrete reason. Do not silence the linter with `//nolint`; fix the
  finding or argue against the rule in the PR.
- **TypeScript.** `strict: true`. No `any` and no `@ts-ignore`. Use
  `unknown` and narrow.
- **Frontend.** `prettier` clean and `eslint --max-warnings 0`.
  Tailwind class order is enforced by the plugin. Svelte 5 runes only —
  no legacy `let`-based reactivity.
- **Storage.** No `localStorage` or `sessionStorage` in client code.
  Persist via the BFF `/api/prefs` endpoint or via the existing
  YAML-backed stores on the server side.
- **Commits.** Conventional Commits: `feat(scope):`, `fix(scope):`,
  `chore:`, `docs:`. Scope is one of `bff`, `frontend`, `compose`,
  `docs`. One logical change per commit; squash inside a PR if needed.
- **Pull requests.** Small and focused. Reference the issue or
  Discussion that motivated the change. Run `make check` to completion
  locally. Do not introduce new top-level dependencies without raising
  it in the PR description first.

## Architecture context

Start with the [README](README.md) for the overview and scope, then
read [docs/api-contract.md](docs/api-contract.md) for the BFF
REST/SSE contract before opening a non-trivial PR. Other useful
entries under `docs/`:

- [docs/api-contract.md](docs/api-contract.md) — the BFF REST/SSE
  contract.
- [docs/ios-clip-playback.md](docs/ios-clip-playback.md) — why the
  event clip pipeline looks the way it does.
- [docs/hikvision-no-web-sku.md](docs/hikvision-no-web-sku.md) —
  the talk-back case study.

## License

By contributing, you agree that your contribution is licensed under
the project's [MIT License](LICENSE).

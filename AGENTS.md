# PROJECT KNOWLEDGE BASE

**Generated:** 2026-06-17
**Commit:** 0e2d582
**Branch:** detached HEAD

## OVERVIEW

Neul is a Go control plane plus machine agent for declarative developer-machine management, with a Vite/React owner dashboard under `web/`. MVP scope is single-owner self-hosted, REST polling, SQLite, packaged-client onboarding, and no secrets runtime surface.

## STRUCTURE

```
neul/
|-- cmd/                 # three process entry points: neul, neul-server, neul-agent
|-- internal/            # server, agent, CLI, domain contracts, store
|-- migrations/          # embedded SQLite schema
|-- scripts/             # demo lifecycle, docs gates, package/dev helpers
|-- web/                 # React/Vite app, Vitest, Playwright, Biome
|-- docs/                # product/security contracts and QA receipts
`-- plans/               # active implementation plans; read before changing planned surfaces
```

## WHERE TO LOOK

| Task | Location | Notes |
| --- | --- | --- |
| API routes, owner auth, pairing, dashboard, resources | `internal/server` | Tests encode most endpoint contracts. |
| Agent heartbeat, reconcile, package/dotfile adapters | `internal/agent` | REST only; no websocket path. |
| User CLI, enroll/install/status/logs/reset/uninstall | `internal/cli` and `cmd/neul` | `cmd/neul/main.go` delegates to `internal/cli`. |
| Server or agent executable startup | `cmd/neul-server`, `cmd/neul-agent` | Keep process flags and env behavior tested. |
| Database schema | `migrations/`, `internal/store` | MVP schema must not grow secret tables. |
| Product/API contract | `internal/domain/contracts.md`, `docs/mvp.md` | These beat older README/design prose when they conflict. |
| Local demo behavior | `Makefile`, `scripts/demo.sh`, `scripts/verify-demo-*` | Demo checks are CI gates. |
| Web dashboard | `web/src` | React 19, strict TS, Korean-first copy contracts. |
| Browser E2E | `web/e2e` | Chromium-only Playwright config. |
| Packaging docs and unsigned macOS dev package | `scripts/build-macos-dev-pkg.sh`, README packaged-primary blocks | Dev `.pkg` is local QA only. |

## CODE MAP

| Symbol | Type | Location | Role |
| --- | --- | --- | --- |
| `server.NewRouter` | Go func | `internal/server/http.go` | Central API and SPA router. |
| `agent.Client.Tick` | Go method | `internal/agent/agent.go` | Heartbeat, desired state, command poll, report cycle. |
| `cli.Run` | Go func | `internal/cli/cli.go` | Root CLI command router. |
| `store.ApplyMigrations` | Go func | `internal/store/migrations.go` | Embedded SQLite migration runner. |
| `App` | React component | `web/src/App.tsx` | Dashboard bootstrap and setup/session state. |
| `DashboardWorkspace` | React component | `web/src/DashboardWorkspace.tsx` | Main dashboard composition. |
| `OnboardingWizard` | React component | `web/src/OnboardingWizard.tsx` | Pairing/onboarding state machine UI. |
| `loadDashboardData` | TS func | `web/src/api.ts` | Maps API dashboard shape to UI data. |

## CONVENTIONS

- Go module version is from `go.mod`; CI uses `go test ./...`.
- Web uses Node 24 and pnpm 11.5.0; install with `pnpm --dir web install --frozen-lockfile`.
- Biome owns web formatting/linting for `web/src`, `web/e2e`, and web config files; indentation is tabs.
- TypeScript is strict with `noUncheckedIndexedAccess`, `exactOptionalPropertyTypes`, and `verbatimModuleSyntax`.
- Go tests use behavior names such as `TestX_whenY_returnsZ`; keep scenario evidence explicit.
- Web unit tests live in `web/src/**/*.test.ts(x)` and use Vitest/jsdom.
- Playwright tests live in `web/e2e` and currently run the Chromium project only.
- Docs are executable contracts: update docs and validation scripts together when product copy or onboarding semantics change.

## ANTI-PATTERNS (THIS PROJECT)

- Do not add `/api/secrets`, secret tables, plaintext secret payloads, or secret UI behavior before the threat model is accepted.
- Do not add websocket routes or browser websocket behavior for MVP agent transport.
- Do not add hosted login, teams, RBAC, SSO, billing, remote terminal, or arbitrary shell execution to MVP surfaces.
- Do not introduce `/install.sh`, `curl | sh`, or copy that makes checkout-local `go run ./cmd/neul ...` the primary product path.
- Do not leak setup, pair, owner session, or machine tokens into URLs, document titles, browser history, logs, or localStorage.
- Do not commit generated `node_modules`, `web/dist`, `.demo`, Playwright traces, package artifacts, or temp QA output.
- Do not claim unsigned macOS dev packages are production-ready; production requires signing, notarization, and stapling.

## UNIQUE STYLES

- README and docs intentionally mix Korean product copy with precise command contracts.
- The primary onboarding path is packaged `neul enroll --server <origin>` plus browser approval; checkout-local commands are fallback/debug only.
- Test and docs gates assert visible UX copy, not just API behavior.
- Local runtime tooling includes `make demo` and a separate tmux-backed `scripts/dev-server`; choose the one matching the task.

## COMMANDS

```bash
go test ./...
make verify-docs
make verify-demo
pnpm --dir web test -- --run
pnpm --dir web exec biome check src index.html package.json tsconfig.json vite.config.ts pnpm-workspace.yaml playwright.config.ts vitest.config.ts biome.json e2e
pnpm --dir web build
cd web && pnpm exec playwright test e2e/smoke.spec.ts e2e/mvp-dashboard.spec.ts --project=chromium
```

## NOTES

- `make demo` starts or reuses `.demo/neul-server`; use `make demo-stop` and `make demo-clean` for cleanup.
- `scripts/demo.sh` reads the setup-token prefix from `internal/server/auth.go`; changing token output can break demo verification.
- `internal/domain/contracts.md` says it wins over conflicting README or design-doc text for MVP behavior.

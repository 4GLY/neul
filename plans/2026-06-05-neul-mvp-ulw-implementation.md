# Neul MVP ULW Implementation Plan

## TL;DR
> **Summary**: Build the first single-owner self-hosted Neul MVP loop: owner bootstrap, first machine pairing, dashboard, package/dotfile desired-state editing, agent heartbeat/reconcile reports, drift detection, and repair commands.
> **Deliverables**:
> - Go `neul-server`, `neul-agent`, and `neul` CLI skeletons with SQLite persistence.
> - REST API contracts for pairing, dashboard, resources, agent polling/reporting, and repair.
> - React dashboard migrated from static mock data to typed API data with Korean-first copy.
> - TDD evidence, real HTTP/CLI/browser QA artifacts, and commit-ready task boundaries.
> **Effort**: Large
> **Parallel**: YES - 6 waves
> **Critical Path**: Task 1 -> Task 2 -> Task 3 -> Task 4 -> Task 5 -> Task 7 -> Task 9 -> Task 10 -> Task 13 -> Task 15

## Context

### Original Request
The user requested `omo:ulw-plan` planning and asked that the written plan be reviewed by Claude CLI's Opus model.

### Interview Summary
No further user interview is required. The current product decisions are already documented:
- `docs/mvp.md:7-15`: first MVP is single-owner self-hosted, focused on machine registration, dashboard state, drift repair, package/dotfile desired state, and secrets deferred.
- `docs/mvp.md:21-32`: web creates pairing code and CLI claims it.
- `docs/mvp.md:55-67`: drift is based on agent reports; repair creates a reconcile command without mutating desired state.
- `docs/mvp.md:69-89`: package schema accepts `brew`, `apt`, and `mise`; executable adapter work starts with Homebrew.
- `docs/mvp.md:91-109`: dotfiles are limited to HOME allowlisted paths.
- `docs/mvp.md:113-135`: secret UI/API is out except disabled/coming-soon affordances.
- `docs/mvp.md:205-263`: web/server/agent/CLI boundaries are defined; agent uses HTTPS outbound REST only.
- `README.md` and `docs/2026-05-27-design.md` are background/vision documents only when they conflict with `docs/mvp.md`; `docs/mvp.md` wins for MVP execution.

### Metis Review (gaps addressed)
Metis identified these guardrails and this plan incorporates them:
- Canonical endpoint naming: agent-owned routes use `/api/agent/*`; machine/dashboard routes use `/api/machines/*` and `/api/dashboard`.
- Pairing direction: web creates pairing code, CLI claims it.
- Auth bootstrap: single-owner local session and machine tokens are explicit MVP tasks.
- No WebSocket/server push: agent polls desired state and pending commands through REST.
- Homebrew-first execution: `apt` and `mise` may exist in schema/API but agent marks them unsupported until later.
- Dotfile safety: normalized HOME-relative allowlist matching, symlink escape prevention, and hostile-path tests are mandatory.
- Secrets disabled: `/api/secrets` returns disabled/not found, no dashboard/ledger secret rows, nav item disabled only.
- SQLite: pure-Go `modernc.org/sqlite` is selected for easier local/CI portability; foreign keys are enabled per connection.
- Test infra: Go module, Vitest, Playwright, and evidence directory are first-class foundation tasks.
- Browser QA: final QA targets the server-served SPA/API, not only Vite mock mode.
- Late Metis additions incorporated: browser API auth uses a first-run local setup token exchanged for an HttpOnly owner session cookie; machine auth uses bearer machine tokens only in MVP while CLI-generated keypairs are reserved for future signing; heartbeat interval is 30 seconds and offline threshold is 5 minutes; pairing TTL is 10 minutes; JSON errors use `{ "error": { "code": "...", "message": "..." } }`; one-way migrations are accepted for MVP; report ingestion and repair creation are transactional and idempotent.

### Claude Opus Review
Status: APPROVE on fourth pass. Review outputs saved to `evidence/plan-claude-opus-review.md`, `evidence/plan-claude-opus-review-2.md`, `evidence/plan-claude-opus-review-3.md`, and `evidence/plan-claude-opus-review-4.md`. Blocking findings were incorporated: critical path fixed, CI included, service install locked to dry-run only, `/api/secrets` fixed to `404 Not Found`, browser auth defined as first-run setup-token unlock plus HttpOnly session cookie, owner-facing QA standardized on cookie jar auth, report and repair idempotency locked to `Idempotency-Key`, and missing acceptance criteria added for hashing, TTL, offline threshold, heartbeat interval, Korean-copy allowlist, repair command ack, and cleanup receipts.

## Work Objectives

### Core Objective
Implement the MVP loop with no judgment calls left to executors: a single-owner user can pair a first machine, see it on the dashboard, edit package/dotfile desired state, receive agent reports, detect drift, and request repair.

### Deliverables
- Root Go module and three binaries: `cmd/neul-server`, `cmd/neul-agent`, `cmd/neul`.
- SQLite schema and migration runner.
- Typed domain contracts and REST handlers.
- Agent one-tick and loop behavior for heartbeat, desired-state polling, command polling, dry-run reports, and Homebrew-first adapter behavior.
- CLI pairing plus MVP-safe `agent install`, `agent status`, and `agent logs` commands.
- Web API client, Korean-first copy, loading/error/empty states, desired-state editor, and real dashboard wiring.
- E2E scenario and evidence artifacts for HTTP, tmux/CLI, and browser QA.

### Definition of Done (verifiable conditions with commands)
- `go test ./...` passes from repo root.
- `cd web && pnpm test -- --run` passes.
- `cd web && pnpm exec biome check src index.html package.json tsconfig.json vite.config.ts pnpm-workspace.yaml` passes.
- `cd web && pnpm build` passes.
- `cd web && pnpm exec playwright test e2e/smoke.spec.ts e2e/mvp-dashboard.spec.ts --project=chromium` passes against `neul-server` serving the built SPA/API with a temp SQLite DB.
- Evidence files exist under `evidence/` for every RED output, GREEN output, HTTP/CLI/browser QA run, Claude Opus review, and final cleanup receipt.

### Must Have
- TDD for every production code task: capture RED before implementation, then GREEN after implementation.
- Real manual QA per task through HTTP, tmux, or browser with concrete commands and binary observables.
- Single-owner bootstrap auth; no anonymous mutation routes.
- Agent polling/reporting over REST only.
- Machine tokens are hashed at rest and revocable.
- Pairing codes are opaque, expiring, hashed at rest, and single-use.
- Pairing code TTL is exactly 10 minutes; expired claims return `410 Gone` with JSON error code `pairing_code_expired`.
- Owner browser API auth uses a generated one-time setup token exchanged through `POST /api/session/local` for an HttpOnly same-site session cookie; API tests use a cookie jar or test helper. Agent API auth uses bearer machine tokens; CLI keypair generation is not used for request signing in MVP.
- Machine heartbeat interval is 30 seconds; machine is `offline` when last heartbeat is older than 5 minutes.
- Desired-state pending is computed by per-resource `desired_version` versus latest machine `applied_version`.
- Reconcile report ingestion and repair command creation are transactionally idempotent by client-provided `Idempotency-Key` HTTP header.
- Dotfile paths are HOME-relative after canonicalization and cannot escape via `..` or symlinks.
- Secret functionality is disabled and testable as disabled.
- Korean-first UI copy except CLI commands, protocol fields, package names, file paths, and API paths.

### Must NOT Have
- No WebSocket push.
- No hosted tier, teams, RBAC, SSO, billing, or GitHub OAuth.
- No secret value editing, E2E secret crypto, `/api/secrets` mutations, or secret rows in dashboard/ledger.
- No root/system path mutation, arbitrary shell command resource, remote terminal execution, or `/etc`/`/usr` writes.
- No full apt/mise implementation before Homebrew is tested through E2E.
- No real launchd/systemd service installation in MVP; `neul agent install --dry-run` is the only install surface.
- No reliance on README or `docs/2026-05-27-design.md` scope when those docs mention hosted, WebSocket, secrets, Postgres/S3, cargo/pipx, or Docker deployment.
- No source-code changes outside the task's named files.

## Verification Strategy
> ZERO HUMAN INTERVENTION - all verification is agent-executed.
- Test decision: TDD (RED-GREEN-REFACTOR) with Go `testing`, Vitest, and Playwright.
- QA policy: Every task has agent-executed scenarios.
- Evidence root: `evidence/`.
- Required evidence naming: `evidence/task-{N}-{slug}-red.txt`, `evidence/task-{N}-{slug}-green.txt`, `evidence/task-{N}-{slug}-qa.{txt|json|png}`.
- Manual-QA channels:
  - HTTP call: `curl -i` against a running `neul-server`.
  - tmux: run CLI/agent commands in `tmux new-session -d -s ulw-qa-task-{N}` and capture transcript.
  - Browser use: Playwright drives the real server-served app and saves screenshot/action log.

## Execution Strategy

### Parallel Execution Waves
Wave 1: Tasks 1-3 establish contracts, test infrastructure, storage, and domain primitives.
Wave 2: Tasks 4-7 implement server APIs and auth/pairing/resource/report contracts.
Wave 3: Tasks 8-10 implement CLI and agent surfaces.
Wave 4: Tasks 11-13 migrate the web prototype to real API data and Korean-first copy.
Wave 5: Tasks 14-16 implement E2E, hardening, and lightweight CI.
Wave 6: Final verification and review.

### Dependency Matrix
| Task | Blocks | Blocked By |
| --- | --- | --- |
| 1. Contracts and copy map | 2, 4, 11 | None |
| 2. Test/build infrastructure | all code tasks | None |
| 3. Domain and SQLite foundation | 4, 5, 6, 7, 8, 10 | 1, 2 |
| 4. Owner bootstrap and auth | 5, 6, 7, 11, 15 | 1, 2, 3 |
| 5. Pairing and machine registration | 8, 10, 15 | 1, 2, 3, 4 |
| 6. Dashboard read API | 11, 13, 15 | 1, 2, 3, 4 |
| 7. Desired-state resources API | 10, 12, 13, 15 | 1, 2, 3, 4 |
| 8. CLI pairing/install/status/logs | 10, 15 | 2, 3, 4, 5 |
| 9. Agent desired-state and command polling | 10, 15 | 2, 3, 4, 5, 7 |
| 10. Agent reconcile reports and Homebrew-first adapter | 13, 15 | 2, 3, 5, 7, 8, 9 |
| 11. Web API client and dashboard state | 12, 13, 15 | 1, 2, 4, 6 |
| 12. Desired-state editor UI | 13, 15 | 1, 2, 7, 11 |
| 13. Repair drift and activity UX wiring | 15 | 6, 7, 10, 11, 12 |
| 14. Security and scope regression tests | 15 | 4, 5, 7, 10, 13 |
| 15. Real E2E MVP scenario | final verification | 5-14 |
| 16. CI and release hygiene | final verification | 2, 15 |

## TODOs
> Implementation + Test = ONE task. Never separate.
> EVERY task MUST have References + Acceptance Criteria + QA Scenarios.

- [x] 1. Lock contracts, endpoint names, auth defaults, and Korean-first copy map

  **What to do**: Create `internal/domain/contracts.md` and `web/src/copy.ts`. Canonicalize endpoints: web/server reads use `/api/dashboard`, `/api/machines`, `/api/machines/:machineId`, `/api/resources/*`; machine commands use `POST /api/machines/:machineId/repair-drift`; auth uses `POST /api/session/local`; pairing uses `/api/pair/init`, `/api/pair/claim`, `/api/pair/poll`; agent uses `/api/agent/heartbeat`, `/api/agent/desired-state`, `/api/agent/commands`, `/api/agent/reconcile-report`, `/api/agent/drift-report`. Define request/response shapes for dashboard metrics, machine rows, machine detail with `events[]`, resource rows, pairing, heartbeat, command polling, repair command, and disabled secrets. Define `/api/secrets` as `404 Not Found`. Define JSON error shape exactly as `{ "error": { "code": "...", "message": "..." } }`. Lock defaults: one-time setup token exchanged for HttpOnly owner session cookie; machine bearer token for agent APIs; 10-minute pairing TTL; 30-second heartbeat interval; 5-minute offline threshold; per-resource `desired_version`/`applied_version` pending logic; `Idempotency-Key` HTTP header for reports and repair commands. Move visible copy keys into Korean-first labels and define explicit English allowlist for component tests: CLI commands, package names, API paths, protocol fields, OS names, semantic status enum values.

  **Must NOT do**: Do not implement handlers or React behavior. Do not introduce WebSocket routes. Do not add `/api/secrets` mutation contracts.

  **Parallelization**: Can Parallel: YES | Wave 1 | Blocks: 2, 4, 11 | Blocked By: None

  **References**:
  - Product scope: `docs/mvp.md:265-298` - current API sketch to canonicalize.
  - UI copy policy: `docs/mvp.md:137-139` - Korean-first copy rule.
  - Current English UI copy: `web/src/App.tsx:100-129`, `web/src/FleetPanels.tsx:202-208`, `web/src/SidePanel.tsx:461-470`.
  - Conflict guardrail: `README.md:57-65` and `docs/2026-05-27-design.md:121-152` are vision/background only where they conflict with `docs/mvp.md`.

  **Acceptance Criteria**:
  - [ ] `rg -n "WebSocket|/ws" internal/domain/contracts.md docs/mvp.md plans/*.md` shows WebSocket only as an exclusion.
  - [ ] `rg -n "/api/session/local|/api/agent/commands|/api/pair/init|/api/dashboard|/api/machines/:machineId/repair-drift|/api/resources/package|/api/resources/dotfile" internal/domain/contracts.md` finds all canonical endpoints.
  - [ ] `rg -n "pairing_code_expired|offline.*5 minutes|heartbeat.*30 seconds|Idempotency-Key|desired_version|applied_version|/api/secrets.*404|English allowlist" internal/domain/contracts.md web/src/copy.ts` finds all locked defaults.
  - [ ] `rg -n "dashboard|pairing|repairDrift|disabledSecrets" web/src/copy.ts` finds copy keys.

  **QA Scenarios**:
  ```
  Scenario: Contract coverage scan
    Tool: bash
    Steps: rg -n "/api/agent/commands|/api/pair/init|/api/dashboard|disabledSecrets" internal/domain/contracts.md web/src/copy.ts
    Expected: all four endpoint/copy surfaces are present.
    Evidence: evidence/task-1-contracts-qa.txt

  Scenario: WebSocket exclusion scan
    Tool: bash
    Steps: rg -n "WebSocket|/ws" internal/domain/contracts.md docs/mvp.md plans/*.md
    Expected: no implementation route is present; only exclusion language is present.
    Evidence: evidence/task-1-contracts-exclusion.txt
  ```

  **Commit**: YES | Message: `docs(contracts): lock MVP API and copy contracts` | Files: `internal/domain/contracts.md`, `web/src/copy.ts`

- [x] 2. Establish test/build infrastructure and evidence harness

  **What to do**: Add root `go.mod` using Go 1.22+; choose pure-Go `modernc.org/sqlite` for SQLite portability; add `internal/testutil/evidence.go` for writing QA artifacts; add Vitest config and `web/package.json` scripts `test`, `test:run`, and `e2e`; add Playwright config with chromium project; add deterministic `web/e2e/smoke.spec.ts` that asserts `expect(true).toBeTruthy()`; add `web/biome.json`; keep existing `build` behavior.

  **Must NOT do**: Do not create production handlers beyond smoke fixtures. Do not rely on `pnpm test` before adding the script.

  **Parallelization**: Can Parallel: YES | Wave 1 | Blocks: all code tasks | Blocked By: None

  **References**:
  - Existing scripts: `web/package.json:6-9`.
  - Current dependencies: `web/package.json:19-23`.
  - Strict TS config: `web/tsconfig.json`.
  - External: `https://pkg.go.dev/testing` - Go test conventions.
  - External: `https://main.vitest.dev/guide/` - Vitest setup.
  - External: `https://playwright.dev/docs/test-configuration` - Playwright config.

  **Acceptance Criteria**:
  - [ ] `go test ./...` passes with at least one smoke test.
  - [ ] `cd web && pnpm test -- --run` passes with at least one smoke test.
  - [ ] `cd web && pnpm exec playwright test e2e/smoke.spec.ts --project=chromium --list` lists one smoke test in the chromium project.
  - [ ] `mkdir -p evidence && test -d evidence` succeeds and `.gitignore` excludes transient DB/build artifacts but not evidence text files.

  **QA Scenarios**:
  ```
  Scenario: Test infra smoke
    Tool: bash
    Steps: go test ./... && cd web && pnpm test -- --run && pnpm exec playwright test e2e/smoke.spec.ts --project=chromium --list
    Expected: all commands exit 0 and Playwright lists the smoke test under chromium.
    Evidence: evidence/task-2-test-infra-qa.txt

  Scenario: Missing test script regression
    Tool: bash
    Steps: cd web && pnpm run test -- --run
    Expected: exits 0; no "Missing script" output.
    Evidence: evidence/task-2-test-script-qa.txt
  ```

  **Commit**: YES | Message: `test: add MVP test infrastructure` | Files: `go.mod`, `go.sum`, `internal/testutil/*`, `web/package.json`, `web/pnpm-lock.yaml`, `web/vitest.config.ts`, `web/playwright.config.ts`, `web/e2e/smoke.spec.ts`, `web/biome.json`, `.gitignore`

- [x] 3. Implement domain models, migration runner, and SQLite schema

  **What to do**: Create typed domain models for owner, machine, resource, report, command, and status computation. Add SQLite migration runner and `migrations/001_initial.sql` with `owners`, `sessions`, `pairing_codes`, `machine_tokens`, `machines`, `profiles`, `segments`, `resources`, `file_versions`, `reconcile_runs`, `reconcile_events`, `agent_reports`, `agent_commands`, and idempotency columns/indexes for report ingestion and command creation. Enable foreign keys on every connection. MVP migrations are one-way; document this in `internal/store/migrations.go` or `internal/domain/contracts.md`.

  **Must NOT do**: Do not add hosted/team/RBAC tables. Do not add `secrets` or secret-value tables.

  **Parallelization**: Can Parallel: NO | Wave 1 | Blocks: 4-10 | Blocked By: 1, 2

  **References**:
  - Schema needs: `docs/mvp.md:223-239`, `docs/mvp.md:265-298`.
  - Current implementation plan table list: `docs/superpowers/plans/2026-06-05-neul-mvp-implementation.md`.
  - External: `https://www.sqlite.org/foreignkeys.html` - enable FK per connection.

  **Acceptance Criteria**:
  - [ ] `go test ./internal/domain ./internal/store -run 'TestMachineStatus|TestMigrations|TestForeignKeys' -count=1` passes.
  - [ ] Migration test asserts all MVP tables exist and no table named `secrets` exists.
  - [ ] Foreign-key test fails insert of orphan `machine_tokens`.
  - [ ] Idempotency test proves duplicate report key does not duplicate `reconcile_runs`.

  **QA Scenarios**:
  ```
  Scenario: Migration table inspection
    Tool: bash
    Steps: go test ./internal/store -run TestMigrationsCreateMvpTables -count=1
    Expected: PASS and output names all MVP tables.
    Evidence: evidence/task-3-schema-green.txt

  Scenario: Secret table exclusion
    Tool: bash
    Steps: go test ./internal/store -run TestMigrationsDoNotCreateSecretTables -count=1
    Expected: PASS because no secrets table exists.
    Evidence: evidence/task-3-schema-secret-exclusion.txt
  ```

  **Commit**: YES | Message: `feat(store): add MVP SQLite schema` | Files: `go.mod`, `go.sum`, `internal/domain/*`, `internal/store/*`, `migrations/001_initial.sql`

- [x] 4. Add server shell, owner bootstrap, local auth, and static SPA serving

  **What to do**: Create `cmd/neul-server/main.go`, `internal/server/http.go`, `internal/server/auth.go`, and tests. On first start with an empty DB, create one owner/profile/base segment and a one-time local setup token. Print the setup token once to stdout and persist only its hash. Browser first-run unlock posts the setup token to `POST /api/session/local`; server sets an HttpOnly same-site owner session cookie and marks the setup token consumed. Serve API routes under `/api` and built SPA assets for non-API paths.

  **Must NOT do**: Do not add Magic Link, GitHub OAuth, hosted tier, RBAC, or anonymous mutation access.

  **Parallelization**: Can Parallel: NO | Wave 2 | Blocks: 5-7, 11, 15 | Blocked By: 1, 2, 3

  **References**:
  - README app shape: `README.md:70-88`.
  - Server responsibilities: `docs/mvp.md:223-239`.
  - External: `https://pkg.go.dev/net/http`.
  - External: `https://pkg.go.dev/net/http/httptest`.

  **Acceptance Criteria**:
  - [ ] `go test ./internal/server -run 'TestBootstrap|TestAuth|TestStaticSpa' -count=1` passes.
  - [ ] Mutating API call without auth returns `401 Unauthorized`.
  - [ ] Setup token hash is stored at rest; plaintext setup token is never stored.
  - [ ] Test captures stdout and proves setup token is printed exactly once on first boot and not printed on subsequent boots with the same DB.
  - [ ] `POST /api/session/local` with setup token sets HttpOnly same-site cookie and consumes the setup token.
  - [ ] `GET /api/healthz` returns `200 OK`.
  - [ ] `GET /` returns built SPA HTML when server is pointed at `web/dist`.

  **QA Scenarios**:
  ```
  Scenario: Health endpoint
    Tool: HTTP call
    Steps: start neul-server with temp DB, then curl -i http://127.0.0.1:<port>/api/healthz
    Expected: HTTP/1.1 200 OK and body contains {"ok":true}.
    Evidence: evidence/task-4-health-http.txt

  Scenario: Unauthorized mutation
    Tool: HTTP call
    Steps: curl -i -X POST http://127.0.0.1:<port>/api/pair/init
    Expected: HTTP/1.1 401 Unauthorized.
    Evidence: evidence/task-4-auth-http.txt

  Scenario: First-run browser unlock
    Tool: HTTP call
    Steps: start neul-server with temp DB; capture setup token from stdout; curl -i -c evidence/task-4-cookie.jar -X POST /api/session/local with token.
    Expected: HTTP/1.1 204 No Content, Set-Cookie has HttpOnly and SameSite, and reusing setup token returns 409.
    Evidence: evidence/task-4-session-local-http.txt
  ```

  **Commit**: YES | Message: `feat(server): add single-owner server shell` | Files: `cmd/neul-server/*`, `internal/server/*`, `internal/store/*`

- [x] 5. Implement pairing and machine registration

  **What to do**: Implement web-created pairing with `POST /api/pair/init`, `POST /api/pair/claim`, and `GET /api/pair/poll`. Pairing code is opaque, expires after exactly 10 minutes, is hashed at rest, and is single-use. Claim creates machine metadata and issues a machine bearer token; store only token hash. CLI-generated machine keypairs are future scope and must not be required for MVP request signing.

  **Must NOT do**: Do not use CLI-created pairing code. Do not store plaintext pairing codes or machine tokens.

  **Parallelization**: Can Parallel: NO | Wave 2 | Blocks: 8, 9, 15 | Blocked By: 1, 2, 3, 4

  **References**:
  - Pairing flow: `docs/mvp.md:21-32`.
  - API endpoints: `docs/mvp.md:267-273`.
  - Contracts: `internal/domain/contracts.md`.

  **Acceptance Criteria**:
  - [ ] `go test ./internal/server -run 'TestPairInit|TestPairClaim|TestPairPoll' -count=1` passes.
  - [ ] Pairing init test freezes time and asserts expiry is exactly created_at + 10 minutes.
  - [ ] Reusing a code returns `409 Conflict`.
  - [ ] Expired code returns `410 Gone` and JSON body contains `{"error":{"code":"pairing_code_expired", ...}}`.
  - [ ] DB inspection shows only hashes, no plaintext code/token.

  **QA Scenarios**:
  ```
  Scenario: Pair and claim machine
    Tool: HTTP call
    Steps: curl -i -b evidence/task-4-cookie.jar -X POST /api/pair/init; then curl -i -X POST /api/pair/claim with code and machine metadata.
    Expected: init returns 201 with code/expiresAt; claim returns 201 with machineId and machineToken once.
    Evidence: evidence/task-5-pairing-http.txt

  Scenario: Reuse pairing code
    Tool: HTTP call
    Steps: repeat the same /api/pair/claim payload.
    Expected: HTTP/1.1 409 Conflict and body contains code_used.
    Evidence: evidence/task-5-pairing-reuse-http.txt
  ```

  **Commit**: YES | Message: `feat(server): add machine pairing API` | Files: `internal/server/handlers_pairing.go`, `internal/server/handlers_pairing_test.go`, `internal/store/*`, `internal/domain/*`

- [x] 6. Implement dashboard, machine read, events, and repair command APIs

  **What to do**: Implement `GET /api/dashboard`, `GET /api/machines`, `GET /api/machines/:machineId`, and `POST /api/machines/:machineId/repair-drift`. `GET /api/machines/:machineId` includes `events[]` for the `Open logs` MVP surface; no separate logs streaming endpoint exists. Dashboard returns metric cards, machine rows, activity feed, selected/representative desired-live ledger rows, and latest reconcile state. Compute status from heartbeat freshness, drift, pending, and blocked counts. Heartbeat older than 5 minutes means `offline`. Pending is computed from resource `desired_version` versus latest per-machine `applied_version`. Repair drift requires owner cookie auth, accepts an idempotency key, creates one queued `repair_drift` agent command scoped to drifted resources, and returns `202 Accepted`.

  **Must NOT do**: Do not let the web compute server-owned statuses from raw tables. Do not return secret rows. Do not stream logs. Do not create duplicate repair commands for the same idempotency key.

  **Parallelization**: Can Parallel: YES | Wave 2 | Blocks: 11, 13, 15 | Blocked By: 1, 2, 3, 4

  **References**:
  - Dashboard spec: `docs/mvp.md:34-53`, `docs/mvp.md:150-204`.
  - Repair behavior: `docs/mvp.md:55-67`, `docs/mvp.md:293-298`.
  - Logs behavior: `docs/mvp.md:186-192`.
  - Current UI composition: `web/src/App.tsx:93-165`.
  - Current type shape: `web/src/types.ts`.

  **Acceptance Criteria**:
  - [ ] `go test ./internal/server -run 'TestDashboard|TestListMachines|TestGetMachine|TestRepairDrift' -count=1` passes.
  - [ ] Status tests assert heartbeat at now-4m59s is not offline and heartbeat at now-5m01s is offline.
  - [ ] Empty fleet response includes first-machine CTA metadata.
  - [ ] Dashboard response never contains resource kind `secret`.
  - [ ] `GET /api/machines/:machineId` includes recent `events[]` used by `Open logs`.
  - [ ] `POST /api/machines/:machineId/repair-drift` returns `401 Unauthorized` without owner cookie.
  - [ ] `POST /api/machines/:machineId/repair-drift` returns `202 Accepted` with an idempotency key and creates exactly one queued `repair_drift` command.
  - [ ] Duplicate repair request with the same idempotency key returns the original queued command without inserting another row.
  - [ ] No `/api/fleet/status` endpoint is needed or referenced by the web; `GET /api/dashboard` is canonical for metric aggregation.

  **QA Scenarios**:
  ```
  Scenario: Dashboard with one drifted machine
    Tool: HTTP call
    Steps: seed temp DB with one machine/report; curl -i -b evidence/task-4-cookie.jar /api/dashboard
    Expected: HTTP 200, metrics.drifted == 1, machine status == drifted.
    Evidence: evidence/task-6-dashboard-http.txt

  Scenario: Empty dashboard CTA
    Tool: HTTP call
    Steps: start fresh DB; curl -i -b evidence/task-4-cookie.jar /api/dashboard
    Expected: HTTP 200 and body contains emptyState.action == "create_pairing_code".
    Evidence: evidence/task-6-dashboard-empty-http.txt

  Scenario: Repair drift command creation
    Tool: HTTP call
    Steps: curl -i -b evidence/task-4-cookie.jar -H "Idempotency-Key: repair-test-1" -X POST http://127.0.0.1:<port>/api/machines/<machine-id>/repair-drift twice.
    Expected: both calls return HTTP 202 with the same command id, and DB/API shows one queued repair_drift command.
    Evidence: evidence/task-6-repair-drift-http.txt
  ```

  **Commit**: YES | Message: `feat(server): add dashboard and repair APIs` | Files: `internal/server/handlers_dashboard.go`, `internal/server/handlers_machines.go`, `internal/server/handlers_reconcile.go`, tests

- [x] 7. Implement package and dotfile desired-state resources API

  **What to do**: Implement `GET /api/resources`, `POST /api/resources/package`, `POST /api/resources/dotfile`, `PATCH /api/resources/:resourceId`, and `DELETE /api/resources/:resourceId`. Package schema accepts `brew`, `apt`, `mise`; agent support status is `supported` for `brew` and `unsupported` for `apt`/`mise` until later. Dotfile path validation expands HOME, normalizes paths, rejects symlink escape, rejects `..` escape, and allows only `~/.zshrc`, `~/.gitconfig`, and `~/.config/**`.

  **Must NOT do**: Do not mutate root/system paths. Do not implement secrets. Do not accept arbitrary shell commands.

  **Parallelization**: Can Parallel: YES | Wave 2 | Blocks: 9, 10, 12, 15 | Blocked By: 1, 2, 3, 4

  **References**:
  - Package scope: `docs/mvp.md:69-89`.
  - Dotfile scope: `docs/mvp.md:91-109`.
  - Explicit exclusions: `docs/mvp.md:311-320`.

  **Acceptance Criteria**:
  - [ ] `go test ./internal/server -run TestResources -count=1` passes.
  - [ ] `/etc/hosts` and `~/.config/../.ssh/id_rsa` are rejected.
  - [ ] Dotfile symlink escape test is rejected.
  - [ ] `apt` and `mise` resources can be stored but return `agentSupport: unsupported`.

  **QA Scenarios**:
  ```
  Scenario: Create Homebrew package resource
    Tool: HTTP call
    Steps: curl -i -b evidence/task-4-cookie.jar -X POST /api/resources/package -d '{"name":"kubectl","sourceKind":"brew","desiredVersion":"latest","targetSegment":"base"}'
    Expected: HTTP 201 and body contains sourceKind brew plus agentSupport supported.
    Evidence: evidence/task-7-package-http.txt

  Scenario: Reject hostile dotfile path
    Tool: HTTP call
    Steps: curl -i -b evidence/task-4-cookie.jar -X POST /api/resources/dotfile -d '{"path":"~/.config/../.ssh/id_rsa","content":"x","mode":"0644","applyMode":"copy","targetSegment":"base"}'
    Expected: HTTP 400 and body contains path_not_allowed.
    Evidence: evidence/task-7-dotfile-reject-http.txt
  ```

  **Commit**: YES | Message: `feat(server): add desired-state resources API` | Files: `internal/server/handlers_resources.go`, tests, `internal/domain/resource.go`

- [x] 8. Implement CLI pairing, agent install, status, and logs

  **What to do**: Create `cmd/neul/main.go` and `internal/cli/*`. Implement `neul init --pair <code> --server <url>`, `neul agent install --dry-run`, `neul agent status`, and `neul agent logs`. Pairing writes machine id and machine bearer token under an overridable config directory for tests with `0600` permissions. `agent install --dry-run` prints the launchd/systemd command that would be installed without mutating the host. Real launchd/systemd service installation is out of MVP. Logs/status may read local files for MVP but must not require a GUI.

  **Must NOT do**: Do not implement desired-state editing in CLI. Do not print machine token after initial storage. Do not install launchd/systemd services.

  **Parallelization**: Can Parallel: YES | Wave 3 | Blocks: 10, 15 | Blocked By: 2, 3, 4, 5

  **References**:
  - CLI responsibilities: `docs/mvp.md:254-263`.
  - Pairing flow: `docs/mvp.md:21-32`.

  **Acceptance Criteria**:
  - [ ] `go test ./internal/cli -run 'TestInitPair|TestAgentInstallDryRun|TestAgentStatus|TestAgentLogs' -count=1` passes.
  - [ ] `neul init --pair bad --server <url>` reports a clear claim failure.
  - [ ] Config file has machine id and token, with permissions `0600`.

  **QA Scenarios**:
  ```
  Scenario: CLI pairs with fake server
    Tool: tmux
    Steps: tmux new-session -d -s ulw-qa-task-8 'NEUL_CONFIG_DIR=$(mktemp -d) go run ./cmd/neul init --pair abc123 --server http://127.0.0.1:<fake-port>'; capture pane.
    Expected: transcript contains "Machine paired" and config file exists.
    Evidence: evidence/task-8-cli-pair-tmux.txt

  Scenario: CLI rejects invalid code
    Tool: tmux
    Steps: run `go run ./cmd/neul init --pair expired --server http://127.0.0.1:<fake-port>` against server returning 410.
    Expected: nonzero exit and transcript contains "pairing code expired".
    Evidence: evidence/task-8-cli-invalid-tmux.txt

  Scenario: Agent install dry run
    Tool: tmux
    Steps: run `go run ./cmd/neul agent install --dry-run --config <test-config>` in tmux.
    Expected: transcript contains launchd or systemd install preview and no service file is written outside temp dir.
    Evidence: evidence/task-8-agent-install-dry-run-tmux.txt
  ```

  **Commit**: YES | Message: `feat(cli): add machine pairing commands` | Files: `cmd/neul/*`, `internal/cli/*`

- [x] 9. Implement agent heartbeat, desired-state polling, and command polling

  **What to do**: Create `cmd/neul-agent/main.go` and `internal/agent/*`. Implement one tick and loop: send heartbeat every 30 seconds by default, fetch `/api/agent/desired-state`, fetch `/api/agent/commands`, execute only recognized dry-run commands, and ack command states. Commands include `reconcile_now` and `repair_drift`.

  **Must NOT do**: Do not use WebSocket. Do not execute arbitrary shell command payloads. Do not apply package/dotfile changes yet beyond dry-run report generation.

  **Parallelization**: Can Parallel: YES | Wave 3 | Blocks: 10, 15 | Blocked By: 2, 3, 4, 5, 7

  **References**:
  - Agent responsibilities: `docs/mvp.md:241-252`.
  - No direct inbound: `docs/mvp.md:252`.
  - Command polling contract: `internal/domain/contracts.md`.

  **Acceptance Criteria**:
  - [ ] `go test ./internal/agent -run 'TestAgentTick|TestCommandPolling|TestNoWebSocket' -count=1` passes.
  - [ ] Agent config default heartbeat interval is exactly 30 seconds.
  - [ ] Agent tick sends heartbeat and command poll over HTTP.
  - [ ] Unknown command returns skipped/unsupported report, not execution.

  **QA Scenarios**:
  ```
  Scenario: Agent one tick
    Tool: tmux
    Steps: run `go run ./cmd/neul-agent --once --config <test-config>` against a fake server recording requests; capture transcript.
    Expected: fake server saw heartbeat, desired-state, commands, and report requests exactly once.
    Evidence: evidence/task-9-agent-tick-tmux.txt

  Scenario: Unknown command is not executed
    Tool: tmux
    Steps: fake server returns command type "shell"; run agent --once.
    Expected: transcript/report contains unsupported_command and no shell execution marker file exists.
    Evidence: evidence/task-9-agent-unknown-command-tmux.txt
  ```

  **Commit**: YES | Message: `feat(agent): poll desired state and commands` | Files: `cmd/neul-agent/*`, `internal/agent/*`

- [x] 10. Implement reconcile reports, drift reports, and Homebrew-first adapter behavior

  **What to do**: Implement report ingestion server handlers and agent report generation. Report ingestion must be transactional and idempotent by `Idempotency-Key` HTTP header. For adapter behavior, dry-run all kinds first; then implement Homebrew check/apply behind a testable adapter interface. Unit/integration tests use fake Homebrew; optional local QA may use real `brew` only if installed. `apt`/`mise` resources produce `unsupported` reports until later. Dotfile adapter can check/apply allowed paths only.

  **Must NOT do**: Do not execute apt/mise apply. Do not write outside HOME allowlist. Do not require real Homebrew in unit tests; use adapter fakes and only gate real Homebrew checks in optional local QA when installed.

  **Parallelization**: Can Parallel: NO | Wave 3 | Blocks: 13, 15 | Blocked By: 2, 3, 5, 7, 8, 9

  **References**:
  - Drift/report behavior: `docs/mvp.md:55-67`, `docs/mvp.md:300-309`.
  - Package implementation order: `docs/mvp.md:79-82`.
  - Dotfile allowlist: `docs/mvp.md:103-109`.

  **Acceptance Criteria**:
  - [ ] `go test ./internal/server ./internal/agent -run 'TestAgentReport|TestDriftReport|TestHomebrew|TestDotfile' -count=1` passes.
  - [ ] Server persists reconcile runs, events, and resource states transactionally.
  - [ ] Handler-level idempotency test posts the same reconcile report `Idempotency-Key` header twice and observes one persisted run/event set.
  - [ ] Homebrew fake adapter can report in_sync, drifted, and apply_success.
  - [ ] `apt`/`mise` produce unsupported status, not failure panic.

  **QA Scenarios**:
  ```
  Scenario: Drift report updates dashboard
    Tool: HTTP call
    Steps: POST /api/agent/drift-report with one drifted brew resource, then GET /api/dashboard.
    Expected: dashboard metric drifted == 1 and selected resource state == drifted.
    Evidence: evidence/task-10-drift-dashboard-http.txt

  Scenario: Unsupported mise adapter
    Tool: tmux
    Steps: run agent --once against desired state containing sourceKind mise.
    Expected: report contains unsupported_adapter and agent exits 0.
    Evidence: evidence/task-10-unsupported-mise-tmux.txt
  ```

  **Commit**: YES | Message: `feat(agent): report reconcile and drift state` | Files: `internal/server/handlers_agent.go`, `internal/agent/*`, tests

- [x] 11. Replace web mock data with typed API client and states

  **What to do**: Create `web/src/api.ts`, `web/src/apiTypes.ts`, and tests. Replace direct `web/src/data.ts` imports in `App.tsx` with API-backed loading/error/empty/data states. Preserve layout and current component boundaries. Use relative `/api/*` paths.

  **Must NOT do**: Do not add React Query unless explicitly justified in this task's notepad. Do not leave dashboard dependent on static mock data.

  **Parallelization**: Can Parallel: YES | Wave 4 | Blocks: 12, 13, 15 | Blocked By: 1, 2, 4, 6

  **References**:
  - Static import to remove: `web/src/App.tsx:37`.
  - Current dashboard composition: `web/src/App.tsx:93-165`.
  - Vite config: `web/vite.config.ts:1-4`.
  - External: `https://vite.dev/config/server-options` - dev proxy options.

  **Acceptance Criteria**:
  - [ ] `cd web && pnpm test -- --run src/api.test.ts src/App.test.tsx` passes.
  - [ ] Network failure renders a visible Korean error state.
  - [ ] Empty machine list renders first-machine registration CTA.
  - [ ] `rg -n "from \"./data\"" web/src/App.tsx web/src/FleetPanels.tsx web/src/SidePanel.tsx` returns no direct dashboard data imports.

  **QA Scenarios**:
  ```
  Scenario: API dashboard renders machine
    Tool: browser use
    Steps: run server with seeded dashboard; Playwright opens http://127.0.0.1:<port>/ and waits for machine "work-macbook".
    Expected: screenshot contains machine row and metric cards.
    Evidence: evidence/task-11-dashboard-browser.png

  Scenario: API failure state
    Tool: browser use
    Steps: Playwright registers page.route("**/api/dashboard", route => route.fulfill({ status: 500, body: "{\"error\":{\"code\":\"server_error\",\"message\":\"test\"}}" })) before page.goto("/"), then opens page.
    Expected: visible Korean error copy appears.
    Evidence: evidence/task-11-api-error-browser.png
  ```

  **Commit**: YES | Message: `feat(web): load dashboard from API` | Files: `web/src/api.ts`, `web/src/apiTypes.ts`, `web/src/App.tsx`, tests

- [x] 12. Add desired-state editor UI for package and dotfile resources

  **What to do**: Add editor panel or route reachable from dashboard CTA. Support create/edit/delete for package and dotfile resources. Show adapter support status: brew supported, apt/mise stored but not yet executable. Show dotfile path validation errors from server.

  **Must NOT do**: Do not add CodeMirror. Use plain textarea for dotfile MVP. Do not add secret editor.

  **Parallelization**: Can Parallel: YES | Wave 4 | Blocks: 13, 15 | Blocked By: 1, 2, 7, 11

  **References**:
  - Desired-state edit scope: `docs/mvp.md:69-109`.
  - Current ledger UI: `web/src/FleetPanels.tsx:338-379`.
  - Current inspector actions: `web/src/SidePanel.tsx:461-470`.

  **Acceptance Criteria**:
  - [ ] `cd web && pnpm test -- --run src/resourceEditor.test.tsx` passes.
  - [ ] Creating package sends `POST /api/resources/package`.
  - [ ] Creating hostile dotfile path displays server error.
  - [ ] No secret input appears in the editor.

  **QA Scenarios**:
  ```
  Scenario: Create package from UI
    Tool: browser use
    Steps: Playwright clicks "desired state editor" CTA, fills package name kubectl, source brew, version latest, saves.
    Expected: network request POST /api/resources/package and success toast/row appears.
    Evidence: evidence/task-12-package-editor-browser.png

  Scenario: Reject invalid dotfile path from UI
    Tool: browser use
    Steps: fill dotfile path /etc/hosts and save.
    Expected: visible Korean validation error and no success row.
    Evidence: evidence/task-12-dotfile-invalid-browser.png
  ```

  **Commit**: YES | Message: `feat(web): add desired-state editor` | Files: `web/src/*Resource*`, `web/src/App.tsx`, tests

- [x] 13. Wire repair drift, logs, activity, and Korean-first copy

  **What to do**: Wire `Repair drift` to `POST /api/machines/:machineId/repair-drift`, show run timeline/activity updates, make `Open logs` show recent agent event list, and migrate visible UI copy through `web/src/copy.ts` to Korean-first. Keep English for API paths, package names, CLI commands, and protocol fields.

  **Must NOT do**: Do not implement live log streaming. Do not leave main UI copy hardcoded in English.

  **Parallelization**: Can Parallel: NO | Wave 4 | Blocks: 15 | Blocked By: 6, 7, 10, 11, 12

  **References**:
  - User actions: `docs/mvp.md:47-53`.
  - Inspector actions: `docs/mvp.md:171-192`.
  - Current hardcoded copy: `web/src/App.tsx:100-129`, `web/src/SidePanel.tsx:429-470`.

  **Acceptance Criteria**:
  - [ ] `cd web && pnpm test -- --run src/copy.test.ts src/repairDrift.test.tsx` passes.
  - [ ] Clicking repair drift sends exact endpoint with selected machine id.
  - [ ] Main dashboard visible labels are Korean-first.
  - [ ] Copy tests enumerate and permit only this English allowlist: CLI commands, package names, API paths, protocol fields, OS names, semantic status enum values.
  - [ ] `Open logs` displays recent event list, not streaming.

  **QA Scenarios**:
  ```
  Scenario: Repair drift button
    Tool: browser use
    Steps: Playwright selects drifted machine, clicks Repair drift, waits for POST /api/machines/<id>/repair-drift.
    Expected: HTTP 202 response and activity list shows repair run queued.
    Evidence: evidence/task-13-repair-browser.png

  Scenario: Korean-first copy regression
    Tool: browser use
    Steps: open dashboard and inspect headings/buttons for Machines/Reconcile now/Repair drift legacy text.
    Expected: visible Korean labels are present and legacy English product copy is absent except allowed technical terms.
    Evidence: evidence/task-13-copy-browser.png
  ```

  **Commit**: YES | Message: `feat(web): wire drift repair UX` | Files: `web/src/copy.ts`, `web/src/App.tsx`, `web/src/FleetPanels.tsx`, `web/src/SidePanel.tsx`, tests

- [x] 14. Add security and scope regression tests

  **What to do**: Add dedicated tests for disabled secrets, no WebSocket routes, auth-required mutations, token/code hashing, dotfile escape prevention, unsupported adapters, and no arbitrary shell command execution.

  **Must NOT do**: Do not implement secret APIs. Do not skip tests because they overlap with earlier tasks.

  **Parallelization**: Can Parallel: YES | Wave 5 | Blocks: 15 | Blocked By: 4, 5, 7, 10, 13

  **References**:
  - Secret exclusions: `docs/mvp.md:113-135`, `docs/mvp.md:309`.
  - Explicit exclusions: `docs/mvp.md:311-320`.
  - Metis guardrails in this plan's Context section.

  **Acceptance Criteria**:
  - [ ] `go test ./... -run 'TestSecurity|TestScope|TestSecretsDisabled|TestDotfilePath' -count=1` passes.
  - [ ] `cd web && pnpm test -- --run src/scope.test.tsx` passes.
  - [ ] `/api/secrets` returns `404 Not Found`.

  **QA Scenarios**:
  ```
  Scenario: Secrets route disabled
    Tool: HTTP call
    Steps: curl -i -b evidence/task-4-cookie.jar http://127.0.0.1:<port>/api/secrets
    Expected: HTTP/1.1 404 Not Found and no secret schema in body.
    Evidence: evidence/task-14-secrets-disabled-http.txt

  Scenario: No WebSocket route
    Tool: HTTP call
    Steps: curl -i http://127.0.0.1:<port>/ws
    Expected: HTTP/1.1 404 Not Found.
    Evidence: evidence/task-14-no-websocket-http.txt
  ```

  **Commit**: YES | Message: `test: cover MVP security guardrails` | Files: `internal/server/*_test.go`, `internal/agent/*_test.go`, `web/src/scope.test.tsx`

- [x] 15. Add real end-to-end MVP scenario

  **What to do**: Create `web/e2e/mvp-dashboard.spec.ts` and `docs/qa/mvp-dashboard.md`. Scenario starts `neul-server` with temp SQLite DB, obtains owner auth by submitting the first-run setup token through `POST /api/session/local`, creates pairing code, registers a machine through CLI or HTTP, posts heartbeat and drift report, opens dashboard in browser, edits a Homebrew package and dotfile, clicks repair drift, runs one agent tick so it polls and acks the `repair_drift` command, and verifies run/activity update.

  **Must NOT do**: Do not run against static Vite mock only. Do not require the user's real machine package manager or real dotfiles.

  **Parallelization**: Can Parallel: NO | Wave 5 | Blocks: final verification | Blocked By: 5-14

  **References**:
  - MVP acceptance criteria: `docs/mvp.md:300-309`.
  - Current browser QA note: `web/design-qa.md`.
  - External: `https://playwright.dev/docs/browser-contexts` - isolated browser state.

  **Acceptance Criteria**:
  - [ ] `cd web && pnpm exec playwright test e2e/mvp-dashboard.spec.ts --project=chromium` passes against real `neul-server`.
  - [ ] `docs/qa/mvp-dashboard.md` records exact commands, seed data, screenshot path, and cleanup receipt.
  - [ ] Playwright screenshot confirms dashboard, selected machine, drift metric, editor, and repair activity.
  - [ ] After clicking repair, agent tick polls `/api/agent/commands`, posts an ack/report for `repair_drift`, and dashboard shows the command moved out of pending.
  - [ ] Temp DB, server process, browser context, and tmux sessions are cleaned up; receipt saved to `evidence/task-15-cleanup.txt`.

  **QA Scenarios**:
  ```
  Scenario: Full MVP browser flow
    Tool: browser use
    Steps: Playwright starts server fixture, pairs machine, posts heartbeat/drift, opens dashboard, creates kubectl brew resource, creates ~/.zshrc dotfile, clicks Repair drift, then runs one agent tick.
    Expected: dashboard first shows drift count 1, then after repair ack shows repair run no longer pending; drift count may remain 1 until the next drift/reconcile report marks the resource in sync.
    Evidence: evidence/task-15-mvp-dashboard-browser.png

  Scenario: Fresh empty instance
    Tool: browser use
    Steps: Playwright starts fresh DB and opens dashboard before any pairing.
    Expected: first-machine CTA is visible and no crash occurs.
    Evidence: evidence/task-15-empty-instance-browser.png
  ```

  **Commit**: YES | Message: `test(e2e): cover MVP dashboard flow` | Files: `web/e2e/mvp-dashboard.spec.ts`, `docs/qa/mvp-dashboard.md`, Playwright fixtures

- [x] 16. Add lightweight CI and final commit hygiene

  **What to do**: Add `.github/workflows/ci.yml` for `go test ./...`, `pnpm --dir web install --frozen-lockfile`, `pnpm --dir web test -- --run`, `pnpm --dir web exec biome check src index.html package.json tsconfig.json vite.config.ts pnpm-workspace.yaml`, `pnpm --dir web build`, `pnpm --dir web exec playwright install --with-deps chromium`, and `cd web && pnpm exec playwright test e2e/smoke.spec.ts e2e/mvp-dashboard.spec.ts --project=chromium`.

  **Must NOT do**: Do not add deployment, releases, Docker image publishing, or hosted infra.

  **Parallelization**: Can Parallel: YES | Wave 5 | Blocks: final verification | Blocked By: 2, 15

  **References**:
  - `.gitignore:1-29` - generated artifacts already ignored.
  - Commit convention from AGENTS instructions: Conventional Commits.

  **Acceptance Criteria**:
  - [ ] Workflow YAML validates structurally and commands match final verification.
  - [ ] CI does not publish/deploy artifacts.

  **QA Scenarios**:
  ```
  Scenario: CI command dry run locally
    Tool: bash
    Steps: run the exact local equivalent of each CI command.
    Expected: all commands exit 0.
    Evidence: evidence/task-16-ci-local-qa.txt

  Scenario: No deploy scope creep
    Tool: bash
    Steps: rg -n "deploy|docker publish|hosted|billing|RBAC|SSO" .github docs plans
    Expected: no deploy/hosted scope added except explicit exclusions.
    Evidence: evidence/task-16-scope-scan.txt
  ```

  **Commit**: YES | Message: `ci: add MVP verification workflow` | Files: `.github/workflows/ci.yml`

## Final Verification Wave (MANDATORY - after ALL implementation tasks)
> ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.

- [x] F1. Plan Compliance Audit
  - Verify every task has RED/GREEN evidence and manual QA evidence.
  - Command: `rg -n "task-[0-9]+.*(red|green|qa|browser|http|tmux)" evidence docs/qa`

- [x] F2. Code Quality Review
  - Spawn `codex-ultrawork-reviewer` with goal, full diff, evidence ledger, and this plan path.
  - Binding result: unconditional approval only.

- [x] F3. Real Manual QA
  - Run full E2E browser scenario against `neul-server` serving built SPA/API.
  - Capture screenshot, Playwright trace on failure, server log, and cleanup receipt.

- [x] F4. Scope Fidelity Check
  - Commands:
    - `rg -n "WebSocket|/ws|Secret|/api/secrets|RBAC|SSO|billing|hosted" .`
    - `rg -n "shell command|arbitrary shell|/etc|/usr" internal web docs plans`
  - Expected: only exclusions, disabled states, or historical design-doc mentions; no MVP implementation routes for out-of-scope features.

## Commit Strategy
- Commit atomically, one logical change per task.
- Use Conventional Commits only.
- Do not commit `node_modules`, `web/dist`, temp DBs, Playwright traces unless intentionally saved under evidence/docs and lightweight.
- Suggested sequence:
  1. `docs(contracts): lock MVP API and copy contracts`
  2. `test: add MVP test infrastructure`
  3. `feat(store): add MVP SQLite schema`
  4. `feat(server): add single-owner server shell`
  5. `feat(server): add machine pairing API`
  6. `feat(server): add dashboard and repair APIs`
  7. `feat(server): add desired-state resources API`
  8. `feat(cli): add machine pairing commands`
  9. `feat(agent): poll desired state and commands`
  10. `feat(agent): report reconcile and drift state`
  11. `feat(web): load dashboard from API`
  12. `feat(web): add desired-state editor`
  13. `feat(web): wire drift repair UX`
  14. `test: cover MVP security guardrails`
  15. `test(e2e): cover MVP dashboard flow`
  16. `ci: add MVP verification workflow`

## Success Criteria
- First machine can be registered through web-created pairing and CLI/agent claim.
- Dashboard shows connected/offline/drifted/pending states from persisted server data.
- Package and dotfile desired state can be edited in web and persisted through server API.
- Agent reports heartbeat, reconcile, and drift state through REST.
- Repair drift queues a command that agent picks up through polling.
- Secrets remain disabled and absent from dashboard/ledger and mutation APIs.
- Every implementation task has RED/GREEN and manual QA artifacts.
- Final browser E2E passes against the real server-served app.

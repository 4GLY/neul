# neul MVP Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the first single-owner self-hosted `neul` MVP loop: register one machine, show it on the dashboard, edit package/dotfile desired state, receive agent heartbeat/reconcile reports, detect drift, and request repair.

**Architecture:** Implement the MVP as a Go monorepo with a single `neul-server` binary serving REST APIs and the React SPA, a `neul-agent` daemon that reports heartbeat and reconcile results, and a small `neul` CLI for pairing/install/status/logs. Start with SQLite and typed JSON specs; hide or disable secrets in MVP.

**Tech Stack:** Go 1.22+, SQLite, SQL migrations, Vite + React 19 + TypeScript, pnpm, Biome, `tsc --noEmit`, Playwright/browser QA.

---

## Source Documents

- Product spec: `docs/mvp.md`
- Original design: `docs/2026-05-27-design.md`
- Current web prototype: `web/src/App.tsx`, `web/src/FleetPanels.tsx`, `web/src/SidePanel.tsx`, `web/src/data.ts`

## File Structure

Create or fill these paths:

- `go.mod`: root Go module.
- `cmd/neul-server/main.go`: server entrypoint.
- `cmd/neul-agent/main.go`: agent entrypoint.
- `cmd/neul/main.go`: CLI entrypoint.
- `internal/server/http.go`: HTTP router, middleware, static SPA serving.
- `internal/server/handlers_*.go`: route handlers grouped by pairing, machines, resources, reconcile, agent reports.
- `internal/store/sqlite.go`: SQLite connection and transaction wrapper.
- `internal/store/migrations.go`: migration runner.
- `internal/domain/*.go`: typed domain models and state transition helpers.
- `migrations/001_initial.sql`: MVP schema.
- `web/src/api.ts`: typed API client.
- `web/src/apiTypes.ts`: API response/request types.
- `web/src/App.tsx` and existing panel files: replace mock data with API-backed state.
- `web/src/*.test.tsx`: component and state tests.
- `docs/mvp.md`: keep updated when implementation changes scope.

## Fixed MVP Decisions

- MVP deployment shape: single-owner self-hosted first.
- Server push/WebSocket is out for MVP; agent polls desired state and posts reports.
- Secret creation/editing/API is out for MVP; hide or disable the UI.
- User-facing web copy is Korean-first.
- Supported package model in MVP: `brew`, `apt`, and `mise`; executable adapter implementation starts with Homebrew, then apt/mise only when the Homebrew loop is proven.
- Dotfile writes are limited to allowlisted HOME paths from `docs/mvp.md`.

## Implementation Waves

### Wave 1: Contracts, Domain, And Storage Foundation

#### Task 0: Lock MVP copy and API contracts

**Files:**

- Modify: `docs/mvp.md`
- Create: `internal/domain/contracts.md`
- Create: `web/src/copy.ts`

- [ ] **Step 1: Write contract checklist**

Create `internal/domain/contracts.md` with the final API response shapes for dashboard machine rows, resource rows, pairing, heartbeat, reconcile report, and repair command.

- [ ] **Step 2: Write Korean-first copy map**

Create `web/src/copy.ts` with Korean labels for dashboard, filters, inspector, pairing, desired-state editor, and disabled secrets.

- [ ] **Step 3: Verify contract coverage**

Run:

```bash
rg -n "pair/init|pair/claim|dashboard|repair-drift|Secrets" docs/mvp.md internal/domain/contracts.md web/src/copy.ts
```

Expected: every MVP surface has a named contract and user-facing copy entry.

#### Task 1: Add Go module and domain model tests

**Files:**

- Create: `go.mod`
- Create: `internal/domain/machine.go`
- Create: `internal/domain/machine_test.go`
- Create: `internal/domain/resource.go`
- Create: `internal/domain/resource_test.go`

- [ ] **Step 1: Write failing tests for machine status computation**

Create `internal/domain/machine_test.go` with tests for heartbeat and report inputs:

```go
func TestMachineStatus_whenHeartbeatFreshAndNoDrift_isHealthy(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	machine := domain.MachineSnapshot{
		LastHeartbeatAt: now.Add(-30 * time.Second),
		DriftCount: 0,
		PendingCount: 0,
		BlockedCount: 0,
	}

	status := domain.ComputeMachineStatus(machine, now)

	require.Equal(t, domain.MachineStatusHealthy, status)
}

func TestMachineStatus_whenHeartbeatStale_isOffline(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	machine := domain.MachineSnapshot{
		LastHeartbeatAt: now.Add(-10 * time.Minute),
		DriftCount: 0,
		PendingCount: 0,
		BlockedCount: 0,
	}

	status := domain.ComputeMachineStatus(machine, now)

	require.Equal(t, domain.MachineStatusOffline, status)
}

func TestMachineStatus_whenDriftExists_isDrifted(t *testing.T) {
	now := time.Date(2026, 6, 5, 12, 0, 0, 0, time.UTC)
	machine := domain.MachineSnapshot{
		LastHeartbeatAt: now.Add(-30 * time.Second),
		DriftCount: 2,
		PendingCount: 0,
		BlockedCount: 0,
	}

	status := domain.ComputeMachineStatus(machine, now)

	require.Equal(t, domain.MachineStatusDrifted, status)
}
```

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./internal/domain -run TestMachineStatus -count=1
```

Expected: FAIL because `domain.MachineSnapshot` and `ComputeMachineStatus` do not exist.

- [ ] **Step 3: Implement minimal domain model**

Implement machine status constants, `MachineSnapshot`, and `ComputeMachineStatus`.

- [ ] **Step 4: Run tests and verify GREEN**

Run:

```bash
go test ./internal/domain -run TestMachineStatus -count=1
```

Expected: PASS.

#### Task 2: Add SQLite schema and migration runner

**Files:**

- Create: `migrations/001_initial.sql`
- Create: `internal/store/sqlite.go`
- Create: `internal/store/migrations.go`
- Create: `internal/store/migrations_test.go`

- [ ] **Step 1: Write failing migration test**

Test that applying migrations creates these tables:

- `users`
- `pairing_codes`
- `machine_tokens`
- `machines`
- `profiles`
- `segments`
- `resources`
- `file_versions`
- `reconcile_runs`
- `reconcile_events`
- `agent_reports`

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
go test ./internal/store -run TestMigrationsCreateMvpTables -count=1
```

Expected: FAIL because migration runner and tables do not exist.

- [ ] **Step 3: Implement migration runner and SQL**

Use SQLite with explicit foreign keys enabled. Store package/dotfile specs as JSON text in `resources.spec_json`.

- [ ] **Step 4: Run test and verify GREEN**

Run:

```bash
go test ./internal/store -run TestMigrationsCreateMvpTables -count=1
```

Expected: PASS.

### Wave 2: Server API

#### Task 3: Pairing and machine registration API

**Files:**

- Create: `internal/server/http.go`
- Create: `internal/server/handlers_pairing.go`
- Create: `internal/server/handlers_pairing_test.go`
- Create: `cmd/neul-server/main.go`

- [ ] **Step 1: Write failing HTTP tests**

Use `httptest` to assert:

- `POST /api/pair/init` returns code, expiry, and stores unused hash.
- `POST /api/pair/claim` with a valid code creates machine metadata.
- Reusing the same code returns `409 Conflict`.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./internal/server -run 'TestPairing|TestMachineRegister' -count=1
```

Expected: FAIL because routes do not exist.

- [ ] **Step 3: Implement pairing/register routes**

Use opaque random pairing codes, hash before storing, issue a machine token, and mark code used on successful machine registration.

- [ ] **Step 4: Run tests and verify GREEN**

Run:

```bash
go test ./internal/server -run 'TestPairing|TestMachineRegister' -count=1
```

Expected: PASS.

#### Task 4: Dashboard machine summary API

**Files:**

- Create: `internal/server/handlers_machines.go`
- Create: `internal/server/handlers_machines_test.go`

- [ ] **Step 1: Write failing HTTP test**

Seed two machines: one healthy, one drifted. Assert `GET /api/machines` returns the rows required by `docs/mvp.md`, including status, drift count, last heartbeat, agent version, OS, arch, tag, and latest reconcile time.

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
go test ./internal/server -run TestListMachinesReturnsDashboardRows -count=1
```

Expected: FAIL because route does not exist.

- [ ] **Step 3: Implement list machines route**

Compute status using `internal/domain.ComputeMachineStatus`.

- [ ] **Step 4: Run test and verify GREEN**

Run:

```bash
go test ./internal/server -run TestListMachinesReturnsDashboardRows -count=1
```

Expected: PASS.

#### Task 4.5: Desired-state read API

**Files:**

- Create: `internal/server/handlers_dashboard.go`
- Create: `internal/server/handlers_dashboard_test.go`

- [ ] **Step 1: Write failing HTTP test**

Assert `GET /api/dashboard` returns metric cards, machine rows, activity feed, and desired/live ledger rows for the current profile.

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
go test ./internal/server -run TestDashboardReturnsMvpHomePayload -count=1
```

Expected: FAIL because route does not exist.

- [ ] **Step 3: Implement dashboard route**

Aggregate machine, resource, reconcile, and latest event state into one payload for the React dashboard.

- [ ] **Step 4: Run test and verify GREEN**

Run:

```bash
go test ./internal/server -run TestDashboardReturnsMvpHomePayload -count=1
```

Expected: PASS.

### Wave 3: Agent Reports And Reconcile

#### Task 5: Heartbeat and reconcile report ingestion

**Files:**

- Create: `internal/server/handlers_agent.go`
- Create: `internal/server/handlers_agent_test.go`
- Modify: `internal/store/sqlite.go`

- [ ] **Step 1: Write failing tests**

Assert:

- `POST /api/agent/heartbeat` updates `last_heartbeat_at`, `agent_version`, and connection metadata.
- `POST /api/agent/reconcile-report` writes a run and resource states.
- Malformed report returns `400 Bad Request`.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./internal/server -run TestAgent -count=1
```

Expected: FAIL because agent routes do not exist.

- [ ] **Step 3: Implement heartbeat/report routes**

Validate machine identity, persist report, and keep report schema narrow: status, step, resource states, timestamps, error message.

- [ ] **Step 4: Run tests and verify GREEN**

Run:

```bash
go test ./internal/server -run TestAgent -count=1
```

Expected: PASS.

#### Task 6: Repair drift command

**Files:**

- Create: `internal/server/handlers_reconcile.go`
- Create: `internal/server/handlers_reconcile_test.go`

- [ ] **Step 1: Write failing HTTP test**

Seed a drifted machine and assert `POST /api/machines/:machineId/repair-drift` creates a reconcile run scoped to drifted resources only.

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
go test ./internal/server -run TestRepairDriftCreatesScopedReconcileRun -count=1
```

Expected: FAIL because route does not exist.

- [ ] **Step 3: Implement repair route**

Do not mutate desired state. Create a reconcile run command with reason `repair_drift`.

- [ ] **Step 4: Run test and verify GREEN**

Run:

```bash
go test ./internal/server -run TestRepairDriftCreatesScopedReconcileRun -count=1
```

Expected: PASS.

### Wave 4: Desired State Editing

#### Task 7: Package and dotfile resources API

**Files:**

- Create: `internal/server/handlers_resources.go`
- Create: `internal/server/handlers_resources_test.go`
- Modify: `internal/domain/resource.go`

- [ ] **Step 1: Write failing tests**

Assert:

- `POST /api/resources/package` accepts `brew`, `apt`, and `mise`.
- Unsupported source kind returns `400 Bad Request`.
- `POST /api/resources/dotfile` rejects `/etc/hosts`.
- Dotfile under `~/.config/nvim/init.lua` is accepted.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
go test ./internal/server -run TestResources -count=1
```

Expected: FAIL because resource routes do not exist.

- [ ] **Step 3: Implement resource routes**

Use typed request structs and JSON specs. Keep secret mutation routes absent in MVP.

- [ ] **Step 4: Run tests and verify GREEN**

Run:

```bash
go test ./internal/server -run TestResources -count=1
```

Expected: PASS.

### Wave 5: Agent And CLI Surface

#### Task 8: CLI pairing command

**Files:**

- Create: `cmd/neul/main.go`
- Create: `internal/cli/pair.go`
- Create: `internal/cli/pair_test.go`

- [ ] **Step 1: Write failing CLI test**

Run the CLI with a fake HTTP server and assert `neul init --pair abc123 --server http://127.0.0.1:<port>` posts machine metadata and writes local machine identity under a temp config dir.

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
go test ./internal/cli -run TestInitPairRegistersMachine -count=1
```

Expected: FAIL because CLI package does not exist.

- [ ] **Step 3: Implement CLI pairing**

Generate machine key material and machine metadata. Do not implement desired state editing in CLI.

- [ ] **Step 4: Run test and verify GREEN**

Run:

```bash
go test ./internal/cli -run TestInitPairRegistersMachine -count=1
```

Expected: PASS.

#### Task 9: Agent heartbeat loop and dry-run adapter report

**Files:**

- Create: `cmd/neul-agent/main.go`
- Create: `internal/agent/agent.go`
- Create: `internal/agent/agent_test.go`
- Create: `internal/agent/adapters.go`

- [ ] **Step 1: Write failing agent test**

With a fake server, assert one agent tick sends heartbeat, fetches desired state, and posts a reconcile report for package/dotfile resources using dry-run adapters.

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
go test ./internal/agent -run TestAgentTickReportsHeartbeatAndReconcile -count=1
```

Expected: FAIL because agent package does not exist.

- [ ] **Step 3: Implement one-tick agent core**

Keep real package manager execution out until the dry-run report loop is working. Add Homebrew check/apply first; add apt and mise only after Homebrew is covered by unit tests and the E2E dashboard scenario.

- [ ] **Step 4: Run test and verify GREEN**

Run:

```bash
go test ./internal/agent -run TestAgentTickReportsHeartbeatAndReconcile -count=1
```

Expected: PASS.

### Wave 6: Web API Integration

#### Task 10: Replace mock data with API client

**Files:**

- Create: `web/src/api.ts`
- Create: `web/src/apiTypes.ts`
- Create: `web/src/api.test.ts`
- Modify: `web/src/App.tsx`
- Modify: `web/src/FleetPanels.tsx`
- Modify: `web/src/SidePanel.tsx`

- [ ] **Step 1: Write failing web API tests**

Assert:

- API client parses `GET /api/machines` into dashboard rows.
- Network failure returns a visible error state.
- Empty machine list shows first-machine registration CTA.

- [ ] **Step 2: Run tests and verify RED**

Run:

```bash
pnpm test -- --run web/src/api.test.ts
```

Expected: FAIL because API client and test runner are not yet configured.

- [ ] **Step 3: Add test runner and API client**

Use Vitest or the existing chosen test runner. Keep TypeScript strict and avoid `any`.

- [ ] **Step 4: Wire dashboard to API data**

Keep current visual layout. Replace `web/src/data.ts` usage with API data and loading/error/empty states.

- [ ] **Step 5: Run tests and verify GREEN**

Run:

```bash
pnpm test -- --run web/src/api.test.ts
pnpm build
```

Expected: PASS.

#### Task 10.5: Convert dashboard copy to Korean-first

**Files:**

- Modify: `web/src/App.tsx`
- Modify: `web/src/FleetPanels.tsx`
- Modify: `web/src/Layout.tsx`
- Modify: `web/src/SidePanel.tsx`
- Modify: `web/src/data.ts`
- Create or modify: `web/src/copy.ts`
- Create: `web/src/copy.test.ts`

- [ ] **Step 1: Write failing copy coverage test**

Assert that visible dashboard labels, filters, inspector actions, pairing CTA, desired-state editor labels, and disabled secrets copy are sourced from `web/src/copy.ts`.

- [ ] **Step 2: Run test and verify RED**

Run:

```bash
pnpm test -- --run web/src/copy.test.ts
```

Expected: FAIL until the current prototype copy is moved into the copy map.

- [ ] **Step 3: Wire Korean-first copy**

Keep protocol names, CLI commands, package manager names, file paths, and API paths in English. Convert product copy to Korean-first labels and helper text.

- [ ] **Step 4: Run tests and verify GREEN**

Run:

```bash
pnpm test -- --run web/src/copy.test.ts
pnpm build
```

Expected: PASS.

### Wave 7: End-To-End MVP Scenario

#### Task 11: Browser and HTTP QA scenario

**Files:**

- Create: `web/e2e/mvp-dashboard.spec.ts`
- Create: `docs/qa/mvp-dashboard.md`

- [ ] **Step 1: Write failing Playwright scenario**

Scenario:

1. Start `neul-server` with temp SQLite DB.
2. Create pairing code via HTTP.
3. Register a machine via HTTP or CLI.
4. Post heartbeat and drift report.
5. Open dashboard in browser.
6. Assert machine row, drift metric, inspector, and desired/live table are visible.
7. Click `Repair drift`.
8. Assert reconcile run appears.

- [ ] **Step 2: Run scenario and verify RED**

Run:

```bash
pnpm exec playwright test web/e2e/mvp-dashboard.spec.ts --project=chromium
```

Expected: FAIL until server/web integration is complete.

- [ ] **Step 3: Implement missing integration glue only**

Do not add new product scope. Fix only the server/web gaps revealed by the scenario.

- [ ] **Step 4: Run scenario and verify GREEN**

Run:

```bash
pnpm exec playwright test web/e2e/mvp-dashboard.spec.ts --project=chromium
```

Expected: PASS with screenshot and trace retained on failure only.

## Final Verification Wave

- [ ] Run Go tests:

```bash
go test ./...
```

- [ ] Run web checks:

```bash
cd web
pnpm exec biome check src index.html package.json tsconfig.json vite.config.ts pnpm-workspace.yaml
pnpm build
```

- [ ] Run browser QA against the real local server:

```bash
curl -i http://127.0.0.1:5173/
pnpm exec playwright test web/e2e/mvp-dashboard.spec.ts --project=chromium
```

- [ ] Verify MVP spec coverage:

Every acceptance criterion in `docs/mvp.md#6-mvp-수용-기준` must map to at least one Go test, one web test, or the Playwright scenario.

## Commit Plan

Commit in these logical chunks:

1. `docs(mvp): define first MVP scope`
2. `feat(domain): add machine and resource state models`
3. `feat(server): add pairing and machine APIs`
4. `feat(server): ingest agent reports and drift repair`
5. `feat(cli): add machine pairing command`
6. `feat(agent): report heartbeat and dry-run reconcile`
7. `feat(web): connect dashboard to server APIs`
8. `test(e2e): cover MVP dashboard flow`

Do not commit generated `node_modules`, `dist`, temporary QA screenshots, or local debug journals.

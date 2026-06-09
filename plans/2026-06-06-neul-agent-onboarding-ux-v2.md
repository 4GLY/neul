# Neul Agent Onboarding UX v2

## TL;DR
> **Summary**: Replace the current curl/setup-token/pair-code/manual-agent flow with a Tailscale-like first-machine onboarding loop: web creates a guided enrollment, the user runs one generated agent command, and the dashboard moves from waiting to connected automatically.
> **Deliverables**:
> - Web first-machine onboarding wizard with copyable enroll command, waiting state, expiry/error states, and connected transition.
> - Typed browser API client for pair init/poll and onboarding state.
> - User-facing `neul agent enroll` command that wraps current pair claim, config write, optional one-shot heartbeat, and friendly status output.
> - E2E scenario that uses the visible wizard plus CLI/agent process instead of direct API setup helpers.
> - Updated docs and CI coverage.
> **Effort**: Medium
> **Parallel**: YES - 4 waves
> **Critical Path**: Task 1 -> Task 2 -> Task 4 -> Task 6 -> Task 8

## Context
### Original Request
The user identified the current first-machine registration as poor UX compared with Tailscale: Tailscale lets a user download a client, run it, log in/approve, and the machine configures itself. The user requested an `omo:ulw-plan` plan and Claude Code CLI review.

### Interview Summary
- The product problem is not basic pairing correctness; existing MVP pairing works but exposes an API/debug ceremony to the user.
- "Tailscale-like" for this iteration means: web-guided enrollment, generated command, user-facing agent enroll flow, automatic heartbeat/connected transition, and no manual curl/API choreography.
- Native GUI, hosted login, OAuth/SSO, real service installation, secrets, WebSocket push, and arbitrary shell execution stay out.

### Research Findings
- Current MVP spec defines first-machine registration as web-created pairing code plus `neul init --pair <code>` and `neul agent install`; see `docs/mvp.md:21-32`.
- Current dashboard empty state displays pair-code CLI instructions and an inert action; see `web/src/App.tsx:182-187` and `web/src/App.tsx:260-271`.
- Current browser API client has dashboard/resources/repair APIs but no pair init/poll methods; see `web/src/api.ts:19-71`.
- Current server routes support owner session, pair init/claim/poll, dashboard, resources, and agent APIs; see `internal/server/http.go:35-52`.
- Current CLI supports `neul init --pair --server` and `agent install/status/logs`, but no `agent enroll`; see `internal/cli/cli.go:25-82`.
- Current pairing server creates 10-minute opaque tokens and claim creates machine credentials; see `internal/server/handlers_pairing.go:13-37` and `internal/server/handlers_pairing.go:40-118`.
- Current E2E proves the MVP flow but bypasses visible onboarding with direct pair API helpers; see `web/e2e/mvp-dashboard.spec.ts:72-74` and `web/e2e/mvp-dashboard.spec.ts:219-245`.

### Metis Review (gaps addressed)
- Authoritative scope: use `docs/mvp.md`, `internal/domain/contracts.md`, and live source files as truth; ignore stale README hosted/WebSocket/secrets ambition for this iteration.
- Approval model: possession of an owner-created short-lived pairing token is the approval for this iteration. Do not introduce pending enrollment approval tables yet.
- Connected event: onboarding completes only after first heartbeat makes the machine visible in `/api/dashboard`; pair claim alone is not enough.
- Generated command: local dev acceptance uses `go run ./cmd/neul agent enroll --server <url> --pair <token> --connect-once`; production copy can show `neul agent enroll ...` only as secondary copy.
- Command prerequisite: because the accepted MVP command uses `go run`, the wizard and docs must state `Run from your neul checkout:` before the generated command. It must not imply the command works from any arbitrary directory until packaged binaries exist.
- Install script: do not ship `curl | sh` in this iteration. Add a non-executing "Download/install command coming next" placeholder only if needed for copy; no `/install.sh` endpoint in this plan.
- Polling: web wizard polls `/api/pair/poll` and `/api/dashboard` over REST. No WebSocket.
- Expiry source: `/api/pair/poll` is the source of truth for invite expiry. Expired unused codes return HTTP 200 with a typed `{status: "expired", expiresAt}` body so the wizard can render retry copy without guessing from the browser clock.
- Waiting timeout: after a claim, the wizard waits up to 120 seconds for the first dashboard heartbeat. If no heartbeat appears, it transitions to `agent_not_responding` with retry/help copy instead of waiting forever.
- Existing config behavior: enroll refuses to overwrite an existing local config unless `--force` is provided. `--force` only replaces the local config; it does not revoke or delete a prior server-side machine/token.

## Work Objectives
### Core Objective
Make first-machine onboarding feel like a product flow: owner starts enrollment in the web UI, user runs one generated agent command, the command configures and connects the machine once, and the web UI transitions from waiting to connected without exposing curl/setup-token/manual API steps.

### Deliverables
- `web/src/OnboardingWizard.tsx` and tests.
- Pairing API functions/types in `web/src/api.ts` and `web/src/apiTypes.ts`.
- CLI command `neul agent enroll --server <url> --pair <token> [--config-dir <dir>] [--connect-once] [--force]`.
- Server/dashboard support only where needed for existing pair poll/dashboard semantics; no new table unless tests prove existing pair poll cannot support the wizard.
- Updated E2E in `web/e2e/mvp-dashboard.spec.ts`.
- Updated docs in `docs/qa/agent-onboarding.md` and `docs/mvp.md` amendment.
- CI still green.

### Definition of Done
- `env -u GOROOT go test ./internal/cli ./internal/server ./internal/agent -run 'TestAgentEnroll|TestPair|TestOnboarding|TestDashboard|TestAgentReport' -count=1` passes.
- `cd web && pnpm test -- --run src/onboardingWizard.test.tsx src/api.test.ts src/App.test.tsx` passes.
- `cd web && pnpm exec playwright test e2e/mvp-dashboard.spec.ts --project=chromium` passes and the setup portion uses the visible wizard plus CLI enroll, not direct pair claim helpers.
- Real manual QA artifacts exist for browser onboarding, tmux CLI enroll, HTTP pair expiry/poll, and cleanup.
- No `/install.sh`, hosted login, OAuth/SSO, WebSocket, secrets, arbitrary shell command resource, or real launchd/systemd installation is added.

### Must Have
- Empty dashboard CTA starts the onboarding wizard.
- Wizard can generate a pair token, show a concrete command, copy it, poll for pair claim, then poll dashboard until first heartbeat.
- Enroll command claims the pair token, writes config with `0600`, optionally runs one agent tick, and prints friendly state: "Machine enrolled", "Connecting", "Connected".
- `--connect-once` invokes the real agent reconcile path with `agent.New(config).Tick(ctx)`; it must not shortcut to heartbeat-only behavior.
- Existing `neul init --pair --server` remains compatible for debug/docs, but user-facing copy moves to `neul agent enroll`.
- Expired/used/bad pair tokens show clear CLI and UI errors.
- Re-running enroll with existing config fails safely unless `--force`; `--force` overwrites only local config and intentionally does not delete/revoke old server records.
- Generated command never includes setup token or machine token.
- Pair tokens are bearer credentials: never place them in URL query strings, document title, browser history, or logs beyond the explicit copyable command text.

### Must NOT Have
- No native GUI/menubar client in this iteration.
- No hosted auth, OAuth, SSO, RBAC, team workflow, billing, or external identity provider.
- No real launchd/systemd service installation.
- No WebSocket or server push.
- No secrets UI/API/value editing.
- No arbitrary shell execution or user-controlled shell interpolation.
- No generated `web/dist` changes committed.

## Verification Strategy
> ZERO HUMAN INTERVENTION - all verification is agent-executed.
- Test decision: TDD, using existing Go tests, Vitest, and Playwright.
- QA policy: Every task has agent-executed scenarios.
- Evidence: `evidence/task-{N}-agent-onboarding-*`.

## Execution Strategy
### Parallel Execution Waves
Wave 1: Tasks 1-3 establish contracts, tests, and browser API/client surface.
Wave 2: Tasks 4-5 implement web wizard and CLI enroll behavior.
Wave 3: Tasks 6-7 wire E2E, docs, copy, and regression guardrails.
Wave 4: Task 8 final CI/hygiene and verification.

### Dependency Matrix
| Task | Blocks | Blocked By |
| --- | --- | --- |
| 1. Contracts and copy | 2, 3, 4, 5, 6, 7 | none |
| 2. Pairing API client/types | 4, 6 | 1 |
| 3. Server pair poll/dashboard guard tests | 4, 6 | 1 |
| 4. Web onboarding wizard | 6, 7 | 1, 2, 3 |
| 5. CLI agent enroll | 6, 7 | 1 |
| 6. Browser+CLI E2E onboarding | 8 | 2, 3, 4, 5 |
| 7. Docs and UX copy cleanup | 8 | 4, 5 |
| 8. CI and final hygiene | final verification | 6, 7 |

## TODOs
- [x] 1. Lock onboarding contract, copy, and scope guardrails

  **What to do**: Update `internal/domain/contracts.md`, `docs/mvp.md`, and `web/src/copy.ts` to define "Agent Onboarding UX v2". Canonicalize the flow as: owner session already established -> web `POST /api/pair/init` -> web displays `Run from your neul checkout:` plus `go run ./cmd/neul agent enroll --server <origin> --pair <token> --connect-once` -> CLI claims token -> CLI writes config -> CLI calls `agent.New(config).Tick(ctx)` when `--connect-once` is set -> web poll sees claimed pair -> web dashboard poll sees first heartbeat -> connected. Define terminal states: `creating`, `ready`, `claimed_waiting_heartbeat`, `connected`, `expired`, `used`, `agent_not_responding`, `error`, `cancelled`. Define pair token possession as owner approval for this iteration.

  **Must NOT do**: Do not add hosted login, pending approval tables, `/install.sh`, native GUI, WebSocket, or real service installation to the contract.

  **Parallelization**: Can Parallel: YES | Wave 1 | Blocks: 2, 3, 4, 5, 6, 7 | Blocked By: none

  **References**:
  - Current MVP onboarding: `docs/mvp.md:21-32`.
  - Current route contract: `internal/server/http.go:35-52`.
  - Current web copy keys: `web/src/copy.ts`.
  - Current empty-state copy: `web/src/App.tsx:182-187`.
  - Scope exclusions: `docs/mvp.md:311-320`, `internal/domain/contracts.md`.

  **Acceptance Criteria**:
  - [ ] `rg -n "agent enroll|claimed_waiting_heartbeat|agent_not_responding|connect-once|Run from your neul checkout|pair token possession|no /install.sh" internal/domain/contracts.md docs/mvp.md web/src/copy.ts` finds the locked contract.
  - [ ] `cd web && pnpm test -- --run src/copy.test.ts` passes with the new onboarding copy allowlist.
  - [ ] `rg -n "curl -i|setupToken|/api/session/local|neul init --pair" web/src docs/mvp.md internal/domain/contracts.md` shows those only in debug/backward-compatibility contexts, not primary onboarding copy.
  - [ ] `rg -n "pair.*URL|document.title|history|bearer" internal/domain/contracts.md docs/mvp.md web/src/copy.ts` proves token-leak guardrails are documented.

  **QA Scenarios**:
  ```
  Scenario: Contract scan
    Tool: bash
    Steps: rg -n "agent enroll|connect-once|claimed_waiting_heartbeat|agent_not_responding|pair token possession|Run from your neul checkout" internal/domain/contracts.md docs/mvp.md web/src/copy.ts
    Expected: all new onboarding contract terms are present.
    Evidence: evidence/task-1-agent-onboarding-contract-scan.txt

  Scenario: Manual-flow regression scan
    Tool: bash
    Steps: rg -n "curl -i|setupToken|/api/session/local|neul init --pair" web/src docs/mvp.md internal/domain/contracts.md
    Expected: no primary product copy instructs the user to curl setup-token APIs or run `neul init --pair`.
    Evidence: evidence/task-1-agent-onboarding-regression-scan.txt
  ```

  **Commit**: YES | Message: `docs(onboarding): define agent enroll flow` | Files: `internal/domain/contracts.md`, `docs/mvp.md`, `web/src/copy.ts`, tests

- [x] 2. Add typed browser pairing/onboarding API client

  **What to do**: Extend `web/src/apiTypes.ts` with pair init and pair poll response types. Extend `web/src/api.ts` with `createPairingInvite()` and `pollPairingInvite(code)` that call existing `POST /api/pair/init` and `GET /api/pair/poll?code=...`. Return typed `expiresAt`, `code`, `status`, and `machineId`. Keep fetch paths relative so the built SPA works from the Go server origin.

  **Must NOT do**: Do not call `/api/pair/claim` from the browser; the agent/CLI owns claim.

  **Parallelization**: Can Parallel: YES | Wave 1 | Blocks: 4, 6 | Blocked By: 1

  **References**:
  - Existing API client style: `web/src/api.ts:19-99`.
  - Existing dashboard types: `web/src/apiTypes.ts`.
  - Existing pair init/poll server handlers: `internal/server/handlers_pairing.go:13-37`, `internal/server/handlers_pairing.go:121-151`.

  **Acceptance Criteria**:
  - [ ] Add `src/api.test.ts` tests that first fail for missing `createPairingInvite` and `pollPairingInvite`, then pass.
  - [ ] `cd web && pnpm test -- --run src/api.test.ts` passes.
  - [ ] `rg -n "/api/pair/claim" web/src` returns no browser client claim call.

  **QA Scenarios**:
  ```
  Scenario: Pair API client calls owner endpoints
    Tool: bash
    Steps: cd web && pnpm test -- --run src/api.test.ts
    Expected: test output includes passing pair init/poll API tests.
    Evidence: evidence/task-2-agent-onboarding-api-tests.txt

  Scenario: Browser client does not claim machines
    Tool: bash
    Steps: rg -n "/api/pair/claim" web/src || true
    Expected: no results.
    Evidence: evidence/task-2-agent-onboarding-no-browser-claim.txt
  ```

  **Commit**: YES | Message: `feat(web): add pairing API client` | Files: `web/src/api.ts`, `web/src/apiTypes.ts`, `web/src/api.test.ts`

- [x] 3. Harden server pair poll and onboarding status semantics

  **What to do**: Add server tests proving pair init/poll semantics are sufficient for the wizard: owner-authenticated init creates token/expiresAt, owner-authenticated poll returns `pending`, claim by agent changes poll to `claimed` with `machineId`, expired unused code returns HTTP 200 with `{status: "expired", expiresAt}`, and `/api/pair/poll` stays owner-authenticated. If current poll does not expose expiry, extend `handlePairPoll` with this typed expired outcome instead of letting the browser infer expiry locally.

  **Must NOT do**: Do not add a pending-enrollment table or approval endpoint in this iteration.

  **Parallelization**: Can Parallel: YES | Wave 1 | Blocks: 4, 6 | Blocked By: 1

  **References**:
  - Pair handlers: `internal/server/handlers_pairing.go:13-151`.
  - Pair tests: `internal/server/handlers_pairing_test.go`.
  - Auth guard: `internal/server/http.go:36-38`.

  **Acceptance Criteria**:
  - [ ] `env -u GOROOT go test ./internal/server -run 'TestPairInit|TestPairPoll|TestOnboarding' -count=1` passes.
  - [ ] Poll without owner cookie returns `401 Unauthorized`.
  - [ ] Poll after claim returns `claimed` and machine id.
  - [ ] `TestOnboardingPairPollUnauthenticated` proves unauthenticated poll returns 401.
  - [ ] `TestOnboardingPairPollExpiredUnusedCode` proves expired unused code returns HTTP 200 with `status: "expired"` and `expiresAt`.
  - [ ] `TestOnboardingPairPollClaimed` proves poll after claim returns `claimed` and `machineId`.
  - [ ] `TestOnboardingPairInitResponseShape` proves init returns `code` and `expiresAt`.

  **QA Scenarios**:
  ```
  Scenario: Owner creates and polls onboarding invite
    Tool: HTTP call
    Steps: start neul-server with temp DB; unlock owner session; curl -i -b cookie -X POST /api/pair/init; curl -i -b cookie "/api/pair/poll?code=<code>"
    Expected: init returns HTTP 201 with code/expiresAt; poll returns HTTP 200 with status pending.
    Evidence: evidence/task-3-agent-onboarding-pair-poll-http.txt

  Scenario: Poll requires owner session
    Tool: HTTP call
    Steps: curl -i "http://127.0.0.1:<port>/api/pair/poll?code=<code>" without cookie
    Expected: HTTP 401 JSON error.
    Evidence: evidence/task-3-agent-onboarding-poll-auth-http.txt

  Scenario: Expired invite is typed for the wizard
    Tool: HTTP call
    Steps: seed/create an expired unused pair token; curl -i -b cookie "/api/pair/poll?code=<code>"
    Expected: HTTP 200 JSON includes status "expired" and expiresAt.
    Evidence: evidence/task-3-agent-onboarding-expired-poll-http.txt
  ```

  **Commit**: YES | Message: `test(server): harden onboarding pair poll` | Files: `internal/server/handlers_pairing_test.go`, optional `internal/server/handlers_pairing.go`

- [x] 4. Build the web onboarding wizard and wire the empty state

  **What to do**: Create `web/src/OnboardingWizard.tsx` and `web/src/onboardingWizard.test.tsx`. Add `onAction?: () => void` to `StatePanel` and replace the inert empty-state action in `App.tsx` with real wizard opening. Route all empty-state title/body/action text through `copy.dashboard.emptyState` instead of duplicating hard-coded strings in `App.tsx`. Wizard states: intro, creating invite, command ready, claimed waiting for heartbeat, connected, expired, `agent_not_responding`, error, cancel/retry. Generate command from `window.location.origin` but display it under `Run from your neul checkout:` as `go run ./cmd/neul agent enroll --server <origin> --pair <code> --connect-once`. Poll pair status every 2 seconds while ready; after `claimed`, refresh dashboard every 2 seconds until the claimed machine appears with `lastHeartbeatAt`. Stop polling on unmount/cancel/connected/expired. If claimed but no heartbeat appears within 120 seconds, transition to `agent_not_responding` with retry/help copy.

  **Must NOT do**: Do not show setup token, curl owner-session commands, machine token, or `/api/pair/claim` browser behavior. Do not put the pair token in `location.href`, `document.title`, `history.pushState`, or `history.replaceState`. Do not use WebSocket.

  **Parallelization**: Can Parallel: NO | Wave 2 | Blocks: 6, 7 | Blocked By: 1, 2, 3

  **References**:
  - App empty state: `web/src/App.tsx:182-187`.
  - `StatePanel` button shape: `web/src/App.tsx:260-271`.
  - Existing test style: `web/src/App.test.tsx:12-44`, `web/src/resourceEditor.test.tsx:12-69`.
  - Existing API client: `web/src/api.ts:19-99`.

  **Acceptance Criteria**:
  - [ ] `cd web && pnpm test -- --run src/onboardingWizard.test.tsx src/App.test.tsx` passes.
  - [ ] Test proves clicking `첫 머신 등록` calls `/api/pair/init` and renders one generated enroll command.
  - [ ] Test proves `claimed` poll state changes copy to heartbeat waiting.
  - [ ] Test proves connected dashboard refresh closes/succeeds the wizard.
  - [ ] Test proves expired invite shows retry copy and no background polling continues after cancel.
  - [ ] Test proves the wizard renders `Run from your neul checkout:` before the `go run` command.
  - [ ] Test proves `agent_not_responding` appears after the 120-second heartbeat timeout.
  - [ ] Test proves `App.tsx` empty state uses `copy.dashboard.emptyState` and `StatePanel` invokes `onAction`.
  - [ ] Vitest/source scan proves pair token is not written to href/title/history.

  **QA Scenarios**:
  ```
  Scenario: Browser wizard happy path
    Tool: Browser use
    Steps: Start neul-server with temp DB and built SPA; unlock owner session in browser; open /; click "첫 머신 등록"; wait for generated command; run CLI enroll in tmux with that command; observe wizard transition to connected.
    Expected: Browser shows connected success and dashboard contains the enrolled machine.
    Evidence: evidence/task-4-agent-onboarding-wizard-browser.png and evidence/task-4-agent-onboarding-wizard-browser-log.txt

  Scenario: Expired invite retry
    Tool: Browser use
    Steps: Stub or seed an expired pair token; open wizard; wait for poll response with expired/not found; click retry.
    Expected: Browser shows retry path and creates a new command without stale token.
    Evidence: evidence/task-4-agent-onboarding-expired-browser.png

  Scenario: Claimed agent never heartbeats
    Tool: Browser use
    Steps: Stub pair poll as claimed but keep dashboard without the machine heartbeat for 120 seconds using fake timers/test harness.
    Expected: Browser shows `agent_not_responding` retry/help copy and stops the infinite waiting state.
    Evidence: evidence/task-4-agent-onboarding-timeout-test.txt

  Scenario: Pair token does not leak into browser chrome
    Tool: bash + Vitest
    Steps: run token leak test and `rg -n "location.href|document.title|history.pushState|history.replaceState" web/src`
    Expected: pair token appears only in the explicit command/copy buffer code path, not URL/title/history mutations.
    Evidence: evidence/task-4-agent-onboarding-token-leak-scan.txt
  ```

  **Commit**: YES | Message: `feat(web): add agent onboarding wizard` | Files: `web/src/OnboardingWizard.tsx`, `web/src/App.tsx`, `web/src/api.ts`, tests, styles

- [x] 5. Add user-facing `neul agent enroll`

  **What to do**: Extend `internal/cli/cli.go` with `neul agent enroll --server <url> --pair <token> [--config-dir <dir>] [--config <path>] [--connect-once] [--force]`. It wraps existing pair claim logic, writes config with `0600`, refuses to overwrite existing config unless `--force`, and when `--connect-once` is set invokes the real agent path with `agent.New(config).Tick(ctx)` so heartbeat, desired-state fetch, drift report, and command handling keep the same semantics as `neul-agent --once`. Output must be friendly and token-safe: server, machine id, config path, "Connected" when one tick succeeds. Keep `neul init --pair` working as backward-compatible debug flow. Document that `--force` only overwrites the local config and does not delete, revoke, or clean up the previously registered server-side machine/token.

  **Must NOT do**: Do not print machine token. Do not execute arbitrary shell. Do not install launchd/systemd. Do not mutate config on failed claim.

  **Parallelization**: Can Parallel: YES | Wave 2 | Blocks: 6, 7 | Blocked By: 1

  **References**:
  - CLI command dispatch: `internal/cli/cli.go:25-82`.
  - Current pair claim: `internal/cli/cli.go:153-188`.
  - Config write and permissions: `internal/cli/cli.go:213-227`.
  - Agent tick: `internal/agent/agent.go:69-105`.
  - CLI tests: `internal/cli/cli_test.go`.

  **Acceptance Criteria**:
  - [ ] Write failing tests first in `internal/cli/cli_test.go` for successful enroll, existing config refusal, `--force`, expired code error, and token redaction.
  - [ ] `TestAgentEnroll_connectOnceRequiresFullAgentTick` fails if implementation shortcuts heartbeat only or skips desired-state/drift failure handling.
  - [ ] `TestAgentEnroll_forceDoesNotDeletePriorServerMachine` proves `--force` does not call any delete/revoke path and records the known limitation.
  - [ ] `env -u GOROOT go test ./internal/cli -run TestAgentEnroll -count=1` passes.
  - [ ] `env -u GOROOT go test ./internal/agent ./internal/cli -run 'TestAgentTick|TestAgentEnroll' -count=1` passes.

  **QA Scenarios**:
  ```
  Scenario: CLI enroll connects once
    Tool: tmux
    Steps: Start neul-server with temp DB; unlock session; create pair token; tmux run `NEUL_CONFIG_DIR=<tmp> env -u GOROOT go run ./cmd/neul agent enroll --server http://127.0.0.1:<port> --pair <code> --connect-once`; capture pane.
    Expected: transcript contains "Machine enrolled" and "Connected"; config exists with 0600; dashboard API shows one machine with heartbeat.
    Evidence: evidence/task-5-agent-enroll-tmux.txt

  Scenario: Existing config requires force
    Tool: tmux
    Steps: Run enroll twice against the same config dir without `--force`.
    Expected: second run exits nonzero, prints "config already exists", and does not print any token.
    Evidence: evidence/task-5-agent-enroll-existing-config-tmux.txt

  Scenario: Force replaces local config only
    Tool: tmux + HTTP call
    Steps: Enroll once, enroll again with `--force` and a new pair token, then inspect dashboard/server state.
    Expected: local config points to the newest machine; old server machine is not deleted or silently revoked.
    Evidence: evidence/task-5-agent-enroll-force-limitation.txt
  ```

  **Commit**: YES | Message: `feat(cli): add agent enroll command` | Files: `internal/cli/cli.go`, `internal/cli/cli_test.go`, optional `internal/agent/*`

- [x] 6. Replace direct setup E2E with visible onboarding E2E

  **What to do**: Update `web/e2e/mvp-dashboard.spec.ts` so the full MVP scenario opens the real web empty state, clicks the onboarding CTA, captures the generated command from the UI, runs the command in a child process/tmux with a stable temp `--config-dir`, waits for connected UI, then continues existing package/dotfile/drift/repair assertions. Owner session setup may still use setup token in test fixture, but pair init/claim must be driven by visible wizard + CLI enroll, not direct helper calls. After CLI enroll, any later repair/drift agent tick in the same E2E must reuse the config written in that `--config-dir` instead of relying on a `PairClaim` object or machine token captured by a direct API helper.

  **Must NOT do**: Do not claim the pair token with a Playwright fetch helper. Do not bypass the wizard for the first-machine registration step.

  **Parallelization**: Can Parallel: NO | Wave 3 | Blocks: 8 | Blocked By: 2, 3, 4, 5

  **References**:
  - Current E2E direct setup: `web/e2e/mvp-dashboard.spec.ts:72-74`.
  - Current pair helper functions to remove/stop using for main path: `web/e2e/mvp-dashboard.spec.ts:219-245`.
  - Current server fixture: `web/e2e/mvp-dashboard.spec.ts:137-199`.

  **Acceptance Criteria**:
  - [ ] `cd web && pnpm exec playwright test e2e/mvp-dashboard.spec.ts --project=chromium` passes.
  - [ ] E2E action log proves it clicked the visible onboarding CTA and extracted the command from the page.
  - [ ] Search proves main full-flow test no longer calls `claimPairingCode(...)` directly.
  - [ ] E2E repair/drift helper reads the CLI-written config from the stable `--config-dir`; no test path depends on direct `PairClaim` machineToken capture.
  - [ ] Screenshots show command-ready and connected states.

  **QA Scenarios**:
  ```
  Scenario: Full onboarding browser flow
    Tool: Browser use
    Steps: Playwright opens fresh instance, clicks "첫 머신 등록", copies generated command, runs it with temp config, waits for connected state, then continues drift/repair flow.
    Expected: first machine appears without direct browser-side pair claim helper; repair flow still passes.
    Evidence: evidence/task-6-agent-onboarding-e2e-browser.png and evidence/task-6-agent-onboarding-e2e-log.txt

  Scenario: No helper bypass
    Tool: bash
    Steps: rg -n "claimPairingCode\\(" web/e2e/mvp-dashboard.spec.ts
    Expected: no call from the main full-flow test; helper removed or only used in lower-level setup comments/tests if justified.
    Evidence: evidence/task-6-agent-onboarding-no-helper-bypass.txt

  Scenario: E2E agent tick reuses enrolled config
    Tool: Playwright + bash
    Steps: Run the MVP E2E and inspect logs for the temp `--config-dir` used by CLI enroll and later repair/drift tick.
    Expected: later agent tick reads the same CLI-written config; no machine token is passed from browser pair helpers.
    Evidence: evidence/task-6-agent-onboarding-config-dir-reuse.txt
  ```

  **Commit**: YES | Message: `test(e2e): cover visible agent onboarding` | Files: `web/e2e/mvp-dashboard.spec.ts`, evidence docs

- [x] 7. Update docs, QA notes, and UX copy cleanup

  **What to do**: Add `docs/qa/agent-onboarding.md` with exact scenario commands, screenshots, cleanup receipts, and known MVP limitations. Update `docs/mvp.md` first-machine registration section from pair-code/manual install to web wizard + `neul agent enroll --connect-once`. State that the dev command must be run from the neul checkout while packaging is not yet available. Document the `--force` limitation: it overwrites local config only and does not revoke/delete old server-side machines. Clean visible copy so primary UI says "첫 머신 등록", "명령 실행 대기 중", "agent 연결 확인 중", "agent 응답 없음", and "연결됨". Keep technical English only where allowed by `web/src/copy.ts`.

  **Must NOT do**: Do not claim native GUI or real background service install is done.

  **Parallelization**: Can Parallel: YES | Wave 3 | Blocks: 8 | Blocked By: 4, 5

  **References**:
  - MVP onboarding text to update: `docs/mvp.md:21-32`.
  - Existing QA doc pattern: `docs/qa/mvp-dashboard.md`.
  - Existing Korean copy tests: `web/src/copy.test.ts`.

  **Acceptance Criteria**:
  - [ ] `rg -n "agent enroll|connect-once|Run from your neul checkout|--force|명령 실행 대기|agent 연결 확인|agent 응답 없음|연결됨" docs/mvp.md docs/qa/agent-onboarding.md web/src/copy.ts` finds the new copy.
  - [ ] `cd web && pnpm test -- --run src/copy.test.ts` passes.
  - [ ] `rg -n "native GUI|menubar|launchd installed|systemd installed|OAuth|SSO|WebSocket" docs/qa/agent-onboarding.md docs/mvp.md` shows only explicit exclusions/next-step notes.

  **QA Scenarios**:
  ```
  Scenario: QA doc matches real flow
    Tool: bash
    Steps: rg -n "go run ./cmd/neul agent enroll|Run from your neul checkout|--connect-once|--force|screenshot|cleanup" docs/qa/agent-onboarding.md
    Expected: QA doc includes command, evidence path, and cleanup receipt.
    Evidence: evidence/task-7-agent-onboarding-doc-scan.txt

  Scenario: No overclaiming
    Tool: bash
    Steps: rg -n "native GUI|menubar|launchd installed|systemd installed|OAuth|SSO|WebSocket" docs/qa/agent-onboarding.md docs/mvp.md
    Expected: matches only say excluded, out-of-scope, or future work.
    Evidence: evidence/task-7-agent-onboarding-no-overclaim.txt
  ```

  **Commit**: YES | Message: `docs(onboarding): document agent enroll UX` | Files: `docs/mvp.md`, `docs/qa/agent-onboarding.md`, `web/src/copy.ts`, tests

- [x] 8. Final CI, cleanup, and regression guardrails

  **What to do**: Ensure `.github/workflows/ci.yml` still covers the updated E2E. Run full local verification: Go tests, Vitest, Biome, build, Playwright smoke+MVP. Add or update scope guard tests proving no WebSocket/secrets/install script/native GUI scope creep. Clean `web/dist`, `web/test-results`, temp DBs, tmux sessions, and ports.

  **Must NOT do**: Do not publish, deploy, create release binaries, or commit generated build output.

  **Parallelization**: Can Parallel: NO | Wave 4 | Blocks: final verification | Blocked By: 6, 7

  **References**:
  - Current CI: `.github/workflows/ci.yml`.
  - Scope guard tests: `internal/server/security_test.go`, `web/src/scope.test.tsx`, `internal/agent/agent_test.go:100-108`.
  - Final cleanup pattern: `evidence/final-generated-cleanup.txt`.

  **Acceptance Criteria**:
  - [ ] `env -u GOROOT go test ./...` passes.
  - [ ] `pnpm --dir web test -- --run` passes.
  - [ ] `pnpm --dir web exec biome check src index.html package.json tsconfig.json vite.config.ts pnpm-workspace.yaml playwright.config.ts vitest.config.ts biome.json e2e` passes.
  - [ ] `pnpm --dir web build` passes.
  - [ ] `cd web && pnpm exec playwright test e2e/smoke.spec.ts e2e/mvp-dashboard.spec.ts --project=chromium` passes.
  - [ ] `rg -n "location.href|document.title|history.pushState|history.replaceState" web/src` has only reviewed token-safe matches or no matches.
  - [ ] `lsof` shows no QA server ports, and generated directories are removed.

  **QA Scenarios**:
  ```
  Scenario: Full local CI equivalent
    Tool: bash
    Steps: run Go tests, pnpm frozen install if needed, Vitest, Biome, build, Playwright smoke+MVP.
    Expected: all commands exit 0.
    Evidence: evidence/task-8-agent-onboarding-ci-local.txt

  Scenario: Cleanup receipt
    Tool: bash
    Steps: rm -rf web/dist web/test-results; lsof -nP -iTCP:5173,18081,18082 -sTCP:LISTEN; tmux ls | rg "neul|ulw-qa" || true.
    Expected: no QA listeners/sessions remain; generated dirs absent.
    Evidence: evidence/task-8-agent-onboarding-cleanup.txt

  Scenario: Browser token leak guard
    Tool: bash
    Steps: rg -n "location.href|document.title|history.pushState|history.replaceState" web/src
    Expected: no token-bearing navigation/title/history code paths are present.
    Evidence: evidence/task-8-agent-onboarding-token-leak-guard.txt
  ```

  **Commit**: YES | Message: `ci(onboarding): verify agent enroll flow` | Files: `.github/workflows/ci.yml`, tests, evidence

## Final Verification Wave (MANDATORY - after ALL implementation tasks)
> ALL must APPROVE. Present consolidated results to user and get explicit "okay" before completing.
- [x] F1. Plan Compliance Audit
  - Verify Tasks 1-8 are checked, every task has RED/GREEN and manual QA evidence, and no source task depends on generated `web/dist`.
  - Command: `rg -n "task-[1-8]-agent-onboarding.*(red|green|qa|browser|http|tmux|cleanup)" evidence docs/qa`
- [x] F2. Code Quality Review
  - In Codex sessions, spawn `codex-ultrawork-reviewer` with this plan, full diff, evidence ledger, and QA artifacts.
  - If that reviewer is unavailable in the execution environment, run Claude Code Assist in read-only diff/evidence review mode with Opus and record the transcript.
  - Binding result: unconditional approval from an available reviewer path only.
- [x] F3. Real Manual QA
  - Run the visible onboarding browser scenario against `neul-server` serving the built SPA/API, plus tmux CLI enroll and `neul-agent --once`.
  - Capture screenshots, tmux transcript, server log, and cleanup receipt.
- [x] F4. Scope Fidelity Check
  - Commands:
    - `rg -n "WebSocket|/ws|/install.sh|curl \\| sh|native GUI|menubar|OAuth|SSO|billing|secret value" cmd internal web/src docs plans`
    - `rg -n "setupToken|/api/session/local|curl -i|neul init --pair" web/src docs/mvp.md internal/domain/contracts.md`
    - `rg -n "location.href|document.title|history.pushState|history.replaceState" web/src`
  - Expected: only exclusions, debug/backward-compatible docs, or tests; no primary user-facing onboarding copy uses the old manual flow.

## Commit Strategy
- Use atomic Conventional Commits.
- Do not auto-commit unless the user asks.
- Suggested sequence:
  1. `docs(onboarding): define agent enroll flow`
  2. `feat(web): add pairing API client`
  3. `test(server): harden onboarding pair poll`
  4. `feat(web): add agent onboarding wizard`
  5. `feat(cli): add agent enroll command`
  6. `test(e2e): cover visible agent onboarding`
  7. `docs(onboarding): document agent enroll UX`
  8. `ci(onboarding): verify agent enroll flow`

## Success Criteria
- A new user no longer needs curl, setup-token API calls, or manual pair-code API steps to register the first machine.
- The web empty state starts a visible onboarding wizard.
- The generated command runs `neul agent enroll --connect-once`, writes config safely, sends one heartbeat, and reports success without printing secrets.
- The browser moves from waiting to connected after first heartbeat.
- Existing package/dotfile/drift/repair MVP E2E still passes.
- Old `neul init --pair --server` remains available for debug/backward compatibility but is no longer primary product copy.
- Scope guardrails remain intact: no WebSocket, no secrets, no hosted auth, no real launchd/systemd install, no `/install.sh`, no native GUI.

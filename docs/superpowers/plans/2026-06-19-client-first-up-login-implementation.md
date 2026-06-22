# Client-First Up Login Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the client-first `neul login` plus `neul up` flow from the approved design spec, keeping browser-visible data non-secret and making connected state depend on a durable agent heartbeat.

**Architecture:** Add a short-lived approval table and approval endpoints beside the existing pair-code system, but keep `/api/pair/claim` as the only machine credential creator. `neul login` performs approval and enrollment only; `neul up` starts/verifies the long-running agent and reports connected only from fresh run-loop status. Docs, web copy, and validation gates move from deep-link/callback enrollment to the login/up split.

**Tech Stack:** Go 1.x, SQLite migrations executed idempotently on each boot, existing `net/http` server, existing CLI package, React 19/Vite/TypeScript, Vitest, Playwright, Biome.

---

## Source Spec

Implement against `docs/superpowers/specs/2026-06-19-client-first-up-login-design.md`.

Do not implement websocket transport, hosted login, teams/RBAC/SSO, secrets runtime surfaces, remote terminal, `curl | sh`, or production package signing.

## File Structure

- Modify `internal/agent/status.go`: add status provenance fields used by `neul up`.
- Modify `internal/agent/run_loop.go`: write `mode: "run_loop"` receipts.
- Modify `internal/cli/agent_enroll.go`: write `mode: "connect_once"` receipts for diagnostic/enroll paths.
- Modify `internal/cli/cli.go`: route `neul login` and `neul up`.
- Create `internal/cli/login.go`: implement approval start/poll/claim and local config write.
- Create `internal/cli/up.go`: implement durable agent install/kickstart/status wait.
- Modify `internal/cli/config.go`: expose a config-exists helper used before `neul login` starts approval.
- Modify `internal/server/http.go`: register approval endpoints and owner approval route.
- Modify `internal/server/handlers_pairing.go`: add approval handlers and approval-aware `/api/pair/claim` metadata binding.
- Create `internal/server/approval_store.go`: approval row persistence helpers.
- Add `migrations/002_approval_records.sql`: idempotent approval table and indexes only.
- Modify `internal/store/migrations_test.go`: prove the new migration re-runs cleanly.
- Modify `internal/domain/contracts.md`, `docs/mvp.md`, `README.md`, `docs/qa/agent-onboarding.md`: update executable product contracts.
- Modify `scripts/validate-packaged-client-docs.sh`: replace old deep-link/callback assertions with login/up assertions.
- Modify `web/src/copy.ts` and `web/src/copy.test.ts`: rename pair-token security copy to pair-code and browser-safe approval copy.
- Modify `web/src/OnboardingWizard.tsx` and `web/src/onboardingWizard.test.tsx`: primary login command, secondary fallback pair-code generator.
- Add or modify web approval route files under `web/src` following the current app routing pattern in `App.tsx`.
- Modify `web/e2e/mvp-dashboard.spec.ts` and `web/e2e/mvp-flow.ts`: smoke the new copy and fallback path.

## Task 1: Status Receipt Provenance

**Files:**
- Modify: `internal/agent/status.go`
- Modify: `internal/agent/run_loop.go`
- Modify: `internal/cli/agent_enroll.go`
- Test: `internal/agent/status_test.go`
- Test: `internal/agent/run_loop_test.go`
- Test: `internal/cli/agent_enroll_test.go`

- [ ] **Step 1: Write failing status serialization tests**

Add tests that expect a `mode` field while preserving existing status fields:

```go
func TestWriteStatus_whenRunLoopMode_writesModeAndAttempt(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 19, 8, 0, 0, 0, time.UTC)
	err := writeStatus(statusWriteOptions{
		Path:                 filepath.Join(dir, "status.json"),
		Mode:                 "run_loop",
		LastHeartbeatAttempt: now,
		LastError:            &StatusError{Kind: "auth_failure", Message: "invalid token"},
	})
	require.NoError(t, err)
	got := readStatusFileForTest(t, filepath.Join(dir, "status.json"))
	require.Equal(t, "run_loop", got.Mode)
	require.Equal(t, now.Format(time.RFC3339Nano), got.LastHeartbeatAttempt)
	require.Equal(t, "auth_failure", got.LastError.Kind)
}

func TestWriteStatus_whenConnectOnceMode_writesDiagnosticMode(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 19, 8, 1, 0, 0, time.UTC)
	err := writeStatus(statusWriteOptions{
		Path:                 filepath.Join(dir, "status.json"),
		Mode:                 "connect_once",
		LastHeartbeatAttempt: now,
		LastHeartbeatAt:      now,
	})
	require.NoError(t, err)
	got := readStatusFileForTest(t, filepath.Join(dir, "status.json"))
	require.Equal(t, "connect_once", got.Mode)
}
```

- [ ] **Step 2: Run focused failing tests**

Run: `go test ./internal/agent ./internal/cli -run 'TestWriteStatus|TestRunLoop|TestEnroll'`

Expected: FAIL because `Mode` is not in the status struct/write options yet.

- [ ] **Step 3: Implement status mode**

Add `Mode string json:"mode"` to the status receipt and write options. Set:

```go
const (
	statusModeRunLoop     = "run_loop"
	statusModeConnectOnce = "connect_once"
)
```

Use `statusModeRunLoop` in `internal/agent/run_loop.go` and `statusModeConnectOnce` in connect-once enrollment/status writes.

- [ ] **Step 4: Run status tests**

Run: `go test ./internal/agent ./internal/cli -run 'Status|RunLoop|Enroll'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agent internal/cli
git commit -m "feat(agent): record status provenance"
```

## Task 2: Approval Persistence

**Files:**
- Add: `migrations/002_approval_records.sql`
- Create: `internal/server/approval_store.go`
- Modify: `internal/store/migrations_test.go`
- Test: `internal/server/handlers_pairing_test.go`

- [ ] **Step 1: Write failing migration idempotency test**

Extend `internal/store/migrations_test.go` so migrations are applied twice against one SQLite DB:

```go
func TestApplyMigrations_whenApprovalMigrationRunsTwice_isIdempotent(t *testing.T) {
	db := openTestSQLite(t)
	require.NoError(t, store.ApplyMigrations(db))
	require.NoError(t, store.ApplyMigrations(db))
	requireTableExists(t, db, "approval_records")
	requireColumnExists(t, db, "approval_records", "approval_pairing_id")
}
```

- [ ] **Step 2: Add idempotent approval migration**

Create `migrations/002_approval_records.sql` using only idempotent DDL:

```sql
CREATE TABLE IF NOT EXISTS approval_records (
  id TEXT PRIMARY KEY,
  nonce_hash TEXT NOT NULL,
  verifier_challenge TEXT NOT NULL,
  csrf_token TEXT NOT NULL,
  comparison_code TEXT NOT NULL,
  state TEXT NOT NULL,
  machine_name TEXT NOT NULL,
  machine_os TEXT NOT NULL,
  machine_arch TEXT NOT NULL,
  machine_agent_version TEXT NOT NULL,
  approval_pairing_id TEXT,
  created_at TEXT NOT NULL,
  expires_at TEXT NOT NULL,
  approved_at TEXT,
  cancelled_at TEXT,
  pair_code_issued_at TEXT,
  claimed_at TEXT,
  claimed_machine_id TEXT,
  claimed_retain_until TEXT,
  claim_failure_count INTEGER NOT NULL DEFAULT 0,
  last_failure_at TEXT,
  last_failure_ip TEXT
);

CREATE INDEX IF NOT EXISTS idx_approval_records_state_expires
  ON approval_records (state, expires_at);

CREATE INDEX IF NOT EXISTS idx_approval_records_pairing
  ON approval_records (approval_pairing_id);
```

- [ ] **Step 3: Add approval store helpers**

Create helpers in `internal/server/approval_store.go` for:

```go
type approvalRecord struct {
	ID                  string
	NonceHash           string
	VerifierChallenge   string
	CSRFToken           string
	ComparisonCode      string
	State               string
	Machine             pairClaimMachine
	ApprovalPairingID   sql.NullString
	ExpiresAt           string
	PairCodeIssuedAt    sql.NullString
	ClaimedAt           sql.NullString
	ClaimedMachineID    sql.NullString
	ClaimedRetainUntil  sql.NullString
	ClaimFailureCount   int
}
```

Implement insert, load-for-update, mark-approved, mark-cancelled, mark-pair-code-issued, mark-claimed, increment-failure, and lock helpers using existing DB patterns.

- [ ] **Step 4: Run migration tests**

Run: `go test ./internal/store ./internal/server -run 'Migration|Approval'`

Expected: PASS for migration tests; approval handler tests may still be absent.

- [ ] **Step 5: Commit**

```bash
git add migrations internal/store internal/server/approval_store.go
git commit -m "feat(server): add approval persistence"
```

## Task 3: Approval Server API

**Files:**
- Modify: `internal/server/http.go`
- Modify: `internal/server/handlers_pairing.go`
- Modify: `internal/server/approval_store.go`
- Create: `internal/server/approval_rate_limit.go`
- Test: `internal/server/handlers_pairing_test.go`
- Test: `internal/server/security_test.go`

- [ ] **Step 1: Write failing endpoint tests**

Add tests covering:

```go
func TestApprovalStart_whenValid_returnsApprovalURLAndComparisonCode(t *testing.T)
func TestApprovalApprove_whenMissingOwnerSession_returnsOwnerSessionRequired(t *testing.T)
func TestApprovalApprove_whenCSRFInvalid_returnsApprovalCSRFInvalid(t *testing.T)
func TestApprovalClaim_whenPending_returnsRetryAfter(t *testing.T)
func TestApprovalClaim_whenVerifierMatches_returnsPairCodeWithFreshTTL(t *testing.T)
func TestApprovalClaim_whenNonceMismatch_incrementsClaimFailureCount(t *testing.T)
func TestApprovalClaim_whenSixthClaimAuthFailure_locksApproval(t *testing.T)
func TestApprovalStart_whenIPRateLimitExceeded_returnsApprovalStartRateLimited(t *testing.T)
func TestApprovalApprove_whenOwnerSessionRateLimitExceeded_returnsApprovalApproveRateLimited(t *testing.T)
func TestApprovalStatus_whenOwnerSessionRateLimitExceeded_returnsApprovalStatusRateLimited(t *testing.T)
func TestApprovalClaim_whenPendingPollRateLimitExceeded_returnsApprovalClaimRateLimited(t *testing.T)
func TestApprovalStatus_whenLocked_returnsTerminalLockedState(t *testing.T)
func TestPairClaim_whenApprovalMetadataMismatch_rejectsBeforeMachineCredential(t *testing.T)
```

- [ ] **Step 2: Run failing server tests**

Run: `go test ./internal/server -run 'Approval|PairClaim'`

Expected: FAIL because routes and handlers do not exist yet.

- [ ] **Step 3: Register routes**

In `internal/server/http.go`, register the approval routes next to the existing
pairing routes and wrap owner-only routes with the same owner-session middleware
used by dashboard routes:

```go
router.Handle("POST /api/pair/approval/start", handleApprovalStart(db, clock))
router.Handle("POST /api/pair/approval/approve", ownerSessionRequired(handleApprovalApprove(db, clock)))
router.Handle("POST /api/pair/approval/claim", handleApprovalClaim(db, clock))
router.Handle("GET /api/pair/approval/status", ownerSessionRequired(handleApprovalStatus(db, clock)))
```

Before editing, inspect `internal/server/http.go` for the exact middleware name.
If it is not `ownerSessionRequired`, use the existing dashboard owner-session
wrapper and keep the route behavior identical to dashboard owner-only routes.

- [ ] **Step 4: Implement approval start**

Validate JSON request shape:

```json
{
  "nonce": "nonce_base64url_32_bytes",
  "verifierChallenge": "sha256_verifier_base64url",
  "machine": {
    "name": "joon-macbook",
    "os": "darwin",
    "arch": "arm64",
    "agentVersion": "0.1.0"
  }
}
```

Generate `approvalId`, `csrfToken`, `comparisonCode`, hash the nonce, store the verifier challenge, and return HTTP 201:

```json
{
  "approvalId": "approval_...",
  "approvalUrl": "<origin>/enroll/approve?approval=approval_...&nonce=...",
  "comparisonCode": "742-918",
  "expiresAt": "2026-06-19T08:10:00Z",
  "pollAfterMs": 2000
}
```

- [ ] **Step 5: Implement approve/status**

Approve validates owner session, same-origin header, CSRF token, approval id, nonce hash, and state. `decision: "approve"` marks approved; `decision: "cancel"` marks cancelled. Status returns pending/approved machine preview + CSRF + comparison code, claimed machine details, or terminal expired/cancelled/locked.

- [ ] **Step 6: Implement claim exchange**

For `approval/claim`:

```go
if approvalIDNotFound {
	writeJSONError(w, http.StatusNotFound, "approval_not_found", "Approval was not found.")
	return
}
if nonceMismatch || malformedVerifier || verifierMismatch {
	incrementClaimFailureOrLock()
	writeJSONError(w, http.StatusForbidden, "approval_claim_denied", "Approval claim was denied.")
	return
}
```

On approved state, create exactly one pair code in `pairing_codes`, set `approvalPairingId`, and return `pairCodeExpiresAt = now + 10m`. Later polls return `approval_pair_code_issued` until `/api/pair/claim` consumes the code; after consumption return `claimed`.

- [ ] **Step 7: Implement approval rate limits**

Create `internal/server/approval_rate_limit.go` with a small process-local
windowed limiter that accepts the request key, current time, max count, and
window duration. Wire the concrete limits from the spec:

```go
allowApprovalStart(ip, now)      // 10/min/IP and 30/hour/IP
allowApprovalApprove(session, now) // 20/min/session and 60/hour/session
allowApprovalStatus(session, ip, now) // 120/min/session and 240/min/IP
allowApprovalClaim(approvalID, ip, now) // 90/min/approval and 120/min/IP
```

Return the mandated 429 codes:

```go
approval_start_rate_limited
approval_approve_rate_limited
approval_status_rate_limited
approval_claim_rate_limited
```

Use fake clocks in tests so threshold cases do not sleep.

- [ ] **Step 8: Enforce approval metadata in pair claim**

After existing pair-code lookup and before machine creation, look up approval record by `approval_pairing_id = pairing.id`. If found, compare `machine.name`, `machine.os`, `machine.arch`, and `machine.agentVersion` to expected metadata. On mismatch, return:

```json
{
  "error": {
    "code": "approval_machine_metadata_mismatch",
    "message": "Machine metadata does not match the approved request."
  }
}
```

Do not create machine credentials on mismatch.

- [ ] **Step 9: Run server tests**

Run: `go test ./internal/server -run 'Approval|PairClaim|Security'`

Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/server
git commit -m "feat(server): add browser approval API"
```

## Task 4: CLI Login And Up

**Files:**
- Modify: `internal/cli/cli.go`
- Create: `internal/cli/login.go`
- Create: `internal/cli/up.go`
- Modify: `internal/cli/config.go`
- Test: `internal/cli/cli_test.go`
- Test: `internal/cli/agent_install_test.go`
- Test: `internal/cli/agent_status_test.go`

- [ ] **Step 1: Write failing `neul login` tests**

Add tests:

```go
func TestLogin_whenConfigExists_failsBeforeApprovalStart(t *testing.T)
func TestLogin_whenApprovalApproved_claimsPairCodeAndWritesConfig0600(t *testing.T)
func TestLogin_whenApprovalExpired_printsRecoverableFailure(t *testing.T)
func TestLogin_whenApprovalCancelled_printsRecoverableFailure(t *testing.T)
func TestLogin_whenPairCodeAlreadyIssued_printsRestartGuidance(t *testing.T)
```

The config-exists test must assert the fake server receives zero `approval/start` requests.

- [ ] **Step 2: Write failing `neul up` tests**

Add tests:

```go
func TestUp_whenNoConfig_printsLoginGuidance(t *testing.T)
func TestUp_whenLaunchAgentInstallFails_returnsAgentNotRunning(t *testing.T)
func TestUp_whenRunLoopHeartbeatFresh_reportsConnected(t *testing.T)
func TestUp_whenFreshAuthFailureAtDeadline_returnsAuthInvalid(t *testing.T)
func TestUp_whenStaleErrorOnly_returnsLocalHeartbeatMissing(t *testing.T)
func TestUp_whenConnectOnceReceiptFresh_doesNotReportConnected(t *testing.T)
```

- [ ] **Step 3: Run failing CLI tests**

Run: `go test ./internal/cli -run 'Login|Up'`

Expected: FAIL because commands do not exist yet.

- [ ] **Step 4: Add CLI routes**

In `internal/cli/cli.go`, route:

```go
case "login":
	return runLogin(ctx, args[1:], deps)
case "up":
	return runUp(ctx, args[1:], deps)
```

Keep existing `agent enroll`, `agent install`, and legacy/debug paths intact.

- [ ] **Step 5: Implement login**

Order is mandatory:

1. Parse `--server`.
2. Check existing config; if present, print machine-already-configured copy and return without server calls.
3. Generate nonce and verifier with at least 32 bytes of randomness.
4. POST `approval/start`.
5. Print URL and comparison code, attempt browser open through existing local helper if present.
6. Poll `approval/claim` every `retryAfterMs` or 2s.
7. On `approved`, call `/api/pair/claim` with returned `pairCode` and machine metadata.
8. Write config with `0600`.
9. Print Korean-first success copy and `neul up` next action.

- [ ] **Step 6: Implement up**

Use existing LaunchAgent install/kickstart helpers. Record `upStartedAt` before install/kickstart. Poll status file up to 60 seconds:

```go
if status.Mode == "run_loop" &&
   status.LastHeartbeatAt >= upStartedAt &&
   status.LastError == nil {
	return connected
}
if status.Mode == "run_loop" &&
   status.LastHeartbeatAttempt >= upStartedAt &&
   status.LastError != nil {
	latestFreshError = status.LastError.Kind
}
```

At deadline, return mapped latest fresh error if present; otherwise `local_heartbeat_missing`. Do not accept `connect_once` receipts.

- [ ] **Step 7: Run CLI tests**

Run: `go test ./internal/cli -run 'Login|Up|Install|Status'`

Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/cli
git commit -m "feat(cli): add login and up commands"
```

## Task 5: Contracts And Docs Gates

**Files:**
- Modify: `internal/domain/contracts.md`
- Modify: `docs/mvp.md`
- Modify: `README.md`
- Modify: `docs/qa/agent-onboarding.md`
- Modify: `scripts/validate-packaged-client-docs.sh`
- Modify: `web/src/copy.ts`
- Modify: `web/src/copy.test.ts`

- [ ] **Step 1: Update docs validation script and copy contract together**

Edit `scripts/validate-packaged-client-docs.sh` exactly as the spec maps:

```bash
require_text "web/src/copy.ts" "neul login --server <origin>" "web target login command"
require_text "docs/mvp.md" "browser-safe approval handoffs" "MVP browser-safe approval handoffs"
require_text "web/src/copy.ts" "browserSafeApprovalHandoffs" "web approval handoff copy"
```

Delete required checks for `neul://enroll?server=`, `local callback`, `Device code is fallback-only`, and `allowedPairTokenHandoffs`. Keep absent checks preventing `--pair` from entering primary packaged sections.

In the same task, update `web/src/copy.ts` and `web/src/copy.test.ts` so the
new validation-script assertions have matching copy before any passing docs
gate is expected:

```ts
expect(copy.onboarding.commandTemplate).toBe("neul login --server <origin>");
expect(copy.onboarding.fallbackCommand).toContain("--pair <pair-code>");
expect(copy.security.pairCodeKind).toContain("/api/pair/claim");
expect(copy.security.browserSafeApprovalHandoffs).toContain("approval id");
expect(JSON.stringify(copy)).not.toContain("allowedPairTokenHandoffs");
```

- [ ] **Step 2: Run copy tests and docs gate to see expected doc failures**

Run: `pnpm --dir web test -- --run src/copy.test.ts`

Expected: PASS.

Run: `make verify-docs`

Expected: FAIL only on markdown/README/contract text that has not been updated
yet, not on `web/src/copy.ts`.

- [ ] **Step 3: Update contracts and docs**

Rewrite packaged-primary sections so:

- `neul login --server <origin>` enrolls only.
- Browser approval receives approval id, nonce, comparison code, machine preview, CSRF, and status only.
- `/api/pair/claim` remains the only machine credential creator.
- `neul up` owns durable running/connected state.
- Fallback/debug uses `--pair <pair-code>` only in secondary copy.

- [ ] **Step 4: Run docs gate**

Run: `make verify-docs`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/domain/contracts.md docs README.md scripts/validate-packaged-client-docs.sh web/src/copy.ts web/src/copy.test.ts
git commit -m "docs: update login up product contracts"
```

## Task 6: Web Copy And Onboarding UI

**Files:**
- Modify: `web/src/OnboardingWizard.tsx`
- Modify: `web/src/onboardingWizard.test.tsx`
- Modify: `web/src/App.tsx`
- Add or modify: approval page component under `web/src`
- Test: `web/src/App.test.tsx`
- Test: `web/e2e/mvp-dashboard.spec.ts`

- [ ] **Step 1: Write failing onboarding and approval-page tests**

Update `web/src/onboardingWizard.test.tsx` and `web/src/App.test.tsx` to assert
that the primary wizard command is `neul login --server <origin>`, the primary
command has no `--pair`, the secondary fallback/debug block can still generate
and display a real pair-code command, and the approval page stops polling on
`locked`.

- [ ] **Step 2: Update onboarding wizard**

Primary block shows `neul login --server <origin>` and no `--pair`. Secondary fallback/debug block retains pair-code generator via `/api/pair/init` and `/api/pair/poll`, displays `--pair <pair-code>`, and remains visually secondary.

- [ ] **Step 3: Add approval page behavior**

Route `/enroll/approve?approval=...&nonce=...` should:

- fetch `GET /api/pair/approval/status`
- show owner-session-required copy on 401
- show machine preview and comparison code
- POST approve/cancel with CSRF token
- stop polling on `expired`, `cancelled`, or `locked`
- show `neul up` guidance on `claimed`

- [ ] **Step 4: Run web unit tests**

Run: `pnpm --dir web test -- --run`

Expected: PASS.

- [ ] **Step 5: Run Biome**

Run: `pnpm --dir web exec biome check src index.html package.json tsconfig.json vite.config.ts pnpm-workspace.yaml playwright.config.ts vitest.config.ts biome.json e2e`

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add web
git commit -m "feat(web): update onboarding for login approval"
```

## Task 7: End-To-End Verification

**Files:**
- Modify: `web/e2e/mvp-dashboard.spec.ts`
- Modify: `web/e2e/mvp-flow.ts`
- Modify: `docs/qa/agent-onboarding.md`

- [ ] **Step 1: Update E2E expectations**

Assert primary onboarding copy contains `neul login --server <origin>` and not `--pair`. Assert fallback/debug block can still create a visible pair code and command.

- [ ] **Step 2: Run full Go tests**

Run: `go test ./...`

Expected: PASS.

- [ ] **Step 3: Run docs and demo verification**

Run:

```bash
make verify-docs
make verify-demo
```

Expected: PASS.

- [ ] **Step 4: Run web build and E2E**

Run:

```bash
pnpm --dir web build
cd web && pnpm exec playwright test e2e/smoke.spec.ts e2e/mvp-dashboard.spec.ts --project=chromium
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web docs
git commit -m "test: cover login up onboarding flow"
```

## Final Verification

- [ ] Run `go test ./...`
- [ ] Run `make verify-docs`
- [ ] Run `make verify-demo`
- [ ] Run `pnpm --dir web test -- --run`
- [ ] Run `pnpm --dir web exec biome check src index.html package.json tsconfig.json vite.config.ts pnpm-workspace.yaml playwright.config.ts vitest.config.ts biome.json e2e`
- [ ] Run `pnpm --dir web build`
- [ ] Run `cd web && pnpm exec playwright test e2e/smoke.spec.ts e2e/mvp-dashboard.spec.ts --project=chromium`
- [ ] Review the final diff for secrets in URLs, logs, localStorage, document title, or browser history.

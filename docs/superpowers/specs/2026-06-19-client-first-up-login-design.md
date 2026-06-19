# Client-first Up/Login Design

작성일: 2026-06-19

## Purpose

Neul의 첫 client-first implementation slice는 아직 구현되지 않은 canonical
packaged-client command인 `neul enroll --server <origin>`을
`neul login --server <origin>`으로 교체하고, `neul up`을 service/fleet
steady-state 명령으로 분리한다.

제품 모델은 Tailscale과 비슷하다.

- `neul login --server <origin>`: 브라우저 approval을 통해 owner workspace에
  로그인하고 현재 machine credential을 만든다.
- `neul up`: 이미 로그인한 machine에서 user-level agent를 켜거나 현재 fleet
  상태를 설명한다.

Setup token, pair code, machine token은 내부 credential이다. Primary CLI output,
browser history, document title, logs, localStorage, general URL query string의
주인공이 되어서는 안 된다.

Vocabulary:

- `pair code`: one-time claim value consumed by `/api/pair/claim`.
- `machine token`: durable bearer credential returned by `/api/pair/claim` and
  stored only in local machine config.
- `pair token`: legacy wording to remove from product/docs copy in this slice;
  use `pair code` for the claim value.

## Scope

이번 implementation spec은 macOS first `Start + Authenticate + Join Fleet`만
다룬다.

Included:

- `neul up` command surface
- `neul login --server <origin>` command surface
- canonical docs/tests migration from planned `neul enroll --server <origin>`
  to `neul login --server <origin>`
- polling based browser approval
- pair claim and local config write with `0600` permissions
- `login` enrollment success separated from `up` connected/running success
- web onboarding copy updated to `neul login` primary path
- fallback/debug command retained as explicit secondary path
- tests that prove token handoff guardrails

Excluded:

- Windows/Linux service rollout
- hosted/team auth
- package discovery/catalog
- secrets UI/API/value handling
- VPN or mesh networking behavior
- arbitrary shell execution
- production macOS signing, notarization, or stapling
- device-code fallback

## Product Shape

### `neul up`

`neul up` is the command users run when they want Neul running on this machine.

Fresh machine behavior:

1. Read the local config path.
2. If no usable machine config exists, print a product-level next action:

   ```text
   이 machine은 아직 Neul fleet에 연결되지 않았습니다.
   먼저 실행: neul login --server <origin>
   ```

3. Do not print setup tokens, pair tokens, or machine tokens.
4. Exit without creating credentials.

Already joined behavior:

1. Read local config.
2. Read local agent status from the existing status file.
3. On macOS, verify the LaunchAgent with the existing `agent status` probe and
   install/kickstart with the existing `agent install` LaunchAgent path when the
   config and agent binary are present.
4. Wait for the long-running agent to write a successful heartbeat status. A
   one-shot diagnostic heartbeat must not be used to satisfy connected state.
   `neul up` must not call owner-session routes such as `/api/dashboard`,
   `/api/machines`, or `/api/machines/:machineId`.
5. The wait is bounded: record `upStartedAt` before install/kickstart, poll the
   status file for up to 60 seconds, and treat connected as true only when the
   status receipt was written by the long-running agent, `lastHeartbeatAt >=
   upStartedAt`, and `lastError` is empty or `null`.
6. Outcome precedence is deterministic. If LaunchAgent install/kickstart fails
   before a run loop is started, print `agent_not_running`. Otherwise, a fresh
   status receipt with `lastError.kind` after `upStartedAt` wins over the bare
   timeout and maps deterministically:
   - `auth_failure` -> `auth_invalid`
   - `connection_failure` -> `server_unreachable`
   - `server_failure` -> `server_error`
   - `rate_limited` -> `rate_limited`
   If no fresh success and no fresh structured error appear within 60 seconds,
   print `local_heartbeat_missing`.
7. Print a short locally-derived running status summary.

`neul up` must not silently overwrite existing credentials. Any force/re-enroll
behavior belongs to an explicit later recovery surface.

### `neul login --server <origin>`

`neul login` is the interactive auth and enrollment command.

Flow:

1. Validate `--server`.
2. Generate a client nonce and a verifier with at least 32 bytes of randomness.
3. Request a browser approval URL from the server.
4. Print the approval URL and try to open it in the owner browser.
   If the local browser has no owner session, the page tells the user to open
   the same URL in an already-authenticated owner browser or first create an
   owner session on this browser.
5. Poll `POST /api/pair/approval/claim` until the
   owner approves, cancels, or the approval expires.
6. Exchange the approval nonce/verifier for an opaque pair code over the server
   API.
7. Claim the pair code with machine metadata through `/api/pair/claim`.
8. Write local config with `0600` permissions.
9. Report enrollment success and point to `neul up`.

`neul login` does not claim durable connected state. It creates local machine
credentials and binds the machine to the owner workspace. `neul up` is
responsible for starting or verifying the long-running agent and producing the
heartbeat that makes the dashboard connected.

Primary success copy should read like:

```text
로그인이 완료되었습니다.
이 machine을 계속 연결하려면 실행: neul up
```

Failure copy should stay product-level and recoverable:

- browser approval expired
- browser approval was cancelled
- server is unreachable
- machine already has config

`owner session is required` is browser-page copy, not a CLI terminal outcome.
The CLI cannot observe browser session state through `approval/claim`; it only
continues polling until the approval succeeds, is cancelled, expires, or the
server polling path fails.

## Server And Browser Handoff

The server adds the thinnest approval surface needed for `neul login`.

Target endpoints:

- `POST /api/pair/approval/start`
- `POST /api/pair/approval/approve`
- `POST /api/pair/approval/claim`
- `GET /api/pair/approval/status`

`approval/start` receives the client nonce, a PKCE-style verifier challenge,
and machine preview metadata. `machine.name` is the display hostname used by
the existing `/api/pair/claim` request shape. The server derives the approval
URL origin from configured external origin or the request origin; the request
body does not trust a client-supplied server origin.

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

It returns an approval id and an approval URL that the CLI opens in the
browser:

```json
{
  "approvalId": "approval_01HX...",
  "approvalUrl": "https://neul.example/enroll/approve?approval=approval_01HX...&nonce=nonce_base64url_32_bytes",
  "comparisonCode": "742-918",
  "expiresAt": "2026-06-19T08:10:00Z",
  "pollAfterMs": 2000
}
```

The approval URL is an owner browser route containing the non-secret approval id
and nonce, for example:

```text
<origin>/enroll/approve?approval=<approval-id>&nonce=<nonce>
```

The CLI prints the `comparisonCode` next to the opened approval URL. The browser
approval page displays the same code after loading `approval/status`, and its
approve button copy tells the owner to approve only when the browser code
matches the terminal code. This is the out-of-band binding for unauthenticated
client-started approval URLs; owner-visible machine metadata is helpful context,
but it is client supplied and not trusted as the binding.

`approval/start` is unauthenticated because a fresh CLI has no owner session.
It must be abuse-bounded with short TTL records, per-IP rate
limits, and no secret material in the response. Approval records expire exactly
10 minutes after `approval/start` while they are pending or approved but
unclaimed, matching the existing pairing TTL.
Concrete limits for MVP: max 10 approval starts per minute per source IP, max
30 approval starts per hour per source IP, and HTTP 429
`approval_start_rate_limited` after either threshold.

`approval/approve` is a CSRF-protected owner-session action. The approval page
must show the requesting machine context before approval: hostname, OS,
architecture, agent version, and requested time. It also includes a per-approval
CSRF token. The approve POST receives:

```json
{
  "approvalId": "approval_01HX...",
  "nonce": "nonce_base64url_32_bytes",
  "csrfToken": "csrf_base64url_32_bytes",
  "decision": "approve"
}
```

`decision` is either `approve` or `cancel`. The approve POST must validate owner
session, same-origin `Origin` or `Referer`, and the per-approval CSRF token
before marking the short-lived approval record as approved or cancelled, bound
to the client nonce, verifier challenge, and machine preview metadata. It does
not put pair code, pair token, or machine token in the browser URL. Its response
contains only the resulting state and expiry:

```json
{
  "status": "approved",
  "expiresAt": "2026-06-19T08:10:00Z"
}
```

If the approval page is opened in a browser without owner session, it must not
create or expose credentials. It shows recoverable copy: open this same approval
URL in an already-authenticated owner browser, or create/restore the owner
session first. The CLI continues polling `approval/claim` until approved,
cancelled, or expired. The approval page must also say not to approve an
approval URL received from another person or chat; this client-started approval
is supported only when the owner personally started `neul login` and can compare
the browser code against the terminal code. Moving the URL to another
authenticated owner browser is acceptable only when the owner still has that
terminal code in view.

`POST /api/pair/approval/claim` is a machine-client action that receives only
the approval id, nonce, and verifier. It does not receive machine metadata and
does not require owner session.

```json
{
  "approvalId": "approval_01HX...",
  "nonce": "nonce_base64url_32_bytes",
  "verifier": "verifier_base64url_32_bytes"
}
```

Before the owner approves, it returns HTTP 200:

```json
{
  "status": "pending",
  "approvalExpiresAt": "2026-06-19T08:10:00Z",
  "retryAfterMs": 2000
}
```

After cancellation or expiry it returns the canonical error envelope described
in `internal/domain/contracts.md`: HTTP 409 `approval_cancelled` or HTTP 410
`approval_expired`. After owner approval and verifier validation, it creates
one opaque pair code from the same `pairing_codes` storage used by
`/api/pair/init` and returns HTTP 200:

```json
{
  "status": "approved",
  "pairCode": "pair_...",
  "pairCodeExpiresAt": "2026-06-19T08:15:00Z"
}
```

Approval expiry and pair-code expiry are separate. The approval record expires
10 minutes after `approval/start`. The pair code is created only after
`approval/claim` validates an approved record, and the pairing row expires 10
minutes after that pair-code creation time. `pairCodeExpiresAt` is the pairing
row expiry, not the original approval expiry.

The server must reject missing or incorrect verifiers by computing
`SHA-256(submitted verifier)` and comparing it in constant time with the
verifier challenge stored by `approval/start`. The CLI then calls the existing
`/api/pair/claim` with that `pairCode` and machine metadata. Metadata matching
is enforced by `/api/pair/claim`, not by `approval/claim`. `approval/claim`
must be abuse-bounded: rate-limit by approval id and source IP, lock or expire
the approval after repeated verifier failures, and never reveal whether the
nonce or verifier was the wrong component.

`approval/claim` is intentionally single-issue. Under a transaction, exactly
one poll may create and receive the pair code. The approval record stores
`pairCodeIssuedAt` and the approval-created pairing row id, but not plaintext
pair code. A later `approval/claim` after the pair code was issued but before
`/api/pair/claim` consumption returns HTTP 409 `approval_pair_code_issued` with
recoverable copy telling the CLI to restart `neul login` if it did not receive
the code locally. A later `approval/claim` after `/api/pair/claim` consumption
returns HTTP 200:

```json
{
  "status": "claimed",
  "machineId": "machine_01HX...",
  "claimedAt": "2026-06-19T08:04:00Z"
}
```

After `/api/pair/claim` consumes the pair code, the approval record remains
queryable as `claimed` for 24 hours from `claimedAt` so the owner browser can
show the "run `neul up`" waiting state. Approval TTL gates pending/approved
credential release only; it does not turn an already claimed record back into
`expired`.

The metadata binding is enforced in `/api/pair/claim`: approval-created pair
codes carry expected hostname/name, OS, architecture, and agent version metadata
on the approval record, not on `pairing_codes`. This avoids non-idempotent
`ALTER TABLE pairing_codes ADD COLUMN ...` migrations because the current
migration runner re-executes every SQL file on each boot. When `approval/claim`
creates the pair-code row, it stores the created pairing row id in
`approvalPairingId`. When `/api/pair/claim` looks up the pair code, it checks
for an approval record with that `approvalPairingId`; if present, it compares
the submitted machine metadata against the approval record's expected metadata
before creating the machine credential. Ordinary `/api/pair/init` rows have no
matching approval record and keep existing fallback/debug behavior. After pair
claim succeeds, the approval record stores the claimed `machineId` so
owner-session browser UI can watch the right machine.

`GET /api/pair/approval/status` is an owner-session status endpoint for the
CLI-opened approval page. It receives approval id and returns one of:
`pending`, `approved`, `claimed`, `expired`, or `cancelled`. The
`pending` and `approved` responses include the machine preview metadata and a
per-approval CSRF token for the approve action. The `claimed` response includes
`machineId` and `claimedAt`. This endpoint never returns pair code, pair token,
machine token, setup token, or plaintext verifier.

Pending or approved response:

```json
{
  "status": "pending",
  "approvalId": "approval_01HX...",
  "expiresAt": "2026-06-19T08:10:00Z",
  "csrfToken": "csrf_base64url_32_bytes",
  "comparisonCode": "742-918",
  "machine": {
    "name": "joon-macbook",
    "os": "darwin",
    "arch": "arm64",
    "agentVersion": "0.1.0"
  }
}
```

Claimed response:

```json
{
  "status": "claimed",
  "machineId": "machine_01HX...",
  "claimedAt": "2026-06-19T08:04:00Z",
  "expiresAt": "2026-06-19T08:10:00Z"
}
```

Approval endpoint HTTP contract:

| Endpoint | Success | Terminal and error cases |
| --- | --- | --- |
| `POST /api/pair/approval/start` | `201 Created` with `approvalId`, `approvalUrl`, `comparisonCode`, `expiresAt`, `pollAfterMs` | `400 bad_json`, `400 approval_start_invalid`, `429 approval_start_rate_limited`, `500 approval_start_failed` |
| `POST /api/pair/approval/approve` | `200 OK` with `status: "approved" \| "cancelled"` and `expiresAt` | `400 bad_json`, `400 approval_approve_invalid`, `401 owner_session_required`, `403 approval_origin_invalid`, `403 approval_csrf_invalid`, `404 approval_not_found`, `409 approval_not_pending`, `410 approval_expired`, `429 approval_approve_rate_limited` |
| `POST /api/pair/approval/claim` | `200 OK` with `status: "pending"`, `status: "approved"` plus `pairCode`, or `status: "claimed"` | `400 bad_json`, `400 approval_claim_invalid`, `403 approval_claim_denied`, `404 approval_not_found`, `409 approval_cancelled`, `409 approval_pair_code_issued`, `410 approval_expired`, `423 approval_locked`, `429 approval_claim_rate_limited` |
| `GET /api/pair/approval/status` | `200 OK` with `pending`, `approved`, `claimed`, `expired`, or `cancelled` UI state | `401 owner_session_required`, `404 approval_not_found`, `429 approval_status_rate_limited` |

All non-2xx responses use the existing canonical JSON error envelope:

```json
{
  "error": {
    "code": "approval_expired",
    "message": "Approval expired."
  }
}
```

`approval/status` may return `expired` or `cancelled` as HTTP 200 because it is
an owner-session UI status poll. `approval/claim` returns HTTP 409/410 for those
terminal states because it is the machine credential-release exchange path.

Approval persistence:

- Add a migration for a new approval-record table before implementing these
  endpoints.
- Minimal fields: approval id, nonce hash, verifier challenge, plaintext CSRF
  token, plaintext comparison code, state, machine preview metadata, createdAt,
  expiresAt, approvedAt, cancelledAt, pairCodeIssuedAt, approvalPairingId,
  claimedAt, claimedMachineId, claimedRetainUntil, claim failure count, and
  last failure metadata needed for rate limiting.
- The table stores no pair code, machine token, setup token, or plaintext
  verifier.
- `approval/claim` creates the one-time pair code only after the approval record
  is approved and verifier validation passes.
- Pairing rows remain schema-compatible with current idempotent migrations.
  Approval-created expected metadata stays on the approval record. The link is
  `approval_records.approvalPairingId = pairing_codes.id`, populated when
  `approval/claim` creates the one-time pair code.

The implementation should reuse the existing pair claim machinery instead of
creating a second machine registration system. Existing `/api/pair/claim`
remains the credential creation point.

Guardrails:

- Owner browser session is required to approve.
- Pair code is single-use and short-lived.
- Approval status never returns pair code, pair token, or machine token.
- Approval start is unauthenticated but rate-limited and TTL-bounded.
- Approval claim rejects absent, malformed, or challenge-mismatched verifier.
- Approval claim is rate-limited and attempt-bounded because it releases the
  one-time pair code.
- Approval approve allows max 20 POST attempts per minute per owner session and
  max 60 POST attempts per hour per owner session.
- Approval status allows max 120 GET requests per minute per owner session and
  max 240 GET requests per minute per source IP.
- Approval claim allows max 90 pending polls per minute per approval id, max
  120 pending polls per minute per source IP, and max 5 verifier failures per
  approval id. The 6th verifier failure locks the approval with HTTP 423
  `approval_locked`; locked approvals never release pair code and must be
  restarted with `neul login`.
- Approval page shows requesting machine context before the owner approves.
- Approval approve validates owner session, same-origin request headers, and a
  per-approval CSRF token.
- Pair code and machine token are never written to server logs.
- Browser code must not receive or store pair code or machine token in
  localStorage.
- Browser code must not place pair code or machine token in `document.title`.
- Primary approval URL must not expose pair code or machine token in a general
  browser URL.
- Polling `POST /api/pair/approval/claim` is the only CLI-side approval wait
  mechanism in this slice. Browser-to-loopback wake-up and deep-link wake-up are
  deferred.
- Concurrent `neul login` runs use distinct approval ids, nonces, and verifier
  challenges.

This avoids relying on browser `fetch`, subresource loads, or top-level
navigation from the server origin to `http://127.0.0.1:<port>`. The sensitive
exchange happens from CLI to server.

## Contract Update

`internal/domain/contracts.md`, `docs/mvp.md`, `docs/qa/agent-onboarding.md`,
README packaged-primary copy, and web onboarding tests must be updated in the
same implementation change.

Required contract edits:

- Update Auth Defaults so pair tokens are no longer allowed in browser code,
  browser handoff payloads, `neul://...&pair=<token>`, or fallback/debug browser copy.
  The only browser-visible approval values are approval id, nonce, and
  non-secret status.
- Replace legacy `pair token` product/docs wording with `pair code` for the
  one-time `/api/pair/claim` value. Keep `machine token` for the durable bearer
  returned by `/api/pair/claim`.
- Require CLI verifier generation with at least 32 bytes of randomness.
- Replace the planned primary packaged-client command
  `neul enroll --server <origin>` with `neul login --server <origin>`.
- Rewrite Scope Guardrails so the old "no pending approval table" rule is
  replaced by this narrower rule: no general pending approval table for
  owner-created pair codes; short-lived client-started approval records are
  allowed only for `neul login`, expire in 10 minutes, contain no bearer
  credentials, and are not a hosted/team approval queue.
- Rewrite Timing And Version Defaults so `GET /api/pair/poll` remains the
  source of truth only for fallback/debug pair-code expiry. Approval expiry uses
  `GET /api/pair/approval/status` and the same 10 minute TTL.
- Remove or rewrite device-code fallback copy for this slice. Device code is
  out of scope and must not be described as a fallback for removed callback or
  `neul://` handoffs.
- Add a migration for approval records. Do not add columns to `pairing_codes`
  unless the migration runner first gains idempotent applied-version tracking.
- Rewrite the approval API subsection so it includes
  `POST /api/pair/approval/claim`, `GET /api/pair/approval/status`,
  nonce/verifier binding, and polling semantics.
- State that `approval/start` is unauthenticated but rate-limited and
  TTL-bounded.
- State that `approval/claim` is the CLI polling/exchange route and does not
  require owner session.
- State that `approval/claim` must verify the submitted verifier against the
  stored verifier challenge before returning a pair code.
- State that `approval/claim` is rate-limited and attempt-bounded by approval id
  and source IP.
- State that `approval/status` is owner-session-only and exists for approval
  page status, not for CLI polling.
- State that approval pages show requesting machine context before owner
  approval.
- Remove the old claim that `approval/approve` delivers a pair token through
  the local callback or `neul://...&pair=<token>`.
- State that browser approval never receives pair code, pair token, or machine
  token.
- Keep `/api/pair/claim` as the only endpoint that creates machine credentials.
- Rewrite the `packaged-primary` numbered flows in `internal/domain/contracts.md`
  and `docs/mvp.md` so `neul login` enrolls but does not start the durable
  agent, `neul up` starts/verifies the durable agent, and connected state is
  heartbeat-gated after `neul up`.
- Rewrite first-run/onboarding state mapping so `claimed` means enrolled and
  waiting for `neul up`, not connected.
- Remove the old 120 second post-claim `agent_not_responding` rule from the
  split flow. Future timeout copy must be anchored to a durable agent-start
  attempt, not login claim.
- Update `scripts/validate-packaged-client-docs.sh` in lockstep with the new
  required strings: `neul login --server <origin>` as primary, no
  `neul://enroll?server=` or `neul://...&pair=<token>` required-string checks,
  no `local callback binds to 127.0.0.1 only` required-string check,
  no `docs/mvp.md` `local callback` required-string check,
  no `Device code is fallback-only` required-string checks,
  and no `allowedPairTokenHandoffs` copy that says pair tokens are allowed in
  browser handoff surfaces.
- Replace `allowed pair-token handoffs`, `Allowed pair-token handoffs`, and
  `allowedPairTokenHandoffs` validation strings with browser-safe approval
  guardrail strings. The replacement doc text should use the phrase
  `browser-safe approval handoffs`; the replacement copy key should be
  `browserSafeApprovalHandoffs`.
- Add positive `scripts/validate-packaged-client-docs.sh` assertions for
  `neul login --server <origin>`, `browser-safe approval handoffs`, and
  `browserSafeApprovalHandoffs`.
- Update `scripts/validate-packaged-client-docs.sh` per assertion:
  - In `packaged_primary_flow`, delete the `docs/mvp.md`
    `neul://enroll?server=` check and replace it with
    `neul login --server <origin>`.
  - In `packaged_primary_flow`, delete the `docs/mvp.md` `local callback`
    check and replace it with `browser approval polling`.
  - In `packaged_primary_flow`, change the `web/src/copy.ts`
    `neul enroll --server <origin>` check to
    `neul login --server <origin>`.
  - In `packaged_primary_flow`, delete the `internal/domain/contracts.md`
    `neul://enroll?server=` check and replace it with
    `POST /api/pair/approval/claim`.
  - In `fallback_debug_separation`, change the `web/src/copy.ts` fallback
    command check from `--pair <token>` to `--pair <pair-code>`.
  - In `fallback_debug_separation`, change all primary-flow absent checks for
    `--pair <token>` to `--pair <pair-code>` while keeping the broader
    `--pair` absent checks.
  - In `security_model_guardrails`, delete the `Device code is fallback-only`
    checks for `docs/mvp.md` and `internal/domain/contracts.md`.
  - In `security_model_guardrails`, change `allowed pair-token handoffs` and
    `Allowed pair-token handoffs` checks to `browser-safe approval handoffs`.
  - In `security_model_guardrails`, replace the
    `local callback binds to 127.0.0.1 only` check with
    `approval claim is machine-client polling`.
  - In `security_model_guardrails`, change the `web/src/copy.ts`
    `allowedPairTokenHandoffs` check to `browserSafeApprovalHandoffs`.
  - Keep the final guard that rejects `packaged-client command bridge`, and add
    an equivalent guard rejecting `allowedPairTokenHandoffs` in docs and web
    copy after the rename.
- Update `web/src/copy.ts` and `web/src/copy.test.ts` together so the security
  copy shape is fully renamed from pair-token wording to pair-code and
  approval-handoff wording:
  - `pairTokenKind` becomes `pairCodeKind` and describes the one-time
    `/api/pair/claim` value.
  - `neverStorePairTokenIn` becomes `neverStorePairCodeIn`.
  - `allowedPairTokenHandoffs` becomes `browserSafeApprovalHandoffs` and says
    browser approval receives only approval id, nonce, non-secret machine
    preview metadata, non-secret status, and the per-approval CSRF token; pair
    code, pair token, and machine token stay on CLI/server paths.
  - `commandTemplate` changes from `neul enroll --server <origin>` to
    `neul login --server <origin>`.
  - fallback/debug copy changes from `--pair <token>` to
    `--pair <pair-code>`.
  - the matching tests assert the renamed security object, the login command
    template, the pair-code fallback wording, and the absence of legacy
    pair-token browser-handoff copy.
- Update the fallback/debug required string in
  `scripts/validate-packaged-client-docs.sh` and `web/src/copy.ts` from
  `--pair <token>` to `--pair <pair-code>`.
- Update `docs/qa/agent-onboarding.md` so it uses `neul login` for enrollment,
  `neul up` for connected/running state, and no longer says enroll success
  equals `Connected`.

Deep link decision:

- This slice drops `neul://` deep-link handoff entirely.
- This slice also drops browser-to-loopback local callback wake-up entirely.
- This slice drops device-code fallback. A future device-code flow must be
  designed as a separate fallback and must not rely on removed callback/deep-link
  handoff copy.
- A future pair-token-free deep link or callback wake-up may be designed later,
  but it is out of scope for this implementation plan.

New primary command:

```sh
neul login --server <origin>
```

Compatibility:

- No top-level `neul enroll` exists in the current CLI, so this slice does not
  need to preserve an implemented top-level alias.
- A top-level `neul enroll` alias may be added only if the implementer wants a
  migration shim for older docs or package scripts.
- `neul agent enroll --server <origin> --pair <pair-code> --connect-once`
  remains fallback/debug only.
- `neul init --pair --server` remains legacy/debug only.
- No fallback/debug command may replace `neul login --server <origin>` as the
  primary product command.

## Web Onboarding

The web onboarding wizard changes from "create pair code first" to "run client
login and approve in browser".

Primary copy:

```sh
neul login --server <origin>
```

Secondary fallback/debug copy stays available until packaged approval ships:

```sh
go run ./cmd/neul agent enroll --server <origin> --pair <pair-code> --connect-once
```

Rules:

- The primary command must not include `--pair`.
- The fallback/debug block must be visually and semantically secondary.
- The existing dashboard onboarding wizard is an instruction surface for the
  primary flow: it shows `neul login --server <origin>` and does not poll
  approval status.
- The secondary fallback/debug block keeps its existing pair-code generator by
  calling `POST /api/pair/init` and polling `GET /api/pair/poll`; it provides
  the real `<pair-code>` used in
  `go run ./cmd/neul agent enroll --server <origin> --pair <pair-code> --connect-once`.
  This generator must remain visually secondary and must not replace
  `neul login --server <origin>` as the primary path.
- The CLI-opened `/enroll/approve?approval=<approval-id>&nonce=<nonce>` page
  owns approval status polling with `GET /api/pair/approval/status`.
- If that page has no owner session, it shows copy telling the user to open the
  same URL in an already-authenticated owner browser or create/restore the owner
  session first.
- After that approval page first sees `claimed` with `machineId`, it shows a
  waiting state that tells the user to run `neul up`.
- Connected state is shown only after the first heartbeat makes the machine
  visible in `GET /api/dashboard`.
- The old 120 second post-claim `agent_not_responding` timer is removed for
  this split flow because `neul login` no longer starts the agent. A future
  `agent_not_responding` timer can start from a durable agent-start event, not
  from login claim.

## State Flow

Fresh machine:

```text
neul up
  -> no config
  -> prints neul login next action
  -> exits without credentials
```

Login:

```text
neul login --server <origin>
  -> browser approval opens
  -> owner approves
  -> CLI exchanges approval for pair code
  -> CLI claims pair code
  -> config saved 0600
  -> login/enrollment complete copy
  -> points user to neul up
```

Already joined:

```text
neul up
  -> config exists
  -> local status and LaunchAgent state checked
  -> macOS user-level agent installed/kickstarted or verified
  -> within 60 seconds, long-running agent writes fresh successful heartbeat status
  -> dashboard heartbeat makes machine connected
  -> local running status summary printed
```

Not healthy:

```text
neul up
  -> config exists
  -> recoverable issue detected
  -> product-level recovery copy printed
```

## Testing And QA

Go tests:

- `neul up` with no config prints `neul login --server <origin>` guidance and
  no token-looking values.
- `neul up` with existing config does not call pair claim and does not overwrite
  config.
- `neul login --server <origin>` starts approval, polls approval claim, claims
  pair code, writes config mode `0600`, and reports enrollment success without
  claiming durable connected state.
- `neul up` with existing config starts/verifies the agent and reports connected
  only after status provenance is long-running agent, `lastHeartbeatAt >=
  upStartedAt`, and `lastError` is empty or `null`.
- `neul up` times out after 60 seconds without a fresh successful heartbeat and
  reports `local_heartbeat_missing`.
- `neul up` maps status `lastError.kind` values deterministically:
  `auth_failure` -> `auth_invalid`, `connection_failure` ->
  `server_unreachable`, `server_failure` -> `server_error`, and `rate_limited`
  -> `rate_limited`.
- `neul up` reports mapped `lastError.kind` instead of
  `local_heartbeat_missing` when a fresh long-running status after `upStartedAt`
  contains both no successful heartbeat and a structured `lastError.kind`.
- `neul up` does not accept a connect-once/diagnostic status receipt as durable
  connected.
- `neul login` fails clearly when approval expires, approval is cancelled,
  server polling fails, or config already exists.
- approval start/approve/claim binds nonce and verifier to approval and requires
  owner session for approval.
- approval start is unauthenticated but rate-limited at 10/minute/IP and
  30/hour/IP, TTL-bounded, and returns a browser/terminal comparison code.
- approval page copy warns not to approve URLs the owner did not personally
  initiate with `neul login`, and requires matching the browser comparison code
  against the terminal code.
- approval records persist nonce hash, verifier challenge, plaintext CSRF
  token, plaintext comparison code, state, machine preview metadata, expiry,
  pairCodeIssuedAt, approvalPairingId, claimedRetainUntil, claim failure count,
  and claimed machine id without storing pair code, machine token, setup token,
  or plaintext verifier.
- approval migration does not add columns to `pairing_codes`; `/api/pair/claim`
  enforces approval-created expected metadata by joining
  `approval_records.approvalPairingId` to the existing pairing row id.
- approval claim is machine-client polling/exchange and does not require owner
  session.
- approval claim rejects missing or incorrect verifier after owner approval.
- approval approve rate-limits owner-session POST attempts at 20/minute and
  60/hour.
- approval status rate-limits owner-session GET requests at 120/minute and
  source-IP GET requests at 240/minute.
- approval claim rate-limits pending polls at 90/minute per approval id and
  120/minute per source IP.
- approval claim locks the approval with `approval_locked` on the 6th verifier
  failure and never releases a pair code after lock.
- approval claim returns `approval_pair_code_issued` after the one-time pair
  code has already been issued but before `/api/pair/claim` consumes it, and
  returns `claimed` after `/api/pair/claim` consumes it.
- approval claim returns canonical error envelopes and status codes for invalid
  input, denied verifier, not found, cancelled, expired, locked, and
  rate-limited states.
- approval-created pair codes expire 10 minutes after pair-code creation, not
  10 minutes after approval start.
- claimed approval records continue returning `claimed` status until
  `claimedRetainUntil`, independent of the original approval expiry.
- verifier generation uses at least 32 bytes of randomness.
- `/api/pair/claim` rejects mismatched machine metadata for approval-created
  pair codes before creating machine credentials.
- ordinary `/api/pair/init` fallback/debug pair codes still accept existing
  machine metadata behavior.
- approval status requires owner session and is not used by CLI polling.
- approval status returns machine preview metadata and a CSRF token for pending
  or approved owner-page UI, returns claimed `machineId` and `claimedAt`, and
  never returns pair code, pair token, machine token, setup token, or plaintext
  verifier.
- pair code can be claimed once.
- `neul up` does not call owner-session dashboard or machine routes.
- `neul up` reuses existing LaunchAgent install/probe helpers but reads the raw
  status file itself for structured `mode`, `lastHeartbeatAt`, and `lastError`
  checks. It does not parse `agent status` text and does not use the current
  unimplemented `agent start` stub unless that stub is completed in the same
  change.
- status receipt provenance is written by both long-running and explicit
  status-writing one-shot paths when those paths write a status file.

Web tests:

- onboarding primary command is `neul login --server <origin>`.
- onboarding primary command does not include `--pair`.
- fallback/debug command includes `--pair` only in the secondary block.
- dashboard onboarding wizard does not poll approval status.
- CLI-opened approval page polling identifies the claimed machine by `machineId`.
- CLI-opened approval page shows "run `neul up`" after approval `claimedAt`.
- CLI-opened approval page does not start the old 120 second timeout from login
  claim.
- claimed machine moves to connected only after dashboard heartbeat evidence.
- docs validation script expects the new login/up copy and no longer requires
  pair-token browser handoff copy.

Manual QA:

Fallback demo QA:

1. Run `make demo`.
2. Create the owner browser session from the setup token.
3. Open the dashboard onboarding wizard.
4. Confirm primary copy is `neul login --server <origin>` and fallback/debug copy
   is visibly secondary.
5. Generate a fallback pair code from the secondary fallback/debug block.
6. Use the fallback/debug `go run ./cmd/neul agent enroll ... --connect-once`
   path with that code only to prove existing checkout demo enrollment and
   heartbeat still work.

Packaged macOS QA:

1. Build/install the unsigned local macOS package or otherwise place executable
   `neul` and `neul-agent` binaries where the LaunchAgent plan expects them.
2. Run the primary `neul login --server <origin>` command.
3. Approve in the browser.
4. Confirm login reports enrollment complete and points to `neul up`.
5. Run `neul up`.
6. Confirm `neul up` waits for a long-running-agent status receipt, not a
   connect-once receipt.
7. Confirm dashboard shows the machine connected only after heartbeat.
8. Confirm primary CLI/web output does not expose setup, pair, or machine token.

Status receipt update:

- Add a provenance field to the agent status receipt, for example `mode:
  "run_loop" | "connect_once"`, and have the long-running agent write
  `run_loop`.
- `neul up` uses that provenance plus freshness to distinguish durable agent
  heartbeat from connect-once or diagnostic heartbeat.

## Open Questions Deferred

- Whether `neul login` should support a future hosted default origin.
- Whether recovery should be `neul logout`, `neul reset`, or a more explicit
  revoke/re-enroll command.
- Windows and Linux service semantics.

These are deferred so the first client-first slice can ship without turning
into the full roadmap.

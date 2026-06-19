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

Setup token, pair token, machine token은 내부 credential이다. Primary CLI output,
browser history, document title, logs, localStorage, general URL query string의
주인공이 되어서는 안 된다.

## Scope

이번 implementation spec은 macOS first `Start + Authenticate + Join Fleet`만
다룬다.

Included:

- `neul up` command surface
- `neul login --server <origin>` command surface
- canonical docs/tests migration from planned `neul enroll --server <origin>`
  to `neul login --server <origin>`
- local callback based browser approval
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
5. Print a short locally-derived running status summary.
6. If the agent is not healthy, explain the recoverable state:
   `server_unreachable`, `agent_not_running`, `local_heartbeat_missing`, or
   `auth_invalid`.

`neul up` must not silently overwrite existing credentials. Any force/re-enroll
behavior belongs to an explicit later recovery surface.

### `neul login --server <origin>`

`neul login` is the interactive auth and enrollment command.

Flow:

1. Validate `--server`.
2. Generate a client nonce.
3. Start a single-shot callback listener bound to `127.0.0.1` on an ephemeral
   port.
4. Request a browser approval URL from the server.
5. Open the owner browser.
6. Wait for the callback or poll `POST /api/pair/approval/claim` until the
   owner approves, cancels, or the approval expires.
7. Exchange the approval nonce/verifier for an opaque pair code over the server
   API.
8. Claim the pair code with machine metadata through `/api/pair/claim`.
9. Write local config with `0600` permissions.
10. Report enrollment success and point to `neul up`.

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
- owner session is required
- local callback could not bind
- machine already has config

## Server And Browser Handoff

The server adds the thinnest approval surface needed for `neul login`.

Target endpoints:

- `POST /api/pair/approval/start`
- `POST /api/pair/approval/approve`
- `POST /api/pair/approval/claim`
- `GET /api/pair/approval/status`

`approval/start` receives:

- server origin
- client nonce
- verifier challenge
- machine preview metadata: hostname, OS, architecture, and agent version
- callback URL bound to `127.0.0.1`

The server validates the callback URL before storing the approval record. It
must reject any callback URL whose parsed host is not exactly `127.0.0.1` or
whose scheme is not `http`.

It returns an approval id and an approval URL that the CLI opens in the browser.
The approval URL is an owner browser route containing the non-secret approval id
and nonce, for example:

```text
<origin>/enroll/approve?approval=<approval-id>&nonce=<nonce>
```

`approval/start` is unauthenticated because a fresh CLI has no owner session.
It must be abuse-bounded with short TTL records, per-callback/per-IP rate
limits, and no secret material in the response.

`approval/approve` is a CSRF-protected owner-session action. The approval page
must show the requesting machine context before approval: hostname, OS,
architecture, agent version, callback host, and requested time. It marks the
short-lived approval record as approved, bound to the client nonce, verifier
challenge, callback URL, and machine preview metadata. It does not put pair
code, pair token, or machine token in the browser URL.

`POST /api/pair/approval/claim` is a machine-client action that receives the
approval id, nonce, and verifier. It does not require owner session. Before the
owner approves, it returns `pending`; after cancellation or expiry it returns
`cancelled` or `expired`; after approval it creates or returns one opaque pair
code from the same `pairing_codes` storage used by `/api/pair/init`. The server
must reject missing or incorrect verifiers by checking the submitted verifier
against the challenge stored by `approval/start`. The CLI then calls the
existing `/api/pair/claim` with that code and machine metadata. The claim
metadata must match the machine preview metadata from `approval/start`. After
pair claim succeeds, the approval record stores the claimed `machineId` so
owner-session browser UI can watch the right machine.

`GET /api/pair/approval/status` is an owner-session status endpoint for the
approval page and onboarding wizard. It receives approval id and returns one of:
`pending`, `approved`, `claimed`, `expired`, `cancelled`, or `error`. The
`claimed` response includes `machineId` and `claimedAt`.

The implementation should reuse the existing pair claim machinery instead of
creating a second machine registration system. Existing `/api/pair/claim`
remains the credential creation point.

Guardrails:

- Owner browser session is required to approve.
- Pair code is single-use and short-lived.
- Approval status never returns pair code, pair token, or machine token.
- Approval start is unauthenticated but rate-limited and TTL-bounded.
- Approval claim rejects absent, malformed, or challenge-mismatched verifier.
- Approval page shows requesting machine context before the owner approves.
- Pair code and machine token are never written to server logs.
- Browser code must not receive or store pair code or machine token in
  localStorage.
- Browser code must not place pair code or machine token in `document.title`.
- Primary approval URL must not expose pair code or machine token in a general
  browser URL.
- Local callback listener is single-shot and closes after success, rejection, or
  timeout.
- Local callback is best-effort. The browser approval page may wake the CLI with
  a top-level navigation or redirect to
  `http://127.0.0.1:<port>/callback?approval=<approval-id>&state=<approved|cancelled>&nonce=<nonce>`.
  The CLI rejects callbacks whose nonce does not match the generated nonce.
- Polling `POST /api/pair/approval/claim` is authoritative; the callback only
  wakes the CLI sooner.
- Concurrent `neul login` runs use distinct nonces and callback ports.

This avoids relying on browser `fetch` from the server origin to
`http://127.0.0.1:<port>` with a bearer credential. The local callback is only a
wake-up signal; the sensitive exchange happens from CLI to server.

## Contract Update

`internal/domain/contracts.md`, `docs/mvp.md`, README packaged-primary copy, and
web onboarding tests must be updated in the same implementation change.

Required contract edits:

- Update Auth Defaults so pair tokens are no longer allowed in browser code,
  local callback payloads, `neul://...&pair=<token>`, or fallback/debug browser
  copy. The only browser-visible approval values are approval id, nonce, and
  non-secret status.
- Replace the planned primary packaged-client command
  `neul enroll --server <origin>` with `neul login --server <origin>`.
- Rewrite the approval API subsection so it includes
  `POST /api/pair/approval/claim`, `GET /api/pair/approval/status`,
  nonce/verifier binding, and loopback callback wake-up semantics.
- State that `approval/start` is unauthenticated but rate-limited and
  TTL-bounded.
- State that `approval/claim` is the CLI polling/exchange route and does not
  require owner session.
- State that `approval/claim` must verify the submitted verifier against the
  stored verifier challenge before returning a pair code.
- State that `approval/status` is owner-session-only and exists for approval
  page/onboarding status, not for CLI polling.
- State that approval pages show requesting machine context before owner
  approval.
- Remove the old claim that `approval/approve` delivers a pair token through
  the local callback or `neul://...&pair=<token>`.
- State that browser approval never receives pair code, pair token, or machine
  token.
- Keep `/api/pair/claim` as the only endpoint that creates machine credentials.

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
- The approval page or onboarding wizard reads the approval id from the
  approval URL opened by `neul login` and polls
  `GET /api/pair/approval/status`.
- After approval status first returns `claimed` with `machineId`, the web shows
  a waiting state that tells the user to run `neul up`.
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
  -> local callback starts
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
  -> long-running agent writes successful heartbeat status
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
- `neul login --server <origin>` starts approval, receives callback, claims
  pair code, writes config mode `0600`, and reports enrollment success without
  claiming durable connected state.
- `neul up` with existing config starts/verifies the agent and reports connected
  only after a server-accepted heartbeat.
- `neul login` fails clearly when callback bind fails, approval expires, owner
  session is missing, or config already exists.
- approval start/approve/claim binds nonce and verifier to callback and requires
  owner session for approval.
- approval start is unauthenticated but rate-limited and TTL-bounded.
- approval start rejects callback URLs that are not `http://127.0.0.1:<port>/...`.
- approval claim is machine-client polling/exchange and does not require owner
  session.
- approval claim rejects missing or incorrect verifier after owner approval.
- approval claim rejects machine metadata that does not match approval start
  preview metadata.
- approval status requires owner session and is not used by CLI polling.
- approval status returns claimed `machineId` and `claimedAt`, but never returns
  pair code, pair token, or machine token.
- callback rejects mismatched nonce.
- callback wake-up is best-effort; claim polling is authoritative.
- pair code can be claimed once.
- `neul up` does not call owner-session dashboard or machine routes.
- `neul up` uses existing `agent install`/`agent status` primitives, not the
  current unimplemented `agent start` stub, unless that stub is completed in the
  same change.

Web tests:

- onboarding primary command is `neul login --server <origin>`.
- onboarding primary command does not include `--pair`.
- fallback/debug command includes `--pair` only in the secondary block.
- onboarding approval polling identifies the claimed machine by `machineId`.
- onboarding shows "run `neul up`" after approval `claimedAt`.
- onboarding does not start the old 120 second timeout from login claim.
- claimed machine moves to connected only after dashboard heartbeat evidence.

Manual QA:

1. Run `make demo`.
2. Create the owner browser session from the setup token.
3. Open the dashboard onboarding wizard.
4. Run the primary `neul login --server <origin>` command.
5. Approve in the browser.
6. Confirm login reports enrollment complete and points to `neul up`.
7. Run `neul up`.
8. Confirm dashboard shows the machine connected only after heartbeat.
9. Confirm primary CLI/web output does not expose setup, pair, or machine token.

## Open Questions Deferred

- Whether `neul login` should support a future hosted default origin.
- Whether recovery should be `neul logout`, `neul reset`, or a more explicit
  revoke/re-enroll command.
- Windows and Linux service semantics.

These are deferred so the first client-first slice can ship without turning
into the full roadmap.

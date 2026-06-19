# Neul MVP API Contracts

This file locks the MVP contract before server, agent, CLI, and web
implementation begins. If `README.md` or `docs/2026-05-27-design.md`
conflicts with this file or `docs/mvp.md`, the MVP contract here wins.

## Scope Guardrails

- MVP deployment shape is single-owner self-hosted.
- Agent transport is outbound HTTPS REST polling and reporting only.
- WebSocket and `/ws` routes are excluded from MVP implementation.
- Secrets are disabled in MVP: `/api/secrets` returns 404 Not Found, no
  secret mutation route exists, and dashboard or ledger payloads never include
  secret rows.
- Hosted tier, teams, RBAC, SSO, billing, root path mutation, remote terminal
  execution, arbitrary shell commands, and real service installation are out.
- Agent Onboarding UX v2 has no `/install.sh`: no /install.sh endpoint, no
  `curl | sh`, and no hosted login. The primary user path is packaged neul
  client install plus browser approval, not a checkout-local command.

## Auth Defaults

- Browser owner auth uses a first-run setup token exchanged through
  `POST /api/session/local`.
- `POST /api/session/local` consumes the setup token and sets an HttpOnly,
  same-site owner session cookie.
- Owner-facing API QA uses a cookie jar, not `Authorization: Bearer`.
- Agent auth uses a machine bearer token.
- Setup tokens, pairing codes, owner sessions, and machine tokens are hashed at
  rest; plaintext values are printed or returned only at creation time.
- Self-hosted owner approval model: an existing owner browser session approves
  a local client enrollment. Browser approval receives only approval id, nonce,
  comparison code, machine preview metadata, CSRF, and status.
- Pair codes are one-time `/api/pair/claim` values. Browser code must not put
  pair codes, pair tokens, machine tokens, setup tokens, or plaintext verifiers
  in browser copy, URL query strings, `document.title`, browser history,
  localStorage, or logs.
- `/api/pair/claim` remains the only machine credential creator.

## Timing And Version Defaults

- Pairing codes expire exactly 10 minutes after creation.
- Pair poll is the source of truth for onboarding expiry. Expired unused pair
  codes return HTTP 200 with `status: "expired"` and `expiresAt`.
- Expired pairing claims return `410 Gone` with error code
  `pairing_code_expired`.
- Agent heartbeat interval is 30 seconds.
- A machine is `offline` when its last heartbeat is older than 5 minutes.
- Pending state is computed from per-resource `desired_version` versus the
  latest machine `applied_version`.
- Reconcile reports and repair commands use the `Idempotency-Key` HTTP header.

## Error Shape

All JSON errors use this shape:

```json
{
  "error": {
    "code": "machine_not_found",
    "message": "Machine was not found."
  }
}
```

## Canonical Endpoints

### Owner Session

- `POST /api/session/local`

Request:

```json
{
  "setupToken": "setup_..."
}
```

Response:

- `204 No Content`
- `Set-Cookie: neul_session=...; HttpOnly; SameSite=Lax`

### Pairing

- `POST /api/pair/init`
- `POST /api/pair/claim`
- `GET /api/pair/poll`

Primary packaged client onboarding flow:

<!-- packaged-primary:start -->

1. Owner session already exists.
2. Web tells the user to install the neul client install package.
   macOS local QA uses an unsigned dev `.pkg`; production macOS distribution
   requires Developer ID Application and Developer ID Installer certificates,
   notarization, and stapling. Linux uses Debian/Ubuntu `.deb` and tarball.
   Local macOS dev packages are built with `scripts/build-macos-dev-pkg.sh`;
   Homebrew tap distribution remains future/alternate unless fully implemented.
3. The macOS `.pkg` installs `/usr/local/bin/neul` and
   `/usr/local/libexec/neul-agent`; LaunchAgent registration remains a
   per-user `neul agent install` action.
4. The packaged client runs `neul login --server <origin>`, starts browser
   approval, and polls `POST /api/pair/approval/claim`; approval claim is machine-client polling, not a browser credential handoff.
5. The owner browser receives only approval id, nonce, comparison code, machine
   preview metadata, CSRF, and status. It never receives pair code, pair token,
   machine token, setup token, or plaintext verifier copy.
6. After owner approval, the CLI receives an opaque pair code from
   `POST /api/pair/approval/claim` and submits it with machine metadata to
   `/api/pair/claim`. `/api/pair/claim` creates the machine credential.
7. `neul login` writes local config with mode `0600`, reports enrollment
   success, and points the user to `neul up`.
8. `neul up` starts or verifies the user-level agent. Web moves to `connected`
   only after a fresh long-running agent heartbeat makes the machine visible in
   `GET /api/dashboard`.
9. If the claimed machine does not heartbeat within 120 seconds, web shows
   `agent_not_responding`.

First-run states are `not_logged_in`, `waiting_for_browser_approval`,
`enrolled`, `offline`, and `error`.

User-facing onboarding states are `creating`, `ready`,
`claimed_waiting_heartbeat`, `connected`, `expired`, `used`,
`agent_not_responding`, `error`, and `cancelled`.

First-run state mapping:

| First-run state | User-facing onboarding state |
| --- | --- |
| `not_logged_in` | `creating` until owner session is restored or requested |
| `waiting_for_browser_approval` | `ready` or `claimed_waiting_heartbeat` |
| `enrolled` | `claimed_waiting_heartbeat` until `neul up` produces a fresh heartbeat |
| `offline` | `agent_not_responding` |
| `error` | `expired`, `used`, `cancelled`, or `error` |

Pairing browser approval API:

Target contract for packaged-client implementation. These endpoints are deferred
until the packaging work that follows this design issue.

- `POST /api/pair/approval/start` receives the client nonce, verifier
  challenge, and machine preview metadata. It returns an approval URL for the
  owner browser.
- `POST /api/pair/approval/approve` is a CSRF-protected owner-session action
  that approves or cancels the short-lived approval record.
- `POST /api/pair/approval/claim` is the CLI polling exchange. It validates the
  nonce and verifier, then returns a one-time pair code only after owner
  approval.
- `GET /api/pair/approval/status` lets the owner browser read approval status,
  comparison code, machine preview metadata, and CSRF only.
- Concurrent `neul login` runs use distinct approval ids, nonces, and
  verifiers; a pair code can be claimed once through `POST /api/pair/claim`.

<!-- packaged-primary:end -->

Fallback/debug checkout-local enrollment:

Development and QA may still run
`go run ./cmd/neul agent enroll --server <origin> --pair <pair-code> --connect-once`
from the repository checkout. Until packaged approval ships, the web wizard
also shows this executable command in a separate fallback/debug block so a
first-time owner can still claim the invite and reach connected state. That
command is not the primary product UX.

Package QA may use the installed binary equivalent,
`neul agent enroll --server <origin> --pair <pair-code> --connect-once`, followed by
`neul agent install`. Legacy/debug compatibility keeps
`neul init --pair --server`; neither command may replace the primary
`neul login --server <origin>` product command.

Unsigned dev `.pkg` artifacts are local-testing only. Contracts, docs, and
validation must not imply they are production-ready, signed, notarized, or
stapled. Production macOS packages require Developer ID Application and
Developer ID Installer certificates, notarization, and stapling.

`POST /api/pair/init` returns:

```json
{
  "code": "NEUL-123456",
  "expiresAt": "2026-06-05T13:10:00Z"
}
```

`GET /api/pair/poll` returns one of:

```json
{
  "status": "pending",
  "expiresAt": "2026-06-05T13:10:00Z"
}
```

```json
{
  "status": "claimed",
  "machineId": "machine_01",
  "expiresAt": "2026-06-05T13:10:00Z"
}
```

```json
{
  "status": "expired",
  "expiresAt": "2026-06-05T13:10:00Z"
}
```

`POST /api/pair/claim` receives machine metadata and returns:

```json
{
  "machineId": "machine_01",
  "machineToken": "mtn_..."
}
```

### Dashboard And Machines

- `GET /api/dashboard`
- `GET /api/machines`
- `GET /api/machines/:machineId`
- `POST /api/machines/:machineId/repair-drift`

Dashboard response includes:

- `metrics`
- `machines`
- `activity`
- `ledger`
- `emptyState`

Machine detail response includes:

- `machine`
- `events[]`
- `latestReconcile`
- `driftSummary`

Repair drift creates one queued `repair_drift` command for drifted resources.
Duplicate `Idempotency-Key` values return the original queued command.

### Desired State

- `GET /api/resources`
- `POST /api/resources/package`
- `POST /api/resources/dotfile`
- `PATCH /api/resources/:resourceId`
- `DELETE /api/resources/:resourceId`

Package source kinds:

- `brew`: stored and agent-supported first.
- `apt`: stored but `agentSupport` is `unsupported` until later.
- `mise`: stored but `agentSupport` is `unsupported` until later.

Dotfile paths must normalize to one of:

- `~/.zshrc`
- `~/.gitconfig`
- `~/.config/**`

The server and agent reject `..` escape, symlink escape, `/etc`, `/usr`, and
root-owned paths.

### Agent

- `POST /api/agent/heartbeat`
- `GET /api/agent/desired-state`
- `GET /api/agent/commands`
- `POST /api/agent/reconcile-report`
- `POST /api/agent/drift-report`

Agent command types:

- `reconcile_now`
- `repair_drift`

Unknown command types are reported as `unsupported_command` and are never
executed.

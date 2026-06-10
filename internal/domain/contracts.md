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
  `curl | sh`, no hosted login, and no pending approval table. The primary
  user path is packaged neul client install plus browser approval, not a
  checkout-local command.

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
  a local client enrollment. The server creates a short-lived pair token only
  after that approval, and the client claims it once.
- Pair tokens are bearer credentials. Browser code must not put pair tokens in
  URL query strings, `document.title`, browser history, logs, or any location
  other than the local callback, `neul://` deep-link handoff, or explicit
  fallback/debug copyable enroll command.

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
   macOS uses Homebrew tap or signed `.pkg`; Linux uses Debian/Ubuntu `.deb`
   and tarball.
3. The packaged client runs `neul enroll --server <origin>`, opens browser
   approval, and starts a local callback; local callback binds to 127.0.0.1 only.
4. After owner approval, the server creates a one-time pair token and returns it
   through the local callback or `neul://enroll?server=<origin>&pair=<token>`.
5. CLI claims the pair token, writes local config with mode `0600`, and starts
   the user-level agent.
6. Web moves from `claimed_waiting_heartbeat` to `connected` only after the
   first heartbeat makes the machine visible in `GET /api/dashboard`.
7. If the claimed machine does not heartbeat within 120 seconds, web shows
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
| `enrolled` | `connected` |
| `offline` | `agent_not_responding` |
| `error` | `expired`, `used`, `cancelled`, or `error` |

Device code is fallback-only for headless or SSH environments where local
callback or `neul://` handoff cannot complete.

Pairing browser approval API:

Target contract for packaged-client implementation. These endpoints are deferred
until the packaging work that follows this design issue.

- `POST /api/pair/approval/start` receives the client nonce, callback URL, and
  server origin. It returns an approval URL for the owner browser.
- `POST /api/pair/approval/approve` is a CSRF-protected owner-session action
  that creates one short-lived, single-use pair token bound to the client nonce.
- `POST http://127.0.0.1:<ephemeral-port>/callback` is the local callback
  delivery. The client listener is single-shot and closes after success,
  rejection, or timeout.
- `neul://enroll?server=<origin>&pair=<token>&nonce=<nonce>` is the deep-link
  delivery only when the packaged client owns the scheme handler.
- Concurrent `neul enroll` runs use distinct nonces and callback ports; a token
  can be claimed once through `POST /api/pair/claim`.

<!-- packaged-primary:end -->

Fallback/debug checkout-local enrollment:

Development and QA may still run
`go run ./cmd/neul agent enroll --server <origin> --pair <token> --connect-once`
from the repository checkout. That command is not the primary product UX.

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

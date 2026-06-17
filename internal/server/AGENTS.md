# SERVER KNOWLEDGE BASE

## OVERVIEW

`internal/server` owns owner auth, pairing, machine APIs, dashboard/resource handlers, repair command queueing, security guardrails, and SPA routing.

## WHERE TO LOOK

| Task | Location | Notes |
| --- | --- | --- |
| Router and API registration | `http.go` | Keep `/ws` absent for MVP. |
| First-run setup token and owner sessions | `auth.go`, `auth_setup.go` | Setup tokens rotate/consume transactionally. |
| Pair init, poll, claim | `handlers_pairing.go` | Pair code expiry and one-time claim are contract-tested. |
| Dashboard and machine detail | `handlers_dashboard*.go` | Server computes machine states from heartbeat and resource events. |
| Resource CRUD and desired state | `handlers_resources*.go` | Desired version increments drive pending state. |
| Repair drift queueing | `handlers_dashboard_repair*.go` | Queue selected drifted resources without mutating desired state. |
| Real-surface agent/server integration | `homebrew_agent_realsurface*_test.go` | Uses fake brew plus real router/agent cycle. |
| Security posture | `security_test.go` | Secrets disabled, no websocket route, hash-only tokens. |

## CONVENTIONS

- Tests use `httptest` routers and real SQLite migrations rather than handler-only mocks when behavior spans storage.
- JSON errors use `{ "error": { "code": "...", "message": "..." } }`.
- Owner mutations require the owner session cookie; agent endpoints use machine bearer tokens.
- Pairing codes expire after 10 minutes; unused expired poll returns 200 `expired`, while expired claim returns 410.
- Resource and dashboard code must preserve unsupported source kinds as explicit blocked/unsupported state.
- Test helper functions usually live at the bottom of the relevant `_test.go`; prefer local fixtures unless reused across packages.

## ANTI-PATTERNS

- Do not add secret runtime tables, handlers, dashboard rows, or plaintext-derived payloads.
- Do not accept bearer owner auth for owner-facing API QA; use session cookies.
- Do not put pair tokens or machine tokens in logs or response fields beyond creation-time contracts.
- Do not let repair drift edit desired state; it queues agent commands.
- Do not add websocket routing for MVP.

# Client-first Up/Login Design

작성일: 2026-06-19

## Purpose

Neul의 첫 client-first implementation slice는 `neul up`을 service/fleet
steady-state 명령으로, `neul login`을 interactive browser auth/enroll 명령으로
분리한다.

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
- local callback based browser approval
- pair claim and local config write with `0600` permissions
- first heartbeat gated "joined fleet" success
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
2. Check the configured server and machine/agent state when possible.
3. On macOS, start or verify the user-level agent using the existing
   LaunchAgent-oriented service surface.
4. Print a short fleet status summary.
5. If the agent is not healthy, explain the recoverable state:
   `server_unreachable`, `agent_not_running`, `heartbeat_not_visible`, or
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
6. Wait for the callback.
7. Claim the pair token with machine metadata.
8. Write local config with `0600` permissions.
9. Run one connection tick or start the user-level agent.
10. Report success as fleet membership, not as pair-token mechanics.

Primary success copy should read like:

```text
이 machine이 Neul fleet에 합류했습니다.
다음 실행: neul up
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

`approval/start` receives:

- server origin
- client nonce
- callback URL bound to `127.0.0.1`

It returns an approval URL that the CLI opens in the browser.

`approval/approve` is a CSRF-protected owner-session action. It creates a
short-lived, single-use pair token bound to the client nonce and delivers it
only through the approved local callback or supported deep link path.

The implementation should reuse the existing pair claim machinery instead of
creating a second machine registration system. Existing `/api/pair/claim`
remains the credential creation point.

Guardrails:

- Owner browser session is required to approve.
- Pair token is single-use and short-lived.
- Pair token is never written to server logs.
- Browser code must not store pair token in localStorage.
- Browser code must not place pair token in `document.title`.
- Primary approval URL must not expose pair token in a general browser URL.
- Local callback listener is single-shot and closes after success, rejection, or
  timeout.
- Concurrent `neul login` runs use distinct nonces and callback ports.

## Web Onboarding

The web onboarding wizard changes from "create pair code first" to "run client
login and approve in browser".

Primary copy:

```sh
neul login --server <origin>
```

Secondary fallback/debug copy stays available until packaged approval ships:

```sh
go run ./cmd/neul agent enroll --server <origin> --pair <pair_...> --connect-once
```

Rules:

- The primary command must not include `--pair`.
- The fallback/debug block must be visually and semantically secondary.
- Connected state is shown only after the first heartbeat makes the machine
  visible in `GET /api/dashboard`.
- If heartbeat does not appear within 120 seconds, web shows
  `agent_not_responding`.

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
  -> CLI claims pair token
  -> config saved 0600
  -> first tick or agent start
  -> joined fleet copy
```

Already joined:

```text
neul up
  -> config exists
  -> server/agent state checked
  -> macOS user-level agent started or verified
  -> fleet status summary printed
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
  pair token, writes config mode `0600`, and reports fleet membership.
- `neul login` fails clearly when callback bind fails, approval expires, owner
  session is missing, or config already exists.
- approval start/approve binds nonce to callback and requires owner session.
- pair token can be claimed once.

Web tests:

- onboarding primary command is `neul login --server <origin>`.
- onboarding primary command does not include `--pair`.
- fallback/debug command includes `--pair` only in the secondary block.
- claimed machine moves to connected only after dashboard heartbeat evidence.
- `agent_not_responding` appears after the 120 second timeout.

Manual QA:

1. Run `make demo`.
2. Create the owner browser session from the setup token.
3. Open the dashboard onboarding wizard.
4. Run the primary `neul login --server <origin>` command.
5. Approve in the browser.
6. Run `neul up`.
7. Confirm dashboard shows the machine connected only after heartbeat.
8. Confirm primary CLI/web output does not expose setup, pair, or machine token.

## Open Questions Deferred

- How `neul up` should choose a default server when no config exists and no
  `--server` is passed.
- Whether `neul login` should support a future hosted default origin.
- Whether recovery should be `neul logout`, `neul reset`, or a more explicit
  revoke/re-enroll command.
- Windows and Linux service semantics.

These are deferred so the first client-first slice can ship without turning
into the full roadmap.

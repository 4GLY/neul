# Agent onboarding QA

This QA note covers Agent Onboarding UX v2. The target product path is packaged
client install, browser approval, local callback or `neul://` handoff, user
agent start, and dashboard connected state after the first heartbeat.

## Browser happy path

- Command: `pnpm --dir web build`
- Server: `NEUL_ADDR=127.0.0.1:18085 NEUL_DB=<temp>/neul.sqlite NEUL_STATIC_DIR=web/dist ./neul-server`
- Browser action: open the dashboard, click `첫 머신 등록`, wait for `Run with packaged neul client:` and the separate fallback/debug command.
- Enroll command with packaged client:
  `neul enroll --server http://127.0.0.1:18085`
- Transitional executable command before packaged approval ships:
  `go run ./cmd/neul agent enroll --server http://127.0.0.1:18085 --pair <pair-token> --connect-once`
- Expected browser result: the generated command disappears after the CLI connects, and the machine row shows `Connected`.
- Screenshot: `evidence/task-4-agent-onboarding-wizard-browser.png`
- CLI transcript: `evidence/task-4-agent-onboarding-wizard-browser-log.txt`
- Cleanup receipt: `evidence/task-4-agent-onboarding-cleanup.txt`

## Expired invite retry

- Browser action: click `첫 머신 등록`, then route `/api/pair/poll` to return `{"status":"expired","expiresAt":"2026-06-06T12:10:00Z"}`
- Expected result: browser shows `등록 시간이 만료되었습니다` and `다시 만들기`.
- Screenshot: `evidence/task-4-agent-onboarding-expired-browser.png`
- Log: `evidence/task-4-agent-onboarding-expired-browser-log.txt`

## CLI enroll

- Command:
  `neul enroll --server http://127.0.0.1:18084`
- Expected stdout: `Machine enrolled`, `Connecting`, `Connected`
- Expected config: `<temp>/config/config.json` exists with mode `0600`
- Expected server state: `GET /api/dashboard` shows one healthy machine with `lastHeartbeatAt`
- Transcript: `evidence/task-5-agent-enroll-tmux.txt`

## Fallback/debug checkout-local enrollment

- Command:
  `env -u GOROOT go run ./cmd/neul agent enroll --server http://127.0.0.1:18084 --pair <pair-token> --config-dir <temp>/config --connect-once`
- Expected stdout: `Machine enrolled`, `Connecting`, `Connected`
- Expected config: `<temp>/config/config.json` exists with mode `0600`
- Expected server state: `GET /api/dashboard` shows one healthy machine with `lastHeartbeatAt`
- Use this path only for local checkout QA before packaged binaries exist.
- The web wizard may also show this command in a separate fallback/debug block
  until the packaged approval and deep-link flow ships.

## Existing config and force

- Without `--force`, running `neul agent enroll` against an existing config dir fails with `config already exists`.
- With `--force`, `neul agent enroll` overwrites only the local config. It does not delete, revoke, or clean up a previously registered server-side machine.
- Evidence: `evidence/task-5-agent-enroll-existing-config-tmux.txt`, `evidence/task-5-agent-enroll-force-limitation.txt`

## Known MVP limits

- Checkout-local `go run` enrollment is fallback/debug only while packaged binaries are not available.
- no /install.sh endpoint is shipped.
- `curl | sh` is not shipped.
- Native GUI and menubar client are not shipped.
- Real launchd/systemd installation is not shipped.
- OAuth, SSO, hosted login, teams, billing, and WebSocket onboarding push are out of scope.
- Pair tokens are bearer credentials. Allowed pair-token handoffs are
  `127.0.0.1` local callback, `neul://` enrollment deep link, and the
  fallback/debug command. Pair tokens must not be stored in general URL query
  strings outside those handoffs, `document.title`, browser history, or logs.

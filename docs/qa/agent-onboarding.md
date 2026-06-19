# Agent onboarding QA

This QA note covers Agent Onboarding UX v2. The target product path is packaged
client install, `neul login --server <origin>` browser approval, `neul up`
agent start, and dashboard connected state after the first long-running agent
heartbeat.

## Browser happy path

- Command: `pnpm --dir web build`
- Server: `NEUL_ADDR=127.0.0.1:18085 NEUL_DB=<temp>/neul.sqlite NEUL_STATIC_DIR=web/dist ./neul-server`
- Browser action: open the dashboard, click `첫 머신 등록`, wait for `Run with packaged neul client:` and the separate fallback/debug command.
- macOS package QA: unsigned dev `.pkg` is local-testing only, installs
  `/usr/local/bin/neul` and `/usr/local/libexec/neul-agent`, and is not a
  production signed/notarized/stapled artifact.
- Local macOS dev package build: `scripts/build-macos-dev-pkg.sh`.
- Production macOS distribution requires Developer ID Application and Developer
  ID Installer certificates, notarization, and stapling.
- Login command with packaged client:
  `neul login --server http://127.0.0.1:18085`
- Durable agent command after login:
  `neul up`
- Explicit package-QA fallback command with installed binary:
  `neul agent enroll --server http://127.0.0.1:18085 --pair <pair-code> --connect-once`
- LaunchAgent registration after package-QA enrollment:
  `neul agent install`
- Transitional executable command before packaged approval ships:
  `go run ./cmd/neul agent enroll --server http://127.0.0.1:18085 --pair <pair-code> --connect-once`
- Expected browser result: the generated command disappears after the CLI connects, and the machine row shows `Connected`.
- Screenshot: `evidence/task-4-agent-onboarding-wizard-browser.png`
- CLI transcript: `evidence/task-4-agent-onboarding-wizard-browser-log.txt`
- Cleanup receipt: `evidence/task-4-agent-onboarding-cleanup.txt`

## Expired invite retry

- Browser action: click `첫 머신 등록`, then route `/api/pair/poll` to return `{"status":"expired","expiresAt":"2026-06-06T12:10:00Z"}`
- Expected result: browser shows `등록 시간이 만료되었습니다` and `다시 만들기`.
- Screenshot: `evidence/task-4-agent-onboarding-expired-browser.png`
- Log: `evidence/task-4-agent-onboarding-expired-browser-log.txt`

## CLI login/up

- Command:
  `neul login --server http://127.0.0.1:18084`
- Expected login stdout: enrollment succeeds and points the user to `neul up`.
- Follow-up command:
  `neul up`
- Expected up stdout: durable agent running/connected state is reported only
  after a fresh long-running heartbeat.
- Expected config: `<temp>/config/config.json` exists with mode `0600`
- Expected server state: `GET /api/dashboard` shows one healthy machine with `lastHeartbeatAt`
- Transcript: `evidence/task-5-agent-enroll-tmux.txt`

## Fallback/debug checkout-local enrollment

- Command:
  `env -u GOROOT go run ./cmd/neul agent enroll --server http://127.0.0.1:18084 --pair <pair-code> --config-dir <temp>/config --connect-once`
- Expected stdout: `Machine enrolled`, `Connecting`, `Connected`
- Expected config: `<temp>/config/config.json` exists with mode `0600`
- Expected server state: `GET /api/dashboard` shows one healthy machine with `lastHeartbeatAt`
- Use this path only for local checkout QA before packaged binaries exist.
- The web wizard may also show this command in a separate fallback/debug block
  until the packaged approval polling flow ships.

## Existing config and force

- Without `--force`, running `neul agent enroll` against an existing config dir fails with `config already exists`.
- With `--force`, `neul agent enroll` overwrites only the local config. It does not delete, revoke, or clean up a previously registered server-side machine.
- Evidence: `evidence/task-5-agent-enroll-existing-config-tmux.txt`, `evidence/task-5-agent-enroll-force-limitation.txt`

## Known MVP limits

- Checkout-local `go run` enrollment is fallback/debug only while packaged binaries are not available.
- Unsigned dev `.pkg` artifacts are local-testing only.
- Production macOS distribution requires Developer ID Application and Developer
  ID Installer certificates, notarization, and stapling.
- macOS package paths are `/usr/local/bin/neul` and
  `/usr/local/libexec/neul-agent`.
- no /install.sh endpoint is shipped.
- `curl | sh` is not shipped.
- Native GUI and menubar client are not shipped.
- Real launchd/systemd installation is not shipped.
- OAuth, SSO, hosted login, teams, billing, and WebSocket onboarding push are out of scope.
- Browser-safe approval handoffs are approval id, nonce, comparison code,
  machine preview metadata, CSRF, and status only. Browser copy, URL query
  strings, `document.title`, browser history, localStorage, and logs must not
  contain pair code, pair token, machine token, setup token, or plaintext
  verifier values.
- Fallback/debug copy may show `--pair <pair-code>` only in the secondary
  checkout-local or package-QA command.

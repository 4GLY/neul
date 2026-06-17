# CLI KNOWLEDGE BASE

## OVERVIEW

`internal/cli` contains the real `neul` command tree: legacy init compatibility, agent enrollment, macOS LaunchAgent install/reset/uninstall/status/logs, and local config handling.

## WHERE TO LOOK

| Task | Location | Notes |
| --- | --- | --- |
| Command dispatch | `cli.go` | `cmd/neul/main.go` only calls `cli.Run`. |
| Enrollment and config write | `agent_enroll.go`, `config.go` | Config mode and force behavior are tested. |
| LaunchAgent plist and launchctl args | `agent_launchd.go` | Pure render/argv helpers with tests. |
| Install lifecycle | `agent_install.go` | Dry-run, rollback, existing plist handling. |
| Reset/uninstall lifecycle | `agent_reset.go`, `agent_uninstall_test.go` | Preserve installed binaries; remove selected state only. |
| Status and logs | `agent_status.go`, `agent_logs.go` | Support custom status/log paths. |

## CONVENTIONS

- User-facing process entry is `neul agent ...`; keep `neul init --pair --server` as legacy/debug compatibility only.
- Non-darwin service actions return unsupported without writing plist files or running launchctl.
- LaunchAgent label is `com.4gly.neul.agent`; default binary path is `/usr/local/libexec/neul-agent`.
- Install writes plist atomically and rolls back only the plist it created when bootstrap fails.
- Reset/uninstall accept selected `--plist`, `--status`, and `--log` paths; do not assume defaults after flags.
- CLI tests stub runtime OS and launchctl command execution instead of invoking host launchd.

## ANTI-PATTERNS

- Do not make checkout-local `go run` enrollment the primary product UX.
- Do not run launchctl after validation fails or on unsupported OS paths.
- Do not delete installed binaries during reset/uninstall.
- Do not expose `machineToken` in status output.
- Do not claim unsigned dev package installation is production distribution.

# AGENT KNOWLEDGE BASE

## OVERVIEW

`internal/agent` owns the machine-side REST polling client, desired-state evaluation, command reconcile, status receipt writing, package adapter safety, and dotfile application.

## WHERE TO LOOK

| Task | Location | Notes |
| --- | --- | --- |
| Tick and REST endpoints | `agent.go` | Heartbeat, desired state, command poll, report. |
| Run loop/backoff/config reload | `run_loop.go` | Preserve retry without log spam or config mutation. |
| Status receipt output | `status.go` | Receipts must exclude `machineToken`. |
| Package adapter dispatch | `adapter.go`, `homebrew_adapter.go` | Default adapter blocks brew unless enabled. |
| Dotfile reconcile | `dotfile_adapter.go`, `dotfile_filesystem.go` | Path and symlink safety are central. |
| Repair and reconcile commands | `command_reconcile.go` | Command idempotency and payload resource filtering matter. |

## CONVENTIONS

- Agent transport is outbound HTTP REST only; `Endpoints()` and tests should stay websocket-free.
- `DefaultConfig()` keeps the heartbeat interval at 30 seconds.
- Idempotency keys must change when desired state or resulting events change and remain stable otherwise.
- Package work supports brew as the real adapter path; apt/mise and unsupported hosts should report blocked/unsupported without mutation.
- Dotfile writes go through managed content paths, safe parent checks, atomic writes, and symlink replacement helpers.
- Tests use fake HTTP servers and fake adapters to prove the exact outbound sequence.

## ANTI-PATTERNS

- Do not execute unknown agent commands.
- Do not run package manager mutations through PATH brew unless production discovery was explicitly enabled.
- Do not mutate config on transient network or reload failures.
- Do not write `machineToken` into status receipts, logs, or evidence.
- Do not allow dotfile targets to escape the configured home directory.

# MVP dashboard QA

- Command: `cd web && pnpm exec playwright test e2e/mvp-dashboard.spec.ts --project=chromium`
- Server: http://127.0.0.1:18082
- Temp DB: /var/folders/bd/dq0gbrc917g7x6vd531rxcwc0000gn/T/neul-mvp-gIxb9E/neul.sqlite
- Enrolled config dir: /var/folders/bd/dq0gbrc917g7x6vd531rxcwc0000gn/T/neul-mvp-gIxb9E/agent-config
- Seed machine: machine_4cf8aa5bb114b18a
- Seed resources: brew package `kubectl`, dotfile `~/.zshrc`
- Drift seed: `POST /api/agent/drift-report` with one drifted brew event
- Repair: browser clicked `drift 복구`, then `go run ./cmd/neul-agent --once --config <cli-written-config>` acked the queued command
- Screenshot: `evidence/task-6-agent-onboarding-e2e-browser.png`
- Cleanup receipt: `evidence/task-6-agent-onboarding-cleanup.txt`

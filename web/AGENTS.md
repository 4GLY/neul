# WEB KNOWLEDGE BASE

## OVERVIEW

`web` is a standalone Vite/React owner dashboard with strict TypeScript, Biome formatting, Vitest unit/component tests, and Chromium-only Playwright E2E.

## STRUCTURE

```
web/
|-- src/                 # dashboard, onboarding, API mappers, copy contracts
|-- e2e/                 # Playwright scenarios and server fixtures
|-- biome.json           # tabs + recommended lint
|-- vitest.config.ts     # jsdom unit/component tests
`-- playwright.config.ts # Chromium E2E project
```

## WHERE TO LOOK

| Task | Location | Notes |
| --- | --- | --- |
| App setup/session state | `src/App.tsx`, `src/FirstRunSetup.tsx` | Owner session setup token flow. |
| Dashboard layout | `src/DashboardWorkspace.tsx`, `src/FleetPanels.tsx`, `src/SidePanel.tsx` | Machine list, metrics, inspector. |
| Onboarding UI | `src/OnboardingWizard.tsx`, `src/enrollCommand.ts`, `src/enrollmentShell.ts` | Packaged-primary copy, fallback/debug separation. |
| API mapping | `src/api.ts`, `src/apiTypes.ts`, `src/apiResources.ts`, `src/resourceApi.ts` | Keep server shape mapping explicit. |
| Resource editing | `src/ResourceEditor.tsx`, `src/DotfileResourceEditor.tsx` | Package and dotfile desired state. |
| Repair flow | `src/repairController.ts`, `src/apiRepair.ts` | Polling and stale/terminal outcomes. |
| Copy contracts | `src/copy.ts`, `src/copy.test.ts` | Korean-first UI copy and English allowlist. |
| Test harnesses | `src/appTestHarness.tsx`, `src/repairDriftTestHarness.tsx` | Reuse fetch and render helpers. |
| E2E flow | `e2e/mvp-flow.ts`, `e2e/server-fixture.ts` | Browser plus server fixture evidence. |

## CONVENTIONS

- Use tabs; Biome enforces formatting and recommended lint rules.
- TypeScript is strict; avoid index access assumptions because `noUncheckedIndexedAccess` is on.
- Tests run in jsdom with globals; E2E runs only Chromium unless config changes deliberately.
- API helpers should convert raw server types into UI types at the boundary.
- User-visible copy is mostly Korean; English labels must be intentional and covered by `copy.test.ts`.
- Onboarding primary command must be packaged-client oriented; fallback/debug copy must stay visually and semantically separate.

## ANTI-PATTERNS

- Do not put pair tokens into URL query strings, document title, history, localStorage, or logs.
- Do not show `go run ./cmd/neul` or `--pair` as the primary onboarding command.
- Do not add secret management UI before server/domain contracts enable it.
- Do not bypass visible UI in Playwright tests for flows that are meant to prove user behavior.
- Do not add a second frontend package without revisiting the single-app pnpm workspace assumptions.

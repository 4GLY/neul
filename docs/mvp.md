# neul MVP Scope And Product Spec

작성일: 2026-06-05

## 1. MVP 목표

`neul`의 첫 MVP는 single-owner self-hosted 제품이다. 개인 개발자가 2-5대의 macOS/Linux 머신을 중앙 웹 control plane에서 등록하고, 각 머신의 desired state 적용 상태와 drift를 확인하며, package/dotfile 선언을 편집하고 reconcile 결과를 신뢰할 수 있게 보는 데 집중한다.

MVP의 핵심 성공 기준은 다음이다.

- 첫 머신을 등록하고 agent가 heartbeat를 보내면 웹 대시보드에 연결 상태가 보인다.
- 머신 대시보드에서 healthy, pending, drifted, offline 상태를 구분할 수 있다.
- 사용자는 drift를 확인하고 repair reconcile을 요청할 수 있다.
- 사용자는 package와 dotfile desired state를 편집하고, 어떤 머신에 적용될지 확인할 수 있다.
- secret은 MVP에서 숨기거나 비활성 상태로만 둔다. E2E secret 생성/회전 UI와 API는 후순위로 둔다.

## 2. MVP 범위 결정

### 포함

#### 첫 머신 등록

사용자는 웹에서 첫 머신 등록 wizard를 열고, packaged neul client를 설치한
뒤 browser approval로 machine credential을 만든다. 이번 MVP의 target
flow는 self-hosted owner session을 가진 브라우저가 승인 주체이고, agent는
outbound REST만 사용한다. 별도의 hosted login, OAuth/SSO, secret runtime
surface는 만들지 않는다.

MVP 플로우:

<!-- packaged-primary:start -->

1. 웹 empty state에서 `첫 머신 등록`을 클릭한다.
2. 웹은 packaged neul client 설치 안내를 보여준다.
   - macOS local QA: unsigned dev .pkg
   - Production macOS distribution requires Developer ID Application and
     Developer ID Installer certificates, notarization, and stapling.
   - Linux: Debian/Ubuntu .deb와 tarball
3. macOS `.pkg` installs `neul` at `/usr/local/bin/neul` and `neul-agent` at
   `/usr/local/libexec/neul-agent`; per-user LaunchAgent registration is
   handled by `neul agent install`.
4. 사용자가 `neul login --server <origin>`을 실행하면 client가 approval
   record를 만들고 browser approval URL을 연 뒤 browser approval polling을
   시작한다.
5. owner session이 있는 브라우저는 approval id, nonce, comparison code,
   machine preview metadata, CSRF, status만 받는다. 브라우저 copy, URL,
   title, history, localStorage, log에는 pair code, pair token, machine
   token, setup token, plaintext verifier를 넣지 않는다.
6. owner가 승인하면 client는 `POST /api/pair/approval/claim`을 polling해
   opaque pair code를 받고, machine metadata와 함께 `/api/pair/claim`에
   제출한다. `/api/pair/claim`만 machine credential을 만든다.
7. `neul login`은 local config를 `0600` 권한으로 저장하고 enrollment
   success를 보고한다. durable running/connected state는 claim하지 않고
   다음 실행 명령으로 `neul up`을 안내한다.
8. `neul up`은 user-level agent를 시작하거나 확인하고, long-running agent의
   fresh heartbeat가 보일 때만 connected 상태를 만든다.
9. heartbeat 대기 timeout은 `/api/pair/claim` 또는 `neul login` 성공 시각이
   아니라 durable `neul up` agent-start attempt 시각에서 시작한다. 그
   attempt 이후 timeout 안에 fresh heartbeat가 보이지 않으면 웹은
   `agent_not_responding` 상태와 retry/help copy를 보여준다.

First-run states: `not_logged_in`, `waiting_for_browser_approval`, `enrolled`,
`offline`, `error`.

Self-hosted owner approval model: self-hosted 서버에 이미 로그인한 owner
session만 browser approval을 승인할 수 있다. pair code는 `/api/pair/claim`이
한 번 소비하는 값이며, browser-safe approval handoffs는 approval id, nonce,
comparison code, machine preview metadata, CSRF, status뿐이다. Pair code,
pair token, machine token, setup token, plaintext verifier는 browser copy,
general URL query string, `document.title`, browser history, localStorage,
server log에 남기지 않는다.

Approval API and package artifacts are target contract for the packaged-client
implementation. Until they ship, executable local QA uses only the
fallback/debug path below, and the wizard exposes that command separately from
the primary packaged-client command.

Unsigned dev `.pkg` artifacts are local-testing only. They must not be described
as production-ready, signed, notarized, or stapled. Production macOS
distribution needs a Developer ID Application certificate for binaries, a
Developer ID Installer certificate for the installer package, Apple
notarization, and stapling before publishing.

<!-- packaged-primary:end -->

fallback/debug path:

개발자나 QA가 packaged binary 없이 로컬 checkout에서 검증할 때만 기존
copyable command를 사용한다. packaged approval flow가 구현되기 전까지 웹
wizard도 이 명령을 fallback/debug로 별도 표시해 첫 사용자가 실제로 machine을
claim하고 heartbeat까지 진행할 수 있게 한다. 이 경로는 primary UX가 아니며
`Run with packaged neul client:` 명령과 분리한다.

이 iteration에는 no /install.sh, no `curl | sh`, no native GUI, no hosted
login, no WebSocket을 유지한다.

#### 머신 대시보드

현재 프로토타입의 `Machines` 화면을 MVP 홈으로 사용한다.

사용자가 이 화면에서 판단해야 하는 것:

- 전체 fleet이 정상인지
- drift가 있는 머신이 있는지
- pending changes가 남아 있는지
- 마지막 reconcile이 언제였는지
- agent가 연결되어 있는지
- 어떤 resource가 desired state와 어긋났는지

사용자가 클릭해야 하는 것:

- 머신 row: 오른쪽 inspector를 해당 머신으로 전환한다.
- `Reconcile now`: 현재 profile 대상 머신에 reconcile을 요청한다.
- `Repair drift`: 선택된 머신의 drifted resources를 다시 적용한다.
- status/OS filter: fleet table을 좁힌다.
- `Show ledger`: desired/live 비교 테이블을 더 크게 확인한다.

#### drift 확인과 repair

MVP drift는 server가 판단하지 않고 agent report를 신뢰한다. Agent는 reconcile 또는 periodic check 결과로 resource별 observed state를 서버에 보고한다.

Drift 상태:

- `in_sync`: desired와 observed가 같다.
- `pending`: desired가 바뀌었지만 agent가 아직 적용하지 않았다.
- `drifted`: observed가 desired와 다르다.
- `blocked`: agent가 적용을 시도했지만 실패했다.
- `unknown`: agent report가 없거나 너무 오래되었다.

Repair는 새 desired state를 만들지 않는다. 기존 desired state를 선택 머신에 다시 적용하라는 reconcile command를 만든다.

#### package desired state 편집

MVP package resource는 native package manager adapter를 통해 적용한다.

지원 resource model:

- `homebrew` package on macOS
- `apt` package on Linux
- `mise` tool version on macOS/Linux

첫 구현 순서:

- Homebrew check/apply loop를 먼저 완성한다.
- `apt`와 `mise`는 같은 resource schema에 남기되, Homebrew loop가 test와 E2E에서 안정화된 뒤 adapter를 추가한다.

MVP 편집 필드:

- name
- source kind: `brew`, `apt`, `mise`
- desired version: exact version 또는 `latest`
- target segment: `base`, OS override, tag override

#### dotfile desired state 편집

MVP dotfile은 server blob storage에 파일 내용을 저장하고 agent가 사용자 홈 아래 allowlisted path에 symlink 또는 copy로 적용한다.

지원 필드:

- path
- mode
- apply mode: `symlink` 또는 `copy`
- content
- target segment

MVP allowlist:

- `~/.zshrc`
- `~/.gitconfig`
- `~/.config/**`

`/etc`, `/usr`, root-owned path는 MVP에서 제외한다.

### 후순위

#### secret UI/API

Secret은 제품 정체성상 중요하지만, MVP 구현 범위에서는 후순위다.

MVP에서 포함하는 것:

- sidebar에 `Secrets` 항목을 남길 수는 있지만 disabled 또는 coming-soon 상태로 둔다.
- dashboard/ledger에서 secret row를 제거한다.

MVP에서 제외하는 것:

- `/api/secrets`
- 브라우저 master key 생성
- age recipients 관리
- secret value 입력/수정 UI
- secret rotation flow
- secret material을 agent로 전달하는 전체 E2E 암호화 경로

이 결정의 이유:

- secret은 보안 설계 검증 비용이 크다.
- package/dotfile/drift loop가 먼저 작동해야 secret 적용 결과도 의미가 있다.
- MVP에서 secret을 억지로 구현하면 server compromise 방지 설계를 덜 검증한 채 제품에 넣게 된다.

Post-MVP secret 설계는 [`docs/secrets-threat-model.md`](secrets-threat-model.md)
에서 별도로 threat model을 accepted 상태로 만든 뒤 시작한다.

## 3. 화면 명세

모든 사용자-facing copy는 Korean-first로 작성한다. CLI 명령, package name, file path, protocol field, API path는 영어 원문을 유지한다.

### 3.1 Machines Dashboard

프로토타입 기준 파일:

- `web/src/App.tsx`
- `web/src/FleetPanels.tsx`
- `web/src/SidePanel.tsx`
- `web/src/data.ts`

#### Metric cards

| UI 항목 | 사용자가 판단하는 것 | 데이터 출처 |
| --- | --- | --- |
| Machines | 등록된 머신 수와 online 수 | `GET /api/machines` |
| Healthy | healthy 머신 수와 비율 | `GET /api/fleet/status` 또는 machine summary 계산 |
| Drifted | drifted 머신 수 | 최신 agent report |
| Pending changes | 적용 대기 resource 수 | desired state version과 agent applied version 비교 |
| Last reconcile | fleet 전체의 최근 reconcile 시간 | latest reconcile run |

#### Machine table

| Column | 의미 | 데이터 출처 |
| --- | --- | --- |
| Machine | machine name, OS, arch, tag | machine registration metadata |
| Status | healthy, drifted, pending, offline | heartbeat + latest reconcile report |
| Desired state | in sync, applying, unknown | computed machine state |
| Drift | drifted resource count | latest observed resource states |
| Last reconcile | latest run timestamp and progress | reconcile_runs |
| Agent | agent version and connection state | heartbeat |

#### Inspector

선택된 머신의 현재 상태를 보여준다.

필수 정보:

- machine name
- OS/version/arch
- agent version
- last seen
- current status
- current reconcile step
- drift summary
- latest changes

필수 액션:

- `Repair drift`
- `View diff`
- `Open logs`

MVP에서 `Open logs`는 실제 로그 스트리밍 대신 최근 agent event 목록으로 이동한다.

#### Desired vs Live Ledger

선택 머신과 대표 비교 머신의 resource 상태를 보여준다.

사용자가 판단하는 것:

- desired state가 어떤 resource를 요구하는지
- 선택 머신이 desired를 적용했는지
- 다른 머신과 차이가 있는지
- pending/drifted/blocked가 package인지 dotfile인지

## 4. 데이터/API 경계

### 4.1 Web

책임:

- server API 데이터를 표시한다.
- package/dotfile desired state 편집 폼을 제공한다.
- reconcile command를 요청한다.
- agent와 직접 통신하지 않는다.

Web은 다음을 직접 판단하지 않는다.

- package 설치 성공 여부
- dotfile hash 비교
- disabled 상태인 secret route의 암호화/복호화 가능 여부
- OS별 adapter 동작

### 4.2 Server

책임:

- auth, pairing, machine registration
- desired state 저장
- segment resolution
- agent heartbeat 수신
- reconcile command 큐잉
- agent report 저장
- dashboard summary 계산

Server는 다음을 하지 않는다.

- 사용자의 로컬 파일 시스템 직접 접근
- package manager 직접 실행
- secret plaintext 보관

### 4.3 Agent

책임:

- heartbeat 전송
- machine metadata 보고
- desired state fetch
- adapter별 plan/apply/check 실행
- reconcile report 전송
- drift report 전송

Agent는 HTTPS outbound REST만 사용한다. Agent가 desired state를 poll하고 report를 post하며, Server가 agent에 직접 inbound 접속하지 않는다.

### 4.4 CLI

책임:

- `neul login --server <origin>` packaged-client browser approval enrollment flow
- `neul up` durable agent running/connected flow
- `neul agent enroll --server <url> --pair <pair-code> --connect-once` fallback/debug flow
- `neul init --pair <code>` backward-compatible debug flow
- `neul agent install` dry-run oriented future install flow
- `neul agent status`
- `neul agent logs`

MVP CLI는 desired state 편집 기능을 갖지 않는다. 편집은 웹 중심이다.

macOS LaunchAgent packaging contract:

- Unsigned dev `.pkg` artifacts are local-testing only.
- Local dev packages are built with `scripts/build-macos-dev-pkg.sh`; Homebrew
  tap distribution remains a future/alternate path unless fully implemented.
- Production macOS distribution requires Developer ID Application and
  Developer ID Installer certificates, notarization, and stapling.
- The `.pkg` installs `/usr/local/bin/neul` and
  `/usr/local/libexec/neul-agent`.
- Package QA may enroll explicitly with
  `neul agent enroll --server <origin> --pair <pair-code> --connect-once`, then run
  `neul agent install`.
- Legacy/debug compatibility keeps `neul init --pair --server`; it is not the
  primary product command.

## 5. 핵심 API 초안

### Machine registration

- `POST /api/pair/init`
- `POST /api/pair/claim`
- `GET /api/pair/poll`
- `GET /api/machines`
- `GET /api/machines/:machineId`

### Agent

- `POST /api/agent/heartbeat`
- `GET /api/agent/desired-state?machine_id=...`
- `POST /api/agent/reconcile-report`
- `POST /api/agent/drift-report`

### Desired state

- `GET /api/profiles`
- `POST /api/profiles`
- `GET /api/segments`
- `POST /api/segments`
- `GET /api/resources`
- `POST /api/resources/package`
- `POST /api/resources/dotfile`
- `PATCH /api/resources/:resourceId`

### Reconcile

- `POST /api/reconcile-runs`
- `GET /api/reconcile-runs`
- `GET /api/reconcile-runs/:runId`
- `POST /api/machines/:machineId/repair-drift`

## 6. MVP 수용 기준

1. 새 머신을 등록하면 dashboard에 `Connected`로 나타난다.
2. agent heartbeat가 끊기면 machine row가 `Offline`으로 전환된다.
3. package desired state를 추가하면 대상 머신에 pending change가 표시된다.
4. agent reconcile report가 성공하면 해당 resource가 `Applied`로 전환된다.
5. agent drift report가 들어오면 dashboard metric과 machine inspector에 drift가 표시된다.
6. `Repair drift`를 누르면 reconcile command가 만들어지고 run timeline에 표시된다.
7. dotfile content를 변경하면 새 file version이 저장되고 대상 머신에 pending change가 생긴다.
8. secret route/API/UI는 disabled 또는 coming-soon으로 남고, value 생성/수정 UI는 없다.

## 7. 명시적 제외

- Windows support
- team/RBAC/SSO
- billing/subscription
- container dev environment
- root/system package mutation
- secret value editing and rotation
- remote terminal execution
- arbitrary shell command resource
- `/install.sh` endpoint and `curl | sh`
- native GUI or menubar client
- WebSocket onboarding push

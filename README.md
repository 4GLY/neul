# neul

> **늘**: 내 개발 머신들을 한 곳에서 보고, 맞추고, 실행하고, 지켜본다.

항상 켜진 Mac 한 대(허브)가 tailnet 위의 개인 개발 머신(macOS·Linux·Windows)에 SSH로 접속해 상태를 수집하고 웹 UI로 보여 주는 개인 도구. 머신에는 아무것도 설치하지 않는다.

설계와 단계별 범위: [`docs/design.md`](docs/design.md). v1(Go server/agent)은 `archive/v1` 태그에 남아 있다.

## 상태

P0(가시화) — 머신 목록·상세, 머신 간 brew·mise·pacman·추적 파일 비교, 주기 수집.

## 개발

```sh
bun install
NEUL_DB=.local/dev.db bun run dev   # http://127.0.0.1:7430
bun test
bun run typecheck && bun run lint
```

## 허브에 배포

허브 Mac의 개발 저장소에서 실행한다. `origin/main`을 별도 worktree(`~/.local/share/neul/hub`)에 받아 LaunchAgent로 띄우므로, 개발 중인 체크아웃은 돌고 있는 허브에 영향을 주지 않는다.

```sh
deploy/update-hub.sh <tailscale-login>
```

`~/.ssh/config`의 별칭으로 머신을 추가한다. 허브에서 `ssh -o BatchMode=yes <별칭> true`가 성공해야 한다.

## 라이선스

Apache-2.0

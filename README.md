# neul

> **늘**: 항상 desired state로 맞춰지는 머신 control tower

여러 개발 머신(macOS·Linux)을 **중앙 웹 control plane**에서 선언적으로 관리하는 OSS 클라우드 도구.

## 무엇인가

- 웹 UI에서 패키지·dotfiles·시크릿·AI 도구 설정을 선언한다.
- 각 머신의 **agent 데몬**이 desired state로 자동 reconcile한다.
- Nix·home-manager의 *declarative + reproducible* 정신은 유지하되, *빌드 시간·격리·OS별 호환성*의 부담은 제거한다.
- Native 패키지 매니저(Homebrew·apt·mise·cargo·pipx 등)를 어댑터로 추상화한다.
- **Hosted + self-hosted** 모두 1급 시민이다. self-hosted는 `docker run` 한 줄.
- 시크릿은 **E2E 암호화** (age recipients 모델). 서버는 평문을 보지 않는다.

## 상태

🚧 **M0 프로토타입** — Go 서버, Vite 웹 UI, 로컬 agent 등록 흐름이 동작한다. 제품 설계 배경은 [`docs/2026-05-27-design.md`](docs/2026-05-27-design.md)를 참고한다.

## 로컬 데모 시작

이 흐름은 현재 프로토타입을 한 번에 빌드하고 로컬 서버를 띄운 뒤, 첫 머신을 등록하는 가장 짧은 경로다.

### 준비

- Go: `go.mod`의 버전 사용
- make
- Python 3 (`make verify-demo`의 임시 포트 선택과 JSON 확인에 사용)
- Node.js 24
- pnpm 11.5.0 이상
- curl

### 서버와 웹 UI 시작

```sh
make demo
```

`make demo`는 데모에 필요한 산출물이 있는지 확인하고, 없으면 다음 작업을 수행한다.

- `pnpm --dir web install --frozen-lockfile`
- `pnpm --dir web build`
- `go build -o .demo/neul-server ./cmd/neul-server`
- `NEUL_STATIC_DIR=web/dist`, `NEUL_DB=.demo/neul.sqlite`, `NEUL_HOME_DIR=.demo/home`, `NEUL_ADDR=127.0.0.1:8080`로 서버 시작

이미 `web/node_modules`, `web/dist/index.html`, `.demo/neul-server`가 있으면 빠른 재시작을 위해 재사용한다. 웹이나 서버를 새로 빌드해야 하면 해당 산출물을 지우거나 `make demo-clean`으로 `.demo` 서버 바이너리와 DB를 지운 뒤 다시 시작한다. 같은 `NEUL_ADDR`로 이미 실행 중이면 실행 중인 pid, 접속 주소, 중지 명령을 다시 출력한다.

성공하면 다음처럼 접속 주소와 setup token이 출력된다.

```text
neul demo running
NEUL_ADDR=127.0.0.1:8080
Open: http://127.0.0.1:8080
Setup token: setup_...
Log: .demo/neul-server.log
Stop: make demo-stop
```

`NEUL_ADDR`는 서버가 bind한 주소이고, `Open:`은 이 머신에서 열 로컬 접속 URL이다. 예를 들어 `HOST=0.0.0.0 PORT=18090`이면 `NEUL_ADDR=0.0.0.0:18090`, `Open: http://127.0.0.1:18090`처럼 출력된다.
이 경우 `Remote:` 줄도 함께 출력되며, 다른 신뢰한 기기에서 사용할 LAN IP 또는 Tailscale hostname 형식을 보여준다.

로그의 setup token 줄은 다음 형식이다.

```text
neul setup token: <token>
```

포트나 bind host를 바꿔야 하면 `HOST`와 `PORT`를 넘긴다.

```sh
make demo HOST=0.0.0.0 PORT=18090
```

`HOST=0.0.0.0`은 서버를 모든 IPv4 인터페이스에 bind한다. 로컬 브라우저에서는 계속 `http://127.0.0.1:18090`로 접속할 수 있고, 같은 LAN 또는 tailnet의 다른 기기에서는 머신의 LAN IP, Tailscale hostname, 또는 tailnet IP로 `http://<host-or-ip>:18090`에 접속한다. `HOST=` 빈 값, `HOST=host:port`, IPv6 host, IPv6 wildcard, `HOST=*`는 지원하지 않는다.

### setup token으로 로컬 세션 만들기

정식 로그인 화면이 생기기 전까지 첫 실행 setup token을 API로 교환해 로컬 세션을 만든다.

```sh
PORT="${PORT:-8080}"
TOKEN="$(awk '/neul setup token:/ { print $4; exit }' .demo/neul-server.log)"
curl -i -c .demo/cookies.txt \
  -H 'Content-Type: application/json' \
  -d "{\"setupToken\":\"$TOKEN\"}" \
  "http://127.0.0.1:${PORT}/api/session/local"
```

`PORT`는 `make demo`에 넘긴 포트와 같아야 한다. 예를 들어 `make demo HOST=0.0.0.0 PORT=18090`로 시작했다면 token 교환도 `PORT=18090`으로 실행한다.
이 token 교환 명령은 서버가 떠 있는 머신에서 실행하고, `HOST=0.0.0.0`으로 시작했더라도 curl 대상은 loopback 주소인 `127.0.0.1`을 사용한다.

`POST /api/session/local`은 setup token을 한 번만 소비하고 `neul_session` 쿠키를 만든다. 이미 소비한 token이거나 기존 `.demo/neul.sqlite`를 재사용하는 경우 token이 다시 출력되지 않는다. 새 token이 필요하면 `make demo-clean` 후 다시 `make demo`를 실행한다.

### 첫 머신 등록

<!-- packaged-primary:start -->

제품의 primary path는 packaged `neul` client를 설치한 뒤
`neul login --server <origin>`으로 browser approval enrollment를 완료하고,
`neul up`으로 durable agent running/connected 상태를 확인하는 흐름이다.
`neul login`은 local machine credential만 만들고, connected 상태는
long-running agent heartbeat를 확인하는 `neul up`이 맡는다. 현재 로컬
데모에서는 packaged binary 배포 전이므로 아래 checkout-local fallback/debug
명령으로 같은 server, `/api/pair/claim`, heartbeat 경로를 검증한다.

4GL-87 macOS package QA uses an unsigned dev `.pkg` for local testing only. The
package installs `/usr/local/bin/neul` and `/usr/local/libexec/neul-agent`;
production distribution requires Developer ID Application and Developer ID
Installer certificates, notarization, and stapling before publishing.

<!-- packaged-primary:end -->

### fallback/debug: checkout-local enrollment

개발자나 QA가 packaged client 없이 로컬 checkout에서 enroll 경로를 검증해야
할 때만 pair code를 만든 뒤 다음 fallback/debug 명령을 직접 실행한다. 이
명령은 웹 wizard의 primary copy가 아니다. packaged approval flow가 구현되기
전까지 wizard는 이 명령을 primary packaged 명령 아래 fallback/debug로 별도
표시한다. 명령 형식은 `--pair <pair-code>`이고, 아래의 `pair_...`는 실제
one-time pair code 값의 예시다.

```sh
go run ./cmd/neul agent enroll --server http://127.0.0.1:<PORT> --pair pair_... --connect-once
```

표시된 명령을 같은 checkout에서 실행하면 agent 설정을 쓰고, `--connect-once`로 한 번 heartbeat를 보내서 웹 UI가 등록 완료 상태로 바뀐다. 반복 가능한 로컬 데모를 위해 설정 파일까지 `.demo` 아래에 두려면 `--connect-once` 앞에 `--config-dir .demo/agent-config`를 추가한다.

```sh
go run ./cmd/neul agent enroll --server http://127.0.0.1:<PORT> --pair pair_... --config-dir .demo/agent-config --connect-once
```

packaged `.pkg` QA는 checkout-local `go run` 대신 설치된 binary 표면을
사용한다.

```sh
neul agent enroll --server http://127.0.0.1:<PORT> --pair pair_... --connect-once
neul agent install
```

Legacy/debug 호환성 확인이 필요할 때만 `neul init --pair --server` 경로를
사용한다.

### http, https, Tailscale 접근

로컬 `make demo` 서버는 TLS를 직접 종료하지 않는다. 그래서 기본 접속 주소는 `http://127.0.0.1:<PORT>`다. `https://`가 필요하면 Caddy, Cloudflare Tunnel, Tailscale Serve 같은 로컬 프록시 앞에 두고, 프록시가 `http://127.0.0.1:<PORT>`로 전달하도록 설정한다.

tailnet 데모에서는 서버를 모든 인터페이스에 bind해야 한다.

```sh
make demo HOST=0.0.0.0 PORT=18090
```

그 다음 같은 LAN 또는 tailnet의 기기에서 `http://<lan-ip>:18090`, `http://<tailscale-hostname>:18090`, 또는 `http://<tailnet-ip>:18090`로 접속한다.

### 중지와 정리

서버만 중지하려면 다음 명령을 실행한다.

```sh
make demo-stop
```

상태만 확인하려면 다음 명령을 실행한다.

```sh
make demo-status
```

`make demo-stop`은 로컬 데모 프로세스에 종료 신호를 보내고, 응답하지 않으면 강제로 종료한다. 이 동작은 로컬 데모 런타임용이다.

로컬 데모 DB, 로그, 빌드한 서버 바이너리, `.demo/home` agent 상태, `.demo/` 디렉터리까지 지우려면 다음 명령을 실행한다.

```sh
make demo-clean
```

문서 계약이 바뀌었는지 확인하려면 다음 명령을 실행한다.

```sh
make verify-docs
```

실제 demo 시작, setup token 교환, bind mismatch 거부, stop cleanup까지 확인하려면 다음 명령을 실행한다.

```sh
make verify-demo
```

## 아키텍처 한눈에

```
┌─────────────────────────┐         ┌───────────────────┐
│  Web UI (Vite+React)    │         │  neul-agent       │
│  ─ 머신 대시보드          │         │  ─ reconcile loop │
│  ─ 프로필·세그먼트 편집     │         │  ─ adapters       │
│  ─ 시크릿 (브라우저 암호화)  │         │    • brew         │
│                         │         │    • apt          │
└──────────┬──────────────┘         │    • mise         │
           │ HTTPS                  │    • cargo/pipx   │
           ▼                        │    • dotfile      │
┌─────────────────────────┐         │    • secret       │
│  neul-server (Go)       │◄────────┤                   │
│  ─ REST API + WS hub    │  HTTPS  └───────────────────┘
│  ─ 정적 SPA embed       │   only
│  ─ SQLite → Postgres    │
│  ─ Local FS → S3        │
└─────────────────────────┘
```

## 디렉토리

```
neul/
├── cmd/
│   ├── neul-server/      # 서버 진입점
│   ├── neul-agent/       # 머신 데몬
│   └── neul/             # CLI (init / agent install / agent status / agent logs)
├── internal/             # 도메인·인프라 코드
├── migrations/           # SQL 마이그레이션
├── web/                  # Vite + React (SPA, 정적 export)
└── docs/
    └── 2026-05-27-design.md   # 전체 설계 문서
```

## 라이선스

Apache-2.0

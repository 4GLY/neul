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

🚧 **설계 단계** — 코드는 아직 없다. 설계 문서는 [`docs/2026-05-27-design.md`](docs/2026-05-27-design.md) 참조.

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

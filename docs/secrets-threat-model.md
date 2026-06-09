# Post-MVP secrets threat model

작성일: 2026-06-09

상태: Proposed for 4GL-88. Secret value handling remains disabled until this
threat model is accepted and follow-up implementation issues are approved.
Owner: 4gly maintainers
Review-by: 2026-07-31 or before the first secret value implementation PR,
whichever comes first.

## Overview

이 문서는 `neul`이 MVP 이후 secret을 제품에 넣을 수 있는지, 넣는다면
어떤 경계와 키 관리 모델을 먼저 고정해야 하는지를 정리한다. 현재 MVP
범위는 [MVP spec](mvp.md)의 `secret UI/API` 결정을 따른다. 즉 secret
value 입력, 저장, 전달, 회전, agent 적용은 구현하지 않는다.

목표는 post-MVP 구현을 바로 시작하는 것이 아니라, 안전하게 구현할 수
있는 조건을 명확히 만드는 것이다. 이 문서가 accepted 상태가 되기 전까지
server, web, agent, CLI는 secret value를 받거나 복호화하거나 local disk에
쓸 수 없다.

## Decision

4GL-88의 결정은 다음과 같다.

- MVP에서는 secret value handling을 계속 비활성화한다.
- Post-MVP의 기본 후보는 `age recipients` 모델이다.
- `device public keys`는 age recipient 목록의 단위로 다루되, 장치 등록과
  revoke가 검증되기 전에는 자동 recipient 확장을 허용하지 않는다.
- `owner-held master key`는 복구와 새 장치 추가를 위한 소유자 통제 키로만
  사용한다. Server가 master key material 또는 passphrase를 보관하지 않는다.
- Dashboard는 `dashboard-safe secret metadata`만 표시한다. Secret value,
  ciphertext preview, decrypted sample, raw recipient private material은 표시하지
  않는다.

## MVP non-goals

MVP에서는 다음을 명시적으로 하지 않는다.

- `/api/secrets` 생성, 수정, 조회, 삭제 API
- browser key handling UI
- secret value 입력, 수정, rotation flow
- server-side secret plaintext 저장
- agent로 secret material 전달
- local disk에 secret value 쓰기
- age recipient 관리, machine key rotation, owner-held master key recovery
- dashboard ledger에 secret value 또는 ciphertext 세부 정보 표시

MVP에서 허용되는 것은 disabled 또는 coming-soon copy와 문서 링크뿐이다.
이 제한은 [MVP spec](mvp.md)의 수용 기준과 일치한다.

## Post-MVP architecture

Post-MVP에서 secret 기능을 열려면 다음 구조가 먼저 accepted 상태가 되어야
한다.

1. Browser가 secret value를 plaintext로 받는 유일한 제품 표면이다.
2. Browser는 secret value를 즉시 age recipient set으로 암호화한다.
3. Server는 ciphertext, key name, recipient key IDs, version, timestamps,
   rotation status 같은 metadata만 저장한다.
4. Agent는 자기 device private key로 필요한 ciphertext만 복호화한다.
5. Agent는 allowlisted destination에만 secret을 쓸 수 있고, 기본 destination은
   mode `0600` 파일 또는 process-local environment injection이다.
6. Revoke 또는 machine compromise 이후에는 affected secret versions를 stale로
   표시하고 rotation issue를 요구한다.

### Recovery flow

`owner-held master key`의 목적은 server가 secret plaintext를 갖지 않으면서도
소유자가 새 장치를 추가하거나 분실 장치 이후 복구할 수 있게 하는 것이다.

Post-MVP recovery flow는 다음 절차를 통과해야 한다.

1. Owner는 browser에서 master key를 만들고 passphrase 또는 hardware-backed
   unlock으로 보호한다.
2. Server는 master public key fingerprint와 recovery policy metadata만 저장한다.
3. Lost laptop 또는 새 장치 등록 시 owner browser가 기존 ciphertext를 읽고,
   owner-held master key로 복호화한 뒤 새 `device public keys` recipient를
   포함하는 새 secret version을 만든다.
4. Server는 re-encrypted ciphertext를 저장하지만 plaintext, passphrase,
   private recovery artifact는 받지 않는다.
5. Recovery가 실패하면 secret은 `recovery_blocked` 상태가 되고, UI는 새 value
   입력 또는 외부 rotation을 요구한다.

복구 artifact의 형태는 accepted 전까지 확정하지 않는다. 후보는 printed
recovery code, hardware token, OS keychain-backed private key export다. 어떤
후보를 선택하든 server custody는 금지한다.

### Audit and access log

Secret metadata도 보안 이벤트로 취급한다. Server는 다음 이벤트를 audit log에
남겨야 한다.

- secret metadata create, update, delete
- recipient approval, recipient revoke, stale-recipient warning
- rotation requested, rotation completed, rotation skipped
- agent secret apply success, failure, and redacted error class
- admin/operator action that changes backup, storage, or retention behavior

Audit log는 secret value, plaintext-derived preview, private key material,
passphrase, raw decrypted error를 포함하지 않는다. Compromised server 탐지는
완전한 방어가 아니라 사후 조사와 owner-visible anomaly detection을 돕는
보조 통제다.

### Transport security

E2E ciphertext는 transport security를 대체하지 않는다. Browser to server,
server to agent, agent to server 통신은 TLS가 기본 요구사항이다.

- Hosted mode는 managed TLS를 사용한다.
- Self-hosted mode는 reverse proxy 또는 built-in TLS 설정을 명시해야 한다.
- Agent enrollment는 pair token과 device public key fingerprint를 owner가
  확인할 수 있어야 한다.
- mTLS는 MVP 이후 hardening 후보지만 initial post-MVP secret unlock gate는
  browser-side encryption, recipient approval, and bearer-token rotation이다.

Certificate pinning은 OSS self-hosted 운영성을 해칠 수 있으므로 기본 요구사항은
아니다. 대신 dashboard는 origin, server identity, recipient fingerprints를
명확히 보여줘야 한다.

### Revocation timeline

Machine revoke는 즉시 새 desired-state fetch와 새 secret version 접근을 막아야
한다. 이미 발급된 ciphertext와 이미 local disk에 기록된 plaintext는 revoke만으로
회수할 수 없으므로 별도 rotation timeline이 필요하다.

Expected post-MVP sequence:

1. Owner revokes machine.
2. Server invalidates machine token immediately.
3. Dashboard marks all versions addressed to that device as stale.
4. Owner starts rotation for affected secrets.
5. Browser creates new ciphertext versions excluding the revoked recipient.
6. Agents acknowledge the new version or report redacted apply failure.
7. Dashboard keeps the warning until all targeted machines acknowledge or the
   owner explicitly marks external rotation complete.

Propagation SLO 후보는 revoke token invalidation 즉시, dashboard stale warning
within 10 seconds, agent acknowledgement on next poll/reconcile이다. Accepted 전
구체 값은 follow-up issue에서 고정한다.

### Backups

Server backups may contain ciphertext, recipient IDs, key names, target labels,
and audit metadata. Backups must preserve the same no-plaintext invariant as the
primary database.

Required backup posture:

- Database dumps and object-store snapshots may include ciphertext only.
- Backup restore must not resurrect revoked machine access as current.
- Backup retention must be visible to the owner/operator.
- Restored stale versions must stay stale until owner review.

### Crypto agility

`age recipients` is the preferred starting point, but secret versions must carry
envelope metadata so the project can migrate later.

Each secret version needs:

- envelope format, for example `age-v1`
- recipient key IDs and fingerprints
- creation time and rotation reason
- ciphertext checksum or integrity metadata
- deprecation status when an algorithm or recipient type is no longer allowed

Future migration must create new versions rather than rewriting audit history in
place. The server may help enumerate stale versions but must not decrypt them.

### Trust boundaries

| Boundary | Trusted side | Untrusted or lower-trust side | Required invariant |
| --- | --- | --- | --- |
| Browser to server | Owner browser session before encryption | Server, network, logs | Server never sees secret plaintext. |
| Server to agent | Authenticated desired-state API | Compromised server or stale command queue | Server can request only typed secret refs, not shell commands. |
| Agent to local disk | Agent process and allowlisted paths | Other local users, backups, malware | Secret writes are minimal, mode restricted, and auditable. |
| Owner key to device key | Owner-held recovery key | Lost, revoked, or compromised device | New recipients require owner approval and rotation semantics. |

### Attacker-controlled inputs

- Secret key names, descriptions, target segment names, and labels entered in the
  web UI
- Machine enrollment metadata and device public keys reported during pairing
- Server-stored desired state, especially if the server is compromised
- Agent reports sent back to the server
- Local filesystem state around secret destination paths

### Operator-controlled inputs

- Self-hosted server configuration
- TLS termination and backup policy
- Local OS user account, launchd/systemd user service, and file permissions
- Whether apt or other elevated adapters are enabled for the agent

## Attack surface and attacker stories

### server compromise

If an attacker compromises `neul-server`, they may read the database, mutate
desired state, enqueue reconcile commands, or alter dashboard responses. The
secret design must assume this happens.

Required controls:

- Server stores ciphertext and dashboard-safe secret metadata only.
- Server cannot mint a fake device recipient without owner-visible approval.
- Server cannot ask agent to run arbitrary commands; it can only expose typed
  desired state.
- Secret value handling remains disabled while these controls are unimplemented.

Residual risk:

- A compromised server can deny service, hide rotation warnings, present stale
  metadata, or trick the owner into approving a malicious recipient. Browser UI
  must display recipient fingerprints from locally verified state before upload.

### agent compromise

If an attacker compromises one enrolled agent or its device private key, they can
decrypt secrets addressed to that device. They must not be able to decrypt
secrets for other devices or force global re-encryption silently.

Required controls:

- Each machine has a distinct device public key and private key.
- Recipient membership is per secret version.
- Revoked machines stop receiving new versions.
- Dashboard shows affected secrets and rotation-required status without showing
  values.

Residual risk:

- Secrets already decrypted on that machine may be lost. The product can reduce
  blast radius with versioning, rotation prompts, and local file permissions, but
  cannot make a compromised endpoint safe retroactively.

### local disk exposure

Local disk exposure includes accidental backups, another local user reading
files, malware in the same account, or a stolen laptop. This matters because the
agent may eventually write decrypted values so local tools can use them.

Required controls:

- Destination paths must be allowlisted and normalized before writing.
- Default file mode is `0600`.
- Secret values are never written into repo files, shell history, logs, browser
  storage, or world-readable paths.
- Local cache entries include version and expiry metadata but no plaintext unless
  the destination itself is the intended secret material.

Residual risk:

- Any local process running as the same user may read files that user can read.
  For high-risk secrets, process-local injection or external secret managers
  should remain an option instead of mandatory disk writes.

### browser key handling

Browser key handling is high risk because the browser sees plaintext at creation
time and may hold owner key material for encryption and recovery workflows.

Required controls:

- Browser never stores secret value in URL, `document.title`, localStorage,
  sessionStorage, analytics, crash reports, or long-lived logs.
- Owner-held master key passphrase is not sent to the server.
- Clipboard interactions are explicit and short-lived.
- Encryption happens before upload, and upload payload tests assert there is no
  plaintext field.

Residual risk:

- XSS or malicious extension access can exfiltrate plaintext during entry.
  Secret UI must therefore require strong CSP, no inline script assumptions,
  careful dependency review, and visible recipient confirmation before enabling.

## Secret model comparison

| Approach | Strengths | Failure mode | Decision |
| --- | --- | --- | --- |
| `age recipients` | Mature file-oriented envelope encryption; simple multi-recipient model; server can store ciphertext only | Recipient changes require re-encryption; compromised recipient keeps old versions | Preferred post-MVP base model. |
| `device public keys` | Natural per-machine blast-radius boundary; maps to enrollment and revoke | Fake or stale device keys become dangerous if owner verification is weak | Use as recipient identities, with owner-visible approval and rotation. |
| `owner-held master key` | Enables owner recovery and adding new device recipients without server plaintext | Loss blocks recovery; browser handling is sensitive; server custody would defeat E2E | Owner-held only, never server-held. |

Rejected for now:

- Server-held master encryption key: rejected because server compromise would expose
  all secrets.
- Agent-shared symmetric fleet key: rejected because one agent compromise would
  expose every machine's secret set.
- Plaintext server storage with access controls: rejected because it violates the
  product's E2E security goal.

## Dashboard-safe secret metadata

Dashboard-safe secret metadata is data the server and UI may store or display
without revealing usable secret values.

Allowed:

- Secret key name, for example `GITHUB_TOKEN`
- Description, owner, target segment, and intended destination path label
- Secret version ID and created/updated timestamps
- Recipient key IDs, recipient display names, and verified fingerprints
- Rotation status: current, stale, rotation required, revoked recipient present
- Last applied machine ID, status, and error class without secret contents
- Ciphertext size bucket, for example `<1 KiB`, `1-10 KiB`, `>10 KiB`

Not allowed:

- Secret value
- Secret value prefix, suffix, length-exact preview, or masked format that leaks
  structure
- Decrypted sample
- Private key material
- Owner-held master key material or passphrase
- Raw ciphertext preview in dashboard tables
- Logs that include upload payloads or decrypt errors with value fragments

Metadata must be treated as sensitive enough for normal auth and audit controls,
but it is dashboard-safe because it is not sufficient to reconstruct a secret.

## Acceptance gates before enabling secret values

Secret value handling remains disabled until all gates below are complete.

- This threat model is accepted.
- Follow-up implementation issues are created from this model.
- Upload tests prove browser payloads never include plaintext after encryption.
- Server tests prove `/api/secrets` rejects plaintext fields and stores metadata
  plus ciphertext only.
- Agent tests prove destination path allowlisting, file mode, and no-log behavior.
- Browser QA proves key entry, recipient confirmation, encryption, and cleanup.
- Incident UX exists for revoke, stale recipient, and rotation-required states.

## Follow-up implementation issues

These follow-up implementation issues can be created from the accepted model.
They are issue seeds, not created tracker links. After this document is accepted,
each seed should become a Linear issue linked back to 4GL-88 and this document.

1. Add secret metadata schema and dashboard-safe list view only.
   - Scope: key name, version, recipient IDs, rotation status, last applied status.
   - Non-goal: secret value input or ciphertext upload.
2. Add browser encryption proof-of-concept behind a disabled feature flag.
   - Scope: local-only age encryption fixture and payload no-plaintext tests.
   - Non-goal: production `/api/secrets`.
3. Add device public key enrollment verification.
   - Scope: machine key fingerprint display, owner confirmation, revoke metadata.
   - Non-goal: automatic recipient expansion.
4. Add server ciphertext-only secret API.
   - Scope: reject plaintext fields, store metadata and ciphertext, audit writes.
   - Non-goal: decrypting on the server.
5. Add agent secret apply adapter.
   - Scope: decrypt only assigned versions, write allowlisted `0600` files, redact logs.
   - Non-goal: root-owned paths or shell command interpolation.
6. Add rotation and compromised-device workflow.
   - Scope: stale version detection, owner action queue, dashboard warning copy.
   - Non-goal: automatic silent rotation without owner review.

## Production-test exemption

This change is docs-only plus a docs validation script. It does not enable or
change runtime secret handling, HTTP routes, browser secret input, agent apply
logic, storage schema, or deployment configuration. Production/runtime tests are
therefore exempt for 4GL-88. The required executable verification is the docs
contract and markdown local link check in
[`scripts/validate-docs.sh`](../scripts/validate-docs.sh), plus tmux manual QA
that opens and searches this document.

## Document governance

Owner: 4gly maintainers

Review-by: 2026-07-31 or before the first secret value implementation PR,
whichever comes first.

Status transition rules:

- `Proposed`: secret value handling remains disabled and the validator enforces
  that no runtime `/api/secrets`, `SecretAdapter`, or secret encryption/decryption
  implementation has landed.
- `Accepted`: follow-up implementation issues may start, but each one still needs
  its own tests and manual QA.
- `Superseded`: a new threat model must link back to this document and explain
  the replacement decision.

# Astrahold Server

Astrahold 是全新設計的 Go authoritative MMORPG Server Core，目標是支援 **3D 王城、多人攻城、可驗證的 Server authority、持久化角色狀態，以及可量測的網路／Replication 擴充**。

> `myriad-throne-server` 只作為經驗參考；Astrahold 不沿用舊 Lineage protocol、2D `gx/gy` 世界模型或舊私服相容包袱。

## 目前狀態

目前主線已完成到 **S4-F.9 — Account Recovery / Password Reset / KDF Migration Workflow**。

目前 production vertical slice 已具備：

- XYZ + Layer authoritative 3D world model
- Server authoritative movement、Navigation、LOS、AOI、Combat、Siege
- Reliable Spawn / Despawn、Vitals、Dynamic World State、Siege Match State
- Network LOD、per-session transform cap 與 lifecycle work budget
- Durable Character state / castle ownership recovery
- Trusted TLS 1.3 game ingress、duplicate-session fence、Server-authorized takeover
- Formal TLS 1.3 login control plane與短生命週期 opaque issued session credential
- Protocol v9 authenticated realtime UDP
- connection-generation realtime key rotation / revocation
- authenticated same-IP NAT-like UDP source-port migration
- WAN-like latency / jitter / loss / burst-loss / reorder / duplication recovery
- Account-authentication provider seam
- schema-v1 high-entropy SHA-256 compatibility account map
- schema-v2 Argon2id human-password reference provider
- schema-v3 durable account lifecycle compatibility
- **schema-v4 durable account store + operator recovery grants**
- `accountctl` account create / password rotation / disable / enable lifecycle
- `accountctl migrate / issue-recovery / reset-password / rehash-password`
- schema-v3 / v4 `SIGHUP` account reload with live issued-bearer revocation
- stale pre-reload password proof cannot mint a post-reload bearer
- bounded source-IP login throttling before password KDF work
- F.8 normal Godot `Main.tscn` product login / reauthentication integration
- F.9 production recovery reset → live retirement → product reauthentication E2E

核心 production contract：

```text
Account proof
    │
    └── HTTPS / TLS 1.3
          ↓
    Account Auth Provider
          ↓
Server-owned CharacterID / takeover grant
          ↓
opaque short-lived issued session credential
          ↓
TLS 1.3 ASTRAH1 game bootstrap
          ↓
Session / Ownership Fence
          ↓
Single-owner World Loop
          ↓
Movement / Combat / Siege
          ↓
Reliable state + Protocol v9 realtime UDP
```

## 核心不變量

Astrahold 的 Server authority 不因 Client、Transport、Account UI 或 Presentation 需求而放寬：

1. **Go Server authoritative**：Client 只送 intent，不送 position truth、damage、HP、team、winner 或 castle ownership。
2. **單一 world owner**：Network / DB / GM 不直接修改 mutable world state，只能提交 bounded command。
3. **World tick 不做 blocking I/O**：outbound 只能進非阻塞 connection / outbox。
4. **Snapshot absence != despawn**：lifecycle 只由 Reliable Spawn / Despawn 管理。
5. **Spawn / Despawn confirm only after `TrySend` success**。
6. **Self correction authoritative**：Client prediction 必須收斂到 `PositionCorrection`。
7. **Gameplay Proxy 是真相**：Visual Mesh 不得反過來成為 Navigation / LOS / Gate authority。
8. **Account proof != gameplay authority**：登入或 recovery 成功不代表 Client 可以指定 CharacterID、takeover、team、HP 或 ownership。
9. **Measure → Profile → Optimize**：沒有數據前不拆 world actor、不做激進 quantization / delta compression。

## Gameplay World

目前 production world：

```text
schema_version = 2
world_id       = castle-sandbox
revision       = s3d-001
main gate id   = main-gate
main gate HP   = 1000
```

權威位置：

```go
type Position struct {
    X     float32
    Y     float32
    Z     float32
    Layer LayerID
}
```

`worlds/castle-sandbox/gameplay.json` 是 Gameplay World authoritative proxy。Surface、Portal、Blocker、Gate 與 agent 規格由 Server strict validate；Visual geometry 只負責呈現。

## Movement / Combat / Siege

Movement：

```text
ClientMoveInput
→ Envelope.Sequence
→ fenced Session ingress
→ bounded Command Queue
→ fixed 20 Hz World loop
→ Navigation / collision
→ authoritative XYZ + Layer
→ WorldSnapshot + PositionCorrection
```

Combat Client intent 只有：

```text
ClientUseAction
├── action_id
├── target_kind
└── target_id
```

Damage、range、cooldown、HP、Defeated 與 hit result 皆由 Server 決定。

Siege authoritative flow：

```text
Round start
→ Attacker / Defender role
→ main-gate HP 1000
→ Gate breach / blocker disabled
→ Throne capture / contest
→ Completed
→ winner + castle owner
→ durable ownership
→ next-round role rotation
```

`SiegeMatchState` 是 Reliable full state；Client 只保存與呈現，不自行推進 phase、winner 或 castle ownership。

## Protocol v9 / Transport

目前 wire contract 為 **Protocol v9**。

### Reliable Ordered / TCP

主要承載：

- `SessionWelcome`
- `EntitySpawn` / `EntityDespawn`
- `WorldDynamicState`
- `EntityVitalsState`
- `ClientUseAction`
- `SiegeMatchState`
- 其他低頻且不可遺失的 authoritative state

### Realtime Sequenced / UDP

承載：

- `ClientMoveInput`
- `WorldSnapshot`
- `PositionCorrection`

Realtime payload 使用 GameV1 compact binary；UDP MTU 上限維持 **1200 bytes**，`WorldSnapshot` 每 chunk 最多 **43 entities**。

### Authenticated ASTU

Protocol v9 不把 bearer realtime secret 放在 UDP packet 上：

```text
Session realtime secret
       │
       ├── one-way public RoutingID
       │
       └── HMAC-SHA256
             └── truncated 128-bit auth tag
```

- ASTU header 只帶 public per-session RoutingID。
- C2S / S2C 使用不同 HMAC domain。
- Server 先以 route 找 peer，再驗 HMAC，通過後才信任 frame / sequence / endpoint。
- stale / replay movement 同時受 transport sequence gate 與 world-runtime sequence gate 約束。
- UDP **提供 authenticity / integrity，不提供 confidentiality**。

### NAT-like endpoint migration

Server 只有在現有 connection generation 的 packet 已經通過：

```text
live RoutingID
→ valid C2S HMAC
→ valid ClientMoveInput
→ strictly newer realtime sequence
→ same source IP
```

之後才更新觀察到的 UDP source port。Unauthenticated、tampered、stale / replay、retired generation 或 cross-IP packet 都不能搶走 S2C route。

## Trusted Identity / Login / Account Lifecycle

Production `worldd` 可選擇互斥的 trusted credential source：

1. `-trusted-character-auth-file` static trusted credential mode；或
2. `-session-login-account-file` issued-session login mode。

兩種模式最後都進入既有 `sessioncredential.Provider`、ASTRAH1 trusted game admission 與 Session / Ownership Fence。

Issued-session flow：

```text
login_id + login_secret
→ HTTPS / TLS 1.3
→ Server account-authentication provider
→ Server-owned CharacterID / takeover grant
→ 32-byte CSPRNG opaque session credential
→ TLS 1.3 trusted game ingress
→ ASTRAH1 credential preface
→ Session Credential Provider
→ authoritative Character session
```

### Account schema compatibility

```text
schema v1
└── login_secret_sha256
    └── high-entropy machine/bootstrap secret only

schema v2
└── password_argon2id PHC verifier
    └── restart-only reference human-password provider

schema v3
└── durable account lifecycle compatibility
    ├── stable account_id
    ├── login_id / password_argon2id
    ├── credential_version
    ├── lifecycle timestamps
    ├── Server-owned character_id
    └── optional allow_active_takeover

schema v4
└── current durable account contract
    ├── all schema-v3 account fields
    └── recovery_grants
        ├── recovery_id
        ├── account_id
        ├── credential_version binding
        ├── token_sha256 only
        └── issued_at / not_before / expires_at
```

Schema v1/v2 保留 compatibility；schema v3 仍可讀取與 reload。**Schema v4 是目前 durable recovery contract**，而 schema v3 不會 silent accept `recovery_grants`。

### Argon2id policy

目前 Argon2id verifier envelope：

```text
version       v=19
memory        64..128 MiB
passes        3..10
parallelism   1..8
salt          16..64 bytes
digest        32 bytes
```

`accountctl` 新增／輪替／recovery reset密碼預設使用：

```text
m = 65536 KiB
t = 3
p = 4
random salt = 16 bytes
digest = 32 bytes
```

Unknown `login_id` 仍執行同成本 dummy Argon2id derivation後回相同 `invalid_credentials` shape；單一 `worldd` 最多同時執行四個 password KDF，以限制 memory pressure。

**Running durable login仍要求所有帳號使用同一 Argon2 cost policy。** `rehash-password` 的 multi-account migration 必須在 maintenance / 未 publish partial state的情況下先收斂所有帳號，再一次 start / SIGHUP。若 partial mixed-cost store 被送給 `worldd`，reload fail-closed並保留 last-known-good snapshot。

### Durable account operations

`cmd/accountctl` 提供 bounded operator surface：

```bash
accountctl init
accountctl migrate
accountctl create
accountctl set-password
accountctl issue-recovery
accountctl reset-password
accountctl rehash-password
accountctl disable
accountctl enable
```

Password 只從 `-password-stdin` 讀取，不要求把 plaintext password 放入 command-line argument。

Durable store write：

```text
validate
→ temp file in target directory
→ chmod 0600
→ write + fsync
→ atomic rename
→ chmod 0600
→ directory fsync
```

Store 使用 monotonic revision；mutation 依 expected revision fail-closed。這是 **single-writer durable JSON backend**，不宣稱 multi-host distributed transaction / consensus。

### F.9 recovery / password reset

F.9 recovery proof由 operator / verified out-of-band process發行；**沒有 public forgot-password endpoint**。

```text
accountctl issue-recovery
→ 32-byte OS CSPRNG bearer proof
→ new exclusive 0600 handoff file
→ durable store只保存 SHA-256(token)
→ proof綁定發行當下 credential_version
```

Recovery grant使用 UTC RFC3339時間窗，規則為：

```text
issued_at <= not_before < expires_at
maximum lifetime = 24h
exact expires_at = rejected
```

同帳號新 proof會 supersede舊 outstanding grant。發 proof只推進 store revision，**不推進 `credential_version`**，因此單純發 recovery proof不應踢掉 live session。

Password reset：

```text
read recovery token from file
→ SHA-256 lookup
→ require not_before <= now < expires_at
→ require current credential_version == grant credential_version
→ hash new password with current Argon2id default
→ credential_version++
→ password_changed_at update
→ consume all account recovery grants
→ store revision++
→ durable SaveIfRevision
```

Password變更與 recovery proof consumption在同一 durable update完成，因此 successful proof是一次性。任何 password rotation、disable/enable、reset或 KDF rehash只要先改變 credential generation，舊 recovery proof即使尚未到期也會 stale而 fail-closed。

Reset disabled account不會順便 enable；enable/disable仍是獨立 Server lifecycle authority。

### KDF migration

```bash
printf '%s\n' 'current password' | accountctl rehash-password \
  -path /secure/accounts.json \
  -login alice \
  -password-stdin \
  -memory-kib 65536 \
  -time 3 \
  -threads 4
```

`rehash-password` 會先驗證現有密碼，再以新 random salt產生目標 policy verifier；若 policy真的改變，會推進 credential generation與 store revision，但刻意保留 `password_changed_at`，因為 human password沒有改變。已在目標 policy時是 no-op。

KDF migration publish後，舊 verifier generation所發出的 bearer會沿既有 account-generation fence退休。

### SIGHUP account reload / live revocation

Schema v3 / v4 可在 durable store 更新後對 `worldd` 發 `SIGHUP`。

有效 reload 要求 store revision 嚴格前進。Invalid / stale / unsupported schema / mixed KDF policy reload保留 last-known-good。

安全順序：

```text
load + validate new account snapshot
→ serialize with issuance mutation lock
→ clone current issued-bearer provider
→ prune expired bearers
→ remove bearers whose account generation is no longer active
→ publish reduced transport revocation-scope allow-set
→ retire affected live peers
→ publish reduced bearer provider
→ publish replacement account authenticator
```

因此 password rotation、account disable、password reset、KDF verifier generation或其他 proof-generation 變更可讓舊 issued bearer 與 live game session立即失效。

Argon2id verification刻意在 issuance lock外執行；`Issue` 會在同一 serialization boundary重新檢查 `AuthenticationSubject + AuthenticationGeneration`。即使舊 password verification在 reload前已完成，只要新的 generation reload已 commit，stale grant就不能再 mint bearer。

F.9 production recovery E2E另外證明：

```text
schema v3 → explicit migrate → v4
→ normal Main old-password login
→ issue recovery proof
→ SIGHUP: removed_bearers=0 / retired_peers=0
→ one-time password reset
→ proof reuse rejected
→ SIGHUP: removed_bearers=1 / retired_peers=1
→ old password returns 401
→ existing F.8 normal Main requires reauthentication
→ new password login succeeds
→ explicit logout succeeds
```

Recovery proof、password與issued bearer均有 log-leak fail-fast檢查。

### Login abuse control

Login listener 在 JSON decode / password KDF 前套用 bounded fixed-window source-IP guard。

預設：

```text
30 login POST attempts / 1 minute / observed source IP
max tracked source entries = 4096
```

可調：

```text
-session-login-ip-attempt-window
-session-login-ip-max-attempts
```

超限回：

```text
HTTP 429
Retry-After: <seconds>
{"error":"login_throttled"}
```

來源身份只相信 TLS socket 的實際 `RemoteAddr`；**不信任 `X-Forwarded-For`**。未來若部署 trusted reverse proxy，必須另設計 proxy-aware attribution，而不能直接信任任意 forwarding header。

### Issued-session lifecycle

- login request 只接受 `login_id` + `login_secret`；Client 不送 CharacterID / takeover bit。
- successful login response 只包含 `session_credential` + `expires_at`。
- 每個 bearer 使用 32-byte `crypto/rand` entropy；Server provider map只保存 digest。
- TTL 預設 15 分鐘，可設定 1 分鐘到 24 小時。
- `POST /v1/session/logout` 會撤銷 bearer；known / unknown well-formed bearer都回 204。
- logout / expiry / account-generation revocation 都沿既有 F.3 transport fence退休 live peer。
- bearer 仍是 process-local；`worldd` restart 後要求重新 login。
- `allow_active_takeover` 只由 Server account / credential grant提供；Client沒有 takeover bit。

完整身份／登入文件：

- [`docs/S4F9_ACCOUNT_RECOVERY_KDF_MIGRATION.md`](docs/S4F9_ACCOUNT_RECOVERY_KDF_MIGRATION.md)
- [`docs/S4F7_DURABLE_ACCOUNT_LIFECYCLE.md`](docs/S4F7_DURABLE_ACCOUNT_LIFECYCLE.md)
- [`docs/S4F6_ACCOUNT_AUTH_PROVIDER.md`](docs/S4F6_ACCOUNT_AUTH_PROVIDER.md)
- [`docs/S4F4_FORMAL_LOGIN_SESSION_ISSUANCE.md`](docs/S4F4_FORMAL_LOGIN_SESSION_ISSUANCE.md)
- [`docs/S4F3_RUNTIME_CREDENTIAL_REVOCATION.md`](docs/S4F3_RUNTIME_CREDENTIAL_REVOCATION.md)
- [`docs/S4F2_CREDENTIAL_LIFECYCLE.md`](docs/S4F2_CREDENTIAL_LIFECYCLE.md)
- [`docs/S4F1_SESSION_CREDENTIAL_PROVIDER_SEAM.md`](docs/S4F1_SESSION_CREDENTIAL_PROVIDER_SEAM.md)
- [`docs/S4E4_SECURE_TRUSTED_INGRESS_TAKEOVER.md`](docs/S4E4_SECURE_TRUSTED_INGRESS_TAKEOVER.md)

## Replication / Scaling

目前重要 scaling gates：

```text
UDP MTU                         1200 bytes
Snapshot entities / chunk      43
Per-session transform cap      64
Lifecycle churn max            6000 / snapshot
Dirty Vitals max               4000 / tick
```

S3-E 已包含 Network LOD / tier cadence、shared AOI work、encode/buffer ownership、lifecycle convergence與 churn budget。24-client Vertical Siege、100-client Gate Zerg持續作為 correctness / performance gate；500-client soak用於較重 scaling evidence。

## Production E2E Matrix

| Stage | 驗證內容 | 狀態 |
|---|---|---|
| S4-E.1 | 真實 Go Server + 2 Godot Clients；movement / PvP / death / respawn / gate / throne / result | ✅ |
| S4-E.2 | Server SIGKILL + restart + fresh Clients；durable ownership / round recovery | ✅ |
| S4-E.3 | Production `worldd` trusted identity；HP / position autosave + crash restore | ✅ |
| S4-E.4 | TLS 1.3 ingress；duplicate fail-closed；authorized takeover / cooldown | ✅ |
| S4-E.5 | UDP HMAC、tamper / replay rejection、loss / reorder / duplicate impairment recovery | ✅ |
| S4-E.6 | Connection-generation realtime route rotation + old-generation revocation | ✅ |
| S4-E.7 | NAT-like endpoint migration、spoof protection、WAN impairment、long-session health | ✅ |
| S4-F.5 | TLS login → issued bearer；duplicate/takeover；logout/expiry live retirement | ✅ |
| S4-F.6 | Schema-v2 Argon2id human-password provider + real-Godot login / takeover / logout | ✅ |
| S4-F.7 | Schema-v3 durable store；password rotation / disable live revocation；enable/relogin；source-IP throttle | ✅ |
| S4-F.8 | Normal Godot `Main.tscn` login / live-revocation reauth / logout / 401 / 429 UX | ✅ |
| **S4-F.9** | **Schema-v4 recovery；digest-only proof；issue不踢人；one-time reset後live retirement；normal Main reauth** | **✅** |

S4-F.9 Server implementation exact final head通過 Test / Vet / Race；production recovery E2E另以 F.8 Client main exact fence跑 normal `Main.tscn`，並驗證 recovery proof issuance與 password reset對 live-session generation的不同效果。

## 文件入口

### Current production contract

- 本 `README.md` — current Server / Protocol v9 / account lifecycle / recovery / known limitations
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — Current Architecture Baseline
- [`docs/S4F9_ACCOUNT_RECOVERY_KDF_MIGRATION.md`](docs/S4F9_ACCOUNT_RECOVERY_KDF_MIGRATION.md) — schema-v4 recovery / one-time reset / KDF migration
- [`docs/S4F7_DURABLE_ACCOUNT_LIFECYCLE.md`](docs/S4F7_DURABLE_ACCOUNT_LIFECYCLE.md) — durable schema-v3 lifecycle / SIGHUP / abuse control
- [`docs/S4F6_ACCOUNT_AUTH_PROVIDER.md`](docs/S4F6_ACCOUNT_AUTH_PROVIDER.md) — account-auth provider seam / Argon2id reference backend
- [`docs/S4F4_FORMAL_LOGIN_SESSION_ISSUANCE.md`](docs/S4F4_FORMAL_LOGIN_SESSION_ISSUANCE.md) — TLS login / opaque issuance / logout / expiry
- [`docs/S4F3_RUNTIME_CREDENTIAL_REVOCATION.md`](docs/S4F3_RUNTIME_CREDENTIAL_REVOCATION.md) — runtime credential revocation
- [`docs/S4E5_AUTHENTICATED_REALTIME_BINDING.md`](docs/S4E5_AUTHENTICATED_REALTIME_BINDING.md) — Protocol v9 authenticated ASTU
- [`docs/S4E6_REALTIME_KEY_LIFECYCLE.md`](docs/S4E6_REALTIME_KEY_LIFECYCLE.md) — realtime generation rotation / revocation
- [`docs/S4E7_WAN_NAT_SECURE_DEPLOYMENT_READINESS.md`](docs/S4E7_WAN_NAT_SECURE_DEPLOYMENT_READINESS.md) — NAT / WAN readiness

### Production E2E / durability evidence

- [`docs/S4E1_TRUE_GO_GODOT_E2E_SERVER.md`](docs/S4E1_TRUE_GO_GODOT_E2E_SERVER.md)
- [`docs/S4E2_RESTART_RECOVERY_HARNESS.md`](docs/S4E2_RESTART_RECOVERY_HARNESS.md)
- [`docs/S4E3_PRODUCTION_WORLDD_CHARACTER_RESTORE_E2E_SERVER.md`](docs/S4E3_PRODUCTION_WORLDD_CHARACTER_RESTORE_E2E_SERVER.md)
- [`docs/S4D4A_SIEGE_RESULT_CONTRACT.md`](docs/S4D4A_SIEGE_RESULT_CONTRACT.md)

### Historical milestone documents

`S1*` / `S2*` / `S3*` / early `S4*` 文件刻意保留該 milestone 當時的 protocol、schema、transport與量測結果；不要把舊 Protocol / raw bearer / Gameplay World schema v1 / Full AOI內容當成目前 production contract。

常用歷史文件：

- [`docs/S2_PROTOCOL.md`](docs/S2_PROTOCOL.md)
- [`docs/S2B_TRANSPORT.md`](docs/S2B_TRANSPORT.md)
- [`docs/S3_GAMEPLAY_WORLD.md`](docs/S3_GAMEPLAY_WORLD.md)
- [`docs/S3C6_REALTIME_REPLICATION.md`](docs/S3C6_REALTIME_REPLICATION.md)
- [`docs/S3E1_REPLICATION_TIER_CADENCE.md`](docs/S3E1_REPLICATION_TIER_CADENCE.md)
- [`docs/S3E6_LIFECYCLE_WORK_BUDGET.md`](docs/S3E6_LIFECYCLE_WORK_BUDGET.md)
- [`docs/S3E9_LONG_MIXED_GAMEPLAY_SOAK.md`](docs/S3E9_LONG_MIXED_GAMEPLAY_SOAK.md)

## 開發 / Deployment

一般驗證：

```bash
go test ./...
go vet ./...
go run ./cmd/worldd
```

Account lifecycle / recovery tool：

```bash
go run ./cmd/accountctl --help
```

預設 development transport：

```text
TCP             127.0.0.1:7777
UDP             127.0.0.1:7778
World Tick      20 Hz
Snapshot        10 Hz
Protocol        v9
Gameplay World  castle-sandbox@s3d-001
```

Production static trusted deployment：

```text
-trusted-character-auth-file
-trusted-tls-listen
-trusted-tls-cert
-trusted-tls-key
```

Production issued-session deployment：

```text
-session-login-account-file
-session-login-tls-listen
-session-login-tls-cert
-session-login-tls-key
-session-credential-ttl
-session-login-ip-attempt-window
-session-login-ip-max-attempts
-trusted-tls-listen
-trusted-tls-cert
-trusted-tls-key
```

Static trusted mode 與 issued-session mode互斥。Login control plane與 trusted game ingress都要求 TLS 1.3。Realtime UDP仍是 Protocol v9 authenticated plaintext datagram。

Static trusted credential schema-v2可用原有 SIGHUP runtime reload；issued-session account schema-v1/v2為 restart-only compatibility，durable schema-v3/v4支援 SIGHUP account-generation reload。TLS certificate/key目前仍為 process-start載入。

## 目前刻意保留的限制

- Realtime UDP尚未加密；目前只有 authenticity / integrity。
- Schema-v4目前仍是 **single-writer durable JSON account backend**；尚未有 public registration、distributed account DB或 multi-writer recovery CAS。
- F.9 已有 operator/out-of-band recovery proof與one-time password reset，但尚未有 public self-service forgot-password endpoint、email/SMS ownership verification / delivery、breached-password corpus或完整 recovery UX。
- 尚未加入 MFA / TOTP / WebAuthn / passkeys / OIDC external IdP adapter。
- Login已有 direct-listener source-IP fixed-window throttling，但尚未有 trusted reverse-proxy attribution、distributed rate limit、IP reputation、credential-stuffing intelligence或 CAPTCHA。
- Issued session credential仍為 process-local short-lived proof；Server restart強制重新 login，尚無 refresh token、remembered-device session、durable bearer recovery或 cross-server revocation propagation。
- F.8 normal產品 Client已有login/reauth state machine，但尚未加入 OS keychain / secure remembered-session storage；human password與issued bearer都不落地持久化。
- TLS game/login certificate與 key尚未 hot reload。
- NAT-like migration目前只允許 same-IP UDP source-port change；跨 IP migration fail-closed。
- 尚未做 multi-server distributed ownership / failover。
- 尚未加入 periodic in-session realtime rekey、DTLS或 QUIC；目前 evidence沒有證明 MVP需要它們。
- 500 rendered Godot actors、VAT / MultiMesh與 final commercial art不是目前 Server MVP gate。

## 下一個 bounded focus

S4-F.9 已把 account recovery收斂成安全的 durable primitive：recovery bearer只以 digest保存、綁定 account credential generation、短生命週期且一次性消耗；password reset沿既有 SIGHUP generation fence撤銷舊 bearer/live peer，並已用 normal F.8 Client完成真實 reauthentication acceptance。KDF migration也有明確的 maintenance convergence與 uniform-online-policy邊界。

下一個 bounded stage 應進入 **S4-F.10 — Verified Recovery Provider / Public Reset Exchange**：在不把 recovery authority交給 Client、也不改 Protocol v9的前提下，建立可插拔的 verified-recovery provider seam與enumeration-resistant public reset exchange，定義 challenge/attempt/rate-limit/one-time consumption邊界；實際 email/SMS provider、MFA/WebAuthn/OIDC與distributed recovery仍保持獨立 decision gate。

Astrahold 的原則保持不變：**Server State 是真相；先證明 correctness，再用量測決定複雜度。**

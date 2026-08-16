# Astrahold Server

Astrahold 是全新設計的 Go authoritative MMORPG Server Core，目標是支援 **3D 王城、多人攻城、可驗證的 Server authority、持久化角色狀態，以及可量測的網路／Replication 擴充**。

> `myriad-throne-server` 只作為經驗參考；Astrahold 不沿用舊 Lineage protocol、2D `gx/gy` 世界模型或舊私服相容包袱。

## 目前狀態

目前主線已完成到 **S4-F.6 — Account Authentication Provider / Human Credential Hardening**。

核心 production contract：

```text
Godot Client
    │
    ├── Reliable Ordered / TCP
    │     └── optional TLS 1.3 trusted ingress
    │
    └── Realtime Sequenced / UDP
          └── Protocol v9 authenticated ASTU datagram
                    ↓
              tcpudp adapter
                    ↓
          Session / Ownership Fence
                    ↓
             Bounded Commands
                    ↓
          Single-owner World Loop
                    ↓
       Movement / Combat / Siege
                    ↓
              Replication
```

目前已具備：

- XYZ + Layer authoritative 3D world model
- Server authoritative movement、Navigation、LOS、AOI
- Generic `ClientUseAction` combat intent
- Gate / Character HP、Defeated、Respawn
- Reliable entity vitals / dynamic world state
- Network LOD、per-session transform cap、lifecycle work budget
- Durable trusted character identity / state restore / autosave
- Trusted active-session ownership fencing / takeover
- Formal Server-side session credential provider seam；保留 static SHA-256 compatibility provider，並新增 issued-session provider
- Credential `not_before` / expiry / revocation policy with bounded rotation overlap
- Schema-v2 runtime credential `SIGHUP` reload、emergency revocation、live-session invalidation
- TLS 1.3 formal login control plane、32-byte CSPRNG opaque session credential issuance、explicit logout、exact Server-clock expiry
- Formal account-authentication provider seam；schema v1 高熵 SHA-256 compatibility 與 schema v2 Argon2id human-password verifier 共用同一 issued-session lifecycle
- Argon2id provider 的最低 KDF policy、成本上限、unknown-login equivalent KDF work 與 bounded concurrent verification
- Authoritative Siege match state、Gate breach、Throne capture、winner、castle ownership、next-round role rotation
- Durable castle ownership recovery
- Production `worldd` TLS 1.3 trusted ingress
- Protocol v9 authenticated realtime UDP
- Connection-generation realtime key rotation / revocation
- Authenticated same-IP NAT-like UDP source-port migration
- WAN-like latency / jitter / loss / burst-loss / reorder / duplication recovery E2E
- 真實 Go Server ↔ Godot multi-client production E2E

## 核心不變量

Astrahold 的 Server authority 不因 Client、Transport 或 Presentation 需求而放寬：

1. **Go Server authoritative**：Client 只送 intent，不送 damage、HP、winner、team、position truth。
2. **單一 world owner**：Network / DB / GM 不直接修改 mutable world state，只能提交 bounded command。
3. **World tick 不做 blocking I/O**：outbound 只能進非阻塞 connection / outbox。
4. **Snapshot absence != despawn**：lifecycle 由 Reliable Spawn / Despawn 管理。
5. **Spawn / Despawn confirm only after `TrySend` success**。
6. **Self correction authoritative**：Client prediction 必須收斂到 `PositionCorrection`。
7. **Gameplay Proxy 是真相**：Visual Mesh 不得反過來成為 Navigation / LOS / Gate authority。
8. **Measure → Profile → Optimize**：沒有數據前不拆 world actor、不做激進 quantization / delta compression。

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

### Movement

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

### Combat

Client 只送：

```text
ClientUseAction
├── action_id
├── target_kind
└── target_id
```

Combat 數值由 Server Action Catalog 管理；damage、range、cooldown、HP、Defeated 與 hit result 都不是 Client authority。

### Siege

目前 authoritative Siege flow：

```text
Round start
→ Attacker / Defender role
→ main-gate HP 1000
→ Gate breach / blocker disabled
→ Throne presence + contest
→ trusted team capture progress
→ Completed
→ winner + castle owner
→ durable ownership
→ next round role rotation
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
- 其他低頻且不可遺失的 gameplay state

### Realtime Sequenced / UDP

承載：

- `ClientMoveInput`
- `WorldSnapshot`
- `PositionCorrection`

Realtime payload 使用 GameV1 compact binary；UDP MTU 上限維持 **1200 bytes**，`WorldSnapshot` 每 chunk 最多 **43 entities**。

### Protocol v9 authenticated datagram

Protocol v9 的 UDP packet 不再直接暴露 bearer token：

```text
Session realtime secret
       │
       ├── one-way derived public RoutingID
       │
       └── HMAC-SHA256
             └── truncated 128-bit auth tag
```

- ASTU header 只帶 public per-session route。
- HMAC domain separation 區分 Client→Server 與 Server→Client，阻止方向反射。
- Server 先以 route 找 peer，再用 secret 驗證 HMAC，通過後才信任 frame / sequence / endpoint。
- stale/replayed movement 受 transport realtime sequence gate 與 world-runtime sequence gate 雙重約束。
- UDP **提供 authenticity / integrity，不提供 confidentiality**。

### NAT-like endpoint migration

S4-E.7 將既有 same-IP rebind policy 做成 production E2E gate。Server 只有在以下條件都成立後才允許更新 realtime destination port：

```text
live RoutingID generation
→ valid C2S HMAC
→ valid realtime ClientMoveInput
→ strictly newer sequence
→ same source IP
→ publish new UDP source port
```

因此 unauthenticated、tampered、stale / replay、retired old-generation 或 cross-IP packet 都不能修改 S2C route。合法 NAT source-port change 不改 EntityID、ownership 或 gameplay state。

完整文件：

- [`docs/S4E5_AUTHENTICATED_REALTIME_BINDING.md`](docs/S4E5_AUTHENTICATED_REALTIME_BINDING.md)
- [`docs/S4E6_REALTIME_KEY_LIFECYCLE.md`](docs/S4E6_REALTIME_KEY_LIFECYCLE.md)
- [`docs/S4E7_WAN_NAT_SECURE_DEPLOYMENT_READINESS.md`](docs/S4E7_WAN_NAT_SECURE_DEPLOYMENT_READINESS.md)

## Trusted Identity / Login / TLS / Takeover

Production `worldd` 現在可選擇兩種互斥的 trusted credential source：既有 `-trusted-character-auth-file` static credential mode，或 issued-session login mode。兩種模式最後都進入同一個 `sessioncredential.Provider`、`ASTRAH1` trusted game admission、Session / Ownership Fence 與既有 world authority path。

Issued-session mode 的控制面與 game transport 分離：

```text
TLS 1.3 Login Client
→ /v1/session/login
→ Server account-authentication provider
→ Server-owned CharacterID / takeover grant
→ 32-byte CSPRNG opaque session credential
→ Client
→ TLS 1.3 trusted game ingress
→ ASTRAH1 credential preface
→ literal-loopback worldd TCP backend
→ Session Credential Provider
→ trusted CharacterID
```

Account-authentication provider 目前有兩種 file-backed reference schema：

```text
schema v1
└── login_secret_sha256
    └── high-entropy bootstrap / compatibility only

schema v2
└── password_argon2id PHC verifier
    ├── Argon2id v=19
    ├── memory 64..128 MiB
    ├── passes 3..10
    ├── parallelism 1..8
    ├── salt 16..64 bytes
    └── digest 32 bytes
```

Schema v2 在 unknown `login_id` 也會執行同成本的 dummy Argon2id derivation，再回相同 `invalid_credentials` shape；單一 `worldd` 最多同時執行四個 password KDF，以限制 account verification 的記憶體壓力。這是 application-level enumeration hardening，不宣稱網路層 perfect timing indistinguishability。

Static compatibility mode 則維持：

```text
TLS 1.3 trusted game ingress
→ ASTRAH1 credential preface
→ literal-loopback worldd TCP backend
→ static SHA-256 Session Credential Provider
     ├── schema v1 timeless / restart-only compatibility
     └── schema v2 lifecycle + runtime reload / invalidation
→ trusted CharacterID
```

重要語意：

- 未設定 trusted auth 時，原本 ephemeral development identity 仍可使用。
- Trusted bearer bootstrap 的 backend TCP 必須維持 loopback。
- `sessioncredential.Provider` 不擁有 TCP / SessionID / EntityID / world state；只解析 opaque credential 成 trusted grant。
- account-authentication provider 只驗證 account proof 並回 Server-owned `sessioncredential.Grant`；issued bearer、revocation scope、logout / expiry lifecycle 仍由原有 issuance runtime 管理。
- provider grant 必須是 `AssuranceTrusted`，否則 fail-closed before normal GameV1 admission。
- issued-session mode 與 `-trusted-character-auth-file` 刻意互斥，避免同一 process 出現兩套 credential namespace / revocation-scope authority。
- login request 只接受 `login_id` + `login_secret`；CharacterID 與 `allow_active_takeover` 只由 Server account record 決定，未知 JSON authority 欄位直接拒絕。
- 每個 session credential 使用 32-byte `crypto/rand` entropy，Client 看到的是 43-char unpadded base64url opaque bearer；provider map 只保留 SHA-256 digest，不保存 plaintext bearer。
- issued credential TTL 預設 15 分鐘，可設定 1 分鐘到 24 小時；exact expiry 由 Server clock 判斷。
- `POST /v1/session/logout` 會撤銷 issued bearer；known / unknown well-formed bearer 都回 204，避免 credential existence oracle。
- issued credential 的 logout / expiry 都先撤 transport scope，再移除 provider record；live peer 因此沿 S4-F.3 `ready=false` → token/route revocation → TCP close → fenced leave 路徑退休。
- issued credential 是 process-local；`worldd` restart 後舊 bearer 全部失效並要求重新 login，不把短期 auth proof 變成新的 durable gameplay authority。
- schema v1 `login_secret_sha256` 只適合高熵 machine/bootstrap secret；human password 使用 schema v2 Argon2id verifier。
- schema v2 account file 仍只是 reference/bootstrap backend，不等於完整 account DB、registration/recovery、MFA、OIDC 或 password reset service。
- static schema v1 仍維持 timeless 相容性，而且不能偷偷使用 lifecycle 欄位；v1 runtime reload / live invalidation 刻意不支援。
- static schema v2 要求唯一 Server-side `credential_id`，可設定 RFC3339 `not_before` / `expires_at` / `revoked_at`。
- `not_before` 邊界當下可用；`expires_at` / `revoked_at` 邊界當下立即失效。
- schema v2 會產生 Server-only proof scope，綁定 credential ID / token digest / CharacterID / takeover authority；scope 不送給 Client。
- lifecycle 時間不參與 static proof scope fingerprint；future expiry/revocation 由 Server-clock boundary timer 在指定時間移除 scope。
- operator 可 atomically replace schema-v2 credential JSON 後對 `worldd` 發 `SIGHUP`；合法 reload 更新 provider 與 live scope policy，非法 reload 保留 last-known-good。
- reload 先安裝 transport scope fence，再 publish 新 provider；舊 in-flight authentication 無法在 revocation 後晚到並取得 realtime/world authority。
- `allow_active_takeover` 預設 `false`，duplicate active session fail-closed。
- 只有 Server credential / account provider grant 可授權 active takeover；Client 沒有 takeover bit。
- takeover authority 綁定 exact CharacterID，並轉成既有 connection-scoped authorizer。
- world ownership transfer 使用 fence / epoch CAS。
- 舊 peer retirement 後 stale Leave 不能刪掉新 owner。
- takeover candidate 有 TTL / cooldown，避免連續重奪。

完整文件：

- [`docs/S4F6_ACCOUNT_AUTH_PROVIDER.md`](docs/S4F6_ACCOUNT_AUTH_PROVIDER.md)
- [`docs/S4F4_FORMAL_LOGIN_SESSION_ISSUANCE.md`](docs/S4F4_FORMAL_LOGIN_SESSION_ISSUANCE.md)
- [`docs/S4F3_RUNTIME_CREDENTIAL_REVOCATION.md`](docs/S4F3_RUNTIME_CREDENTIAL_REVOCATION.md)
- [`docs/S4F2_CREDENTIAL_LIFECYCLE.md`](docs/S4F2_CREDENTIAL_LIFECYCLE.md)
- [`docs/S4F1_SESSION_CREDENTIAL_PROVIDER_SEAM.md`](docs/S4F1_SESSION_CREDENTIAL_PROVIDER_SEAM.md)
- [`docs/S4E4_SECURE_TRUSTED_INGRESS_TAKEOVER.md`](docs/S4E4_SECURE_TRUSTED_INGRESS_TAKEOVER.md)

## Realtime Key Lifecycle

每次新 TCP connection 都建立新的 128-bit realtime secret，因此 reconnect / trusted takeover 自然形成新的 **connection generation**。

S4-E.6 把 lookup lifecycle 做成 fail-closed：

- token collision → reject
- derived route collision → reject
- token / route map 必須 atomic publish
- `closePeer` 先 `ready=false`
- revocation 只刪除仍指向該 exact peer generation 的 token / route
- stale close 不能誤刪 replacement generation

Production E2E 已證明：authorized TLS takeover 後 public route 旋轉，舊 generation 的 authenticated datagram 即使從另一 UDP source port replay，也不能重新取得 authoritative entity control；replacement Client 保留同一 EntityID / live position，並可繼續正常 movement / snapshot / correction。

S4-E.7 沒有加入 periodic in-session rekey。現階段 connection-generation rotation + retirement revocation 已涵蓋 reconnect / takeover 的 key lifecycle；若未來長時 session 的 secret-lifetime policy 要求比 connection lifetime 更短，再導入有 acknowledgement / overlap semantics 的 periodic rekey。

## WAN / Long-session Readiness

S4-E.7 paired production E2E 使用透明 UDP relay，在 real Godot Client 與 production `worldd` 之間驗證：

- same-IP NAT-like source-port rebind
- authenticated newer-sequence migration
- attacker wrong-HMAC / tamper rejection
- stale replay rejection
- old endpoint 不再接收後續 realtime output
- latency / jitter
- packet loss / burst loss
- reorder / duplication
- correction sequence lag bounded
- snapshot / correction freshness recovery
- bounded post-stop convergence
- snapshot loss 不產生 local fake despawn

目前沒有 E2E evidence 支持為 MVP 立即引入 explicit rebind challenge、DTLS 或 QUIC。若未來需求包含 UDP confidentiality、跨 IP connection migration、不同 congestion-control contract 或 profiling 顯示現有 split transport 不足，再以具體需求評估。

## Replication / Scaling

S3-E 已完成從早期 Full AOI snapshot 到 bounded replication 的演進：

- Network LOD / tier cadence
- shared AOI replication work
- realtime encode efficiency
- buffer ownership
- lifecycle semantic convergence
- lifecycle churn work budget
- teleport / repeated churn / mixed gameplay soak

目前重要 scaling gates：

```text
UDP MTU                         1200 bytes
Snapshot entities / chunk      43
Per-session transform cap      64
Lifecycle churn max            6000 / snapshot
Dirty Vitals max               4000 / tick
```

24-client Vertical Siege 與 100-client Gate Zerg regression 持續作為 correctness / performance gate；500-client soak workflow 用於較重的 scaling 驗證。

## Production E2E Matrix

| Stage | 驗證內容 | 狀態 |
|---|---|---|
| S4-E.1 | 真實 Go Server + 2 Godot Clients；movement / PvP / death / respawn / gate / throne / result | ✅ |
| S4-E.2 | Server SIGKILL + restart + fresh Clients；durable ownership / round recovery | ✅ |
| S4-E.3 | Production `worldd` trusted identity；HP / position autosave + crash restore | ✅ |
| S4-E.4 | TLS 1.3 ingress；duplicate fail-closed；authorized takeover / cooldown | ✅ |
| S4-E.5 | UDP HMAC、tamper / replay rejection、loss / reorder / duplicate impairment recovery | ✅ |
| S4-E.6 | Connection-generation realtime route rotation + old-generation revocation | ✅ |
| S4-E.7 | NAT-like endpoint migration、rebind spoof protection、WAN impairment、long-session health | ✅ |
| S4-F.6 | Schema-v2 Argon2id account provider；real Godot password login → issued bearer → duplicate/takeover/logout/reuse rejection | ✅ |

## 文件入口

### Current production contract

以下文件描述**目前**架構與 deployment / realtime security boundary：

- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — Current Architecture Baseline
- [`docs/S4F6_ACCOUNT_AUTH_PROVIDER.md`](docs/S4F6_ACCOUNT_AUTH_PROVIDER.md) — account-auth provider seam / Argon2id human-password reference backend
- [`docs/S4F4_FORMAL_LOGIN_SESSION_ISSUANCE.md`](docs/S4F4_FORMAL_LOGIN_SESSION_ISSUANCE.md) — TLS login / short-lived opaque session issuance / logout / expiry
- [`docs/S4F3_RUNTIME_CREDENTIAL_REVOCATION.md`](docs/S4F3_RUNTIME_CREDENTIAL_REVOCATION.md) — runtime reload / emergency revocation / live-session invalidation
- [`docs/S4F2_CREDENTIAL_LIFECYCLE.md`](docs/S4F2_CREDENTIAL_LIFECYCLE.md) — credential expiry / rotation / revocation lifecycle
- [`docs/S4F1_SESSION_CREDENTIAL_PROVIDER_SEAM.md`](docs/S4F1_SESSION_CREDENTIAL_PROVIDER_SEAM.md) — opaque session credential → trusted Server claims provider boundary
- [`docs/S4E4_SECURE_TRUSTED_INGRESS_TAKEOVER.md`](docs/S4E4_SECURE_TRUSTED_INGRESS_TAKEOVER.md) — TLS trusted ingress / takeover policy
- [`docs/S4E5_AUTHENTICATED_REALTIME_BINDING.md`](docs/S4E5_AUTHENTICATED_REALTIME_BINDING.md) — Protocol v9 authenticated ASTU
- [`docs/S4E6_REALTIME_KEY_LIFECYCLE.md`](docs/S4E6_REALTIME_KEY_LIFECYCLE.md) — connection-generation rotation / revocation
- [`docs/S4E7_WAN_NAT_SECURE_DEPLOYMENT_READINESS.md`](docs/S4E7_WAN_NAT_SECURE_DEPLOYMENT_READINESS.md) — NAT / WAN / deployment readiness

### Production E2E / durability evidence

- [`docs/S4E1_TRUE_GO_GODOT_E2E_SERVER.md`](docs/S4E1_TRUE_GO_GODOT_E2E_SERVER.md)
- [`docs/S4E2_RESTART_RECOVERY_HARNESS.md`](docs/S4E2_RESTART_RECOVERY_HARNESS.md)
- [`docs/S4E3_PRODUCTION_WORLDD_CHARACTER_RESTORE_E2E_SERVER.md`](docs/S4E3_PRODUCTION_WORLDD_CHARACTER_RESTORE_E2E_SERVER.md)
- [`docs/S4D4A_SIEGE_RESULT_CONTRACT.md`](docs/S4D4A_SIEGE_RESULT_CONTRACT.md)
- [`docs/S3F18_TRUSTED_SESSION_OWNERSHIP_FENCE_SEAM.md`](docs/S3F18_TRUSTED_SESSION_OWNERSHIP_FENCE_SEAM.md)
- [`docs/S3F20_TRUSTED_TCPUDP_ACTIVE_TAKEOVER_SEAM.md`](docs/S3F20_TRUSTED_TCPUDP_ACTIVE_TAKEOVER_SEAM.md)
- [`docs/S3F23_TRUSTED_CONNECTION_AUTHENTICATION_CONTEXT_SEAM.md`](docs/S3F23_TRUSTED_CONNECTION_AUTHENTICATION_CONTEXT_SEAM.md)

### Historical milestone documents

`S1*` / `S2*` / `S3*` / early `S4*` 文件刻意保留該 milestone 當時的 protocol、schema、transport 與量測結果。看到 Protocol v1/v2/v3/v6/v8、raw bearer realtime token、Gameplay World schema v1、Full AOI snapshot 等內容時，**不要把它當成目前 production contract**。

常用歷史文件：

- [`docs/S2_PROTOCOL.md`](docs/S2_PROTOCOL.md)
- [`docs/S2B_TRANSPORT.md`](docs/S2B_TRANSPORT.md)
- [`docs/S3_GAMEPLAY_WORLD.md`](docs/S3_GAMEPLAY_WORLD.md)
- [`docs/S3C6_REALTIME_REPLICATION.md`](docs/S3C6_REALTIME_REPLICATION.md)
- [`docs/S3E1_REPLICATION_TIER_CADENCE.md`](docs/S3E1_REPLICATION_TIER_CADENCE.md)
- [`docs/S3E6_LIFECYCLE_WORK_BUDGET.md`](docs/S3E6_LIFECYCLE_WORK_BUDGET.md)
- [`docs/S3E9_LONG_MIXED_GAMEPLAY_SOAK.md`](docs/S3E9_LONG_MIXED_GAMEPLAY_SOAK.md)

## 開發

一般驗證：

```bash
go test ./...
go vet ./...
go run ./cmd/worldd
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

Production static trusted deployment 可設定：

```text
-trusted-character-auth-file
-trusted-tls-listen
-trusted-tls-cert
-trusted-tls-key
```

Production issued-session deployment 可設定：

```text
-session-login-account-file
-session-login-tls-listen
-session-login-tls-cert
-session-login-tls-key
-session-credential-ttl
-trusted-tls-listen
-trusted-tls-cert
-trusted-tls-key
```

Static trusted mode 與 issued-session mode 互斥。TLS flag 必須成組使用；login control plane 與 trusted game ingress 都要求 TLS 1.3。Realtime UDP 仍是 Protocol v9 authenticated plaintext datagram。Static schema-v2 credential file 可在 atomically replace 後以 `SIGHUP` reload；login account verifier 與兩組 TLS certificate/key 目前都是 process-start 載入。

## 目前刻意保留的限制

- Realtime UDP 尚未加密；目前只有 authenticity / integrity。
- S4-F.6 已有 formal account-auth provider seam 與 Argon2id human-password reference verifier，但仍是 static file-backed bootstrap backend；尚未有 durable account DB、registration/recovery、password reset/KDF migration、MFA/WebAuthn 或 OIDC adapter。
- issued session credential 是 process-local、短生命週期 proof；尚無 refresh token、durable/distributed session credential store 或 cross-server revocation propagation。
- real Godot F.5/F.6 E2E 已走 `/v1/session/login` → issued bearer → `ASTRAH1`，但一般產品 Client 尚未接完整 visual login state machine / secure local credential handling。
- schema v1 trusted credential config 僅保留 restart-only 相容性，不提供 runtime revocation scope。
- TLS terminator 與 `worldd` 目前在同一 process，game/login certificate/key 尚未 hot reload。
- login abuse throttling / IP reputation / CAPTCHA 等 anti-abuse policy 尚未加入。
- NAT-like migration 目前只允許 same-IP UDP source-port change；跨 IP migration fail-closed。
- 尚未做 multi-server distributed ownership / failover。
- 尚未加入 periodic in-session realtime rekey、DTLS 或 QUIC；S4-E.7 目前沒有證明 MVP 需要它們。
- 500 rendered Godot actors、VAT / MultiMesh 與 final commercial art 不是目前 Server MVP gate。

## 下一個 bounded focus

S4-F.6 已把 human-password verification 從 issued-session lifecycle 中抽成正式 account-authentication seam，並以 Argon2id reference provider + real Godot E2E 證明 account proof 不需要改寫 game-session contract。下一個 bounded stage 應進入 **S4-F.7 — Durable Account Backend / Credential Lifecycle & Abuse Controls**：把 static reference account file 往可管理的 durable account store、password reset/KDF migration、reauthentication 與 login abuse controls 推進；MFA/OIDC 或 distributed issuance 仍應保持可插拔，不把它們硬綁進 Protocol v9。

Astrahold 的原則保持不變：**Server State 是真相；先證明 correctness，再用量測決定複雜度。**
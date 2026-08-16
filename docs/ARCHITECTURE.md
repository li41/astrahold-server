# Astrahold Server Current Architecture Baseline

> **Current production contract.** 本文件描述截至 **S4-F.23 — Authenticated Proxy Identity-Aware Retirement / Partial Binding Rotation** 的現行 Server 架構；paired Godot Client runtime 維持 **S4-F.11 — Client Recovery UX / Provider-Neutral Reset Flow**，wire contract 維持 **Protocol v9**。早期 `S1*`、`S2*`、`S3*` 與較早 `S4*` stage 文件保留當時的 Protocol / Transport / Gameplay / Security contract 作為歷史證據；若內容與本文件或 root `README.md` 衝突，以目前 production contract 為準。

## 核心決策

Astrahold Server 是全新 Go authoritative MMORPG Server Core。舊系統只能作為經驗參考，不能把舊 protocol、2D position model 或私服相容層當成 Astrahold 的架構母體。

目前核心原則：

1. **Server State 是 gameplay truth。** Client 只送 intent，不送 position truth、damage、HP、team、winner 或 ownership。
2. **單一 World owner。** Network、DB、GM、管理 API 不直接修改 mutable world state，只能 enqueue bounded command。
3. **World tick 不做 blocking I/O。** Network / persistence 都在 owner loop 外處理。
4. **Gameplay Proxy 是權威契約。** Visual Mesh 永遠不能成為 Navigation、LOS、Gate 或 objective truth。
5. **Snapshot absence != despawn。** Entity lifecycle 只相信 Reliable Spawn / Despawn。
6. **Account / recovery proof != gameplay authority。** Login 或 recovery 成功不代表 Client 可以指定 CharacterID、takeover、team、HP 或 ownership。
7. **Measure → Profile → Optimize。** 沒有 profiling 證據前，不拆 world actor、不做激進 quantization / delta compression，也不因技術偏好替換 transport。

## 世界模型

Astrahold 使用權威 XYZ + Layer 世界模型：

```text
Position
├── X (m)
├── Y / Height (m)
├── Z (m)
└── LayerID
```

`LayerID` 是邏輯拓樸，不是單純高度。城牆、橋面、地下層即使 X/Z 重疊，仍可由 Layer 區分。

目前 production Gameplay World：

```text
schema_version = 2
world_id       = castle-sandbox
revision       = s3d-001
main gate id   = main-gate
main gate HP   = 1000
```

`worlds/castle-sandbox/gameplay.json` 是 Server authoritative gameplay proxy。Surface、Portal、Blocker、Gate 與 agent 規格由 Server strict validate；Client Visual World 只能呈現。

## Runtime ownership

```text
Network Reader / Trusted Ingress / Persistence
                  ↓
        Session / Ownership Fence
                  ↓
          Bounded Command Queue
                  ↓
        Single-owner World Runtime
          ┌───────┴────────┐
          ↓                ↓
     Command apply     Fixed 20 Hz Tick
                           ↓
          Movement / Combat / Siege
                           ↓
              AOI / Replication
                           ↓
       Bounded Reliable / Realtime Outbox
                           ↓
                 Network Writers
```

### Concurrency 不變量

- Network、DB、GM、管理 API 不得直接修改 World mutable state。
- World tick 不得直接做 blocking socket / file / DB I/O。
- Join、Leave、restore、ownership transfer 都必須經 WorldRuntime transaction / fence。
- outbound queue 必須 bounded；backpressure 不能靠無限記憶體吸收。
- Account generation、issued bearer、recovery reset、connection takeover 與 edge reload 都必須在各自的 mutation / publication fence 內保持可驗證 ordering。

## Protocol v9

目前 wire contract 是 **Protocol v9**。

### ASTR frame

Gameplay envelope 使用固定 28-byte ASTR frame header：

```text
0   uint32  Magic = ASTR
4   uint16  Protocol Version = 9
6   uint16  Header Size = 28
8   uint16  Message Type
10  uint8   Delivery Class
11  uint8   Flags
12  uint32  Sequence
16  uint64  Server Tick
24  uint32  Payload Length
28  bytes   Payload
```

Frame decoder 必須驗證 magic、version、header size、delivery 與 payload length。Wire-incompatible 變更必須升 Protocol Version；不能做隱性相容。

### Reliable Ordered / TCP

主要承載：

- `SessionWelcome`
- `EntitySpawn` / `EntityDespawn`
- `WorldDynamicState`
- `EntityVitalsState`
- `ClientUseAction`
- `SiegeMatchState` (`MessageType = 106`)

Production trusted ingress 與 issued-session game bootstrap 都使用 TLS 1.3。Client 取得 gameplay admission 的 authority 來自 Server-owned credential / issued-session state，不來自 Client 自報 CharacterID 或 takeover bit。

### Realtime Sequenced / UDP

承載：

- `ClientMoveInput`
- `WorldSnapshot`
- `PositionCorrection`

Realtime payload 使用 GameV1 compact binary。

Protocol v9 ASTU datagram：

```text
ASTU header                     24 bytes
├── magic / version / size
└── public RoutingID            16 bytes
ASTR frame + GameV1 payload
HMAC-SHA256 auth tag            16 bytes (truncated 128-bit)
```

Realtime secret 只由 protected reliable bootstrap 交付；UDP packet 不直接攜帶 bearer secret。RoutingID 由 secret 單向導出。Client→Server / Server→Client 使用不同 HMAC domain，避免方向反射。

UDP 提供 **authenticity / integrity，不提供 confidentiality**。

### MTU contract

```text
MaxDatagramSize                 1200 bytes
MaxSnapshotEntitiesPerChunk       43
```

最大 43-entity snapshot 在 Protocol v9 下為：

```text
24 ASTU + 28 ASTR + 1132 GameV1 snapshot + 16 auth tag = 1200 bytes
```

不提高 MTU、不依賴 IP fragmentation。

## Realtime endpoint / NAT policy

Same-IP NAT-like source-port migration 已是 production gate。

Server 對 C2S realtime datagram 的安全順序：

```text
live public RoutingID
→ resolve exact peer generation
→ validate C2S HMAC
→ validate realtime ClientMoveInput
→ require strictly newer realtime sequence
→ require same source IP
→ publish observed source port as S2C endpoint
→ enqueue bounded movement intent
```

因此：

- wrong HMAC / tampered packet 不能改 endpoint；
- stale / captured replay 不能改 endpoint；
- retired old-generation packet 不能重新取得 route；
- cross-IP migration fail closed；
- legitimate same-IP newer authenticated packet 可以更新 NAT source port；
- endpoint migration 不改 EntityID、ownership 或 gameplay state。

目前沒有 explicit rebind challenge、DTLS、QUIC 或 cross-IP connection migration。若未來 confidentiality、跨 IP migration、congestion-control contract 或 profiling 形成具體需求，再評估 transport evolution。

## Session / identity / ownership

Production `worldd` 有兩個互斥的 trusted identity 入口：

1. **Static trusted credential mode**：`-trusted-character-auth-file`；
2. **Issued-session login mode**：`-session-login-account-file` + TLS 1.3 login control plane。

兩條路徑最後都進入 Server-owned session credential provider、ASTRAH1 trusted game admission 與 Session / Ownership Fence。

Client 不提交：

- CharacterID
- ownership epoch
- takeover permission
- account generation
- team
- HP / damage
- restore transform

Active takeover authority 只存在 Server policy。Ownership transfer 使用 exact fence / epoch CAS；成功 takeover 保留 authoritative EntityID / live state，advance ownership epoch，retire old peer，再建立 replacement connection generation。

每個新 reliable connection 都取得新的 realtime secret / public route generation。Old peer retirement 立即撤銷該 exact generation 的 route lookup；stale close 不能刪除 replacement generation。

### Issued-session login

Public login request 只接受：

```text
login_id + login_secret
```

成功後 Server 發出短生命週期 opaque `session_credential`；Client 不從 bearer 推導 CharacterID 或 account authority。Issued bearer 為 process-local proof；logout、expiry、account-generation change、password reset 或其他明確 revocation 都可沿既有 transport/session fence 退休 live peer。

Account provider compatibility：

- schema-v1：high-entropy SHA-256 compatibility map；
- schema-v2：Argon2id human-password reference provider；
- schema-v3：durable account lifecycle；
- schema-v4：durable account + recovery generation contract。

Schema-v3 / v4 支援 `SIGHUP` account reload；public recovery 啟用時要求 schema-v4。Account disable、password rotation、recovery reset 或 verifier generation 變更都不能讓 pre-change proof 在 post-change generation 繼續取得 authority。

## Public recovery / delivery lifecycle

F.10–F.15 建立的 recovery contract 不把 provider 或 destination authority交給 Client：

```text
Client sends login_id
→ Server resolves eligibility + destination internally
→ generic public request response returns opaque request_id + expires_at
→ Server-owned recovery provider / delivery path emits proof out of band
→ Client submits opaque request_id + proof + replacement password
→ Server validates provider proof + account generation + attempt/TTL bounds
→ successful reset advances account generation
→ old issued bearer / live peer retired immediately
→ Client returns to fresh normal login
```

Current recovery layers：

- **F.10** provider-neutral public request/reset seam；known/unknown request維持 enumeration-safe response shape。
- **F.12** `DeliveryAdapter` seam；destination只由 Server/provider config 決定。
- **F.13** vendor-isolated `https-json-v1` relay；TLS 1.3、owner-only Bearer credential、bounded retry、stable idempotency identity與 secret-safe observability。
- **F.14** schema-v2 provider / credential / private-CA `SIGHUP` generation reload；invalid replacement保留 LKG，pre-cutover challenge仍路由到原 verifier。
- **F.15** optional bounded single-host durable delivery/challenge outbox；pending delivery可跨 `worldd` restart 重播，terminal/success state會 scrub raw proof與destination。

這不是 distributed broker、multi-writer recovery CAS 或 exactly-once vendor delivery。

## Durable character state

Trusted character state 的 durability / restore 路徑維持：

```text
World owner captures authoritative state
→ bounded save intent / autosave path
→ durable character store
→ production worldd restart
→ trusted credential resolves CharacterID
→ exact world provenance validation
→ immutable CharacterRestore
→ WorldRuntime join transaction
```

Restore truth 來自 durable Server state；Client 不提供 HP、position 或 revision。Production E2E 已用真實 `cmd/worldd` + Godot Client 驗證 HP / position autosave、SIGKILL、restart、fresh reconnect restore。

## Replication semantics

### Lifecycle

```text
AOI enter  → Reliable EntitySpawn
AOI leave  → Reliable EntityDespawn
```

Spawn / Despawn 只有在 outbound `TrySend` 成功後才 confirm。Snapshot loss / absence 永遠不能隱含 despawn。

### Transform / correction

```text
WorldSnapshot       → Realtime partial transform update batch
PositionCorrection  → Realtime authoritative self correction
```

`WorldSnapshot` 是 chunked update batch，不代表 Full AOI entity list。Client 只有在同 tick chunk set 完整時才提交該 batch；缺少某 Entity transform 不代表 lifecycle change。

目前 replication scaling gates：

```text
UDP MTU                         1200 bytes
Snapshot entities / chunk        43
Per-session transform cap        64
Lifecycle churn max            6000 / snapshot
Dirty Vitals max               4000 / tick
```

Network LOD / tier cadence、shared AOI work、buffer ownership、lifecycle budget、teleport/repeated churn/mixed soak 都已納入 Server scaling contract。

## Navigation / gameplay geometry

Navigation backend 可以演進，但 gameplay result 必須由 Server 可重現判定。Gameplay proxy 至少負責：

- walkable surface / height
- Layer transition
- movement blocker
- LOS blocker
- Gate topology
- authoritative world identity / revision

Client raycast、ground probe、Navigation preview、Visual Mesh collision 都只是 prediction / presentation，不可取代 Server legality。

## TLS certificate lifecycle

自 **F.16** 起，session-login/public-recovery TLS listener 與 trusted game ingress certificate/key 都可由同一 process `SIGHUP` 觸發各自獨立的 fail-closed generation reload：

```text
load candidate certificate + private key
→ validate key match
→ validate X.509 validity / ServerAuth usage
→ failure: keep listener LKG
→ success: publish immutable certificate generation
→ established TLS connection keeps negotiated state
→ new handshake resolves new generation
```

因此「Server certificate只能 process start 載入」已不是目前 contract。Client trust-store / CA hot reload、ACME/PKI automation、OCSP lifecycle與multi-host certificate atomic cutover仍未納入本階段。

## Trusted reverse proxy / source attribution

### F.17 source attribution

Direct/untrusted TLS socket peer 永遠只以實際 `RemoteAddr` 作 login/recovery abuse-control source，並忽略 forwarding headers。只有 operator 信任的 reverse-proxy socket peer 才能使用選定的 `X-Forwarded-For` 或 `Forwarded`。

Forwarding parser維持：

- field size ≤ 1024 bytes；
- hop count ≤ 16；
- IPv4/IPv6 normalization；
- right-to-left trusted-hop stripping；
- trusted peer缺少或 malformed selected metadata時，在 password KDF / recovery provider之前 fail closed。

### F.18 legacy upstream mTLS

Legacy F.17 allowlisted proxy可再要求 upstream client certificate chain + exact DNS SAN identity。Direct Godot Client仍是 TLS 1.3 **server-auth-only**，不持有 reverse-proxy client certificate。

### F.19 atomic edge-policy generation

F.19 strict schema-v1 edge-policy file把下列 authority 一次 validate 後原子 publish：

- forwarding mode；
- client CA bundle；
- trusted network prefixes；
- per-binding exact DNS SAN identity allowlist。

Configured prefixes禁止 overlap。每條 trusted proxy TLS connection完整 pin 在握手時的 immutable edge snapshot，因此 existing connection不會混用另一 generation 的 header / prefix / identity rules。

### F.20 immediate retirement fence

`-session-login-trusted-proxy-edge-retire-old-connections` 是 F.19 mode 的 optional cutover fence。未啟用時保留 graceful pinned-generation behavior；啟用後，successful real authority publication 才會退休與 current authority不相容的 trusted-proxy connections。Invalid candidate不觸發 retirement，direct/untrusted Client connection也不在集合內。

### F.21 semantic no-op detection

Validated candidate 只有在 effective authority真的變更時才 publish新 edge generation。Revision、JSON/binding order、等價 prefix spelling、DNS case/duplicates、相同 CA PEM 的重排/重複等 representation-only 變更不會製造新 generation，也不會造成 proxy reconnect。

### F.22 binding-aware cutover

Immediate retirement啟用時，global authority變更仍全域 fail closed：

- forwarding mode；
- actual CA root certificate set；
- normalized trusted-prefix topology。

只有 global authority相同、純 identity mapping變更時，才允許 peer-specific selective retirement。

### F.23 authenticated identity-aware preservation

F.23 將同一 binding 的 compatibility 再細化到**握手真正被原 generation 授權的 exact DNS identity**。

TLS verify成功時保存：

```text
normalized leaf DNS SANs
INTERSECT
pinned generation binding allowlist
= bounded matched identity set
```

Identity-only reload後，只要 current peer binding仍允許至少一個原本已授權的 matched identity，old connection可保留原 pinned snapshot；所有原授權 identity都被移除時才 retirement。

Multi-SAN certificate 不會 retroactive promotion：certificate雖可能早已有某個 SAN，但若原握手 generation沒有授權它，之後 policy新增該 SAN 也不能讓 old connection用它續命。Fresh handshake才依 current generation重新取得 authority。

Header mode、CA root set或trusted-prefix topology一變仍沿 F.22 全域 fail closed。Late old-generation handshake使用同一 compatibility rule。

目前剩餘的最細 edge credential gap 是：同一 CA + 同一 exact DNS identity下，尚不能單獨撤銷某一張已外洩 leaf credential / key instance；目前需移除整個 identity或做較大範圍 CA cutover。這是 F.24 的獨立 bounded focus，不代表 F.23 identity authority未完成。

## Security / deployment boundary

目前 bounded production deployment：

- login / public recovery control plane：TLS 1.3；
- trusted game ingress：TLS 1.3；
- Client→Server identity：static trusted credential或issued-session bearer；
- durable account lifecycle：schema-v3/v4，public recovery要求schema-v4；
- recovery delivery：provider-neutral seam + optional HTTPS relay + optional single-host durable outbox；
- login/game TLS certificate：F.16 independent fail-closed runtime generations；
- reverse-proxy source attribution：F.17 bounded HTTP forwarding boundary；
- reverse-proxy upstream identity：F.18 legacy mTLS或F.19 atomic edge-policy mode；
- edge reload：F.21 semantic no-op detection；
- optional immediate proxy cutover：F.20 + F.22/F.23 selective compatibility；
- direct/untrusted Godot Client：永遠不需要 reverse-proxy client certificate；
- realtime secret：fresh per reliable connection generation；
- realtime integrity：Protocol v9 HMAC-SHA256-128；
- realtime confidentiality：**沒有**；
- NAT migration：same-IP source-port only；
- issued bearer：process-local short-lived proof；
- account backend：目前仍是 single-writer durable JSON；
- recovery outbox：single-host bounded durability，不是 distributed broker；
- multi-server distributed ownership / CAS / failover：尚未實作。

尚未加入的獨立 decision gates包括：distributed rate limit / IP reputation / WAF/CDN、PROXY protocol、CRL/OCSP、ACME/PKI automation、Client mTLS enrollment、public registration、MFA/WebAuthn/passkeys/OIDC、refresh-token / remember-session、distributed account DB、multi-host edge coordination，以及 Protocol v10。

## Current contract 與歷史 stage 文件

本文件與 root `README.md` 是 current architecture 的主要入口。

目前 account / recovery / TLS / edge security 的直接 current references：

- `S4F23_AUTHENTICATED_PROXY_IDENTITY_RETIREMENT.md`
- `S4F22_BINDING_AWARE_TRUSTED_PROXY_RETIREMENT.md`
- `S4F21_EDGE_POLICY_NOOP_RELOAD.md`
- `S4F20_TRUSTED_PROXY_CONNECTION_REVOCATION.md`
- `S4F19_TRUSTED_PROXY_EDGE_POLICY_RUNTIME_RELOAD.md`
- `S4F18_TRUSTED_PROXY_MTLS_EDGE_IDENTITY.md`
- `S4F17_TRUSTED_PROXY_SOURCE_ATTRIBUTION.md`
- `S4F16_TLS_CERTIFICATE_RUNTIME_RELOAD.md`
- `S4F15_DURABLE_RECOVERY_DELIVERY_OUTBOX.md`
- `S4F14_RECOVERY_DELIVERY_RUNTIME_RELOAD.md`
- `S4F13_PRODUCTION_RECOVERY_DELIVERY_PROVIDER.md`
- `S4F12_VERIFIED_RECOVERY_DELIVERY.md`
- `S4F10_VERIFIED_RECOVERY_PUBLIC_RESET.md`
- `S4F9_ACCOUNT_RECOVERY_KDF_MIGRATION.md`
- `S4F7_DURABLE_ACCOUNT_LIFECYCLE.md`

以下文件刻意保留當時的 milestone contract，因此看到 Protocol v1/v2/v3/v6/v8、raw realtime bearer token、Gameplay World schema v1、Full AOI snapshot 或較早 deployment assumptions 時，應視為**歷史證據**而不是目前 production contract：

- `S2_PROTOCOL.md`
- `S2B_TRANSPORT.md`
- `S3_GAMEPLAY_WORLD.md`
- `S3C6_REALTIME_REPLICATION.md`
- 其他以 S1/S2/S3/早期 S4 stage 命名的 milestone 文件

Realtime / trusted-ingress基線仍可參考：

- `S4E4_SECURE_TRUSTED_INGRESS_TAKEOVER.md`
- `S4E5_AUTHENTICATED_REALTIME_BINDING.md`
- `S4E6_REALTIME_KEY_LIFECYCLE.md`
- `S4E7_WAN_NAT_SECURE_DEPLOYMENT_READINESS.md`

Astrahold 的架構原則保持不變：**Server State 是真相；先證明 correctness，再用量測決定下一層複雜度。**

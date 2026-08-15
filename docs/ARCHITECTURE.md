# Astrahold Server Current Architecture Baseline

> **Current production contract.** 本文件描述截至 **S4-E.7 — WAN / NAT & Secure Deployment Readiness E2E** 的現行架構。早期 `S2*`、`S3*` stage 文件保留當時的 Protocol / Transport / Gameplay World contract 作為歷史紀錄；若內容與本文件或 `README.md` 衝突，以目前 production contract 為準。

## 核心決策

Astrahold Server 是全新 Go authoritative MMORPG Server Core。舊系統只能作為經驗參考，不能把舊 protocol、2D position model 或私服相容層當成 Astrahold 的架構母體。

目前核心原則：

1. **Server State 是 gameplay truth。** Client 只送 intent，不送 position truth、damage、HP、team、winner 或 ownership。
2. **單一 World owner。** Network、DB、GM、管理 API 不直接修改 mutable world state，只能 enqueue bounded command。
3. **World tick 不做 blocking I/O。** Network / persistence 都在 owner loop 外處理。
4. **Gameplay Proxy 是權威契約。** Visual Mesh 永遠不能成為 Navigation、LOS、Gate 或 objective truth。
5. **Snapshot absence != despawn。** Entity lifecycle 只相信 Reliable Spawn / Despawn。
6. **Measure → Profile → Optimize。** 沒有 profiling 證據前，不拆 world actor、不做激進 quantization / delta compression，也不因技術偏好替換 transport。

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

## Protocol v9

目前 wire contract 是 **Protocol v9**。

### ASTR frame

Gameplay envelope 仍使用固定 28-byte ASTR frame header：

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

Production trusted bootstrap 可使用同 process **TLS 1.3 ingress**。TLS terminator 只允許轉發到 literal-loopback backend；trusted bearer credential 不應暴露在 Internet-facing plaintext listener。

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

S4-E.7 已把 same-IP NAT-like source-port migration 做成 production E2E gate。

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

Development path 可使用 ephemeral identity；production trusted path 使用 Server-owned credential map 將 opaque credential 綁到 trusted `CharacterID`。

Client 不提交：

- CharacterID
- ownership epoch
- takeover permission
- team
- HP / damage
- restore transform

Active takeover authority 只存在 Server credential policy。Ownership transfer 使用 exact fence / epoch CAS；成功 takeover 保留 authoritative EntityID / live state，advance ownership epoch，retire old peer，再建立 replacement connection generation。

每個新 reliable connection 都取得新的 realtime secret / public route generation。Old peer retirement 立即撤銷該 exact generation 的 token / route lookup；stale close 不能刪除 replacement generation。

目前沒有 periodic in-session realtime rekey。若 secret-lifetime policy 要求比 connection lifetime 更短，才加入有 acknowledgement / overlap semantics 的 rotation。

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

Restore truth 來自 durable Server state；Client 不提供 HP、position 或 revision。S4-E.3 production E2E 已用真實 `cmd/worldd` + Godot Client 驗證 HP / position autosave、SIGKILL、restart、fresh reconnect restore。

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

`WorldSnapshot` 是 chunked update batch，不再代表 Full AOI entity list。Client 只有在同 tick chunk set 完整時才提交該 batch；缺少某 Entity transform 不代表 lifecycle change。

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

## Security / deployment boundary

目前 bounded production deployment：

- trusted reliable bootstrap：TLS 1.3；
- trusted plaintext backend：literal loopback only；
- trusted credential map：Server static SHA-256 config；
- active takeover policy：Server-owned credential entry；
- realtime secret：fresh per reliable connection generation；
- realtime integrity：Protocol v9 HMAC-SHA256-128；
- realtime confidentiality：**沒有**；
- NAT migration：same-IP source-port only；
- TLS certificate：process start 載入，尚無 hot reload；
- multi-server distributed ownership / CAS / failover：尚未實作。

## Current contract 與歷史 stage 文件

本文件與 root `README.md` 是 current architecture 的主要入口。

以下文件刻意保留當時的 milestone contract，因此看到 Protocol v1/v2/v3/v6/v8、raw realtime bearer token、Gameplay World schema v1、Full AOI snapshot 等敘述時，應視為**歷史證據**而不是目前 production contract：

- `S2_PROTOCOL.md`
- `S2B_TRANSPORT.md`
- `S3_GAMEPLAY_WORLD.md`
- `S3C6_REALTIME_REPLICATION.md`
- 其他以 S1/S2/S3/S4 stage 命名的 milestone 文件

目前 realtime / deployment security 的直接參考：

- `S4E4_SECURE_TRUSTED_INGRESS_TAKEOVER.md`
- `S4E5_AUTHENTICATED_REALTIME_BINDING.md`
- `S4E6_REALTIME_KEY_LIFECYCLE.md`
- `S4E7_WAN_NAT_SECURE_DEPLOYMENT_READINESS.md`

Astrahold 的架構原則保持不變：**Server State 是真相；先證明 correctness，再用量測決定下一層複雜度。**

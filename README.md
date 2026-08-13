# Astrahold Server

Astrahold 的全新權威 MMORPG Server Core。

> 目標不是替舊天堂私服換名字，而是保留已驗證的 MMO domain 經驗，重新建立適合 **3D 王城、多人攻城、長期商用開發與可量測擴充** 的底層。

## 現階段狀態

目前已完成 **S3-D Gate Siege Interaction**。

```text
World Position (XYZ + Layer)
        ↓
Spatial / AOI
        ↓
Authoritative Movement
        ↓
Bounded Command Queue
        ↓
Fixed 20 Hz World Loop
        ↓
Session / Input + Action Sequence
        ↓
Replication
        ↓
Protocol v4 + World Identity
        ↓
TCP Reliable + UDP Realtime
        ↓
GameV1 Compact Realtime Codec
        ↓
MTU-safe Snapshot Chunks
        ↓
Gameplay World schema v2
        ↓
Ground L0 → Ramp L1 → Wall L2
        ↓
Gate Siege Domain
        ↓
Attack Intent → HP → Destroyed → Blocker Open
```

S3-C.5 / S3-C.6 已先建立 Siege Load Lab 並修掉第一個明確 scaling blocker：24 人 Full AOI JSON Snapshot 就會超過 1200-byte UDP budget。Protocol v3 將高頻 Move / Snapshot / Correction 改為 compact binary 與 MTU-safe chunks，100-client Gate Zerg 的 `datagram_too_large` 已由 8000 次降為 0，Server 8 秒 allocation 約下降 47%。

S3-D 再把第一個真正 Siege gameplay loop 接進同一套權威架構：Client 只送 `ClientAttackGate(gate_id)`，Server 驗證 Layer / Range / LOS / Cooldown，成功才扣 Gate HP；HP=0 時在同一個 world-owner tick 關閉 Gate blocker，並以 Reliable `WorldDynamicState` 同步 HP / Destroyed / blocker state。

下一個 gameplay 基礎是 **S3-D.1 Combat Action / Damage Foundation**；Replication Tier / dirty / delta 等大規模 fan-out 優化仍留在 S3-E。

## 與 Myriad Throne 的關係

`myriad-throne-server` 是參考來源，不是 Astrahold 的基底。

舊 Lineage protocol、2D `gx/gy`、舊地圖格式與私服相容包袱不直接搬入。角色、道具、技能、Buff、Party、Guild、持久化等 domain 經驗，之後只在符合 Astrahold 新架構時逐項移植。

## 架構文件

- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)
- [`docs/S1_RUNTIME.md`](docs/S1_RUNTIME.md)
- [`docs/S2_PROTOCOL.md`](docs/S2_PROTOCOL.md)
- [`docs/S2B_TRANSPORT.md`](docs/S2B_TRANSPORT.md)
- [`docs/S3_GAMEPLAY_WORLD.md`](docs/S3_GAMEPLAY_WORLD.md)
- [`docs/S3B_WORLD_HANDSHAKE.md`](docs/S3B_WORLD_HANDSHAKE.md)
- [`docs/S3C_CASTLE_BLOCKOUT.md`](docs/S3C_CASTLE_BLOCKOUT.md)
- [`docs/S3C5_SIEGE_LOAD_LAB.md`](docs/S3C5_SIEGE_LOAD_LAB.md)
- [`docs/S3C6_REALTIME_REPLICATION.md`](docs/S3C6_REALTIME_REPLICATION.md)
- [`docs/S3D_GATE_SIEGE.md`](docs/S3D_GATE_SIEGE.md)

## 世界模型

Astrahold 的權威位置：

```go
type Position struct {
    X     float32
    Y     float32
    Z     float32
    Layer LayerID
}
```

- `X/Z`：水平世界位置
- `Y`：高度
- `Layer`：邏輯樓層／拓樸層

Grid 只作為 Spatial / AOI acceleration structure，不是 gameplay position。這讓城牆、坡道、樓梯、地下層與重疊表面不必被壓回 2D Grid。

## Runtime 不變量

Astrahold 維持兩條核心規則：

1. Network、DB、GM、管理介面不得直接修改 World mutable state，只能提交 bounded Command Queue。
2. World Tick 不得直接做 blocking socket I/O，只能將 outbound message 送入非阻塞 Connection / Outbox。

每個 World / Zone 目前維持 **單一 simulation owner goroutine**，以換取 deterministic ordering、清楚 ownership 與容易驗證的 gameplay state。

S3-C.5 / S3-C.6 的 100-client Gate Zerg 中，20Hz Tick p99 約 8～9ms，仍遠低於 50ms budget，因此目前沒有數據支持拆 Cell Actor。

若未來 single owner 成為實際瓶頸，平行化優先序：

```text
① Read-only validation jobs
② Navigation / LOS batch jobs
③ AOI / replication build workers
④ Encode / network fan-out workers
⑤ 最後才拆 mutable state ownership
```

## Server Authoritative Movement

Client 只送方向；時間推進與位置裁定由 Server fixed tick 決定。

```text
ClientMoveInput
(direction)
        ↓
Envelope.Sequence
        ↓
Gateway / Session validation
        ↓
Command Queue
        ↓
Server Fixed Tick
        ↓
Movement + Navigation
        ↓
Authoritative XYZ + Layer
        ↓
Snapshot + PositionCorrection
```

Movement input sequence 的唯一來源是 Frame / Envelope，不在 payload 重複保存。

## Gameplay Action Sequence

S3-D 起，Movement 與低頻 gameplay action 是不同 semantic stream：

```text
UDP Move
→ Session input sequence

TCP Gate Attack
→ Session action sequence
```

Gate attack 的 sequence 表示「這個 intent 已處理」，不是「一定成功造成傷害」。即使因距離、Layer、LOS 或 cooldown 被拒絕，同一 action sequence 也不可重播。

## Gameplay World / Shared Proxy

Server 啟動時 strict load / validate `gameplay.json`。

目前 `castle-sandbox@s3d-001` 使用 **schema v2**：

```text
Gameplay World
├── Surface
├── Portal
├── Blocker
├── Gate
│   └── blocker_id
├── Agent defaults
├── world_id
├── revision
└── raw SHA-256
```

Client 收到 `SessionWelcome` 後，必須先驗證本地 `gameplay.json` 的 `world_id / revision / SHA-256`，成功後才可啟用 realtime UDP。

目前 Layer：

```text
Layer 0 = Siege Field / Ground / Courtyard
Layer 1 = West / East Ramp
Layer 2 = Front Wall Walk (Y = 8m)
```

Portal 是世界拓樸契約；Blocker 是 Navigation / LOS proxy；Gate 是 Siege domain state。Gate 只透過 `blocker_id` 引用 blocker，不把 HP 塞進 Navigation。

未來 World Compiler 由單一 canonical world source 同時輸出 Server Gameplay Proxy 與 Godot 對應資料。

## S3-D Gate Siege Domain

S3-D prototype `main-gate`：

```text
Max HP            1000
Attack range       4.5 m
Prototype damage   100
Cooldown           0.5 s
```

這些數值由 Server Gameplay World 載入。Client 不提供 damage、range、cooldown 或 destroyed 判定。

權威驗證順序：

```text
ClientAttackGate(gate_id)
        ↓
Reliable Gateway
        ↓
Bounded Command Queue
        ↓
Session action sequence
        ↓
Authoritative player Position
        ↓
Gate exists / alive
        ↓
Layer
        ↓
Range to nearest Gate blocker bounds
        ↓
Blocker enabled
        ↓
Server LOS
        ↓
Cooldown
        ↓
Server-side damage
```

普通 LOS 會被關閉的 Gate blocker 擋住；攻擊 Gate 本體時只忽略**目標 Gate blocker 自己**，其他 blocker 仍照常遮蔽。

致命一擊在同一個 world-owner tick 內完成：

```text
Gate HP → 0
    ↓
SetBlockerEnabled(main-gate, false)
    ↓
commit Gate HP = 0 / Destroyed
    ↓
Dynamic revision++
    ↓
Reliable WorldDynamicState
```

Blocker disable 失敗時不提交 HP=0，避免 Siege state 與 Navigation state 分裂。

### Gameplay rejection != Server fault

以下屬於正常 gameplay rejection：

- unknown Gate
- Gate 已摧毀
- Layer 不符
- 超出距離
- LOS 被其他 blocker 阻擋
- cooldown 尚未結束

這些記錄在 `ActionRejections`，不污染 `CommandErrors`。Session / Entity 遺失、Siege 未配置、Gate / blocker state 不一致等才屬 runtime/configuration fault。

完整規約：[`docs/S3D_GATE_SIEGE.md`](docs/S3D_GATE_SIEGE.md)。

## Protocol v4 / Transport

```text
Gameplay Message
        ↓
protocol.Envelope
        ↓
PayloadCodec
        ↓
Astrahold Frame (ASTR)
        ↓
Transport Adapter
```

任何 wire-incompatible contract 變更都必須升 Protocol Version；錯版 Client 在 Frame 邊界直接拒絕。

目前為 **Protocol v4**：

```text
ReliableOrdered / TCP
→ SessionWelcome
→ Spawn / Despawn
→ WorldDynamicState
→ ClientAttackGate
→ 其他低頻重要事件
→ JSON bridge（開發期）

Realtime / UDP
→ ClientMoveInput
→ WorldSnapshot
→ PositionCorrection
→ GameV1 compact binary
```

Protocol v4 沒有改動 v3 的 realtime compact payload；它新增 Gate Siege reliable control contract 與 Gate dynamic state。

開發 Transport 的 TCP 尚未使用 TLS，預設只綁 `127.0.0.1`；這不是 Internet-facing security boundary。

### MTU-safe Snapshot

Realtime datagram 上限維持 **1200 bytes**，不依賴 IP fragmentation。

```text
Snapshot header            14 bytes
Compact transform          26 bytes
Max entities / chunk       43
ASTU + ASTR + payload    1184 bytes max
```

Client 只有在同一 `Tick` 的所有 chunks 收齊後，才可提交完整 Snapshot。

### Realtime semantic streams

```text
Realtime Mailbox
├── latest PositionCorrection
└── current WorldSnapshot set
```

不同 realtime message type 不使用單一全域 sequence 判 stale：

```text
Snapshot
→ Tick + ChunkIndex / ChunkCount

PositionCorrection
→ authoritative Tick + LastProcessedInputSequence

ClientMoveInput ingress
→ Session input sequence
```

完整 contract：[`docs/S3C6_REALTIME_REPLICATION.md`](docs/S3C6_REALTIME_REPLICATION.md)。

## Siege Load Lab

Load Lab 使用兩個獨立 process，且 Bot 走真實 TCP / UDP / Protocol：

```text
cmd/loadserver
    │
    │ Astrahold network path
    │
cmd/loadbot
```

支援：

```text
distributed
→ 一般 AOI

gate-zerg
→ Main Gate hotspot / fan-out

vertical-siege
→ L0 + L1 + L2 verticality
```

### 24-client Vertical Siege：S3-C.5 → S3-C.6

| 指標 | S3-C.5 | S3-C.6 |
|---|---:|---:|
| Tick p99 | 0.802 ms | 0.805 ms |
| TotalAlloc / 5s | 18,587,552 B | **10,352,952 B** |
| Mallocs | 78,985 | **40,657** |
| GC | 5 | 4 |
| `datagram_too_large` | **1,200** | **0** |
| Completed snapshots | N/A | **1,571** |
| Bot decode/network error | 0 / 0 | **0 / 0** |

### 100-client Gate Zerg：S3-C.5 → S3-C.6

| 指標 | S3-C.5 | S3-C.6 |
|---|---:|---:|
| Tick average | 2.991 ms | **2.068 ms** |
| Tick p99 | 8.979 ms | **8.292 ms** |
| AOI avg / Tick | 1.412 ms | **0.975 ms** |
| Replication Build / Tick | 1.397 ms | **0.906 ms** |
| TotalAlloc / 8s | 339,939,416 B | **179,654,016 B** |
| Mallocs | 534,712 | **372,872** |
| GC | 160 | **80** |
| `datagram_too_large` | **8,000** | **0** |
| Snapshot chunks received | 幾乎無 steady-state | **28,965** |
| Completed snapshots | 幾乎無 steady-state | **9,970** |
| Bot decode/network error | 0 / 0 | **0 / 0** |

S3-C.6 解決的是 **MTU correctness**，不是 500 人 Full AOI bandwidth。S3-D 的 24 / 100 Load Lab regression 必須繼續維持全綠。

### Regression Gate

```text
ready_clients == requested_clients
completed_snapshots > 0
bot decode_errors == 0
bot network_errors == 0
server datagram_too_large == 0
unexpected_tick_errors == 0
delivery_errors == 0
server network_errors == 0
```

GitHub hosted runner 用於 regression / scaling curve；正式 capacity 必須在固定硬體與固定 network topology 的 dedicated environment 重跑。

## 規模化演進原則

Astrahold 採 **Measure → Profile → Optimize**。

### Hot Path Allocation

```text
profile / metrics
        ↓
定位 hotspot
        ↓
減少資料量 / dirty tracking
        ↓
preallocate / scratch reuse
        ↓
必要時才 sync.Pool
```

### Replication Tier / Network LOD

```text
Tier 0 — Self / Target / Boss / Critical Objective
Tier 1 — Near Crowd
Tier 2 — Mid Crowd
Tier 3 — Far / Peripheral Crowd
```

Server Network LOD 應盡量對齊 Client Presentation LOD / VAT：

```text
Tier 0 → Skeletal LOD0
Tier 1 → Skeletal / LOD1
Tier 2 → VAT Crowd
Tier 3 → Low-rate VAT / Impostor
```

### AOI ViewList / Dirty Tracking

```text
Spatial enter / leave
        ↓
Session ViewList
        ↓
Dirty Entity tracking
        ↓
只複寫必要更新
```

### Shared Serialization

不假設所有 Client 共用整張 Snapshot，而是重用較小單位：

```text
Entity Update Block
或 Spatial Cell / Tier Chunk
        ↓
serialize once
        ↓
多 observer 組合／複用
```

### Layer-aware Spatial Bucket

可演進為：

```text
SpatialKey
├── CellX
├── CellZ
└── Layer
```

但 Gate Zerg 同 Layer 全互見時 candidate/visible = 1.0，因此它不是目前 hotspot 的第一解法。

## 目前目錄

```text
astrahold-server/
├── cmd/
│   ├── worldd/
│   ├── loadserver/
│   └── loadbot/
├── internal/
│   ├── world/
│   ├── spatial/
│   ├── navigation/
│   ├── movement/
│   ├── simulation/
│   ├── protocol/
│   ├── codec/
│   │   ├── jsonv1/
│   │   └── gamev1/
│   ├── transport/
│   ├── gateway/
│   ├── session/
│   ├── replication/
│   ├── siege/
│   ├── worldruntime/
│   ├── loadlab/
│   └── netadapter/tcpudp/
├── worlds/
│   └── castle-sandbox/
└── docs/
```

## Roadmap

### S0 — World Core ✅

- XYZ + Layer
- Spatial / AOI
- Authoritative Movement
- Navigation abstraction

### S1 — World Runtime ✅

- Fixed Tick
- bounded Command Queue
- Session / sequence
- Replication baseline

### S2 — Thin Client / Transport ✅

- TCP Reliable + UDP Realtime
- SessionWelcome / Token
- Astrahold Frame
- Godot C# Thin Client
- Snapshot interpolation / soft reconciliation

### S3-A ～ S3-C — Gameplay World / Castle Verticality ✅

- shared `gameplay.json`
- World Identity / SHA-256
- Surface / Portal / Blocker
- L0 Ground → L1 Ramp → L2 Wall
- Castle Front Siege Blockout

### S3-C.5 — Siege Load Lab ✅

- 真 TCP/UDP Headless Bot
- 24-client Vertical Siege
- 100-client Gate Zerg
- Tick / AOI / Queue / allocation / GC metrics

### S3-C.6 — Realtime Replication Foundation ✅

- Protocol v3 realtime compact binary
- 1184-byte MTU-safe Snapshot chunks
- Snapshot / Correction semantic mailbox
- Client complete-snapshot assembly contract
- Load Lab regression gate

### S3-D — Gate Siege Interaction ✅

- Protocol v4
- Gameplay World schema v2 / Gate definition
- Gate HP / State
- Reliable `ClientAttackGate`
- Server Layer / Range / LOS / cooldown validation
- HP=0 → `main-gate` blocker disabled
- Reliable Gate HP / Destroyed dynamic state
- Godot Gate HP / destroyed debug presentation
- 24 / 100 Load Lab regression

### S3-D.1 — Combat Action / Damage Foundation（下一步）

- [ ] 共用 Combat Action intent
- [ ] authoritative action timing / cooldown seam
- [ ] Damage source / result model
- [ ] Gate prototype damage 改走共用 Damage pipeline
- [ ] Character target 與 Siege objective 共用 validation seam
- [ ] 保留 Gate / Navigation ownership 分離

### S3-E — Siege Replication Scaling

- [ ] Replication Tier / Network LOD
- [ ] AOI ViewList / Dirty Tracking
- [ ] Quantized / Delta updates
- [ ] Shared Entity / Cell update blocks
- [ ] cadence / bandwidth budget
- [ ] dedicated 500+ regression

### S4 — Full Siege Mechanics

- [ ] Attacker / Defender
- [ ] Gate / Throne / Crown
- [ ] Zone occupation
- [ ] Siege State Machine
- [ ] Guild objective ownership

### S5 — Full Combat Stress

- [ ] Dedicated environment 500+ clients
- [ ] Skills / Damage / Buff / VFX event load
- [ ] 固定硬體正式 capacity baseline
- [ ] p99 / GC / network / CPU / memory regression gates

## 開發

正常 Server：

```bash
go test ./...
go vet ./...
go run ./cmd/worldd
```

Load Lab：

```bash
# Terminal A
go run ./cmd/loadserver \
  -clients 100 \
  -scenario gate-zerg \
  -duration 30s

# Terminal B
go run ./cmd/loadbot \
  -clients 100 \
  -scenario gate-zerg \
  -input-rate 20 \
  -ramp-up 2s \
  -duration 33s
```

預設開發 Server：

```text
TCP 127.0.0.1:7777
UDP 127.0.0.1:7778
World 20 Hz
Snapshot 10 Hz
Protocol v4
Gameplay World castle-sandbox@s3d-001
```

目前核心盡量只使用 Go 標準函式庫；效能改動必須由 Load Lab / profiling 數據驅動。

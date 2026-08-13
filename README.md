# Astrahold Server

Astrahold 的全新權威 MMORPG Server Core。

> 目標不是替舊天堂私服換名字，而是保留已驗證的 MMO domain 經驗，重新建立適合 **3D 王城、多人攻城、長期商用開發與可量測擴充** 的底層。

## 現階段狀態

目前主線已推進到 **S3-C.6 Realtime Replication Foundation**。

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
Session / Input Sequence
        ↓
Replication
        ↓
Protocol v3 + World Identity
        ↓
TCP Reliable + UDP Realtime
        ↓
GameV1 Compact Realtime Codec
        ↓
MTU-safe Snapshot Chunks
        ↓
Godot Runtime E2E
        ↓
Gameplay World / Dynamic Blocker
        ↓
Ground L0 → Ramp L1 → Wall L2
        ↓
Castle Front Siege Blockout
        ↓
Siege Load Lab 24 / 100 Clients
```

S3-C.5 已用真實 TCP/UDP Headless Bot 證明第一個 scaling blocker 不是 single World owner，而是 **Full AOI + JSON Snapshot**：24 人全互見時就超過 1200-byte UDP budget。

S3-C.6 因此升級 Protocol v3，將高頻 Realtime Move / Snapshot / Correction 改為 compact binary，並將 Snapshot 切成 MTU-safe chunks。100-client Gate Zerg 實測已由 `8000` 次 `datagram_too_large` 降為 **0**，同時 Server 8 秒 allocation 約下降 **47%**。

下一個 gameplay milestone 是 **S3-D Gate HP / Attack / Destroy**。

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

## 世界模型

Astrahold 的權威位置從一開始就是：

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

因此可以表達城牆、坡道、樓梯、地下層、橋面與高低差，而不把 3D 世界壓回 2D Grid。

Grid 只作為 Spatial / AOI acceleration structure，不是 gameplay position。

## Runtime 不變量

Astrahold 維持兩條核心規則：

1. Network、DB、GM、管理介面不得直接修改 World mutable state，只能提交 bounded Command Queue。
2. World Tick 不得直接做 blocking socket I/O，只能將 outbound message 送入非阻塞 Connection / Outbox。

每個 World / Zone 目前維持 **單一 simulation owner goroutine**，以換取 deterministic ordering、清楚 ownership 與容易驗證的 gameplay state。

S3-C.5 / S3-C.6 的 100-client Gate Zerg 中，20Hz Tick p99 約 8～9ms，仍遠低於 50ms budget，因此目前沒有數據支持拆 Cell Actor。

若未來 single owner 成為實際瓶頸，平行化優先序為：

```text
① Read-only validation jobs
② Navigation / LOS batch jobs
③ AOI / replication build workers
④ Encode / network fan-out workers
⑤ 最後才拆 mutable state ownership
```

## Server Authoritative Movement

Client 只送方向與 Session-scoped input sequence；時間推進由 Server fixed tick 決定。

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

Input sequence 的唯一來源是 Frame / Envelope，不在 payload 重複保存。

## Gameplay World / Shared Proxy

Server 啟動時 strict load / validate `gameplay.json`：

```text
Gameplay World
├── Surface
├── Portal
├── Blocker
├── Agent defaults
├── world_id
├── revision
└── raw SHA-256
```

Client 收到 `SessionWelcome` 後，必須先驗證本地 `gameplay.json` 的 `world_id / revision / SHA-256`，成功後才可啟用 realtime UDP。

目前 `castle-sandbox@s3c-001`：

```text
Layer 0 = Siege Field / Ground / Courtyard
Layer 1 = West / East Ramp
Layer 2 = Front Wall Walk (Y = 8m)
```

Portal 是世界拓樸契約；Gate / Wall 由 dynamic blocker 控制 Movement 與 LOS。

未來 World Compiler 由單一 canonical world source 同時輸出 Server Gameplay Proxy 與 Godot 對應資料，避免 Client / Server 各自維護世界規則。

## Protocol v3 / Transport

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

目前為 **Protocol v3**：

```text
ReliableOrdered
→ TCP
→ SessionWelcome / Spawn / Despawn / WorldDynamicState / 重要事件
→ JSON bridge（開發期）

Realtime
→ UDP
→ ClientMoveInput / WorldSnapshot / PositionCorrection
→ GameV1 compact binary
```

開發 Transport 的 TCP 尚未使用 TLS，預設只綁 `127.0.0.1`；這不是 Internet-facing security boundary。

### MTU-safe Snapshot

Realtime datagram 上限維持 **1200 bytes**，不依賴 IP fragmentation。

Protocol v3：

```text
Snapshot header            14 bytes
Compact transform          26 bytes
Max entities / chunk       43
ASTU + ASTR + payload    1184 bytes max
```

100 個可見 Entity 會拆成：

```text
43 + 43 + 14
```

Client 只有在同一 `Tick` 的所有 chunks 收齊後，才可提交完整 Snapshot。

### Realtime semantic streams

Realtime 不再是一個全域 latest-state slot：

```text
Realtime Mailbox
├── latest PositionCorrection
└── current WorldSnapshot set
```

Correction 可以優先送出，即使它的 `Envelope.Sequence` 大於尚未送出的 Snapshot chunk。因此不同 realtime message type **不能用單一全域 sequence 判 stale**。

Freshness 規則：

```text
Snapshot
→ Tick + ChunkIndex / ChunkCount

PositionCorrection
→ authoritative Tick + LastProcessedInputSequence

ClientMoveInput ingress
→ Session input sequence
```

完整 contract：[`docs/S3C6_REALTIME_REPLICATION.md`](docs/S3C6_REALTIME_REPLICATION.md)。

## S3-C.5 / S3-C.6 Siege Load Lab

Load Lab 使用兩個獨立 process：

```text
cmd/loadserver
    │
    │ 真 TCP / UDP / Protocol v3
    │
cmd/loadbot
```

Bot 完整走：

```text
TCP connect
→ SessionWelcome
→ World Identity
→ Realtime Token
→ UDP Move
→ Gateway
→ Command Queue
→ World Tick
→ AOI / Replication
→ UDP Snapshot / Correction
→ Bot decode / Snapshot assembly
```

Server 與 Bot 分進程，避免 bot allocation 污染 Server `runtime.MemStats`。

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

24 人 Tick latency 約持平，但 allocation 約下降 44%，而 Snapshot 已可正常送達。

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

S3-C.6 已解決 **MTU correctness**，但沒有宣稱 500 人 Full AOI bandwidth 已解決。

500 人全互見、10Hz、每 transform 26 bytes 時，光 raw transform payload 粗估就約 65MB/s，尚未計入 frame/datagram overhead。因此 S3-E 仍需要 Replication Tier、dirty/delta、cadence 與 shared block reuse。

## Load Lab Regression Gate

24-client 與 100-client CI 現在必須同時滿足：

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

`internal/codec/**`、`internal/protocol/**`、`internal/replication/**`、Runtime / Transport 等變更都會觸發 Siege Load Lab。

GitHub hosted runner 只用於 regression / scaling curve；正式 capacity 必須在固定硬體與固定 network topology 的 dedicated environment 重跑。

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

S3-C.6 已先導入 per-session view set 雙 buffer與避免不必要排序 copy；不把 zero-allocation 當成全專案宗教。

### Replication Tier / Network LOD

後續攻城不要求所有 Entity 使用相同 cadence / precision：

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
- 第一個 scaling blocker 定位

### S3-C.6 — Realtime Replication Foundation ✅

- Protocol v3
- GameV1 compact realtime codec
- 1184-byte MTU-safe Snapshot chunks
- Snapshot / Correction semantic mailbox
- Client complete-snapshot assembly contract
- per-session view set reuse
- 24 / 100 人 `datagram_too_large = 0`
- Load Lab regression gate

### S3-D — Gate Siege Interaction（下一步）

- [ ] Gate Entity / HP / State
- [ ] Attack Gate command
- [ ] Server 驗距離 / Layer / LOS / cooldown
- [ ] HP=0 → 關閉 `main-gate` blocker
- [ ] Godot Gate HP / destroyed presentation
- [ ] Ground → Gate → Courtyard → Ramp → Wall 最小攻城 E2E

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
Protocol v3
```

目前核心盡量只使用 Go 標準函式庫；效能改動必須由 Load Lab / profiling 數據驅動。

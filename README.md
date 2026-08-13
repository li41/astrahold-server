# Astrahold Server

Astrahold 的全新權威 MMORPG 伺服器核心。

> 目標不是把天堂私服 Server 換名字，而是保留已驗證的 MMO 經驗，重新建立適合 **3D 王城、多人攻城、長期商用開發與可量測擴充** 的底層。

## 現階段狀態

目前已完成 **S3-C.5 Siege Load Lab**：除了 Godot Thin Client、XYZ + Layer、Gameplay World、Castle Front Siege Blockout 之外，Server 現在也有走真實 TCP/UDP Protocol 的 headless 負載測試工具與可比較的 Tick / AOI / Replication / GC 指標。

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
Session / Sequence
        ↓
Replication
        ↓
Protocol v2 + World Identity
        ↓
TCP Reliable + UDP Realtime
        ↓
Godot Runtime E2E
        ↓
Gameplay World / Dynamic Blocker
        ↓
Ground L0 → Ramp L1 → Wall L2
        ↓
Castle Front Siege Blockout
        ↓
Siege Load Lab
```

S3-C.5 的第一輪數據已經定位出第一個真正的 scaling blocker：**Full AOI + JSON v1 `WorldSnapshot` 在 24 人全互見時就會超過 1200-byte UDP payload budget**。因此下一階段不是拆 Cell Actor，而是 **S3-C.6 Realtime Replication Foundation**。

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

因此可以表達城牆、樓梯、地下層、橋面與高低差，而不需要把 3D 世界硬壓回 2D Grid。

Grid 只作為 Spatial / AOI acceleration structure，不是 gameplay position。

## Runtime 不變量

Astrahold 維持兩條核心規則：

1. Network、DB、GM、管理介面不得直接修改 World mutable state，只能透過 bounded Command Queue。
2. World Tick 不得直接做 blocking socket I/O，只能把 outbound message 送到非阻塞 Connection / Outbox。

目前每個 World / Zone 仍採 **單一 simulation owner goroutine**。這是刻意的基線：先換取 deterministic ordering、容易測試與清楚 ownership，避免 Combat、Skill、NPC AI、Siege 一開始就充滿 mutex、cross-cell transaction 與 ordering 問題。

S3-C.5 的 100-client Gate Zerg 實測中，20 Hz Tick 的 p99 約 **9.9ms**，仍遠低於 50ms budget，因此目前沒有證據支持把 mutable world state 拆成 Cell Actor。

若未來單 owner 真正成為瓶頸，演進優先順序仍是：

```text
① Read-only validation jobs
② Navigation / LOS batch jobs
③ AOI / replication build workers
④ Encode / network fan-out workers
⑤ 最後才拆 mutable state ownership
```

## Server Authoritative Movement

Client 只送方向與 **Session-scoped input sequence**；時間推進完全由 Server fixed tick 決定。

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

Sequence 只有 Frame / Envelope 是唯一真相來源，不重複塞進 payload。

## Gameplay World / Shared Proxy

Server 啟動時會 strict load / validate `gameplay.json`，並把它視為 Gameplay World contract：

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

Client 收到 `SessionWelcome` 後，必須先驗證本地 `gameplay.json` 的 `world_id / revision / SHA-256`，驗證成功才允許啟用 realtime UDP。

目前 `castle-sandbox@s3c-001`：

```text
Layer 0 = Siege Field / Ground / Courtyard
Layer 1 = West / East Ramp
Layer 2 = Front Wall Walk (Y = 8m)
```

Portal 是世界拓樸契約；Gate / Wall 則由 dynamic blocker 控制 Movement 與 LOS。

未來 World Compiler 應由單一 canonical world source 同時輸出 Server Gameplay Proxy 與 Godot 對應資料，避免 Client / Server 各自手工維護世界規則。

## Protocol / Transport 分層

```text
Gameplay Message
        ↓
protocol.Envelope
        ↓
PayloadCodec
        ↓
Astrahold Frame
        ↓
Transport Adapter
```

目前使用 **Protocol v2**。任何不相容的 wire contract 變更都必須升 Protocol Version，錯版 Client 在 Frame 邊界直接拒絕。

開發期 Transport：

```text
ReliableOrdered
→ TCP
→ SessionWelcome / Spawn / Despawn / Dynamic World / 重要事件

RealtimeSequenced
→ UDP
→ Move / Snapshot / PositionCorrection
```

JSON v1 是 Thin Client / bootstrap 的開發 Codec，不再被視為未來高頻 Crowd Transform 的正式格式。

Realtime datagram 維持 **1200 bytes** 上限，不為壓測放寬，也不依賴 IP fragmentation。

> 開發 Transport 的 TCP 目前沒有 TLS，預設只綁 `127.0.0.1`。這是本機／受控環境 Transport，不可直接當成 Internet-facing security boundary。

## S3-C.5 Siege Load Lab

Load Lab 使用兩個獨立 process：

```text
cmd/loadserver
    │
    │ 真 TCP / UDP
    │ Protocol v2 / JSON v1
    │
cmd/loadbot
```

Bot 不繞過 Gateway / Command Queue，而是完整模擬 SessionWelcome、Realtime Token、UDP Move、Snapshot / Correction decode。

Server / Bot 分進程，避免 100～500 個 bot goroutine 的 allocation 污染 Server `runtime.MemStats`。

支援三種 deterministic scenario：

```text
distributed
→ 分散玩家；測一般 AOI

gate-zerg
→ 玩家集中 Main Gate；測 hotspot / fan-out

vertical-siege
→ L0 攻方 + L1 Ramp + L2 守方；測 Verticality
```

### 24-client Vertical Siege 基線

GitHub Actions `ubuntu-24.04` / Go 1.26.5：

| 指標 | 結果 |
|---|---:|
| Tick p50 | 0.180 ms |
| Tick p95 | 0.318 ms |
| Tick p99 | 0.478 ms |
| Tick max | 0.620 ms |
| Max command queue | 25 |
| TotalAlloc / 5s | 18.6 MB |
| GC / 5s | 5 |
| `datagram_too_large` | **1,200 / 1,200 snapshot cadence** |

### 100-client Gate Zerg 基線

| 指標 | 結果 |
|---|---:|
| Tick average | 3.276 ms |
| Tick p50 | 0.513 ms |
| Tick p95 | 8.508 ms |
| Tick p99 | **9.908 ms** |
| Tick max | 10.754 ms |
| Simulation avg / Tick | 0.055 ms |
| AOI avg / Tick | 1.479 ms |
| Replication Build avg / Tick | 1.590 ms |
| Max command queue | 102 / 4096 |
| Commands / Tick | 100.006 |
| TotalAlloc / 8s | **340.6 MB** |
| Mallocs / 8s | **534,920** |
| GC / 8s | **164** |
| GC pause total | 33.6 ms |
| `datagram_too_large` | **8,001** |

這份資料的結論不是「100 人效能很好，所以可以直接宣稱容量」，而是：

- Simulation / single-owner mutation 目前不是主要成本。
- AOI + per-session Replication Build 已成為主要 World Tick 成本。
- Full JSON Snapshot 是第一個已證實的 network scaling blocker。
- Allocation churn 已足以列入近期修正，但先從 replication 資料量／生命週期下手，不盲目全面 `sync.Pool`。
- Gate Zerg 同 Layer 全互見時 `candidate / visible = 1.0`，因此 Layer-aware bucket 不是解這個 hotspot 的第一優先。

完整方法與數據：[`docs/S3C5_SIEGE_LOAD_LAB.md`](docs/S3C5_SIEGE_LOAD_LAB.md)。

## 規模化與效能演進原則

Astrahold 採 **Measure → Profile → Optimize**。

### Hot Path Allocation Budget

```text
pprof / metrics
        ↓
定位 allocation hotspot
        ↓
減少資料量 / dirty tracking
        ↓
preallocate / scratch reuse
        ↓
必要時才 sync.Pool
```

### Layer-aware Spatial Bucket

規模化階段可演進為：

```text
SpatialKey
├── CellX
├── CellZ
└── Layer
```

同 Layer AOI 走 fast path；跨 Layer interaction 由 Portal adjacency、Vertical Combat、LOS 與 Siege visibility 顯式查詢相關 Layer。

這是候選集最佳化，不是所有攻城 hotspot 的萬靈丹。

### Replication Tier / Network LOD

未來攻城不要求所有 Entity 用相同頻率與精度同步：

```text
Tier 0 — Self / Target / Boss / Critical Objective
Tier 1 — Near Crowd
Tier 2 — Mid Crowd
Tier 3 — Far / Peripheral Crowd
```

實際 cadence 不先寫死，由 Load Lab、Godot interpolation 與 VAT 表現共同決定。

Server Network LOD 應盡量和 Client Presentation LOD 對齊：

```text
Server Tier 0 → Client Skeletal LOD0
Server Tier 1 → Skeletal / LOD1
Server Tier 2 → VAT Crowd
Server Tier 3 → Low-rate VAT / Impostor
```

### AOI ViewList / Dirty Tracking

現有 full visible-set rebuild 是 correctness baseline。若 profiling 確認值得，演進為：

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

不假設 500 個 Client 可以共享整份 Snapshot。可共享單位應縮小到：

```text
Entity Update Block
或 Spatial Cell / Tier Chunk
        ↓
serialize once
        ↓
多 observer 組合／複用
```

### Navigation / LOS Batch API

目前仍使用 Go 內部 Surface / Portal / Blocker。若將來導入 Recast/Detour 或其他 C/C++ backend，必須支援：

```text
ResolveMoveBatch(...)
LineOfSightBatch(...)
PathBatch(...)
```

避免每 Entity 每 Query 都跨一次 CGO boundary。

## 目前目錄

```text
astrahold-server/
├── cmd/
│   ├── worldd/              # 正常 World Server composition root
│   ├── loadserver/          # S3-C.5 獨立 Load Server
│   └── loadbot/             # 真 TCP/UDP Headless Bot
├── internal/
│   ├── world/               # XYZ + Layer / Entity 型別
│   ├── spatial/             # AOI spatial grid + query stats
│   ├── navigation/          # Gameplay navigation / LOS / blocker
│   ├── movement/            # Server-authoritative movement
│   ├── simulation/          # World mutable state
│   ├── protocol/            # Message semantic / Envelope
│   ├── codec/jsonv1/        # 開發 Payload Codec
│   ├── transport/           # Astrahold Frame / Stream helper
│   ├── gateway/             # Untrusted ingress policy
│   ├── session/             # Session / sequence / connection
│   ├── replication/         # Spawn / Despawn / Snapshot / Correction
│   ├── worldruntime/        # Command Queue / Fixed Tick / metrics seam
│   ├── loadlab/             # Scenario / bot / collector / JSON report
│   └── netadapter/tcpudp/   # TCP Reliable + UDP Realtime adapter
├── worlds/
│   └── castle-sandbox/
└── docs/
```

## Roadmap

### S0 — World Core ✅

- XYZ + Layer
- Spatial Grid / AOI
- Authoritative Movement
- Navigation abstraction
- Protocol semantic DTO

### S1 — World Runtime ✅

- Fixed Tick
- bounded Command Queue
- Session / sequence
- Reliable / Realtime delivery semantics
- Replication baseline
- backpressure seam

### S2 — Thin Client / Transport ✅

- TCP Reliable + UDP Realtime
- SessionWelcome / Realtime Token
- Astrahold Frame
- Godot C# Thin Client
- Snapshot interpolation / soft reconciliation
- Runtime E2E probe

### S3-A ～ S3-C — Gameplay World / Castle Verticality ✅

- Shared `gameplay.json`
- World Identity / SHA-256 handshake
- Surface / Portal / Blocker
- Dynamic blocker state
- L0 Ground → L1 Ramp → L2 Wall
- Castle Front Siege Blockout

### S3-C.5 — Siege Load Lab ✅

- 真 TCP/UDP Headless Bot
- Server / Bot 分離進程
- 24-client Vertical Siege smoke
- 100-client Gate Zerg baseline
- Tick p50 / p95 / p99
- AOI / Queue / Replication metrics
- allocation / heap / GC metrics
- GitHub Actions artifact report
- `workflow_dispatch` 500+ capability
- 第一個 scaling blocker 已定位

### S3-C.6 — Realtime Replication Foundation（下一步）

- [ ] `WorldSnapshot` / `PositionCorrection` 明確 stream / coalescing semantics
- [ ] MTU-safe realtime payload budget
- [ ] Compact binary transform payload baseline
- [ ] 減少 per-session full snapshot allocation
- [ ] Load Lab regression：24 / 100 人不再 100% `datagram_too_large`
- [ ] 再決定 delta / quantization / chunk 的下一層形式

### S3-D — Gate Siege Interaction

- [ ] Gate Entity / HP / State
- [ ] Attack Gate command
- [ ] Server 驗距離 / Layer / LOS / cooldown
- [ ] HP=0 → WorldRuntime 關閉 `main-gate` blocker
- [ ] Godot Gate HP / destroyed presentation
- [ ] Ground → Gate → Courtyard → Ramp → Wall 最小攻城 E2E

### S3-E — Siege Replication Scaling

- [ ] Replication Tier / Network LOD
- [ ] AOI ViewList / Dirty Tracking
- [ ] Quantized / Delta updates
- [ ] Shared Entity / Cell update blocks
- [ ] Layer-aware Spatial Bucket（有數據需要時）
- [ ] allocation budget / scratch reuse

### S4 — Full Siege Mechanics

- [ ] Attacker / Defender
- [ ] Gate / Throne / Crown
- [ ] Zone occupation
- [ ] Siege State Machine
- [ ] Guild objective ownership

### S5 — Full Combat Stress

- [ ] Dedicated environment 500+ clients
- [ ] Skills / Damage / Buff / VFX event load
- [ ] 固定硬體下的正式 capacity baseline
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

正常開發 Server 預設：

```text
TCP 127.0.0.1:7777
UDP 127.0.0.1:7778
World 20 Hz
Snapshot 10 Hz
```

目前核心仍盡量只使用 Go 標準函式庫；效能最佳化必須由 Load Lab / profiling 數據驅動，而不是提前增加不必要的架構複雜度。

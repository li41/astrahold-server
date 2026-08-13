# Astrahold Server

Astrahold 的全新權威 MMORPG 伺服器核心。

> 目標不是把天堂私服 Server 換名字，而是保留已驗證的 MMO 經驗，重新建立適合 **3D 王城、多人攻城、長期商用開發與可量測擴充** 的底層。

## 現階段狀態

目前主線已完成從 World Core、Runtime、Godot Thin Client 對接，到 **S3-C Castle Front Siege Blockout**：

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
```

目前下一個開發節點不是直接把 Server 複雜化成 Cell Actor，而是先進行 **S3-C.5 Siege Load Lab**，用可重現的 500+ headless client 壓測取得 AOI、Replication、GC、Queue 與 Tick latency 數據，再決定哪些高併發最佳化值得正式導入。

## 與 Myriad Throne 的關係

`myriad-throne-server` 是參考來源，不是 Astrahold 的基底。

舊 Lineage protocol、2D `gx/gy`、舊地圖格式與私服相容包袱不直接搬入。角色、道具、技能、Buff、Party、Guild、持久化等 domain 經驗，之後只在符合 Astrahold 新架構時逐項移植。

詳細規約：

- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)
- [`docs/S1_RUNTIME.md`](docs/S1_RUNTIME.md)
- [`docs/S2_PROTOCOL.md`](docs/S2_PROTOCOL.md)
- [`docs/S2B_TRANSPORT.md`](docs/S2B_TRANSPORT.md)
- [`docs/S3_GAMEPLAY_WORLD.md`](docs/S3_GAMEPLAY_WORLD.md)
- [`docs/S3B_WORLD_HANDSHAKE.md`](docs/S3B_WORLD_HANDSHAKE.md)
- [`docs/S3C_CASTLE_BLOCKOUT.md`](docs/S3C_CASTLE_BLOCKOUT.md)

## 世界座標

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

只有在 Siege Load Lab / profiling 證明單一 owner 已成為明確瓶頸後，才逐步平行化 read-only validation、navigation query、AOI fan-out 或 encode；**mutable world ownership 最後才考慮拆成 Cell Actor / Zone Actor。**

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

目前 `castle-sandbox@s3c-001` 已具備：

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

JSON v1 只是 Godot Thin Client 的開發橋接 Codec，不是最終商用 wire format。

Realtime datagram 初始限制在 1200 bytes，避免默默依賴 IP fragmentation。Realtime outbox 採 latest-state mailbox，舊 snapshot 可以被較新 snapshot 取代；Reliable queue 則不得靜默遺失。

> 開發 Transport 的 TCP 目前沒有 TLS，預設只綁 `127.0.0.1`。這是本機／受控環境 Transport，不可直接當成 Internet-facing security boundary。

## 規模化與效能演進原則

Astrahold 的效能架構採 **Measure → Profile → Optimize**，不以「看起來更高效」為理由提前引入高複雜度 concurrency。

### 1. Hot Path Allocation Budget，而不是盲目 Zero Allocation

主 Tick、AOI、Replication 與 Network encode 的 allocation 必須可量測、可設定預算並持續追蹤。

優化順序：

```text
pprof / metrics
        ↓
找出 allocation hotspot
        ↓
preallocate / reuse scratch buffers
        ↓
dirty tracking / incremental update
        ↓
必要時才使用 sync.Pool
```

不把 `sync.Pool` 或 zero-allocation 當成所有 package 的架構規定；只有被 profiling 證明是 hot path 的資料結構才值得增加生命週期管理複雜度。

### 2. Layer-aware Spatial Bucket

目前 Spatial Grid 已保存 XYZ + Layer 並支援 Layer filter。規模化階段預計演進為：

```text
SpatialKey
├── CellX
├── CellZ
└── Layer
```

同 Layer AOI 可走 fast path；跨 Layer interaction 則由 Portal adjacency、Vertical combat、LOS 與 Siege visibility 明確查詢相關 Layer。

不採「不同 Layer 永遠互相不可見」，因為城牆 L2 與城下 L0 本來就可能需要合法 LOS / ranged combat。

### 3. Replication Tier / Network LOD

攻城場景不要求所有 Entity 以相同頻率、相同精度同步。

初步規劃：

```text
Tier 0 — Self / Target / Boss / Critical Objective
高頻；完整 Transform / Action / HP / Buff 關鍵狀態

Tier 1 — Near Crowd
中高頻；Transform / Active Animation / Combat readability

Tier 2 — Mid Crowd
中低頻；Quantized Transform / 精簡狀態

Tier 3 — Far / Peripheral Crowd
低頻或事件式；只維持戰場辨識需要的狀態
```

實際頻率 **不先寫死 30 / 15 / 10 Hz**，由 Siege Load Lab 與 Godot interpolation / VAT 表現共同決定。

理想上 Server Network LOD 與 Client Presentation LOD 對齊：

```text
Server Tier 0 → Client Skeletal LOD0
Server Tier 1 → Skeletal / LOD1
Server Tier 2 → VAT Crowd
Server Tier 3 → Low-rate VAT / Impostor
```

### 4. AOI ViewList / Dirty Tracking

目前 replication 基線仍可重建 visible set，因為它最容易驗證。

壓測若證明 AOI diff / allocation 成為主要成本，演進方向為：

```text
Spatial Cell enter / leave
        ↓
Session ViewList cache
        ↓
Dirty Entity tracking
        ↓
只複寫有變化或需要 cadence 更新的狀態
```

先保證 correctness，再用 edge-trigger / cached ViewList 降低每 Tick brute-force 工作量。

### 5. Shared Serialization 不共享整份 Client Snapshot

多個 Client 的 AOI、Target priority、Known Entity set、Sequence 與 Self Correction 不完全相同，因此不假設「500 個 Client 共用一份 snapshot bytes」。

可共享的單位預計縮小成：

```text
Entity Update Block
或
Spatial Cell / Tier Chunk
        ↓
serialize once
        ↓
多個 observer 組合／複用
```

這樣可以降低重複 encode 成本，又不破壞 per-client replication semantics。

### 6. Navigation / LOS Batch API 預留

目前 Gameplay Navigator 仍採 Go 內部 Surface / Portal / Blocker 模型；現在不急著導入 Recast/Detour。

如果未來使用 C/C++ Navigation backend，必須避免每個 Entity 每個 query 都跨一次 CGO boundary。介面應能演進為：

```text
ResolveMoveBatch(...)
LineOfSightBatch(...)
PathBatch(...)
```

讓同 Tick 的大量導航／LOS 查詢可以批次送入 backend。

### 7. Concurrency 演進順序

若單 World Owner 最後真的成為瓶頸，優先順序為：

```text
① Read-only validation worker jobs
② Navigation / LOS batch jobs
③ AOI / replication build workers
④ Encode / network fan-out workers
⑤ 最後才拆 mutable state ownership
```

`Spatial Cell Actor`、cross-cell deferred event / two-phase commit、lock-free ring buffer 都保留為未來選項，**不是目前基線**。只有 profiling 證明 single-owner mutation 無法達標時才引入。

## S3-C.5 — Siege Load Lab

在 Gate HP / Combat 大量邏輯加入前，先建立可重現的 headless bot 壓測工具。

至少包含三種 Scenario：

```text
A. Distributed
500+ 玩家分散世界
→ 測正常 AOI / replication

B. Gate Zerg
100 / 250 / 500+ 玩家集中 Main Gate
→ 測 hotspot / queue / fan-out

C. Vertical Siege
攻方 L0 + Ramp L1 + 守方 L2
→ 測 Layer / LOS / AOI / Snapshot
```

主要指標：

```text
Tick duration p50 / p95 / p99
Commands processed / tick
Command queue utilization
AOI query time
Candidate / visible entity counts
Replication build time
Snapshot bytes / sec
Packets / sec
Allocations / tick
Heap growth / GC pause
Reliable queue pressure
Realtime drop / coalescing
CPU / RSS memory
```

這些數據會決定：

- World simulation 是否需要由 20 Hz 提高到 30 Hz 或其他頻率
- 是否需要 Layer-aware bucket
- AOI ViewList cache 的優先級
- Replication Tier 的 cadence
- JSON v1 何時必須換成 binary codec
- Snapshot delta / quantization / bit-packing 的必要性
- 是否需要 `sync.Pool` / scratch buffer reuse
- 是否值得開始平行化 read-only stage
- 單 World Owner 是否真的已經成為瓶頸

## 目前目錄

```text
astrahold-server/
├── cmd/
│   └── worldd/              # World Loop + TCP/UDP composition root
├── internal/
│   ├── world/               # XYZ + Layer / Entity 型別
│   ├── spatial/             # AOI spatial grid
│   ├── navigation/          # Gameplay navigation / LOS / blocker
│   ├── movement/            # Server-authoritative movement
│   ├── simulation/          # World mutable state
│   ├── protocol/            # Message semantic / Envelope
│   ├── codec/jsonv1/        # 開發 Payload Codec
│   ├── transport/           # Astrahold Frame / Stream helper
│   ├── gateway/             # Untrusted ingress policy
│   ├── session/             # Session / sequence / connection
│   ├── replication/         # Spawn / Despawn / Snapshot / Correction
│   ├── gameplayworld/       # Surface / Portal / Blocker / World Identity
│   ├── worldruntime/        # Command Queue / Dynamic World / Fixed Tick
│   └── netadapter/tcpudp/   # TCP Reliable + UDP Realtime adapter
├── worlds/
│   └── castle-sandbox/
│       └── gameplay.json
└── docs/
```

## 里程碑

### S0 — World Core

- [x] XYZ + Layer
- [x] Entity 基礎型別
- [x] Spatial Grid / AOI
- [x] Authoritative Movement
- [x] Navigation abstraction
- [x] Protocol semantic DTO

### S1 — World Runtime

- [x] 固定 20 Hz world loop
- [x] bounded command queue
- [x] connection/session abstraction
- [x] Session-scoped input sequence
- [x] Reliable / Realtime outbound sequence
- [x] Astrahold Frame
- [x] spawn/despawn/snapshot/correction
- [x] backpressure seam

### S2-A — Protocol / Ingress

- [x] Input sequence 單一來源化
- [x] Gateway / Ingress 白名單
- [x] JSON v1 開發 Codec
- [x] Protocol DTO 與 wire struct 分離

### S2-B — Server Transport

- [x] TCP Reliable listener
- [x] UDP Realtime listener
- [x] SessionWelcome
- [x] 128-bit realtime token
- [x] UDP token → Session routing
- [x] `EnqueueJoin` / `EnqueueLeave`
- [x] TCP stream frame reader/writer
- [x] 1200-byte UDP datagram guard
- [x] Realtime latest-state mailbox
- [x] ephemeral-port integration test
- [x] `worldd` 真實 listener 啟動

### S2-C / S2-D — Godot Thin Client + Runtime E2E

- [x] Godot C# Astrahold Frame / JSON v1
- [x] TCP SessionWelcome
- [x] UDP Realtime activation
- [x] Capsule Entity
- [x] Snapshot interpolation
- [x] Local prediction / soft reconciliation
- [x] 官方 Godot headless runtime probe
- [x] TCP Welcome → UDP Move → Snapshot / Correction E2E

### S3-A — Gameplay World v1

- [x] `gameplay.json` strict schema / validation
- [x] Surface / Portal / Blocker
- [x] 多高度 Surface
- [x] Layer transition
- [x] Dynamic Gate blocker
- [x] Server-side LOS

### S3-B — World Identity / Dynamic World

- [x] Protocol v2
- [x] `world_id / revision / gameplay_sha256`
- [x] World mismatch fail-fast
- [x] World 驗證成功後才啟用 realtime UDP
- [x] `WorldDynamicState`
- [x] blocker revision / Reliable replication

### S3-C — Castle Front Siege Blockout

- [x] Siege Field / Courtyard
- [x] Main Gate
- [x] West / East Ramp
- [x] Front Wall Walk Y=8m
- [x] L0 → L1 → L2 雙向 traversal
- [x] Portal topology validation
- [x] Gate closed/open Movement + LOS tests
- [x] Client / Server `gameplay.json` byte identity

### S3-C.5 — Siege Load Lab（下一步）

- [ ] Headless bot simulator
- [ ] 100 / 250 / 500+ concurrent clients
- [ ] Distributed / Gate Zerg / Vertical Siege scenarios
- [ ] Tick p50 / p95 / p99 metrics
- [ ] AOI / Replication / Network metrics
- [ ] allocation / GC / heap profiling
- [ ] queue / backpressure metrics
- [ ] 建立第一份 performance budget

### S3-D — Siege Interaction Prototype

- [ ] Gate Entity / GateID
- [ ] Gate MaxHP / HP / State
- [ ] Client → Server Attack Gate command
- [ ] Server distance / Layer / LOS / cadence validation
- [ ] Gate HP = 0 → WorldRuntime 關閉 `main-gate` blocker
- [ ] Reliable Gate state / HP replication
- [ ] Godot Gate visual / HP bar / destroyed state
- [ ] 攻方進 Courtyard → Ramp → Wall 的最小攻城 E2E

### S3-E — Siege Replication Scaling

- [ ] Layer-aware Spatial Bucket
- [ ] Replication Tier 0 / 1 / 2 / 3
- [ ] AOI ViewList cache / dirty tracking（依壓測決定）
- [ ] hot-path preallocation / buffer reuse
- [ ] shared entity/cell serialization chunks
- [ ] snapshot quantization / delta / bit-packing（依數據決定）
- [ ] Batch Navigation / LOS API seam
- [ ] read-only / fan-out worker profiling prototype

### S4 — Full Siege Mechanics

- [ ] Attacker / Defender
- [ ] Gate destruction
- [ ] Siege zones
- [ ] Throne / Crown objective
- [ ] Zone occupation
- [ ] Siege state machine
- [ ] Objective replication / reconnect state restore

### S5 — Full Combat Stress Test

- [ ] 500+ players with combat
- [ ] Skill / Damage / Buff load
- [ ] Gate / Objective contention
- [ ] Packet loss / jitter / reconnect tests
- [ ] Capacity target 與 production performance budget
- [ ] 依 profiling 決定是否需要 Cell Actor / deeper parallel mutation

## 開發

```bash
go test ./...
go vet ./...
go run ./cmd/worldd
```

預設監聽：

```text
TCP 127.0.0.1:7777
UDP 127.0.0.1:7778
World 20 Hz
Snapshot 10 Hz
```

可調整：

```bash
go run ./cmd/worldd \
  -tcp 127.0.0.1:7777 \
  -udp 127.0.0.1:7778 \
  -tick-rate 20 \
  -snapshot-rate 10
```

目前核心仍盡量維持少依賴與清楚 ownership。效能最佳化必須建立在可重現壓測與 profiling 上，而不是提早用複雜度換取尚未證明需要的吞吐量。

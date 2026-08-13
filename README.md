# Astrahold Server

Astrahold 的全新權威 MMORPG 伺服器核心。

> 目標不是把天堂私服 Server 換名字，而是保留我們已經學會的 MMO 經驗，重新建立適合 **3D 王城、多人攻城與長期商用開發** 的底層。

## 現階段目標

目前已完成 S0 世界核心與 S1 即時世界 Runtime 基線：

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
Protocol Envelope / Frame
        ↓
Godot Thin Client
```

## 與 Myriad Throne 的關係

`myriad-throne-server` 是參考來源，不是 Astrahold 的基底。

我們會逐項評估真正值得保留的 domain 邏輯，例如角色、道具、技能、Buff、Party、Guild、持久化與資料驅動經驗；舊 Lineage protocol、2D 地圖座標、舊地圖格式與私服相容包袱不直接搬入。

詳細原則見：

- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)
- [`docs/S1_RUNTIME.md`](docs/S1_RUNTIME.md)

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

因此未來可以正確表達城牆、樓梯、地下層、橋面與高低差，而不必把 3D 世界硬壓回 2D Grid。

## S1 Runtime 核心原則

Astrahold 現在明確維持兩條不變量：

1. Network、DB、GM、管理介面不得直接修改 World mutable state，只能透過 Command Queue。
2. World Tick 不得直接做 blocking socket I/O，只能把 outbound message 丟到非阻塞 Connection / Outbox。

這樣可以讓每個 World/Zone 維持單一 simulation owner，避免未來 Combat、Skill、NPC AI、Siege 到處加 mutex。

## Server Authoritative Movement

Client 只送方向與 **Session scoped input sequence**；時間推進完全由 Server fixed tick 決定。

```text
ClientMoveInput
(direction + sequence)
        ↓
Session sequence validation
        ↓
Command Queue
        ↓
Server 20 Hz Tick
        ↓
Movement + Navigation
        ↓
Authoritative Position
        ↓
Snapshot + Correction
```

Sequence 不再放在 Movement actor state。玩家重新連線取得新 Session 後可以重新建立 sequence 空間，不會被舊 actor 狀態污染。

## Protocol / Transport 分層

```text
Gameplay Message
        ↓
protocol.Envelope
        ↓
PayloadCodec
        ↓
Astrahold Frame v1
        ↓
Transport Adapter
```

目前不綁死 UDP、QUIC、TCP，也不綁死 Protobuf、FlatBuffers 或自訂 binary encoding。

Delivery class 目前分成：

- `ReliableOrdered`：Spawn、Despawn、重要狀態事件
- `RealtimeSequenced`：Snapshot、PositionCorrection 等最新狀態優先資料

## 目前目錄

```text
astrahold-server/
├── cmd/
│   └── worldd/             # 固定 Tick 世界程序入口
├── internal/
│   ├── world/              # XYZ + Layer 與 Entity 基礎型別
│   ├── spatial/            # AOI spatial grid
│   ├── navigation/         # Navigation / LOS 抽象
│   ├── movement/           # Server authoritative movement
│   ├── simulation/         # World mutable state
│   ├── protocol/           # Astrahold message semantic / Envelope
│   ├── transport/          # Frame v1 / PayloadCodec 邊界
│   ├── session/            # Session / Connection / sequence
│   ├── replication/        # Spawn / Despawn / Snapshot / Correction
│   └── worldruntime/       # Command Queue / Fixed Tick / orchestration
└── docs/
    ├── ARCHITECTURE.md
    └── S1_RUNTIME.md
```

## 里程碑

### S0 — World Core

- [x] XYZ + Layer
- [x] Entity 基礎型別
- [x] Spatial Grid / AOI
- [x] Authoritative Movement
- [x] Navigation abstraction
- [x] Protocol semantic DTO
- [x] 基礎單元測試

### S1 — World Runtime + Realtime Protocol Boundary

- [x] 固定 20 Hz world loop
- [x] bounded command queue
- [x] connection/session abstraction
- [x] Session scoped input sequence
- [x] Reliable / Realtime outbound sequence
- [x] Astrahold packet frame v1
- [x] 可替換 PayloadCodec 介面
- [x] spawn/despawn/snapshot replication
- [x] server tick / position correction
- [x] non-blocking outbound / backpressure error
- [x] 單元測試、`go vet`、race detector 驗證

### S2 — Godot Thin Client

- [ ] 選定第一版 Payload Codec
- [ ] 實作第一個 Transport Adapter
- [ ] Godot 連線
- [ ] 進入測試世界
- [ ] Capsule 玩家
- [ ] XYZ movement
- [ ] 第二個 client 可互相看到
- [ ] AOI enter/leave
- [ ] interpolation / reconciliation prototype

### S3 — Castle Sandbox

- [ ] World Compiler gameplay proxy
- [ ] 多高度導航
- [ ] 樓梯／Layer transition
- [ ] Gate blocker
- [ ] LOS
- [ ] 城牆上下同時有人

之後才開始 Combat / Skill / Siege 與大規模 VAT crowd 壓測。

## 開發

```bash
go test ./...
go vet ./...
go run ./cmd/worldd
```

目前核心仍只使用 Go 標準函式庫，先把依賴面與架構責任維持乾淨。

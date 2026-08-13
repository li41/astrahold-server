# Astrahold Server

Astrahold 的全新權威 MMORPG 伺服器核心。

> 目標不是把天堂私服 Server 換名字，而是保留已驗證的 MMO 經驗，重新建立適合 **3D 王城、多人攻城與長期商用開發** 的底層。

## 現階段狀態

目前已完成 S0、S1、S2-A 與 **Server 端 S2-B**：

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
Astrahold Frame + JSON v1
        ↓
TCP Reliable + UDP Realtime
        ↓
Godot Thin Client（下一步 S2-C）
```

## 與 Myriad Throne 的關係

`myriad-throne-server` 是參考來源，不是 Astrahold 的基底。

舊 Lineage protocol、2D `gx/gy`、舊地圖格式與私服相容包袱不直接搬入。角色、道具、技能、Buff、Party、Guild、持久化等 domain 經驗，之後只在符合 Astrahold 新架構時逐項移植。

詳細規約：

- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)
- [`docs/S1_RUNTIME.md`](docs/S1_RUNTIME.md)
- [`docs/S2_PROTOCOL.md`](docs/S2_PROTOCOL.md)
- [`docs/S2B_TRANSPORT.md`](docs/S2B_TRANSPORT.md)

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

## Runtime 不變量

Astrahold 維持兩條核心規則：

1. Network、DB、GM、管理介面不得直接修改 World mutable state，只能透過 bounded Command Queue。
2. World Tick 不得直接做 blocking socket I/O，只能把 outbound message 送到非阻塞 Connection / Outbox。

這讓每個 World/Zone 可以維持單一 simulation owner，避免未來 Combat、Skill、NPC AI、Siege 到處加 mutex。

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
Server 20 Hz Tick
        ↓
Movement + Navigation
        ↓
Authoritative Position
        ↓
Snapshot + PositionCorrection
```

Sequence 只有 Frame / Envelope 是唯一真相來源，不重複塞進 payload。

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

S2 開發期目前使用：

```text
ReliableOrdered
→ TCP
→ SessionWelcome / Spawn / Despawn / 重要事件

RealtimeSequenced
→ UDP
→ Move / Snapshot / PositionCorrection
```

JSON v1 只是 Godot Thin Client 的開發橋接 Codec，不是最終商用 wire format。

## S2-B 連線流程

```text
TCP connect
    ↓
SessionID / EntityID
    ↓
128-bit Realtime Token
    ↓
SessionWelcome (TCP)
    ↓
WorldRuntime.EnqueueJoin
    ↓
Client 第一個 UDP frame + token
    ↓
Server 綁定 UDP endpoint
    ↓
Move → Gateway → Runtime
    ↓
Snapshot / Correction → UDP
```

Realtime datagram 初始限制在 1200 bytes，避免默默依賴 IP fragmentation。Realtime outbox 採 latest-state mailbox，舊 snapshot 可以被較新 snapshot 取代；Reliable queue 則不得靜默丟失。

> S2-B 的 TCP 目前沒有 TLS，預設只綁 `127.0.0.1`。這是本機／受控環境開發 Transport，不可直接當成 Internet-facing security boundary。

## 目前目錄

```text
astrahold-server/
├── cmd/
│   └── worldd/              # World Loop + TCP/UDP composition root
├── internal/
│   ├── world/               # XYZ + Layer / Entity 型別
│   ├── spatial/             # AOI spatial grid
│   ├── navigation/          # Navigation / LOS 抽象
│   ├── movement/            # Server-authoritative movement
│   ├── simulation/          # World mutable state
│   ├── protocol/            # Message semantic / Envelope
│   ├── codec/jsonv1/        # S2 開發 Payload Codec
│   ├── transport/           # Astrahold Frame / Stream helper
│   ├── gateway/             # Untrusted ingress policy
│   ├── session/             # Session / sequence / connection
│   ├── replication/         # Spawn / Despawn / Snapshot / Correction
│   ├── worldruntime/        # Command Queue / Join / Leave / Fixed Tick
│   └── netadapter/tcpudp/   # S2-B TCP Reliable + UDP Realtime adapter
└── docs/
    ├── ARCHITECTURE.md
    ├── S1_RUNTIME.md
    ├── S2_PROTOCOL.md
    └── S2B_TRANSPORT.md
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
- [x] Astrahold Frame v1
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

### S2-C — Godot Thin Client（下一步）

- [ ] Godot C# Astrahold Frame parser
- [ ] JSON v1 Codec
- [ ] TCP SessionWelcome
- [ ] UDP Realtime bind / movement
- [ ] Capsule Entity
- [ ] XYZ movement
- [ ] 第二個 Client 互相看到
- [ ] Snapshot Interpolation
- [ ] Prediction / Reconciliation prototype

### S3 — Castle Sandbox

- [ ] World Compiler gameplay proxy
- [ ] 多高度導航
- [ ] 樓梯／Layer transition
- [ ] Gate blocker
- [ ] LOS
- [ ] 城牆上下同時有人

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

目前核心仍只使用 Go 標準函式庫，先把依賴面與架構責任維持乾淨。

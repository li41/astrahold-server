# S2-B：TCP Reliable + UDP Realtime 開發 Transport

> **Historical milestone document.** 本文保留 S2-B 當時以 raw 128-bit realtime bearer token 做 ASTU routing 的開發 transport contract。這不是目前 production security boundary；現況請以 `README.md`、`docs/ARCHITECTURE.md`、S4-E.5～E.7 為準，目前使用 **Protocol v9 public RoutingID + HMAC-SHA256-128**，trusted bootstrap 可走 TLS 1.3。

S2-B 的目標是讓 Godot Thin Client **真的可以連進 Astrahold Server**，同時維持 S0/S1/S2-A 已定義的 package 邊界。

這是一個開發用 Transport Adapter，不是最終商用網路安全方案。

---

## 為什麼先採 TCP + UDP 雙通道

第一版需要同時驗證兩種完全不同的 delivery semantics：

```text
Reliable Ordered
- Session bootstrap
- Spawn / Despawn
- 重要狀態事件

Realtime Sequenced
- Movement input
- WorldSnapshot
- PositionCorrection
- 未來 Aim / Facing
```

若現在把所有資料塞進單一 TCP stream，Godot Thin Client 無法提早驗證 realtime channel 的 jitter、loss、snapshot age 與 reconciliation 行為。

S2-B 因此採：

- TCP：Reliable Ordered
- UDP：Realtime Sequenced
- JSON v1：開發 Payload Codec
- Astrahold Frame v1：兩個 channel 共用的 protocol frame

未來換成 QUIC、ENet/KCP 類型傳輸或其他方案時，只替換 `netadapter` / `PayloadCodec`，不修改 World、Simulation、Replication 與 Gameplay Domain。

---

## 連線流程

```text
Godot Client
    │
    │ TCP connect
    ▼
TCP Adapter
    │
    ├─ 建立 SessionID
    ├─ 建立 EntityID
    ├─ crypto/rand 產生 128-bit Realtime Token
    │
    ▼
SessionWelcome (TCP)
    │
    ├─ session_id
    ├─ entity_id
    ├─ realtime_port
    ├─ realtime_token
    ├─ tick_rate_hz
    └─ snapshot_rate_hz
    │
    ▼
WorldRuntime.EnqueueJoin
    │
    ▼
Fixed Tick World
```

Client 收到 Welcome 後，第一個 UDP realtime datagram 會帶同一個 opaque token：

```text
UDP Datagram
├─ ASTU header
├─ 128-bit token
└─ Astrahold Frame
   └─ ClientMoveInput
```

Server 驗證 token 後才把 UDP remote endpoint 綁定到該 Session。

---

## Realtime Token

Token 使用 `crypto/rand` 產生 128-bit 隨機值。

規則：

- 只透過 TCP Welcome 傳給 Client
- 不寫 log
- 不放進角色資料庫
- 不作為帳號登入憑證
- UDP endpoint 第一次綁定後，只允許同 IP 的 port 更新，以支援基本 NAT rebinding

**S2-B 的 TCP 目前沒有 TLS。** 因此此 token 只是開發 Transport routing capability，不是 Internet-facing security boundary。

預設 `worldd` 只綁 `127.0.0.1`。正式對外連線前必須另外設計：

- Auth / Session credential
- 加密傳輸（TLS / QUIC 或等價方案）
- replay / abuse protection
- rate limiting
- DoS 策略
- token rotation / expiry

---

## World Join / Leave 不可繞過 Runtime

Network Adapter 不得直接：

```text
simulation.World.Spawn(...)
simulation.World.Remove(...)
```

S2-B 新增：

```text
WorldRuntime.EnqueueJoin(...)
WorldRuntime.EnqueueLeave(...)
```

因此資料流保持：

```text
TCP accept / disconnect
        ↓
Bounded Command Queue
        ↓
World Runtime
        ↓
Spawn / Session Registry / Replication
```

Join 會把 Entity Spawn 與 Session Registry Add 視為同一個 runtime operation；若 Session Add 失敗，會回滾 Entity Spawn。

---

## TCP Stream

Astrahold Frame 已包含 payload length，因此 TCP 不需要再發明第二套 packet length protocol。

```text
Read 28-byte Astrahold Header
        ↓
驗證 payload length 上限
        ↓
Read exact payload bytes
        ↓
PayloadCodec
        ↓
protocol.Envelope
```

TCP reader 只接受 `ReliableOrdered` delivery。Realtime frame 若出現在 TCP channel，視為 channel violation。

---

## UDP Datagram

UDP 使用額外 24-byte adapter header：

```text
0   uint32   Magic = ASTU
4   uint16   Protocol Version
6   uint16   Header Size = 24
8   16 bytes Realtime Token
24  ...      Astrahold Frame
```

整個 datagram 初始限制為 **1200 bytes**，避免開發階段默默依賴 IP fragmentation。

如果 JSON snapshot 超過此上限：

- 不自動切 IP fragment
- 不靜默放大 MTU
- 回報 network error / metric
- 後續透過較緊湊 Codec、snapshot chunk、delta compression 或 priority replication 解決

---

## Realtime Outbox 是 Latest-State Mailbox

Realtime queue 不使用無限制 FIFO。

```text
snapshot N
snapshot N+1
snapshot N+2
```

若 writer 還來不及送，舊 snapshot 可以被更新的 snapshot 取代。

這符合 `RealtimeSequenced` 語意，避免慢 Client 讓記憶體一直堆積過期位置資料。

Reliable queue 則維持 bounded FIFO；滿載時回報 backpressure，不得靜默丟失 Spawn / Despawn 等可靠事件。

---

## S2-B 完成條件

- [x] TCP Reliable listener
- [x] UDP Realtime listener
- [x] SessionWelcome
- [x] 128-bit realtime token
- [x] UDP token → Session routing
- [x] same-IP NAT port rebinding
- [x] `EnqueueJoin` / `EnqueueLeave`
- [x] TCP stream frame reader/writer
- [x] UDP datagram size guard
- [x] Realtime latest-state mailbox
- [x] Network error reporting seam
- [x] `worldd` 可直接啟動 TCP/UDP server
- [x] ephemeral-port integration test
- [x] `go test ./...`
- [x] `go vet ./...`
- [x] race detector（transport/runtime 關鍵 package）

下一步 S2-C 才進入 Godot repository：實作相同 Frame / JSON v1 / TCP Welcome / UDP Realtime，先用 Capsule 跑通第一個真人 Client。

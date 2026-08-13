# S2：Protocol、Ingress 與開發 Codec 維護規約

本文件記錄 Astrahold Server 進入 Godot Thin Client 階段後的協定邊界。

目標不是現在就把最終商用網路格式拍板，而是先確保 **任何 Codec / Transport 的替換都不會污染 World、Simulation、Replication 與 Gameplay Domain**。

---

## 資料流

```text
Untrusted Network Bytes
        ↓
Astrahold Frame Decoder
        ↓
PayloadCodec
        ↓
protocol.Envelope
        ↓
Gateway / Ingress Policy
        ↓
Bounded WorldRuntime Command Queue
        ↓
Fixed Tick Simulation
```

反向資料流：

```text
Simulation
    ↓
Replication
    ↓
protocol.Envelope
    ↓
PayloadCodec
    ↓
Astrahold Frame
    ↓
Transport Adapter
```

### 不變量

1. Socket / Transport Adapter 不得直接修改 `simulation.World`。
2. Codec 不得包含 Gameplay Rule。
3. Gateway 只負責「Client 有沒有資格送這種 message、delivery 是否正確、要轉成哪種 Runtime command」。
4. WorldRuntime 仍然是 mutable world state 的唯一 application boundary。

---

# Sequence：只能有一個真相來源

S1 初版的 `ClientMoveInput` payload 與外層 Frame 都曾有 `Sequence`，這會產生雙重來源問題。

S2 起正式改為：

```text
Astrahold Frame / Envelope
└── Sequence = 42

ClientMoveInput Payload
├── DirectionX
└── DirectionZ
```

**Input Sequence 只存在 Envelope / Frame。**

Gateway 將 `Envelope.Sequence` 一起送進：

```go
EnqueueMove(sessionID, sequence, input)
```

WorldRuntime 再交給 Session 做 stale / duplicate sequence 驗證。

`PositionCorrection.LastProcessedInputSequence` 回傳的也是這個相同 sequence 空間，供 Godot Client reconciliation。

這樣不會出現：

```text
frame.sequence = 42
payload.sequence = 41
```

到底該信哪一個的問題。

---

# Gateway / Ingress 白名單

所有 Client message 必須經過 `internal/gateway`。

目前 Client 只允許：

| Message | Delivery | Runtime Command |
|---|---|---|
| `ClientMoveInput` | `RealtimeSequenced` | `EnqueueMove` |

以下行為必須拒絕：

- Client 用 Reliable channel 傳 Move
- Sequence = 0
- Nil message
- Client 偽造 `EntitySpawn`
- Client 偽造 `EntityDespawn`
- Client 偽造 `WorldSnapshot`
- Client 偽造 `PositionCorrection`

未來加入 Cast Skill、Interact、Target、Use Item 等 message 時，必須逐項加入明確 ingress policy，不使用「任意 protocol.Message 都往 WorldRuntime 丟」的泛型捷徑。

---

# JSON v1 的定位

`internal/codec/jsonv1` 是 **S2 Godot Thin Client 開發橋接 Codec**。

用途：

- 讓 Go / C# 都能快速檢查 payload
- Wireshark / log / unit test 容易閱讀
- 先驗證 Frame、Session、Snapshot、Prediction、Reconciliation 資料流
- 在沒有真實攻城頻寬數據前避免過早做 bit packing

它不是以下承諾：

- 不是最終商用 Payload Codec
- 不代表未來不使用 Protobuf / FlatBuffers / 自訂 binary
- 不代表 Realtime Snapshot 最後會用 JSON

### 為什麼 Codec 不直接 Marshal Go DTO

JSON v1 使用自己的 wire struct，例如：

```json
{"dx":1,"dz":0}
```

而不是直接依賴 Go struct field name。

因此：

```text
protocol.ClientMoveInput
        ↓
jsonv1 wire struct
        ↓
JSON bytes
```

未來換其他 Codec 時，不需要在 `protocol` DTO 上塞滿 JSON-specific tag 或 encoding rule。

---

# JSON v1 驗證規則

目前採較嚴格的開發期解碼：

- unknown field 拒絕
- trailing JSON 拒絕
- unknown MessageType 拒絕
- Frame 層先驗證 Magic / Protocol Version / Delivery / Payload Length

這讓 Godot Thin Client 初期若送錯欄位，可以快速發現 schema mismatch，而不是靜默忽略錯誤。

若未來需要同一 Protocol Version 內做寬鬆 schema evolution，應明確設計 compatibility policy，不直接偷偷取消驗證。

---

# PayloadCodec 邊界

```go
type PayloadCodec interface {
    Marshal(protocol.Message) ([]byte, error)
    Unmarshal(protocol.MessageType, []byte) (protocol.Message, error)
}
```

因此：

```text
Simulation
Replication
WorldRuntime
Session
```

都不知道 JSON / Protobuf / FlatBuffers。

Codec 替換只應影響：

```text
codec implementation
transport/bootstrap wiring
client codec implementation
protocol compatibility tests
```

---

# Transport 仍保持可替換

S2 這一批**尚未把實際 socket transport 寫死**。

下一步要以 Godot Thin Client 的實際可維護性比較：

- reliable + realtime channel 能力
- Go Server 實作成熟度
- Godot C# 跨平台支援
- NAT / reconnect / session binding 複雜度
- jitter / packet loss 行為
- 是否容易做 metrics / packet capture / load test
- 100 / 200 / 500 人攻城的 fan-out 成本

無論最後選什麼，Transport Adapter 都不得繞過：

```text
Frame → Codec → Gateway → Runtime Queue
```

---

# S2-A 完成狀態

- [x] Move input sequence 單一來源化（Envelope / Frame）
- [x] `ClientMoveInput` 移除 payload sequence
- [x] Gateway / Ingress policy seam
- [x] Client message 白名單
- [x] JSON v1 開發 Codec
- [x] JSON wire struct 與 Protocol DTO 分離
- [x] Frame + JSON Envelope round-trip test
- [x] stale input sequence 仍由 Session 驗證
- [x] `go test ./...`
- [x] `go vet ./...`
- [x] Gateway / JSON Codec / WorldRuntime race detector 驗證

## 下一步：S2-B

- [ ] 第一個實際 Transport Adapter
- [ ] Session connect / disconnect bootstrap
- [ ] Godot C# Astrahold Frame decoder
- [ ] Godot JSON v1 Codec
- [ ] Server Tick / NetworkClock 初始同步
- [ ] 第一個 Capsule Entity
- [ ] Move Input → Server Tick → PositionCorrection 完整網路往返

S2-B 跑通後，再以實際封包大小、CPU、jitter 與開發成本決定最終 Payload Codec 與 Transport 方向。

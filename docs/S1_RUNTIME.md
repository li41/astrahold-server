# S1：World Runtime 與即時協定邊界

本文件記錄 S1 的維護規則。之後新增 Combat、Skill、Siege、NPC AI 或跨 Zone 時，應優先維持這些邊界，而不是為單一功能繞過 runtime。

## Package 責任

| Package | 責任 | 禁止事項 |
|---|---|---|
| `simulation` | Entity mutable state、movement tick、AOI query | 不知道 Session、Socket、Codec |
| `worldruntime` | Command Queue、固定 Tick、Session command 套用、觸發 replication | 不做 blocking I/O |
| `session` | Session identity、Entity binding、input sequence、outbound connection | 不修改 World |
| `replication` | AOI known-set 與 spawn/despawn/snapshot/correction 建構 | 不做網路 I/O |
| `protocol` | Gameplay message semantic、delivery class、Envelope | 不知道 encoding/socket |
| `transport` | Frame、PayloadCodec 邊界 | 不知道 Combat/Item/Guild 規則 |

## Tick 順序

每一個固定 Tick 必須依序：

1. Drain bounded commands（有每 Tick 上限）
2. 套用 Session/Register/Move 等 command
3. 推進 `simulation.World.Tick(fixedDelta)`
4. 到 replication cadence 時做 AOI diff
5. 產生 spawn/despawn/snapshot/correction
6. 非阻塞送到 per-session outbound connection
7. 回報 command/tick/delivery errors 給 metrics/logging 層

這個順序讓同一 Tick 收到的 input 可以在當 Tick 被 simulation 使用，並讓 correction 的 sequence 對應到已處理的 input。

## 頻率

初始基線：

- World simulation：20 Hz
- Snapshot：每 2 Tick 一次，即 10 Hz

兩者刻意分離。日後可以把 world simulation 提高到 30/60 Hz，而不必等比例放大網路 snapshot 頻寬。

## Backpressure

Network reader 若塞爆 command queue，enqueue 必須立即失敗，不得無限配置。

World tick 若塞爆 outbound queue：

- Realtime：可丟棄舊/當前 snapshot，因下一個較新的 snapshot 會取代它；日後可進一步做 latest-state coalescing。
- Reliable：不能靜默遺失，應累積 metric 並由 connection policy 決定斷線或重新同步。

S1 只回報 `ErrBackpressure`，不先寫死 disconnect policy。

## Sequence

Inbound movement sequence 是 **Session scoped**。新 Session 可重新從 1 開始。

Outbound sequence 分成 Reliable 與 Realtime 各自遞增，因兩種 delivery class 不應互相造成 Head-of-Line 依賴。

`ServerTick` 是權威 simulation 時間軸，用於 interpolation、reconciliation、debug 與未來 replay/telemetry。

## 未在 S1 綁死的選擇

- UDP / QUIC / TCP / KCP 等底層 transport
- Protobuf / FlatBuffers / 自訂 binary payload codec
- Snapshot delta 壓縮格式
- 跨 Zone / shard topology
- Login/Auth/Persistence

這些都刻意留在清楚的 adapter seam 後面，等到 S2 Godot Thin Client 與壓測數據出現後再決定。

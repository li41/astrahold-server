# S3-E.4：Replication Buffer Ownership / Bounded Selection

S3-E.4 延續 S3-E.3，不預設 Quantized / Delta，也不拆 mutable World ownership。本階段先用 measurement-window allocation profile 找出 500-client Gate Zerg 剩餘 allocation 的實際來源，再針對 ownership boundary 與 64-transform budget selection 做實測導向的修正。

本階段維持：

- Protocol v6
- GameV1 wire layout
- `EntityTransform` 26-byte layout
- `WorldSnapshot` partial update batch semantics
- Reliable Spawn / Despawn lifecycle truth
- 1200-byte UDP MTU
- single mutable World Owner
- 64 transforms / Session / build budget

Client wire contract 沒有改變，因此不需要 Client 修改。

## S3-E.3 起點

S3-E.3 已把 realtime writer 的多層 allocation/copy 收斂成 append-based single-pass encode 與 reusable MTU buffer。500-client 代表性結果：

```text
Tick avg                  7.732 ms
Tick p99                 19.825 ms
Replication Build         6.194 ms
TotalAlloc               178.39 MB / 10s
Mallocs                    1.251 M
Realtime throughput       53.241 Mbit/s
Encode avg                 0.48 us/datagram
```

S3-E.4 不再猜測 wire encode，而是先 profile 剩餘 allocation。

## Measurement-window Allocation Profile

`cmd/loadserver` 新增可選診斷參數：

```text
-alloc-profile-prefix
-alloc-profile-rate
```

指定 prefix 時，Load Lab 會在正式 measurement 前後各寫一份 `allocs` profile，利用 `go tool pprof -base` 取得 measurement-window delta。正式 capacity run 不開 profiler，避免 sampling 擾動 tail latency。

S3-E.3 code path 的 500-client sampled `alloc_space`：

| 函式 | Flat alloc | 比例 |
|---|---:|---:|
| `replication.(*Service).buildFrame` | 36.53 MB | 48.87% |
| `replication.rebuildDesiredTracks` | 19.99 MB | 26.75% |
| `transport.EncodeFrame` | 5.88 MB | 7.87% |
| `encoding/json.Marshal` | 5.82 MB | 7.78% |

`buildFrame + rebuildDesiredTracks` 合計約 **75.7% sampled alloc_space**。

因此資料沒有指向 UDP ingress decode，也沒有要求升 Protocol；主要問題在 Replication ownership 與 AOI membership buffer lifecycle。

## 問題：TrySend 後誰擁有 Snapshot backing storage？

原本正確但保守的 lifetime：

```text
Replication Build
→ []EntityTransform
→ Envelope
→ Connection.TrySend
→ asynchronous mailbox / writer
```

`TrySend` 返回不代表 writer 已 encode，所以 Replication 不能直接覆寫 `WorldSnapshot.Entities` backing array。若只把 `make([]EntityTransform, ...)` 改成 reused slice，下一 tick 可能在 writer 尚未讀取舊 snapshot 時覆寫資料，造成 silent wire corruption。

因此本階段先把 ownership contract type 化，而不是用 `sync.Pool` 掩蓋 lifetime 問題。

## Immediate Realtime Ownership Capability

原 `session.Connection` contract 保留：

```go
type Connection interface {
    TrySend(protocol.Envelope) error
    Close() error
}
```

新增 optional capability：

```go
type ImmediateRealtimeConnection interface {
    Connection
    RealtimeConsumedBeforeReturn()
}
```

只對 `DeliveryRealtimeSequenced` 表示：

```text
TrySend success returns
→ realtime message 已 materialize / encode / copy 到 Connection-owned storage
→ Connection 不再引用 caller mutable backing storage
→ caller 可立即重用 snapshot scratch
```

ReliableOrdered 不受此 capability 影響。

`session.QueueConnection` 仍直接保存 Envelope，因此刻意不實作這個 capability；generic / test connection 仍走 owned snapshot allocation path。

## TCP/UDP Mailbox Ownership

production `tcpudp.clientConnection` 實作 `ImmediateRealtimeConnection`。

Realtime mailbox 不再保存 caller 的 Realtime Envelope，而是保存 connection-owned bounded packet slots：

```text
latest PositionCorrection packet slot

current WorldSnapshot set
├── chunk 0 packet slot
├── chunk 1 packet slot
└── ...
```

`TrySend` 成功返回前，ASTU + ASTR + GameV1 已 materialize 到 mailbox-owned storage。writer 會在 mailbox lock 內複製到自己的 reusable 1200-byte scratch，再於 lock 外 `WriteToUDP`。

這保留了以下不變量：

- World Owner 不做 blocking socket I/O
- caller 在 TrySend 返回後可重用 transform storage
- mailbox 可 coalesce / replace 舊 snapshot set
- writer 不會讀到被下一 tick 覆寫的 slot
- 每 datagram 不需要新的 heap packet buffer

Regression test 會在 `PutEncoded` 返回後立即修改 caller 的 `[]EntityTransform`，再 decode mailbox packet；wire bytes 必須仍保持 Put 時的原值。

## Replication Borrowed Snapshot Path

Replication 保留兩條 path：

```text
BuildFrame
→ generic Connection
→ owned []EntityTransform

BuildFrameBorrowed
→ ImmediateRealtimeConnection only
→ per-session reusable []EntityTransform
```

Runtime 只有在 Connection 明確宣告 capability 時才走 borrowed path，因此 lifetime 假設不是 hidden convention。

另有 regression test 要求連續 `BuildFrameBorrowed` 在 capacity 足夠時重用同一 backing array。

## Dense AOI Membership Buffer Reuse

`rebuildDesiredTracks` 原本在 membership 改變時配置新的：

```text
desiredIDs
tracks
```

S3-E.4 改為 resize / clear 既有 dense buffers，再從 lifecycle/map mirrors 重建 rare-path state。容量足夠時不更換 backing array。

帶 profiler 的實作 run 中，`rebuildDesiredTracks` 已由約：

```text
19.99 MB flat
```

下降到：

```text
約 2.55 MB cumulative
```

此 profile 只用來確認 hot path 被命中，不作 capacity gate。

## 第一輪無 profiler 500：allocation 成功、p99 邊界失敗

ownership reuse 後的第一輪正式 500 run：

```text
Tick p95                    20.05 ms
Tick p99                    50.138 ms
TotalAlloc                   85.02 MB
Mallocs                       0.996 M
Realtime throughput          53.04 Mbit/s
```

相對 S3-E.3，TotalAlloc 已下降約 52%，但 `p99 = 50.138ms` 超過 50ms，因此沒有把它四捨五入成 PASS。

第二次無 profiler hard-gate run 又觀察到：

```text
Tick p99                    65.934 ms
TotalAlloc                   66.82 MB
Realtime throughput          53.27 Mbit/s
```

這證明不能只用「單次 Hosted Runner noise」解釋，需要直接拆 slow tick stage。

## Slow Tick Stage Breakdown

Load Server 新增固定上限的 top slow-tick report，不改 simulation/wire，也不在 hot path 做無界 allocation。

第三個無 profiler樣本通過 hard gate：

```text
Tick avg                     9.03 ms
Tick p95                    19.80 ms
Tick p99                    32.44 ms
TotalAlloc                   48.89 MB
```

但最慢單 tick 仍是 68.47ms。stage breakdown：

```text
Tick 124 total              68.47 ms
Replication Build           61.53 ms
Delivery                     3.33 ms
AOI                          1.28 ms
Vitals                       1.45 ms
```

因此 synchronous ownership handoff 並不是主要 spike；尖峰仍在 `Replication Build`。

該 tick 同時具有：

```text
Snapshot candidates         28,393
Snapshot transforms         20,908
Snapshot deferred            7,485
```

這代表大量 Session 正在 bootstrap / convergence，且候選數超過每 Session 64-transform budget。

## 64-budget Full Sort 問題

原 scheduler 在 candidates 超過 budget 時會：

```text
all due candidates
→ full sort by normalized overdue fairness
→ take first 64
```

在 hotspot convergence 中，一個 Session 可同時有接近 500 candidates，但實際只需要最佳 64 個。Full sort 做了不必要的 `N log N` 工作。

S3-E.4 改成 bounded top-K heap selection：

```text
all due candidates
→ maintain best 64 only
→ heap root = current worst selected candidate
→ better candidate replaces root
→ final selected transforms 再依 EntityID 排序供 deterministic wire order
```

Priority comparator 完全不變：

1. normalized overdue fairness
2. dirty 優先
3. Near / Mid / Far tier
4. age
5. EntityID deterministic tie-break

單元測試會把 bounded top-K 的選出集合與舊 full sort 的前 64 名直接比較，要求完全等價；另驗證 top-K buffer 可重用。

## 最終 24-client Vertical Siege

| 指標 | S3-E.3 | S3-E.4 final |
|---|---:|---:|
| Tick avg | 0.084 ms | 0.121 ms |
| Tick p99 | 0.202 ms | 0.330 ms |
| TotalAlloc | 4.62 MB | 4.09 MB |
| Mallocs | 27,594 | 20,423 |
| Realtime | 0.662 Mbit/s | 0.662 Mbit/s |

Errors / delivery / MTU 全部為 0。

## 最終 100-client Gate Zerg

| 指標 | S3-E.3 | S3-E.4 final |
|---|---:|---:|
| Tick avg | 0.615 ms | 0.635 ms |
| Tick p99 | 4.349 ms | 3.257 ms |
| TotalAlloc | 16.70 MB | 10.90 MB |
| Mallocs | 171,645 | 122,818 |
| Realtime | 3.919 Mbit/s | 3.919 Mbit/s |

TotalAlloc 約再下降 35%，wire throughput 基本不變。

## 最終 500-client Gate Zerg

最終 top-K + ownership exact code head：

```text
measurement                 10.001s
completed ticks                 199
Tick avg                     9.224 ms
Tick p50                    14.800 ms
Tick p95                    18.545 ms
Tick p99                    31.018 ms
Tick max                   110.755 ms

AOI avg                      0.755 ms
Replication Build            7.254 ms
Delivery                     0.575 ms
Vitals                       0.022 ms

TotalAlloc                   46.09 MB
Mallocs                       0.759 M
Realtime throughput          53.224 Mbit/s
Encode avg                   0.347 us/datagram
```

相對 S3-E.3：

| 指標 | S3-E.3 | S3-E.4 final | 結果 |
|---|---:|---:|---:|
| Tick p99 | 19.825 ms | 31.018 ms | 仍 < 50ms |
| TotalAlloc | 178.39 MB | 46.09 MB | **約 -74%** |
| Mallocs | 1.251M | 0.759M | **約 -39%** |
| Realtime throughput | 53.241 Mbit/s | 53.224 Mbit/s | 約相同 |
| Encode avg | 0.48 us | 0.347 us | 約 -28% |

500 correctness：

```text
connected / ready              500 / 500
Spawn                    250,000 / 250,000
Reliable messages            500,500
Decode errors                      0
Network errors                     0
Delivery errors                    0
Datagram too large                 0
Incomplete snapshot resets         0
```

## Capacity Gate

S3-E.4 將 500 workflow 正式加入硬性 assertion：

```text
Tick p99 < 50 ms
```

最終 code head：

```text
p99 = 31.018 ms < 50 ms
```

因此 **目前 500-client Gate Zerg 20Hz capacity gate PASS**。

但單次 max 仍有 `110.755ms` spike。slow-tick report 顯示該筆仍主要來自 Replication Build：

```text
Tick 130 total             110.75 ms
Replication Build           98.22 ms
Delivery                     5.10 ms
AOI                          3.24 ms
Vitals                       3.11 ms
```

因此這個 PASS 只代表目前 regression 的 p99 capacity gate，不是宣告所有地圖、所有 bootstrap pattern、所有 gameplay workload 或 Internet production deployment 已具備 500-player SLA。

## 本階段沒有做的事

- 不升 Protocol version
- 不做 transform quantization
- 不做 baseline / delta payload
- 不提高 MTU
- 不放大 Reliable queue
- 不用 `sync.Pool` 掩蓋 ownership 問題
- 不拆 Cell Actor
- 不改 Client interpolation / lifecycle contract

## S3-E.4 結論

實測支持：

1. 剩餘 allocation 的主要來源在 Replication ownership / membership buffers，而不是 realtime wire encode/decode。
2. generic asynchronous `Connection` 必須維持 owned snapshot storage。
3. production TCP/UDP connection 可用明確的 immediate-realtime capability 安全啟用 borrowed snapshot path。
4. AOI membership rare-path 可重用 dense buffers，而不改 Reliable lifecycle truth。
5. 64-transform scheduler 不需要 full sort 全部 candidates；bounded top-K 可保留完全相同的 fairness selection。
6. 500-client TotalAlloc 由 S3-E.3 的約 178 MB 再降到約 46 MB，wire throughput 仍約 53 Mbit/s。
7. 500 hard gate 已正式把 `p99 < 50ms` 寫入 workflow，最終 run 為 31.018ms PASS。
8. max spike 仍存在，且 slow-tick 證據指向 bootstrap / convergence 時的 Replication Build；後續不能把這個風險隱藏成「已完全解決」。

下一階段不應自動進 Quantized / Delta。新的證據更支持先把 **bootstrap / AOI convergence burst** 與 **steady-state siege** 分開量測，必要時再優化 Reliable lifecycle convergence、candidate construction 或 readiness semantics；若目標轉為 Internet egress / 更大戰場頻寬，才重新評估 Quantized / Delta。

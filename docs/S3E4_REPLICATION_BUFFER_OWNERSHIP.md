# S3-E.4：Replication Buffer Ownership Reuse

S3-E.4 延續 S3-E.3 的量測結果，不預設 Quantized / Delta，也不拆 mutable World ownership。這一階段先用 measurement-window allocation profile 找出 500-client Gate Zerg 剩餘 allocation 的實際來源，再針對 ownership boundary 做最小改動。

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

S3-E.3 已把 realtime writer 從：

```text
payload allocation
→ ASTR frame allocation
→ ASTU datagram allocation
```

收斂成 append-based single-pass encode 與 per-writer reusable MTU buffer。

500-client Gate Zerg 的 S3-E.3 代表性結果：

```text
Tick avg                  7.732 ms
Tick p99                 19.825 ms
Replication Build         6.194 ms
TotalAlloc               178.39 MB / 10s
Mallocs                    1.251 M
Realtime throughput       53.241 Mbit/s
Encode avg                 0.48 us/datagram
```

因此 S3-E.4 不再猜測 wire encode，而是直接 profile 剩餘 allocation。

## Measurement-window Allocation Profile

`cmd/loadserver` 新增可選 allocation profiler：

```text
-alloc-profile-prefix
-alloc-profile-rate
```

指定 prefix 時，Load Lab 在所有 Client ready 後、正式 measurement 開始前寫一份 `allocs` profile，measurement 結束後再寫一份；診斷 workflow 使用 `go tool pprof -base` 取得 measurement-window allocation delta。

這個 profiler 只作診斷。正式 24 / 100 / 500 acceptance run 不開 profiler，避免 sampling 對 tail latency 造成干擾。

### 500-client 診斷結果

S3-E.3 code path 的 sampled `alloc_space`：

| 函式 | Flat alloc | 比例 |
|---|---:|---:|
| `replication.(*Service).buildFrame` | 36.53 MB | 48.87% |
| `replication.rebuildDesiredTracks` | 19.99 MB | 26.75% |
| `transport.EncodeFrame` | 5.88 MB | 7.87% |
| `encoding/json.Marshal` | 5.82 MB | 7.78% |

`buildFrame + rebuildDesiredTracks` 合計約 **75.7% sampled alloc_space**。

因此數據沒有指向 UDP ingress decode，也沒有要求修改 wire；最明確的問題是 Replication 建出的資料在 `TrySend` 後仍可能被 asynchronous transport 持有，迫使 Server 每 Session / snapshot pass 配置新的 backing storage。

## 問題：TrySend 後到底誰擁有 Snapshot backing storage？

S3-E.3 前的正確保守規則是：

```text
Replication Build
→ []EntityTransform
→ Envelope
→ Connection.TrySend
→ asynchronous mailbox / writer
```

`TrySend` 返回不代表 writer 已 encode，因此 Replication 不能直接覆寫 `WorldSnapshot.Entities` backing array。

如果只把：

```go
make([]protocol.EntityTransform, selectedCount)
```

改成 per-session reused slice，下一 tick 可能在 writer 尚未讀取舊 snapshot 時覆寫同一塊記憶體，造成 silent wire corruption。

所以本階段不是單純「加 buffer pool」，而是先把 ownership contract 明確化。

## Optional Immediate Realtime Ownership Capability

`session.Connection` 原 contract 保留：

```go
type Connection interface {
    TrySend(protocol.Envelope) error
    Close() error
}
```

S3-E.4 另外增加 optional capability：

```go
type ImmediateRealtimeConnection interface {
    Connection
    RealtimeConsumedBeforeReturn()
}
```

語意只適用 `DeliveryRealtimeSequenced`：

```text
TrySend success returns
→ message 已被 materialize / encode / copy 到 Connection-owned storage
→ Connection / writer 不再引用 caller mutable backing storage
→ caller 可以立即重用 snapshot scratch
```

ReliableOrdered 不受影響。

`session.QueueConnection` 仍直接保存 Envelope，因此刻意**不**實作這個 capability；generic / test connection 仍走 owned snapshot allocation path。

## TCP/UDP Connection：TrySend 時取得 Realtime Ownership

production `tcpudp.clientConnection` 實作 `ImmediateRealtimeConnection`。

S3-E.3 是：

```text
World Owner
→ TrySend(Envelope)
→ mailbox retains Envelope
→ writer
→ encode into writer reusable MTU buffer
→ WriteToUDP
```

S3-E.4 改成：

```text
World Owner
→ TrySend(Envelope)
→ mailbox lock
→ encode ASTU + ASTR + GameV1
→ connection-owned bounded packet slot
→ TrySend returns

writer
→ copy mailbox slot into writer-owned reusable MTU buffer
→ unlock mailbox
→ WriteToUDP
```

這裡刻意保留一次 bounded packet copy。原因是 mailbox slot 可能被下一 tick 覆寫，而 `WriteToUDP` 必須在 lock 外執行；writer 先在 lock 內複製到自己專屬的 1200-byte scratch，就可以同時滿足：

- World Owner 不做 blocking socket I/O
- TrySend 返回後 Replication 可重用原 transform storage
- mailbox 可 coalesce / replace 舊 snapshot set
- writer 不會讀到被 producer 覆寫的 slot
- 每 datagram 不需要 heap allocate packet buffer

## Mailbox Ownership

`realtimeMailbox` 不再保存 Realtime Envelope，而是保存 encoded packet slots：

```text
latest PositionCorrection slot

current WorldSnapshot set
├── chunk 0 slot
├── chunk 1 slot
└── ...
```

每個 slot 是 bounded `MaxDatagramSize` storage，並保留 message type / encode duration 供 metrics 使用。

原本的 semantic stream contract不變：

- correction 只保留最新
- snapshot 以 Tick / ChunkCount 為 set
- 新 tick chunk 0 可以取代舊的未送完 set
- Client 仍只有收齊同 Tick 全部 chunks 才 commit batch

新增 regression test 會在 `PutEncoded` 返回後立即修改 caller 的 `[]EntityTransform`，再 decode mailbox packet；wire bytes 必須仍保持 Put 時的原值，以驗證 Connection 沒有偷留 caller backing storage。

## Replication Borrowed Snapshot Path

`replication.Service` 保留兩條 path：

```text
BuildFrame
→ generic Connection
→ owned []EntityTransform

BuildFrameBorrowed
→ ImmediateRealtimeConnection only
→ per-session reusable []EntityTransform
```

Runtime 只在 Connection 明確實作 capability 時使用 borrowed path。

因此 lifetime 假設不是 hidden convention，而是 type-level capability boundary。

另外新增 regression test，要求連續 `BuildFrameBorrowed` 在容量足夠時重用相同 backing array。

## Dense AOI Membership Buffer Reuse

Profile 另外指出 `rebuildDesiredTracks` 單獨約 20 MB sampled alloc_space。

S3-E.2 原本在 AOI membership 改變時會建立：

```text
new desiredIDs
new tracks
```

S3-E.4 改成 reusable dense buffers：

```text
resize / clear existing desiredIDs
resize / clear existing tracks
→ 從 known + delivered-generation mirrors 重建 rare-path state
```

容量足夠時不換 backing array；只有 membership 超過既有 capacity 時才擴張。

這不改 lifecycle truth：`known` map 仍是 Reliable Spawn / Despawn 的 authoritative knowledge。

## 診斷後 Profile 變化

帶 profiler 的實作 run 中，`rebuildDesiredTracks` 已由原本：

```text
19.99 MB flat
```

下降到約：

```text
2.55 MB cumulative
```

此數據只用來確認 hot path 被命中，不拿來做 capacity gate，因為 profiler sampling 會擾動 tail latency。

## 無 Profiler 24-client Vertical Siege

S3-E.3 → S3-E.4 第一輪正式 run：

| 指標 | S3-E.3 | S3-E.4 | 變化 |
|---|---:|---:|---:|
| Tick avg | 0.084 ms | 0.131 ms | runner noise |
| Tick p99 | 0.202 ms | 0.438 ms | runner noise |
| TotalAlloc | 4.62 MB | 4.07 MB | 約 -11.9% |
| Mallocs | 27,594 | 20,334 | 約 -26.3% |
| Encode avg | 0.32 us | 0.307 us | 約 -4% |
| Realtime throughput | 0.662 Mbit/s | 0.660 Mbit/s | 約相同 |

24-client workload 本來就很輕；主要確認 correctness 與低規模沒有 allocation regression。

## 無 Profiler 100-client Gate Zerg

| 指標 | S3-E.3 | S3-E.4 | 變化 |
|---|---:|---:|---:|
| Tick avg | 0.615 ms | 0.543 ms | 約 -11.7% |
| Tick p99 | 4.349 ms | 4.612 ms | 約 +6.0% |
| TotalAlloc | 16.70 MB | 10.92 MB | **約 -34.6%** |
| Mallocs | 171,645 | 123,621 | **約 -28.0%** |
| Encode avg | 0.43 us | 0.209 us | 約 -51% |
| Realtime throughput | 3.919 Mbit/s | 3.919 Mbit/s | 約相同 |

wire volume 沒有被縮減；allocation 下降來自 ownership / buffer reuse。

## 無 Profiler 500-client Gate Zerg：第一輪

第一輪無 profiler run：

```text
measurement                10.004s
completed ticks                 200
Tick avg                    10.54 ms
Tick p95                    20.05 ms
Tick p99                    50.138 ms
Replication Build            8.20 ms
Delivery                     0.749 ms
TotalAlloc                   85.02 MB
Mallocs                       0.996 M
Realtime throughput          53.04 Mbit/s
Encode avg                   0.364 us/datagram
```

相對 S3-E.3：

| 指標 | S3-E.3 | S3-E.4 第一輪 | 變化 |
|---|---:|---:|---:|
| TotalAlloc | 178.39 MB | 85.02 MB | **約 -52.3%** |
| Mallocs | 1.251M | 0.996M | **約 -20.4%** |
| Realtime throughput | 53.241 Mbit/s | 53.044 Mbit/s | 約 -0.4% |
| Encode avg | 0.48 us | 0.364 us | 約 -24% |

500 correctness 仍成立：

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

但 `p99 = 50.138 ms` 比既定 20Hz capacity gate 的 50ms 高 0.138ms，因此**這一輪不單獨判定 capacity PASS**。

它的 p95 只有約 20ms、GC pause 只有約 0.079ms，尾端存在少數 Hosted Runner spike；無論原因是 runner noise 或 implementation tail cost，都不應把 50.138ms 四捨五入成 PASS。

因此 S3-E.4 把 500 workflow 正式加入：

```text
Tick p99 < 50 ms
```

硬性 assertion。PR 只有在後續無 profiler exact-head run 真正通過這個 assertion 後才可 merge。

## Allocation 與 CPU 的取捨

S3-E.4 將 realtime encode 從 writer goroutine 移到 `TrySend` ownership handoff，因此 encode cost 現在屬於 World Owner 的 Delivery stage，而不是 writer background cost。

這是刻意且可量測的 tradeoff：

- 好處：Replication snapshot backing storage 可安全重用，大幅降低 heap churn
- 成本：Delivery stage 多了 bounded encode + mailbox packet copy

第一輪 500 run 的 Delivery 約 0.75ms/tick，仍遠小於 50ms tick budget；但最終是否接受仍由完整 p99 gate 決定，而不是只看平均值。

## 本階段沒有做的事

- 不升 Protocol version
- 不做 transform quantization
- 不做 baseline / delta payload
- 不提高 MTU
- 不放大 Reliable queue
- 不用 `sync.Pool` 掩蓋 ownership 問題
- 不拆 Cell Actor
- 不改 Client interpolation / lifecycle contract

## S3-E.4 決策

目前數據支持：

1. 剩餘 Server allocation 的主要問題在 Replication ownership，而不是 wire encode / decode。
2. generic asynchronous `Connection` 必須維持 owned snapshot storage。
3. production TCP/UDP connection 可以透過明確的 immediate-realtime capability 安全啟用 borrowed buffer path。
4. AOI membership rare-path 可以重用 dense buffers，而不改 Reliable lifecycle truth。
5. 500-client TotalAlloc 已再下降約一半，且 wire throughput 維持約 53 Mbit/s。
6. 是否維持 20Hz capacity gate，必須由無 profiler `p99 < 50ms` 的最終 workflow assertion 決定。

下一階段不應自動進 Quantized / Delta。若 S3-E.4 最終 capacity gate 通過，新的 profile 已顯示剩餘 allocation 更偏向 Reliable bootstrap / lifecycle / vitals 與 generic JSON encode；後續應先區分「登入 / AOI convergence burst」與「steady-state siege」的目標，再決定是否值得優化 Reliable bootstrap serialization、measurement readiness gate，或轉向 Internet egress / 大型戰場 bandwidth。

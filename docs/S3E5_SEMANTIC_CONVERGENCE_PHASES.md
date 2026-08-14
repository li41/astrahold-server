# S3-E.5：Semantic Convergence / Steady-State Load Phases

S3-E.5 延續 S3-E.4 的 500-client capacity 驗證，但不把下一步預設成 Protocol 壓縮或新的 Replication scheduler。這一階段先修正 Load Lab 的 workload phase 定義：**Session ready 不等於 lifecycle / Reliable state 已經收斂**。

本階段不改：

- Protocol v6
- ASTU / ASTR / GameV1 wire layout
- `EntityTransform` layout
- 1200-byte UDP MTU
- WorldSnapshot partial update batch semantics
- Reliable Spawn / Despawn lifecycle truth
- 64 transforms / Session / build budget
- Client contract

因此 Client 不需要修改，也沒有 Client PR。

## 問題：ready 不是 converged

S3-E.4 以前，Load Server 的 measurement window 是：

```text
all clients ReadyPeerCount == expected
        ↓
collector.Reset()
        ↓
開始量測
```

但 Load Bot 的 `ready` / Server 的 `ReadyPeerCount` 代表的是 bootstrap transport 已建立：

```text
SessionWelcome 已收到
UDP endpoint 已建立 / Join 已排入
```

此時大量 state 仍可能正在收斂：

```text
Reliable EntitySpawn
Reliable EntityVitalsState
Reliable WorldDynamicState
Realtime first snapshots
Reliable TCP queue drain
```

因此舊 measurement window 同時混入：

1. 500 人同時進入世界的 bootstrap / convergence burst
2. 已經穩定存在世界中的 steady-state siege

兩者是不同 workload，不應用同一個 p99 / max 代表。

## S3-E.5 Phase Model

Load Lab 改成兩段：

```text
all clients ready
        ↓
[ Convergence Phase ]
application lifecycle convergence
+ Reliable transport drain
+ stable window
        ↓
semantic convergence
        ↓
Reset metrics / network counters / slow ticks
        ↓
[ Steady-State Phase ]
固定 duration capacity measurement
```

輸出：

- `server-convergence.json`：all-ready → semantic convergence
- `server-convergence-slow-ticks.json`：convergence burst 最慢 ticks
- `server.json`：semantic convergence 後的 steady-state
- `server-slow-ticks.json`：steady-state 最慢 ticks

Optional allocation pprof 也只包住 steady-state window，不再把 bootstrap allocation 混進去。

## Semantic Convergence Gate

### Application-side

`Runtime.ConvergenceSnapshot()` 只在 world owner goroutine 的 `Loop.RunObserved` callback 中取樣，而且只在 convergence tracker 啟用期間、snapshot tick 上執行完整 scan。

必須同時成立：

- Replication views == expected Sessions
- 每個 view 至少完成一次 replication Build
- desired relationships == known desired
- pending Spawn == 0
- pending Despawn == 0
- pending Vitals Sessions == 0
- pending Vitals Entities == 0
- dirty Vitals Entities == 0
- pending Dynamic Sessions == 0

這裡的 `known` 仍維持既有 lifecycle 定義：Reliable Spawn 已成功進入該 Session outbound ownership boundary。

### Transport-side

Application-side 清空仍不代表 TCP writer 已經把最後一筆 Reliable message 寫完，因此另外要求：

- ready peers == expected peers
- Reliable channel queued == 0
- Reliable writer in-flight == 0

`clientConnection` 用 atomic `reliableInFlight` 區分「已從 channel dequeue、但 `WriteEnvelope` 還沒完成」的狀態。

### Stable window

上述 Application + Transport 條件必須連續維持一段時間才算 convergence；CI 預設為 250ms。

這不是任意 warm-up sleep：條件本身是 lifecycle / queue state，stable window 只是避免剛好在兩個 tick / writer transition 之間誤判。

## Convergence Gate 的界線

這個 gate 能證明的是 Server 端：

- lifecycle state 已收斂
- Vitals / Dynamic pending 已清空
- Reliable outbound queue / in-flight write 已 drain

它**不是 wire-level ACK protocol**，因此不能聲稱在 gate 瞬間每個 Client application thread 都已經處理最後一筆 message。

Load Bot 最終 correctness report 仍另外驗證完整接收數量；例如 500-client Gate Zerg 最終為 250,000 Spawn、500,500 Reliable messages。

## Load Bot shutdown error accounting

分相後 Bot 必須活到 Server 完成 convergence + steady-state，最後通常由 Server 正常關閉連線結束 Bot。

在 Linux loopback，Server shutdown 可能造成 UDP `ECONNREFUSED` 比 TCP EOF 更早被 Bot goroutine 看見。若直接在 UDP send/read error 當下記 `network_errors`，正常 shutdown 會產生假陽性。

S3-E.5 使用 bounded 50ms correlation window：

```text
UDP send/read error
        ↓
TCP bot context 在 50ms 內也結束？
  yes → 正常 coordinated shutdown，不計 network error
  no  → 真實 UDP failure，network_errors +1
```

這不是 blanket ignore：TCP control context 若持續存活，UDP failure 仍必須被 gate 捕捉。對應單元測試同時驗證 suppression 與真實 error accounting。

## Final Exact-Head Validation

最終 runtime / test code head：

```text
f065c0bcbed08567785a926ddb6555ddc041aa8e
```

此 head：

- Server CI run 146：PASS（含 test / vet / race detector）
- Siege Load Lab run 125：PASS（24 / 100）
- S3-E Scaling 500 run 53：PASS

### 24-client Vertical Siege

| 指標 | Convergence | Steady-State |
|---|---:|---:|
| ready → converged | 0.375 s | — |
| Tick p99 | 0.397 ms | 0.264 ms |
| Tick max | 0.397 ms | 0.391 ms |
| Replication Build avg | 0.059 ms | 0.041 ms |
| TotalAlloc | 0.38 MB | 3.98 MB / 5s |
| Realtime | 0.800 Mbit/s | 0.627 Mbit/s |

Correctness：24/24 ready、576 Spawn、1,176 Reliable、incomplete reset / decode / network error 全 0。

### 100-client Gate Zerg

| 指標 | Convergence | Steady-State |
|---|---:|---:|
| ready → converged | 0.475 s | — |
| Tick p99 | 5.206 ms | 1.593 ms |
| Tick max | 5.206 ms | 2.155 ms |
| Replication Build avg | 1.447 ms | 0.305 ms |
| TotalAlloc | 1.26 MB | 10.30 MB / 8s |
| Realtime | 13.062 Mbit/s | 3.281 Mbit/s |

Correctness：100/100 ready、10,000 Spawn、20,100 Reliable、incomplete reset / decode / network error 全 0。

### 500-client Gate Zerg

#### Convergence Phase

```text
ready -> converged       0.8257 s
stable window            0.2501 s
Tick avg                18.260 ms
Tick p50                 5.522 ms
Tick p95 / p99 / max   102.946 ms
Replication Build avg   13.941 ms
TotalAlloc              21.15 MB
Mallocs                130,361
Realtime                49.205 Mbit/s
```

Convergence 結束時：

```text
Replication views        500 / 500
Built views              500 / 500
Desired / known      250,000 / 250,000
Pending Spawn                  0
Pending Despawn                0
Pending Vitals                 0
Dirty Vitals                   0
Pending Dynamic                0
Reliable queued                0
Reliable in-flight             0
```

最慢 convergence tick：

```text
total                    102.946 ms
Replication Build         85.045 ms
Vitals Replication        10.836 ms
Delivery                   3.935 ms
AOI                        1.854 ms
snapshot candidates       38,782
snapshot selected         25,301
snapshot deferred         13,481
```

所以 500 人同時 lifecycle convergence 仍然是獨立的 burst workload；S3-E.5 沒有把它假裝成 steady-state，也沒有宣告它符合 50ms p99。

#### Steady-State Phase

```text
measurement               10.000 s
ticks                           200
Tick avg                    9.398 ms
Tick p50                    0.888 ms
Tick p95                   20.770 ms
Tick p99                   21.914 ms
Tick max                   23.337 ms
Replication Build avg       7.353 ms
Delivery avg                0.581 ms
TotalAlloc                  39.16 MB
Mallocs                    736,328
GC count / pause             0 / 0
Realtime                    53.217 Mbit/s
Encode avg                   0.369 us/datagram
```

500-client final Bot correctness：

```text
connected / ready              500 / 500
Spawn                    250,000 / 250,000
Reliable messages            500,500
Dynamic states                   500
Completed snapshots             76,327
Incomplete snapshot resets           0
Decode errors                       0
Network errors                      0
Server delivery/network/MTU errors   0
```

500 workflow 的 `< 50ms` capacity gate 現在只對 semantic convergence 後的 `server.json` steady-state phase 生效；最終 p99 21.914ms，PASS。

## 與 S3-E.4 數字的解讀

S3-E.4 的 500-client 10s measurement 是 all-ready 後立刻開始，會把 bootstrap / lifecycle burst 混在 window 內：

```text
S3-E.4 mixed p99       31.018 ms
S3-E.4 mixed max      110.755 ms
S3-E.4 TotalAlloc      46.09 MB
```

S3-E.5 steady-state：

```text
p99                     21.914 ms
max                     23.337 ms
TotalAlloc               39.16 MB
```

**這不應描述成 S3-E.5 把 runtime p99 加速 29% 或把 max 加速 79%。**

主要改變是 measurement semantics：S3-E.5 把原本混入同一個統計分布的 convergence spike 移到獨立 phase。S3-E.5 沒有改 Replication selection algorithm 或 wire payload。

真正可以下的結論是：

- 目前 Gate Zerg steady-state 500-client p99 有明顯餘裕低於 50ms tick budget。
- 500 simultaneous bootstrap / AOI lifecycle convergence 仍存在 >100ms burst tick。
- 這兩個 workload 應分開設 capacity / latency 目標。

## S3-E.5 決策

1. 保留 Protocol v6。
2. Client 不需修改。
3. 500 `p99 < 50ms` capacity gate 改以 steady-state phase 為準。
4. convergence correctness 另有 semantic + Reliable drain gate，不能用固定 sleep 取代。
5. convergence slow-tick report 保留，不能因 steady-state 很漂亮就隱藏 bootstrap spike。
6. 不根據 S3-E.5 自動進 Quantized / Delta。

## 下一階段方向

如果下一階段目標是**登入潮、Teleport、AOI 大量切換、城戰瞬間聚集**的 latency，資料已指向新的獨立方向：Bootstrap / Lifecycle Work Budget。

應優先量測 / 設計：

- 每 tick Reliable Spawn / Vitals materialization budget
- lifecycle convergence 的 fairness / completion time
- 大量 Session 同時進入 AOI 時的 Replication Build 工作分攤
- convergence p95 / p99 與 ready→converged time 的雙 gate
- 是否需要把 Spawn/Vitals bootstrap 從 steady transform scheduling 拆成 bounded work queue

這些都可以先維持 Protocol v6。只有未來 WAN bandwidth / egress 或 wire size 成為實測瓶頸時，才需要重新評估 Quantized / Delta。

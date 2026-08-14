# S3-E.6：Lifecycle Work Budget / Convergence Tail

S3-E.6 延續 S3-E.5 已拆分的兩種 workload：

- bootstrap / lifecycle convergence
- semantic convergence 後的 steady-state siege

S3-E.5 已證明 500-client Gate Zerg steady-state 有充足 20Hz tick budget，但 500 人同時進入 AOI 時仍存在約 103ms 的 convergence burst。S3-E.6 的目標是直接改善這個 burst workload，而不是再改 measurement semantics。

本階段維持：

- Protocol v6
- GameV1 / ASTR / ASTU layout
- 1200-byte UDP MTU
- Reliable Spawn / Despawn lifecycle truth
- WorldSnapshot partial update batch semantics
- 64 remote transforms / Session / build
- Client contract

因此沒有 Client 變更，也沒有 Client PR。

## Baseline：S3-E.5

500-client Gate Zerg：

```text
ready -> converged       0.8257 s
convergence p99 / max  102.946 ms
slowest RepBuild         85.045 ms
slowest Vitals           10.836 ms
slowest Delivery          3.935 ms

steady p99               21.914 ms
steady max               23.337 ms
```

Convergence 結束時 semantic correctness 為：

```text
views / built               500 / 500
desired / known       250,000 / 250,000
pending Spawn                   0
pending Despawn                 0
pending Vitals                  0
pending Dynamic                 0
Reliable queued / in-flight     0 / 0
```

因此瓶頸不是 correctness，而是大量可重建 lifecycle 工作在少數 world ticks 聚集。

## 問題 1：產生了當下送不出去的 Spawn 工作

S3-E.5 的 `buildFrame` 對每個 unknown desired Entity 都先 materialize `EntitySpawn`：

```text
500 Sessions
  x 最多 500 desired entities
  -> 大量 Spawn message materialization
  -> TrySend Reliable
  -> outbound queue backpressure
  -> 下一 tick 再重建
```

`ConfirmSpawn` 本來就只在 `TrySend` 成功後更新 `known`，所以 Spawn 是天然可重建、可延後的 state。

S3-E.6 因此把 lifecycle materialization 變成 bounded work，而不是把 queue capacity 當成隱式工作預算。

## Lifecycle work budget

Production Runtime 預設：

```text
Spawn       32 / Session / snapshot build
Despawn     64 / Session / snapshot build
Initial HP  32 / Session / world tick
```

`Build` / `BuildFrame` compatibility API 仍保留 unlimited semantics；只有 production Runtime 明確走 bounded lifecycle path。

這避免改變既有測試或非 production caller 的 API 語意。

### Lifecycle truth 不變

Budget 只決定「本次要 materialize 幾筆」，不決定 lifecycle truth：

```text
desired
  != known
  -> future Spawn work

Spawn TrySend success
  -> ConfirmSpawn
  -> known = true

Spawn deferred / backpressure
  -> 不 Confirm
  -> 下一 build retry
```

Despawn 同理：成功前保持 `known`，讓後續 build 可重建最新 Despawn。

## 第一個 backpressure 後停止無效 lifecycle TrySend

同一 Session / build 若第一筆 lifecycle message 已遇到 `ErrBackpressure`，後續 lifecycle TrySend 不再繼續撞同一個滿 queue。

Realtime path 不受影響：

```text
lifecycle backpressure
  -> stop later Spawn/Despawn TrySend for this Session/build
  -> WorldSnapshot / PositionCorrection 仍可繼續各自既有語意
```

這不是 drop：未 Confirm 的 lifecycle state 仍會在未來 build 重建。

## 問題 2：只限制 Spawn message 數仍不足

第一版使用 64 Spawn / 64 Despawn，但仍讓 partially spawned view 執行完整 remote snapshot candidate scheduling。

第一版 500 結果：

```text
ready -> converged        1.127 s
convergence p99          94.100 ms
slowest RepBuild         62.994 ms
slowest Delivery         13.038 ms
slowest Vitals           15.861 ms
```

結論：限制 message materialization 仍沒有限制整個 bootstrap CPU work。

## Lifecycle-first bootstrap

第二版加入 lifecycle-first path：只要該 Session 仍有 unknown desired Entity，就不做 remote WorldSnapshot candidate scheduling。

```text
view still has unknown desired entity
  -> bounded Spawn / Despawn
  -> self PositionCorrection
  -> no remote snapshot candidate scheduling

all desired known
  -> return to normal BuildFrame
  -> Network LOD / dirty / refresh / top-K 全部恢復
```

這是安全的，因為 `EntitySpawn` 本身已帶 authoritative transform；Client 在 lifecycle 尚未建立前也不應依靠 remote snapshot 建立 entity。

### Initial Vitals 同步 bounded

每個成功 Spawn 都會建立 initial Vitals pending。若 Spawn 被攤平但 Vitals 仍一次追完，burst 只會從 Replication Build 搬到 Vitals stage。

因此 initial Vitals 改為：

```text
最多 32 / Session / tick
第一個 Reliable backpressure 後停止該 Session 本 tick initial Vitals
成功才刪 pending
```

Dirty Vitals fan-out 的 latest full-state retry semantics 不變。

## 64 quantum 的變異

Lifecycle-first + Vitals budget 第一個 500 run：

```text
ready -> converged        1.201 s
convergence p99          34.495 ms
slowest RepBuild         22.849 ms
```

但相同 runtime code 第二次 run：

```text
ready -> converged        1.178 s
convergence p99          62.414 ms
slowest RepBuild         52.623 ms
snapshot candidates           0
```

所以不能只採第一次漂亮結果；64 Spawn quantum 在 lifecycle-only tick 仍可能超過 50ms。

## 32 Spawn quantum

將 Spawn quantum 降為 32 後：

```text
ready -> converged        1.876 s
convergence p99          57.200 ms
slowest RepBuild         43.339 ms
```

完成時間仍在 2 秒附近，但 p99 還沒有穩定低於 50ms。

此時再降低 quantum 會直接消耗 ready→converged latency budget，所以繼續找每 build 仍在做的無效工作。

## 問題 3：為了統計 Deferred，仍掃完整份 AOI

原 lifecycle-first helper 即使已經選滿 32 個 Spawn，仍繼續走完約 500 個 visible tracks，只為計算完整 `SpawnDeferred` cardinality。

這違反 bounded-work 的目的：

```text
work 已達 budget
但 CPU 仍掃完所有 deferred work
```

最終版本改為：

1. 找最早 `unknown` desired track。
2. 從該 index 開始選 Spawn。
3. 選滿 budget 後遇到下一筆 unknown 立即停止。
4. `SpawnDeferred = 1` 只表示「仍有工作」，不再代表完整 deferred cardinality。
5. 下個 build 再找新的 earliest unknown 繼續。

所以 lifecycle-first `SpawnDeferred` 是 **more-work sentinel**，不是 exact backlog size。

這讓 CPU work 本身也受 quantum 約束，而不是只有輸出 message 數受約束。

## Fairness / Progress

在 stable desired membership 下，`desiredIDs / tracks` 是 stable EntityID order；每次從 earliest unknown 往後選，成功 `ConfirmSpawn` 後下一輪 earliest unknown 自然前移。

單元測試鎖住：

```text
5 unknown, budget=2
  build 1 -> Spawn 1,2
  build 2 -> Spawn 3,4
  build 3 -> Spawn 5
```

並驗證：

- bootstrap 期間 unknown entity 不進 remote snapshot
- 所有 desired 都 known 後正常 snapshot 自動恢復
- Despawn budget=2 時 4 departed entity 以 2 / 2 完成
- budget 只能延後，不能提前 Confirm 或造成 static membership starvation

本階段沒有宣稱已解決任意 Teleport / 高 churn 下的 global fairness；那是下一個獨立 workload。

## Capacity gates

S3-E.6 不用單一 latency gate，因為過小 quantum 可以很容易把單 tick 壓低、但讓玩家等很久。

500-client workflow 現在同時要求：

```text
semantic convergence correctness
convergence p99 < 50ms
ready -> converged < 2s
steady-state p99 < 50ms
```

因此必須同時證明：

- lifecycle 最終正確收斂
- convergence tail 在 20Hz tick budget 內
- 工作攤平沒有造成過長 bootstrap latency
- steady siege capacity 沒有退化到 tick budget 外

## Final exact code head

最終 runtime / test / workflow code head：

```text
b290d5b3d0ee7e2981a7c8cb1eaaaa2caebeb19c
```

此 head：

- Server CI run 154：PASS（test / vet / race detector）
- Siege Load Lab run 132：PASS（24 / 100）
- S3-E Scaling 500 run 60：PASS，包含 convergence 雙 gate與 steady gate

## 24-client Vertical Siege

### Convergence

```text
ready -> converged        0.3515 s
Tick p99 / max            0.332 ms
Replication Build avg     0.052 ms
TotalAlloc                0.60 MB
```

### Steady-state

```text
measurement               5.000 s
Tick p99                  0.363 ms
Tick max                  0.506 ms
Replication Build avg     0.036 ms
TotalAlloc                3.98 MB
```

Correctness：24/24 ready、576 Spawn、1,176 Reliable、decode / network / incomplete reset 全 0。

## 100-client Gate Zerg

### Convergence

```text
ready -> converged        0.6754 s
Tick p99 / max            5.636 ms
Replication Build avg     0.992 ms
TotalAlloc                6.54 MB
```

### Steady-state

```text
measurement               8.001 s
Tick p99                  1.654 ms
Tick max                  1.692 ms
Replication Build avg     0.288 ms
TotalAlloc               10.31 MB
```

Correctness：100/100 ready、10,000 Spawn、20,100 Reliable、decode / network / incomplete reset 全 0。

## 500-client Gate Zerg — final convergence

```text
measurement / ready->converged    1.878 s
ticks                                  38
Tick avg                           11.402 ms
Tick p50                            0.376 ms
Tick p95                           36.473 ms
Tick p99                           36.562 ms
Tick max                           36.562 ms
Replication Build avg              8.682 ms
Delivery avg                       0.937 ms
Vitals avg                         1.009 ms
TotalAlloc                       169.57 MB
Mallocs                          534,266
Realtime                          50.954 Mbit/s
```

Convergence 結束時：

```text
views / built                500 / 500
desired / known        250,000 / 250,000
pending Spawn                    0
pending Despawn                  0
pending Vitals                   0
pending Dynamic                  0
Reliable queued / in-flight      0 / 0
```

最慢 tick：

```text
total                       36.562 ms
Replication Build           29.080 ms
Delivery                     2.151 ms
Vitals                       4.019 ms
AOI                          0.799 ms
```

因此 final code 同時通過：

```text
36.562ms < 50ms convergence p99 gate
1.878s   < 2s    convergence completion gate
```

## 500-client Gate Zerg — final steady-state

```text
measurement                10.002 s
ticks                           201
Tick avg                     6.857 ms
Tick p50                     7.586 ms
Tick p95                    16.146 ms
Tick p99                    16.485 ms
Tick max                    16.823 ms
Replication Build avg        5.641 ms
Delivery avg                 0.466 ms
Vitals avg                   0.00025 ms
TotalAlloc                  43.00 MB
Mallocs                    742,928
GC count / pause              0 / 0
Realtime                    53.566 Mbit/s
Encode                       0.286 us/datagram
```

Final Bot correctness：

```text
connected / ready             500 / 500
Spawn                   250,000 / 250,000
Reliable messages           500,500
Dynamic states                  500
Completed snapshots            67,935
Incomplete resets                   0
Decode errors                       0
Network errors                      0
```

Server delivery / network / MTU gate 也全部為 0。

## 與 S3-E.5 的正確比較

這次和 S3-E.5 不同：S3-E.6 確實改了 bootstrap work scheduling，因此 convergence tail 的變化可視為 runtime 行為改善。

```text
S3-E.5 convergence p99     102.946 ms
S3-E.6 final               36.562 ms
                          約 -64.5%

S3-E.5 ready->converged      0.826 s
S3-E.6 final                  1.878 s
                          約 +1.05 s
```

這正是 work budgeting 的 trade-off：把單 tick burst 攤到更多 build，換取 tail latency，並用 `<2s` completion gate 限制延後幅度。

### 不應宣稱 steady-state 加速

S3-E.6 lifecycle-first path 只在 view 尚未完成 Spawn 時生效；all-known 後回到原本 full replication scheduler。

所以 steady p99 從 S3-E.5 的 21.914ms 到本 run 的 16.485ms 不應描述成 S3-E.6 的 25% CPU optimization，主要視為 runner / workload sampling variation。

## Allocation caveat

S3-E.6 final convergence TotalAlloc 為約 169.57MB，高於 S3-E.5 約 21.15MB。

這不代表每 tick 的 lifecycle budget 配置了更多 Spawn message；主要原因之一是 convergence window 被拉長，而且各 Session 在完成 lifecycle 後會於 convergence phase 內陸續進入完整 snapshot scheduler，首次 candidate / transform / map growth 的配置因此落在 convergence 統計中。

本階段沒有以 allocation 為 acceptance 目標，也不把這個數字隱藏。若下一階段要優化 bootstrap memory，需要重新用 measurement-window pprof 對 final scheduling profile，而不能直接沿用 S3-E.4 的 allocation ownership結論。

## S3-E.6 決策

1. 保持 Protocol v6。
2. 保持 1200-byte MTU。
3. Client 不需要修改。
4. Production Spawn quantum = 32 / Session / snapshot build。
5. Production Despawn quantum = 64 / Session / snapshot build。
6. Initial Vitals quantum = 32 / Session / tick。
7. bootstrap view 採 lifecycle-first，全部 desired known 後才恢復 remote snapshot scheduling。
8. lifecycle-first Spawn scan 必須在 quantum 用完後立即停止；Deferred 為 more-work sentinel。
9. 500 convergence 正式保留 `p99 < 50ms` 與 `ready->converged < 2s` 雙 gate。
10. 500 steady-state `p99 < 50ms` gate 繼續保留。
11. 不根據本階段進入 Quantized / Delta 或 Protocol v7。

## 下一階段方向

S3-E.6 已處理 **mass join / bootstrap** 的主要 tail。下一個 lifecycle workload 不應直接假設與 bootstrap 相同，較合理的是新增 **Teleport / AOI Churn** 場景，量測同一批 Session 同時大量 Spawn + Despawn 時：

- Despawn / Spawn ordering 與 fairness
- 第一個 lifecycle backpressure 後 Despawn 是否被後續 build 長時間延後
- per-Session budget 是否需要升級成 global per-tick lifecycle budget
- churn completion p95 / p99
- memory / allocation ownership 是否在 membership rebuild 中重新成為瓶頸

這仍可在 Protocol v6 下完成；只有 WAN bandwidth / wire size 成為實測瓶頸時才重新評估 Quantized / Delta。

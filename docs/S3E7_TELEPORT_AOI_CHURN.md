# S3-E.7：Teleport / AOI Churn

S3-E.7 延續 S3-E.6 的 lifecycle work budget，但 workload 從「500 人首次 mass join」改成：

> **500 個已完成 semantic convergence 的 Session，在同一個 world tick 發生大規模 AOI membership 交換。**

這一階段直接驗證同時 Spawn + Despawn + Initial Vitals 的 ordering、fairness、global work budget 與 transition tail。

本階段維持：

- Protocol v6
- GameV1 / ASTR / ASTU wire layout
- 1200-byte UDP MTU
- Reliable Spawn / Despawn lifecycle truth
- `known` 只在 Spawn `TrySend` 成功後成立
- WorldSnapshot partial update batch semantics
- 64 remote transforms / Session / build
- Client contract

因此沒有 Client 變更，也沒有 Client PR。

## Workload

新增 Load Lab scenario：`teleport-churn`。

500 players 初始分成兩個各 250 人的 cluster：

```text
West cluster                         East cluster
250 players                          250 players
all mutually within AOI              all mutually within AOI
         |                                  |
         +---------- > 64m apart -----------+
```

cluster 放在 castle-sandbox 的 blocker-safe 開放區域；最近距離約 71.6m，大於 64m AOI radius。

初始 semantic convergence 後：

- West 前 125 人 teleport 到 East 對應 slot
- East 前 125 人 teleport 到 West 對應 slot
- 共 250 Entity 同一 owner tick 交換 cluster membership

每個 Session 因此同時：

- 失去約 125 個舊 AOI Entity
- 得到約 125 個新 AOI Entity

500-client transition 的 exact lifecycle 工作量：

```text
new Spawn       62,500
Despawn         62,500
new Vitals      62,500
```

整個 Bot run（含 initial convergence）應收到：

```text
Spawn          187,500
Despawn         62,500
Reliable >=    438,000
Dynamic            500
```

## Atomic authoritative teleport

新增 `simulation.World.Teleport`：

- 更新 movement authoritative position
- 清除 teleport 前 move direction
- 更新 Entity Transform
- 更新 spatial index

Load Lab 不排 250 個獨立 command，而是使用：

```text
Runtime.EnqueueTeleportBatch(...)
```

caller slice 會先複製；整批 transition 以一個 owner-thread command，在同一 world tick、simulation 前完成。

所以 churn report 不混入 command queue 分批套用時間。

## Transition semantic gate

S3-E.5 / E.6 的 initial convergence gate 不能直接重用於已收斂世界，因為 trigger 後 owner tick 尚未套用 command 前仍可能看到舊的 converged snapshot。

S3-E.7 新增 transition gate：

1. trigger teleport batch
2. **必須至少觀察一次 non-converged**
3. 再等待：
   - desired == known
   - pending Spawn == 0
   - pending Despawn == 0
   - pending / dirty Vitals == 0
   - pending Dynamic == 0
   - Reliable queued == 0
   - Reliable in-flight == 0
4. 上述條件連續穩定 250ms 才完成

這避免用 fixed sleep 或 trigger 前的舊 converged state 誤判。

## Baseline：S3-E.6 scheduler

第一個 500-client churn baseline correctness 完整通過：

```text
trigger -> converged      0.751 s
churn p99 / max          79.026 ms
Spawn                    62,500
Despawn                  62,500
backpressure stops            0
errors                         0
```

最慢 tick 約：

```text
Total       79.03 ms
RepBuild    31.11 ms
Delivery    23.40 ms
Vitals      23.09 ms
Spawn       16,000
Despawn     32,000
```

因此 baseline 證明：

- lifecycle ordering 沒有 correctness failure
- 問題是所有 Session 的合法 per-session quantum 疊成 **global burst**

## Global lifecycle budget

S3-E.7 對 production Runtime 加入 global per-snapshot lifecycle budget。

per Session 仍保留：

```text
Spawn       <= 32 / build
Despawn     <= 64 / build
combined    <= 32 / build
```

Runtime 再套 global cap，並以 Session round-robin cursor 輪轉起點。

如果 global budget 在某 Session 後耗盡：

- 下一個 snapshot 從下一個 Session 開始
- 後續 Session 本 tick 仍更新 desired dense membership + self correction
- **不再掃 known / sort departed / materialize lifecycle**

因此 global budget 同時限制 message 數與 CPU work，而不是只限制最後送出的 Envelope。

## Despawn-first mixed churn

若同一 view 同時有 stale known 與 unknown desired：

```text
Despawn -> Spawn
```

先清除 stale Client lifecycle，再使用剩餘 combined quantum 建新 Spawn。

這只影響真正 mixed membership churn；pure mass join 沒有 departed，因此不改 S3-E.6 bootstrap ordering。

## Pending departed reuse

AOI membership 真正移除 Entity 時才掃 `known` 建立 sorted pending departed list。

retry build 不再重複：

```text
scan known
binary-search desired
sort departed
```

而是 prune 已 Confirm / 再次 desired 的 pending ID。

另外，desired **只成長**的 mass join 會先用 stable EntityID two-pointer 判斷舊 desired 是否真的有人離開；若只是 pure growth，完全跳過 departed known scan。

這避免 S3-E.7 rare churn logic 回歸 S3-E.6 mass join hot path。

## Reliable Spawn transform 作為 realtime baseline

`EntitySpawn` 已經攜帶 authoritative transform。

S3-E.7 在 lifecycle-first production path materialize Spawn 時，把該 transform generation 記在仍為 `known=false` 的 dense track：

- `TrySend` 若 backpressure：known 仍是 false，realtime scheduler 不會使用它；retry 會覆寫最新 generation
- `TrySend` 成功後 `ConfirmSpawn` 才把 known=true
- 此時 Spawn 內已送達的 transform 成為 dirty / refresh scheduler baseline

所以 unchanged Entity 不需要在 all-known 後立刻再發一份 redundant WorldSnapshot transform。

真正 transform generation 改變或 periodic refresh 到期後，既有 realtime scheduler仍正常工作。

Wire payload 完全沒有改變。

## Initial Vitals global budget

Initial Vitals 同樣使用 global + per-session fair scheduling：

```text
per Session <= 32 / world tick
```

Dirty gameplay Vitals **不共用 bootstrap/churn global cap**；combat / gameplay latest-full-state retry semantics 不受影響。

## Phase-sensitive budget

一開始嘗試單一 global cap，發現會讓 mass join 與 mixed churn 互相牽制。

最終 Runtime 不是 scenario hard-code，而是看真實 lifecycle state：

- pure bootstrap：沒有 Despawn candidate
- mixed churn：第一個 Despawn candidate 出現後切到 churn mode
- lifecycle deferred == 0 且 initial Vitals pending == 0 後退出 churn mode

最終 budget：

```text
Pure bootstrap
  Lifecycle global     16,000 / snapshot
  Initial Vitals       16,000 / tick

Mixed churn
  Lifecycle global      6,000 / snapshot
  Initial Vitals        2,500 / tick
```

這保留 S3-E.6 mass-join completion，同時把 churn tail 壓進 50ms。

## Data-driven tuning

500-client Teleport Churn 實際調整歷程：

```text
S3-E.6 baseline
  0.751s / p99 79.026ms

Global lifecycle 16k
  p99 73.824ms
  Vitals 成為新 burst

Lifecycle 16k + Vitals 8k
  p99 88.109ms
  Delivery 仍可一次吃 16k lifecycle

Lifecycle 8k + Vitals 4k
  ~1.951s / p99 53.260ms

Phase-sensitive + 7k / 3.5k
  ~2.225s / p99 48.310ms（某輪）
  其他輪仍可約 56ms

Final 6k / 2.5k + pure-growth / departed / Spawn baseline optimizations
  進入 final acceptance
```

因此最後不是單純降低常數，而是同時：

- global message budget
- round-robin fairness
- CPU short-circuit
- pending departed reuse
- pure-growth rare-path bypass
- Spawn transform baseline
- phase-sensitive throughput

## 正式 S3-E.7 gates

`teleport-churn-500` 現在同時要求：

1. 500 / 500 ready
2. exact Spawn = 187,500
3. exact Despawn = 62,500
4. Reliable >= 438,000
5. transition 必須先觀察 non-converged
6. churn 結束時 desired / known = 125,000 / 125,000
7. pending lifecycle / Vitals / Dynamic = 0
8. Reliable queued / in-flight = 0 / 0
9. lifecycle global budget = 6,000，單 tick不可超過
10. initial Vitals budget = 2,500，單 tick不可超過
11. **churn p99 < 50ms**
12. **trigger -> converged < 2.75s**
13. churn 後 steady p99 < 50ms
14. decode / Bot network / Server delivery / Server network / MTU errors = 0

2.75s completion gate 是 tail / transition latency 的雙約束：不能用極小 quantum 無限攤平 burst。

## Final runtime / test code head

Runtime / test 最後一個功能性 commit：

```text
42a9402580f3bdfaf29da0bc33b315e19fe75ea4
```

後續：

- `eeb528c...`：只把 S3-E.7 hard gates 固化進 workflow
- `ff8c492...`：只加 final-repeat 註解，刻意重跑完全相同 gates

## Final acceptance #1

Actions：

```text
Server CI run 172           PASS (test / vet / race)
Siege Load Lab run 150      PASS (24 / 100)
S3-E Scaling 500 run 80     PASS
```

500 Teleport Churn：

```text
trigger -> converged       2.400 s
p99                        37.346 ms
max                        37.346 ms
RepBuild avg                7.893 ms
Delivery avg                2.199 ms
Vitals avg                  2.320 ms
Lifecycle max/tick          6,000
Initial Vitals max/tick     2,500
Spawn                      62,500
Despawn                    62,500
errors                          0
```

Churn 後 steady：

```text
p99                         3.326 ms
max                         4.512 ms
```

Gate Zerg regression：

```text
ready -> converged          1.850 s
convergence p99            44.160 ms
steady p99                 14.153 ms
```

## Final acceptance #2

在 runtime / hard gates 完全不變下刻意重跑：

```text
Server CI run 174           PASS (test / vet / race)
Siege Load Lab run 152      PASS (24 / 100)
S3-E Scaling 500 run 82     PASS
```

500 Teleport Churn：

```text
trigger -> converged       2.375 s
p99                        38.833 ms
max                        38.833 ms
RepBuild avg                7.665 ms
Delivery avg                2.301 ms
Vitals avg                  2.506 ms
Lifecycle max/tick          6,000
Initial Vitals max/tick     2,500
Spawn                      62,500
Despawn                    62,500
errors                          0
```

Churn 後 steady：

```text
p99                         3.807 ms
max                         4.934 ms
```

Gate Zerg regression：

```text
ready -> converged          1.850 s
convergence p99            37.397 ms
steady p99                 14.259 ms
```

兩次 final run 都同時通過 S3-E.6 mass-join gates 與 S3-E.7 churn gates。

## Allocation

Final churn convergence window約分配：

```text
acceptance #1   ~48.9 MB
acceptance #2   ~48.9 MB
```

這比 S3-E.6 mass-join convergence allocation 顯著小，且 churn phase 沒有 GC。

本階段的主要 allocation 來源仍包含 membership rebuild 與首次進入新 AOI 的 per-view map/dense-state growth；但 final p99 已低於 50ms，因此沒有為追求單次 allocation 數字而引入更複雜 ownership split。

## 決策

S3-E.7 完成後：

- 保持 Protocol v6
- 保持 1200-byte MTU
- Client 不需要修改
- 不拆 World Actor
- 不進 Quantized / Delta
- 保留 S3-E.6 Gate Zerg hard gates
- 新增 S3-E.7 Teleport Churn hard gates
- production lifecycle scheduler具備：
  - per-session bounded work
  - global phase-sensitive work budget
  - cross-session round-robin fairness
  - Despawn-first mixed churn
  - pending departed reuse
  - pure-growth churn-diff bypass
  - Reliable Spawn transform baseline

## 下一步

下一階段較合理的是 **S3-E.8 Repeated Churn / Soak**：

- 不只一次 teleport，而是多輪 AOI churn
- 驗證 round-robin cursor 在長時間反覆 churn 下沒有 Session starvation
- 驗證 map / buffer high-water mark 是否穩定
- 觀察 heap / GC 是否隨 churn 次數持續成長
- 測試 churn 與 gameplay dirty Vitals / combat 同時存在時的優先權

在這些資料出現前，仍沒有證據要求 Protocol v7、transform quantization 或 delta wire payload。
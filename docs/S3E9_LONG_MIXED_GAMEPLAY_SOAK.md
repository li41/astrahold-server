# S3-E.9 Long-duration Mixed Gameplay Soak

## 目標

S3-E.9 在 S3-E.8 repeated churn 之上增加「長時間、同時發生」的正式 gameplay 壓力，重點不是再提高單一 work quantum，而是驗證：

- 500 個真實 TCP/UDP Session 同時存在時，AOI lifecycle、movement、objective Dynamic State、combat Dirty Vitals 能長時間重疊。
- Dirty Vitals 的 latest full-state retry 不會讓高 SessionID relationship starvation。
- 既有 lifecycle / Initial Vitals / Dirty Vitals budget 不被放寬。
- S3-E.6 / S3-E.7 / S3-E.8 的 50ms 與 convergence hard gates仍保留。
- 長時間 mixed phase 的 fairness tail、allocation envelope、repeated-churn retained footprint都有可重現 gate。

本階段起點：

- Server `main`: `88795366893fb660fa6b17dc3ab81458a2a0fd84`
- Client `main`: `8a8b1b4a3f31647f4ed660c63268494be81b5e0b`
- Protocol 維持 v6；本階段不需要 client protocol / codec變更，因此 client repo不修改。

## 不變量

S3-E.9 沒有用提高 budget換 latency：

- lifecycle global churn budget：每 snapshot最多 `6,000`
- Initial Vitals churn budget：每 world tick base `2,500`，20Hz world / 10Hz snapshot錯峰後單 tick最多 `5,000`
- Dirty gameplay Vitals：每 world tick最多 `4,000`
- S3-E.8 repeated churn：每輪 p99 `< 50ms`、trigger-to-converged `< 2.75s`
- drain後 steady state：p99 `< 50ms`

Reliable lifecycle / Vitals 的進度仍只在 `TrySend` 成功後 Confirm / revision advance；backpressure或delivery error不會假裝已送達。

## 1. Hot Entity relationship fairness

原先 Dirty Vitals 有 Entity-level輪轉，但同一個 hot Entity每個 world tick仍從最低 SessionID開始掃。如果該 Entity在一輪 fan-out尚未完成前就再次產生新 revision，低 SessionID可能重複拿到 latest revision，而高 SessionID長期排不到。

S3-E.9 新增每個 dirty Entity自己的 `next Session` cursor：

- Entity起點仍以 stable EntityID輪轉。
- 同一 Entity budget耗盡時，記住第一個未完成 relationship的 SessionID。
- 下一 tick從該 Session繼續。
- delivered revision仍是唯一進度 truth；未送出的 relationship不前進。
- revision再次增加時直接送 latest full state，不排舊 delta queue。

同時新增 transient fairness telemetry，只有目前 dirty Entity保留一筆 progress：

- `oldest_dirty_age_ticks`
- `oldest_pending_revision_age_ticks`
- oldest pending Entity / Session
- Entity / revision completion ticks
- budget exhaustion
- Session cursor advance / wrap

不建立 `(Session, Entity)` age retention map。

## 2. 500-client mixed fixture

Workflow: `.github/workflows/s3e9-mixed-soak.yml`

固定 workload：

- 500 clients
- `teleport-churn`作 phase trigger
- 56 combat pairs / cluster；其中56對為 stationary hot pairs
- 42秒 sustained gameplay
- 每150ms一個 hot combat wave，共280 waves
- 每1.5秒一次 `main-gate` objective blocker update，共28次
- load-lab專用 `soak-attack`；production `config/combat-actions.json`不修改
- 每個 hot target：initial basic attack 100 damage + 280次 1 damage，共380 damage，不進入 defeat semantics

Exact action contract：

- initial basic attacks：`112`
- sustained actions：`56 × 280 = 15,680`
- total applied：`15,792`
- action rejections：`0`

Dirty Vitals還要求 `dirty_vitals_selected > 3,000,000`，避免只驗 action count、卻沒有真正 fan-out到大量 known relationships。

## 3. Movement用 gameplay semantic window啟停

第一版 mixed fixture曾讓 bots從 process啟動就移動，污染 initial convergence；另一版用 loadserver高 sequence注入 move，又和 bot UDP sequence互相競爭。

最終方案完全走真實 bot UDP `ClientMoveInput`，並用正式 `WorldDynamicState` delivery作啟停訊號：

- bootstrap Dynamic State：500筆，movers維持零方向
- 收到第501筆，即第一個 objective update開始 fan-out後，movers開始送正常 movement input
- 28次 objective update完整 fan-out後，總 Dynamic State必須精確等於 `500 + 28×500 = 14,500`
- 達14,500後 movers立即回到零方向
- 此後不 sleep固定秒數，交給既有 semantic convergence tracker判斷 drain

這讓 initial convergence、active phase、drain三個邊界都由 authoritative gameplay / replication state定義。

## 4. Initial bootstrap snapshot CPU barrier

S3-E.9開發期間重新觸發既有 500-client Gate Zerg gate，暴露 startup CPU overlap：

- 一次 run：initial convergence p99 `56.72ms`，ready-to-converged `1.951s`
- 原樣 rerun：p99 `48.19ms`，ready-to-converged `2.0028s`
- slow tick同時有數千 Spawn lifecycle work與約139k–159k remote snapshot candidates
- 主要成本落在 RepBuild

修正不是降低 lifecycle量，而是把第一次 startup的 work phase分離：

- 第一次真正看到 lifecycle work後啟用 initial-only barrier。
- 還在 startup時，已有完整 known lifecycle的 Session只送 self correction，暫停 remote snapshot candidate scheduling。
- 仍有 Spawn / Despawn work的 Session照原本 bounded lifecycle builder執行。
- lifecycle + Initial Vitals全部 drain後永久解除。
- 一旦出現 Despawn churn立即永久解除；late join、S3-E.8 churn、S3-E.9 active phase都不重新啟用。

修正後 Gate Zerg exact samples：

- p99 `35.46ms`，ready-to-converged約`1.901s`
- exact rerun p99 `29.31ms`，ready-to-converged約`1.975s`
- Spawn pressure slow ticks的 `snapshot_candidates = 0`
- lifecycle完成後 snapshot scheduler正常恢復

既有 50ms / 2s Gate Zerg gate沒有改動。

## 5. Active churn lifecycle rebuild fast path

Repeated churn後續又暴露另一個 hot path：semantic-converged teleport membership change時，每個 Session會重新建立 dense desired tracks；新進 AOI、尚未 Spawn的 Entity仍會讀 `lastDeliveredGeneration` / `lastSentBuild` maps，且 retained / removed membership也重複查 known maps。

S3-E.9分兩層優化：

1. conservative lifecycle rebuild：unknown Entity不讀不可觀察的 transform history；known Entity仍完整保留 history。
2. semantic-converged common churn：當上一份 desired全部 known且沒有 pending departed時，複製舊 dense tracks，和新 sorted visible做單次線性 merge，同時產生：
   - retained tracks：原樣保留 generation / build history
   - removed tracks：進 departed
   - new tracks：unknown + zero history

Scratch透過 `sync.Pool`重用；rare continuous-churn / pending departed path仍走原本完整 known-vs-desired邏輯。

效果：

- 優化前兩次 exact S3-E.8曾出現 max p99 `51.03ms` / `50.44ms`，之後一版 partial fast path仍有 `58.55ms` / `54.66ms` runner samples。
- dense-merge後第一次8-round：max p99 `26.91ms`
- exact repeat：max p99 `44.01ms`
- 典型 `23,500 Despawn candidates + 6,000 selected + 4,000 Dirty Vitals`重疊 tick，RepBuild降到約`4–5ms`，total約`20–21ms`
- 既有6k / 5k / 4k work quantum與S3-E.8 `<50ms` gate完全不變。

## 6. S3-E.9 fairness / tail gates

500-client long mixed hard gates：

- semantic drain `< 45s`（42秒active injection後收斂）
- final `desired_relationships == known_desired`
- final desired relationships `> 140,000`，確保 movers真的改變AOI membership
- all pending lifecycle / vitals / dynamic / reliable queues = 0
- exact actions `15,792`
- action rejections = 0
- Dirty Vitals selected `> 3,000,000`
- Dirty Vitals per tick `<= 4,000`
- budget exhaustions `>= 700`
- Session cursor advances `>= 600`
- cursor wraps `> 0`
- oldest dirty age `<= 32 ticks`
- oldest latest-revision pending age `<= 4 ticks`
- max Entity completion `<= 32 ticks`
- max revision completion `<= 8 ticks`
- active mixed p99 `< 60ms`

Representative exact PASS（dense-merge head）：

- active p99 `54.78ms`
- active max `64.99ms`
- trigger-to-converged `42.55s`
- desired / known relationships `150,140 / 150,140`
- Dirty Vitals selected `3,349,108`
- oldest dirty age `12 ticks`
- latest-revision pending age `3 ticks`
- max revision completion `6 ticks`
- budget exhaustions `831`
- Session cursor advances `752`
- steady p99 `16.53ms`

### 為什麼不 hard-gate single-tick max

Hosted runner曾出現單一 `89.9ms` RepBuild stall，但同一 run：

- active p99 `59.44ms`
- semantic drain正常
- fairness全部在界內
- zero command / delivery / network errors

單一 wall-clock max在非專用 runner容易被 host scheduling / noisy neighbor放大，且S3-E.6～S3-E.8既有 acceptance也是以 p99為 tail hard gate。S3-E.9因此保留每個 slow tick的 max / stage breakdown artifact作診斷，但 hard gate只鎖p99，不把不可重現 host stall轉成任意更大的 max threshold。

## 7. Memory / retention methodology

`runtime.MemStats.HeapAlloc`是讀取當下的 live sample；active report和steady report可能分別落在 GC前/後，所以不能用 `steady HeapAlloc < active HeapAlloc`判定 retention。

S3-E.9採兩層 gate：

### Active / steady absolute envelope

- active total allocation `< 1.25GB`
- active mallocs `< 22M`
- active HeapAlloc `< 128MB`
- active HeapSys `< 160MB`
- active NumGC `<= 25`
- steady HeapAlloc `< 128MB`
- steady HeapSys `< 160MB`

Representative mixed PASS：

- total alloc約`1.11GB`
- mallocs約`19.6M`
- active HeapAlloc約`62MB`
- HeapSys約`135MB`
- GC `22`
- steady HeapAlloc約`85MB`

### S3-E.8 repeated-round retained footprint

8-round summary使用較不受單一GC phase影響的 HeapSys high-water / growth：

- max HeapAlloc `< 128MB`
- max HeapSys `< 128MiB`
- round1→round8 HeapSys growth `< 32MB`
- max single-round allocation `< 80MB`
- total GC `<= 16`

Dense-merge兩次 exact 8-round PASS：

- run A：max p99 `26.91ms`、max HeapSys `114.2MB`、HeapSys growth `12.6MB`、max round alloc `64.7MB`、GC `11`
- run B：max p99 `44.01ms`、max HeapSys `110.1MB`、HeapSys growth `12.6MB`、max round alloc `63.5MB`、GC `11`

## 8. Acceptance matrix

Merge前 final head必須同時滿足：

- Server CI: `go test` / `go vet` / race PASS
- Siege Load Lab: 24-client smoke + 100-client Gate Zerg PASS
- S3-E Scaling 500: Gate Zerg + Teleport Churn PASS
- S3-E.8 Repeated Churn Soak: 500 clients × 8 rounds PASS，包括原有50ms / 2.75s與新 retention growth gates
- S3-E.9 Long Mixed Gameplay Soak: 500 clients PASS，包括 exact workload、fairness、p99、memory、drain gates

任何一項紅燈都不以放寬既有 S3-E gate作 acceptance。

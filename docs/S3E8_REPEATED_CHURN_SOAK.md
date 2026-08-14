# S3-E.8：Repeated Churn / Soak

S3-E.8 延續 S3-E.7 的 Teleport / AOI Churn，但把「單次 transition」提升成多輪往返 soak，目標不是再追單次最低延遲，而是驗證：

- 多輪 AOI membership replacement 不會造成 lifecycle starvation
- lifecycle / Initial Vitals 的 cross-session fairness 可以長時間維持
- heap / GC / reusable buffers 不會呈現明顯逐輪失控
- gameplay dirty Vitals 與大規模 Spawn / Despawn 同時存在時，不會重新製造 unbounded tail
- S3-E.6 mass-join、S3-E.7 single churn 與 steady-state gates 全部保留

本階段維持：

- Protocol v6
- GameV1 wire layout
- 1200-byte UDP MTU
- Reliable Spawn / Despawn / Vitals lifecycle truth
- WorldSnapshot partial update semantics
- 64 remote transforms / Session / build
- Server authoritative world ownership
- Client contract

沒有 Client 變更，也沒有 Client PR。

## 起點校正

本階段開始時重新核對 GitHub 後，先完成一個必要的歷史校正：

- S3-E.7 PR #18 在前一輪對話結束時其實仍未 merge，因此先實際 squash merge。
- S3-E.7 merge 後 Server `main` 起點為 `0f86caa0d49ef482be323c993cf152c141c79094`。
- 重新讀 merged tree 後又發現：PR #18 最終說明曾寫成 churn lifecycle `6,000` / Initial Vitals `2,500` 與正式 p99/time gates，但實際 merge tree仍是較早的 `7,000` / `3,500`，而 churn workflow仍把 performance 視為 baseline。

S3-E.8 因此不沿用描述當作既成事實，而是以實際 merge tree重新量測，並把正確的 gates / budget正式落進程式與 CI。

## Repeated Churn workload

`teleport-churn` 新增 `-churn-rounds`，預設仍為 1，因此 single-churn contract不變。

S3-E.8 dedicated job 使用：

```text
clients              500
initial clusters     250 + 250
movers               125 + 125
rounds               8
transition           swap / restore / swap / restore ...
```

每輪都使用單一 owner-thread teleport batch command，並在 semantic convergence後才開始下一輪。

每輪 expected lifecycle：

```text
Spawn          62,500
Despawn        62,500
Initial Vitals 62,500
```

8輪總計：

```text
initial Spawn       125,000
churn Spawn         500,000
churn Despawn       500,000
churn InitialVitals 500,000
Bot total Spawn     625,000
Bot total Despawn   500,000
Reliable minimum  1,750,500
```

每輪 convergence 都必須：

1. 先觀察到 non-converged
2. 再回到 desired == known == 125,000
3. pending Spawn / Despawn / Initial Vitals / Dynamic = 0
4. dirty Vitals = 0
5. Reliable queued / in-flight = 0
6. stable 250ms 才完成

不使用 fixed warm-up sleep。

## 第一輪 repeated baseline：correctness 正常，但 tail 不穩

實際 merge tree的 `7k lifecycle / 3.5k Initial Vitals` 跑8輪後：

- 8/8 rounds lifecycle counts正確
- 沒有 starvation
- errors = 0
- trigger→converged最高約 2.20s
- 但 churn p99最高約 **61.85ms**

Memory並沒有呈現 live-heap leak型態：第一輪到第8輪 HeapAlloc反而略降；HeapSys在前幾輪取得較大 arena後進入高水位穩定區。

因此問題不是「跑久就失控」，而是單 tick CPU / Reliable materialization tail。

## Work budget 收斂：6k / 2.5k

Mixed churn正式改成：

```text
Lifecycle global        6,000 / snapshot
Initial Vitals base     2,500 / world tick
Per-session lifecycle      32 / build
```

Pure bootstrap仍保留較高吞吐：

```text
Lifecycle global       16,000 / snapshot
Initial Vitals base     8,000 / world tick
```

這讓 repeated churn大部分 round進入50ms內，但單純縮 work quantum會開始逼近 completion gate，所以後續不再靠「繼續縮 budget」換 tail，而是直接省 CPU。

## Initial Vitals 錯峰

Spawn只會在 snapshot tick新增 Initial Vitals pending。因此把 Initial Vitals與 snapshot lifecycle錯峰：

20Hz world / 10Hz snapshot 時：

```text
churn global base 2,500 / tick
-> snapshot tick       0
-> intermediate tick   5,000

per-session base 32 / tick
-> snapshot tick       0
-> intermediate tick  64
```

這保留每100ms cycle完全相同的 global / per-session理論吞吐，同時不讓大量 lifecycle Delivery和Initial Vitals Encode疊在同一個50ms tick。

第一版只放大 global budget、忘記同步放大 per-session 32，曾把 semantic completion拉長；單元測試與 load data確認後，global與per-session一起套同一 stagger比例。

## Snapshot candidate：single-pass bounded top-K

Gate Zerg tail顯示另一個獨立熱點：

- slow tick candidate count可達十幾萬
- 但每個 Session真正只需要 top 64
- 舊 path先 materialize全部 `snapshotCandidate`，再第二趟用 heap挑64

S3-E.8 改成第一趟 visible scan就直接維護 bounded top-K：

```text
visible scan
  -> candidate counter仍完整累加
  -> <=64 保持原輸出順序
  -> >64 從第65個開始維護 worst-at-root heap
  -> 使用完全相同 comparator
  -> 最終仍只有同一組 top-K Entity
```

單元測試對 0 / 32 / 64 / 65 / 500 candidates，比對舊 selector與新 single-pass selector：

- <=64 ordering相同
- >64 selected Entity set完全相同

實際 Gate Zerg slow tick仍可統計到約171k candidates，但 RepBuild從約48ms等級下降到約21ms，證明改善來自少 materialization / 少第二趟掃描，而不是偷偷減少 candidate semantics。

## AOI churn：sorted desired diff

Repeated churn剩餘的 RepBuild tail集中在 Despawn membership change：舊 path會：

```text
scan known map
  x binary-search desired
  -> append departed
  -> sort departed
```

S3-E.8 利用兩邊都有 stable EntityID order：

```text
old desired + new desired
-> O(old + new) two-pointer diff
-> 只保留目前 known 的 removed IDs
-> 輸出天然 sorted
```

只有「沒有舊 pending Despawn」的 semantic-converged common churn path走這個 fast path。

若連續 churn發生在前一批 pending Despawn尚未完成時，仍退回完整 `known vs new desired` rebuild，保留 re-entry / retry correctness。

Desired不變的 retry則只剝掉已成功 ConfirmDespawn 的 sorted prefix，不再每個 snapshot重掃未處理尾段。

單元測試特別鎖住：old desired裡某 Entity若從未成功 Spawn（unknown），就算它離開AOI也不能產生 Despawn。

## No-combat 8-round hard-gate acceptance

在 single-pass top-K、sorted desired diff與6k/2.5k stagger完成後，500-client 8-round repeated churn正式同時通過：

- 每輪 p99 < 50ms
- 每輪 trigger→converged < 2.75s
- exact 500k churn Spawn / 500k Despawn / 500k Initial Vitals
- lifecycle max <= 6,000 / snapshot
- staggered Initial Vitals max <= 5,000 / intermediate tick
- semantic / Reliable state全清
- errors = 0
- churn後 steady p99 < 50ms

其中一輪正式 hard-gate run觀察到：

```text
8-round max p99                 45.432 ms
8-round max trigger->converged   2.700 s
steady p99                      16.315 ms
```

Memory同樣只作high-water觀測，不用任意 leak threshold自我驗證；當時8輪總 allocation約473MB、GC 10次，HeapSys進入約106MB high-water。

## Combat dirty-Vitals overlap

Repeated churn穩定後，S3-E.8再加入 deterministic gameplay overlap。

每輪：

```text
cluster A adjacent pairs  32
cluster B adjacent pairs  32
basic-attack actions       64
```

Pair特性：

- actor / target都是同一原始cluster的 movers
- swap / restore後仍一起落在另一側相鄰 grid slot
- 單元測試確認 initial / swap / restore距離都在 basic-attack 4.5m range內
- actor與target不重複
- 每個 target每輪只被打1次
- basic-attack damage=100；8輪後 HP=200，不混入 defeat semantics

Combat不是直接改HP的測試捷徑，而是走正式：

```text
Runtime.EnqueueUseAction
-> ClientUseAction{basic-attack, entity target}
-> action validation / range / LOS / combat service
-> Character.ReduceHP
-> markEntityVitalsDirty
-> Reliable EntityVitalsState
```

每輪 gate要求：

- `combat_actions_applied == 64`
- `action_rejections == 0`
- `dirty_vitals_selected > 0`
- convergence時 `dirty_vitals_entities == 0`

8輪總計必須：

```text
combat actions applied  512
rejections                0
```

## Combat baseline：unbounded dirty fan-out 重新製造 tail

第一個 combat overlap baseline correctness完全成功：

- 64/64 actions / round
- 0 rejection
- dirty state最後全部清零

但每輪64個 dirty target會對當下 known relationships做 full-state fan-out。沒有budget時，第一個combat tick可一次送出約 **14k–16k dirty Vitals**；實測 Vitals stage最高約 **58.7ms**，round p99可到約 **69–94ms**。

因此「gameplay dirty Vitals不和Initial Vitals共用budget」是對的，但「完全 unbounded」不是。

## Dirty gameplay Vitals 4k budget + entity fairness

Final scheduler新增獨立設定：

```text
MaxDirtyVitalsPerTick = 4,000
```

它不併入 Initial Vitals budget：

- Initial Vitals繼續使用0/5k stagger
- gameplay dirty Vitals保留自己的4k reserved capacity
- budget用完的 Session revision不前進
- 下一 tick retry latest full state

為避免持續 combat時固定低 EntityID壟斷4k quota：

1. 以 reusable scratch收集目前 dirty Entity IDs
2. stable ID sort
3. 從 `dirtyVitalsNextEntity`開始輪轉
4. budget耗盡後，下個 tick從下一個 dirty Entity開始
5. 單一 Entity若只完成部分 Session，delivered revision會讓下次自動跳過已送達關係

這不是 event queue；truth仍然是：

```text
entity revision
+ per-session delivered revision
+ current known lifecycle
```

因此同一 Entity在等待期間若又被打，下一次直接送最新 full-state revision，不需重播中間 HP events。

單元測試用2個dirty Entity × 4個Session、budget=2驗證：

- 每tick selected <=2
- 4 ticks總共送滿8筆
- 兩個Entity都完成
- 每個Session各收到兩個Entity的最新Vitals
- 最後沒有dirty entity
- lazy cursor在下一個空tick清除且不重送

## Final hard gates

S3-E.8 dedicated 500×8 combat soak要求：

1. 500 clients ready，decode/network errors=0
2. Bot Spawn=625,000
3. Bot Despawn=500,000
4. Reliable >=1,750,500（combat dirty只會增加）
5. 每輪 observed non-converged
6. 每輪 desired/known=125,000/125,000
7. pending Spawn / Despawn / Initial Vitals / Dynamic=0
8. dirty Vitals=0
9. Reliable queued/in-flight=0
10. 每輪 p99<50ms
11. 每輪 trigger→converged<2.75s
12. lifecycle max<=6,000/snapshot
13. Initial Vitals max<=5,000/intermediate tick
14. dirty gameplay Vitals max<=4,000/world tick
15. 每輪64 combat actions applied
16. 每輪0 action rejection
17. 8輪 total combat actions=512
18. command/tick/delivery/network/MTU errors=0
19. churn後 steady p99<50ms

此外，S3-E.6 Gate Zerg與S3-E.7 single churn hard gates都繼續在同一 PR中執行，不能為 repeated soak放寬舊 gate。

## Final acceptance

加入4k dirty gameplay Vitals budget後，正式 acceptance與刻意 repeat都在完全相同 workload / thresholds下通過：

- Server CI：test / vet / race PASS
- Siege Load Lab：24 / 100 PASS
- S3-E Scaling 500：Gate Zerg + single churn PASS
- S3-E.8 Repeated Churn Soak：500 clients × 8 rounds + combat PASS

兩輪都證明：

- 512/512 combat actions applied
- 0 rejection
- 8輪 lifecycle exact counts成立
- 每輪 dirty Vitals最後清零
- dirty Vitals單 tick不超4,000
- 每輪 p99<50ms
- 每輪 trigger→converged<2.75s
- steady p99<50ms
- errors=0

Final repeat同時加上 `TestS3E8ReplicationBudgetDefaults`，鎖住：

```text
pure lifecycle      16,000
churn lifecycle      6,000
pure Initial Vitals  8,000
churn InitialVitals  2,500
Dirty gameplay       4,000
```

## 決策

S3-E.8 的資料仍不支持：

- Protocol v7
- transform quantized / delta wire format
- World Actor split
- Client改 lifecycle semantics

本階段改的是 **Server owner-thread work scheduling / hot-path CPU / Reliable full-state retry fairness**，wire contract保持不變。

## 下一階段

建議 S3-E.9 改測長時間 mixed gameplay，而不是繼續放大同一種 teleport：

- 更長 duration / 多批 combat dirty entities
- mover + stationary observer混合
- objective / dynamic state與combat同時更新
- hot entity反覆受擊，驗證 latest full-state coalescing
- 對 heap/map mirrors / scratch high-water做更長時間觀察
- 必要時再決定是否把 dirty-Vitals session fan-out做更細的 cursor / shared serialization

先用資料判斷下一個真正的瓶頸，不因目前500×8 PASS就宣告 production 500-player capacity。

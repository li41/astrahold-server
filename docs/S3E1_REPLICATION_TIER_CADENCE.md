# S3-E.1：Replication Tier / Cadence Foundation

S3-E.1 的目的，是把 S3-C.6 已經 MTU-safe 的 realtime transform replication 從「每個 snapshot cadence 對每個 Session 傳完整 AOI transform」推進到可量測、可節流的 **Network LOD / dirty / budget scheduler**。

這一階段刻意不改 Protocol v6 的 wire layout，也不先做 Quantized Delta 或 Shared Cell Serialization。先用既有 24 / 100 / 500-client Load Lab 確認 transform fan-out、bandwidth、allocation 與 CPU 的下一個真實瓶頸。

## 核心規則

每個 World / Zone 的 mutable simulation ownership 維持不變。S3-E.1 只改 read-side replication scheduling：

```text
AOI visible entities
        ↓
per-session replication view
        ↓
Network Tier
├── Near
├── Mid
└── Far
        ↓
dirty / cadence / periodic refresh
        ↓
hard transform budget
        ↓
MTU-safe WorldSnapshot update batch
```

Self transform 不重複放進 `WorldSnapshot`，仍由 `PositionCorrection` 處理。

## 預設 Network LOD

目前預設 Policy：

```text
NearRadius = 12m
MidRadius  = 32m

Near cadence = every 1 replication build
Mid cadence  = every 2 replication builds
Far cadence  = every 5 replication builds
```

在目前 10Hz snapshot cadence 下，約為：

```text
Near ≈ 10Hz
Mid  ≈ 5Hz
Far  ≈ 2Hz
```

Stationary transform 不會因 dirty=false 永久停止同步。每個 tier 都有 periodic full refresh：

```text
Near refresh = 10 builds
Mid refresh  = 20 builds
Far refresh  = 40 builds
```

這讓偶發 UDP loss 可以靠後續完整 transform 自我恢復，不需要 reliable transform replay。

## Per-session transform budget

每個 Session 每次 replication build 最多選：

```text
MaxTransformsPerBuild = 64
```

目前 GameV1 每個 MTU-safe snapshot chunk 最多 43 transforms，因此 64 transform budget 最多形成兩個 Snapshot datagrams。

如果 candidate 數量超過 budget，不直接丟掉超額 Entity，而是依 **normalized overdue** 排序：

```text
priority ≈ age / tier cadence
```

實作以交叉乘法比較 `age/cadence`，避免 hot path 浮點除法。被延後越久的 Entity 會逐漸取得更高優先權，避免 budget starvation。

只有真的超出 budget 才支付 overdue ranking sort 成本；未超 budget 的 normal path 保留 AOI 既有 EntityID 穩定順序。

## Snapshot semantic change

Protocol v6 wire format沒有改，但 `WorldSnapshot` 的語意被明確收斂成：

```text
Realtime Transform Update Batch
```

而不是：

```text
Full AOI Entity List
```

所以某 Entity 沒出現在本輪 snapshot，不代表 despawn。Entity lifecycle 的真相仍是 Reliable `EntitySpawn` / `EntityDespawn`。

同一 tick 的 chunk set 仍必須完整；「完整」指 Server 本輪選出的 update batch 完整，不代表整個 AOI 所有 Entity 都在其中。

## Dirty tracking

每個 Session 記錄最後成功排入 realtime snapshot 的 Position / Yaw 與 build number。

```text
transform changed
→ dirty
→ cadence 到期才 eligible

transform unchanged
→ 不重送
→ periodic refresh 到期時強制 eligible
```

這一版 dirty tracking 仍是 per-session 比較。500-client 數據顯示這個 read-side 重複工作本身已成為下一階段的主要 CPU 問題，因此 S3-E.2 會優先改成 shared work / generation reuse，而不是先把 wire 再縮小。

## Reliable lifecycle：desired != known

500-client 第一版 S3-E.1 抓到一個重要 correctness bug。

舊 replication view 把「目前 AOI 可見」直接視為「Client 已知道 Entity」。當大量 initial Spawn 讓 Reliable outbound queue backpressure 時，`EntitySpawn` 可能 TrySend 失敗，但 Server 已經把 Entity 標成 known，之後不再重送。

第一次 500-client Gate Zerg 因此出現：

```text
expected Spawn = 500 × 500 = 250,000
received Spawn = 178,585
Server delivery backpressure errors = 11,853
```

問題不是 Client decode，也不是 MTU，而是 Server lifecycle state machine 把「desired visibility」與「delivered lifecycle knowledge」混在一起。

S3-E.1 現在明確拆成：

```text
desired
→ 目前 AOI 想讓 Session 看見

known
→ Reliable EntitySpawn 已成功進入該 Session outbound queue
```

### Spawn

```text
visible && !known
→ emit EntitySpawn
→ TrySend success
   → ConfirmSpawn
   → known = true
→ ErrBackpressure
   → known 保持 false
   → 下一次 replication build retry
```

在 Spawn 成功以前，該 Entity 不會進 realtime snapshot，也不會讓 Vitals replication 誤認 Client 已知該 Entity。Spawn 本身已包含 authoritative initial transform。

### Despawn

```text
!visible && known
→ emit EntityDespawn
→ TrySend success
   → ConfirmDespawn
   → known = false
→ ErrBackpressure
   → known 保持 true
   → 下一次 replication build retry
```

因此 Reliable lifecycle 與 S3-D.3 Vitals 一樣，都具有「暫時 backpressure 後仍能收斂」的 full-state semantics；沒有靠放大 queue 或忽略錯誤解決。

## Load Lab replication metrics

S3-E.1 擴充 Load Lab，除了 Tick / AOI / allocation 外，也直接量測：

```text
SnapshotCandidates
SnapshotTransforms
SnapshotDeferred
SnapshotForcedRefreshes
SnapshotNearTransforms
SnapshotMidTransforms
SnapshotFarTransforms
TransformsPerSession
DeferredCandidateRatio
```

這些數據用來區分「AOI 看得到多少」與「Network scheduler 實際送多少」。

## 24-client Vertical Siege

最終 regression：

```text
clients                     24
measurement                 5s / 100 ticks
Tick avg                    0.281 ms
Tick p99                    0.625 ms
AOI avg                     0.053 ms
Replication Build avg       0.079 ms
Delivery avg                0.014 ms

Snapshot candidates         8,699
Snapshot selected           8,699
Snapshot deferred           0
Near / Mid / Far            4,029 / 3,965 / 705
Transforms / session        7.25

TotalAlloc                  7.47 MB
UDP received by bots        0.534 MB
Completed snapshots         1,571
Incomplete resets           0
Spawn                       576 / 576 expected
Decode / network errors     0 / 0
```

對照 S3-D.3 / S3-C.6 同類基線，UDP receive 約由 1.20 MB 降至 0.53 MB，Server allocation 約由 10.57 MB 降至 7.47 MB；24 人沒有觸發 transform budget。

## 100-client Gate Zerg

最終 regression：

```text
clients                     100
measurement                 8s / 160 ticks
Tick avg                    2.581 ms
Tick p99                    7.336 ms
Simulation avg              0.044 ms
AOI avg                     0.772 ms
Replication Build avg       0.852 ms
Delivery avg                0.055 ms

Snapshot candidates         109,853
Snapshot selected           101,462
Snapshot deferred           8,391
Deferred ratio              7.64%
Near / Mid / Far            89,902 / 11,560 / 0
Transforms / session        12.68

TotalAlloc                  71.06 MB
Mallocs                     285,900
GC                          21
UDP received by bots        5.81 MB
Snapshot packets            11,180
Completed snapshots         9,971
Incomplete resets           0
Spawn                       10,000 / 10,000 expected
Decode / network errors     0 / 0
```

同一 Hosted Runner 類型下的 S3-D.3 基線約為：

```text
TotalAlloc                  181.22 MB
UDP received                27.90 MB
Snapshot packets            29,010
Completed snapshots         9,984
Outbound app messages       32,676
```

S3-E.1 約帶來：

```text
UDP receive                 -79%
Server TotalAlloc           -61%
Snapshot packet count       -61%
Outbound app messages       -47%
Completed snapshot cadence  幾乎不變
```

`ReplicationBuild` CPU 沒有同步下降，反而略高於舊基線；這是重要訊號，不應被 bandwidth 改善掩蓋。原因是這一版仍需要每個 Session 自己掃 visible entities、比較 dirty state、計算 tier/cadence，budget 壓力時再做 ranking。

## 500-client Gate Zerg：correctness PASS，不是 capacity PASS

S3-E.1 新增獨立 `S3-E Scaling 500` workflow，除了基本 decode / MTU / delivery gate，也要求 hotspot lifecycle 必須完整收斂：

```text
Spawn == clients × clients
Reliable messages >= Spawn + Vitals + Dynamic state
```

最終強化 gate PASS：

```text
connected / ready            500 / 500
Spawn                        250,000 / 250,000
Reliable messages            500,500
Decode errors                0
Network errors               0
Delivery errors              0
Datagram too large           0
Incomplete snapshot resets   0
```

這證明 lifecycle backpressure retry 能真正恢復到完整狀態。

但同一個 500 人 run 的容量數據為：

```text
measurement                  10s
completed ticks              157
expected at 20Hz             約 200

Tick avg                     61.96 ms
Tick p50                     95.34 ms
Tick p95                     99.25 ms
Tick p99                     153.13 ms
Tick max                     171.47 ms

Simulation avg               0.127 ms
AOI avg                      13.81 ms
Replication Build avg        23.26 ms
Delivery avg                 1.26 ms

AOI candidate / visible      約 500 / query
AOI total candidates         19.75M
Snapshot candidates          2.01M
Snapshot transforms          1.75M
Snapshot deferred            263k / 13.1%
Transforms / session         44.29

TotalAlloc                   1.29 GB / 10s
Mallocs                      1.97M
```

因此 **不能把這個結果描述為 500-player capacity**。它只代表 correctness / convergence PASS；20Hz 的 50ms tick budget 尚未達成。

## 500 人數據的架構結論

500 hotspot 的 Simulation 仍只有約 0.13ms/tick，所以目前依然沒有數據支持拆 mutable World ownership、Cell Actor 或跨 Cell two-phase commit。

真正的 CPU 主因是 read-side fan-out：

```text
500 Sessions
×
約 500 visible entities / Session
×
AOI scan + per-session dirty/tier/cadence work
```

Gate Zerg 幾乎所有 Session 都反覆處理同一批 Entity；這是明顯的 O(N²) shared-read work duplication。

S3-E.1 已把「wire 太肥」從第一優先級往後推。下一階段不應先做 Quantized Delta，也不應拆 mutable state owner；應先消除 AOI / replication build 的重複工作。

## 下一步：S3-E.2 Shared AOI / Replication Work Reuse

S3-E.2 建議邊界：

```text
World Owner tick
        ↓
immutable replication frame / transform generation
        ↓
shared spatial / cell candidate view
        ↓
per-session filtering + budget
        ↓
existing Protocol v6 GameV1 chunks
```

優先研究：

- 每個 snapshot tick 只建立一次 immutable transform frame
- global transform revision / dirty generation，避免每個 Session 重複 Position/Yaw equality compare
- spatial cell candidate view / query reuse，降低 Gate Zerg 對同一批 Entity 的重複 AOI掃描
- reuse sorted entity order / immutable transform blocks
- 先降低 AOI + Replication Build CPU 與 allocation，再決定 Quantized Delta / Shared Serialization 的 wire 優先序

保持：

- mutable World 仍單一 owner
- Network/DB/Admin 不直接改 World state
- World tick 不 blocking socket I/O
- Protocol v6 wire 不因 read-side reuse 被迫改版

## 本階段刻意不做

- Quantized transform payload
- baseline/delta protocol
- shared encoded packet fan-out
- Cell Actor / mutable ownership split
- remote extrapolation
- Presentation LOD / VAT / MultiMesh
- 500-player production capacity 宣告

S3-E.1 的完成條件不是「500 人已能穩定 20Hz」，而是建立 Network LOD / dirty / budget 基線、修正 500 人才暴露的 Reliable lifecycle convergence bug，並用實測把下一個瓶頸定位到 shared AOI / replication read-side work。

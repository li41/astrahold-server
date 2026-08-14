# S3-E.2：Shared AOI / Replication Work Reuse

S3-E.2 的目的，是消除 S3-E.1 在高密度 Siege hotspot 中暴露的 replication read-side 重複工作，同時維持既有 authoritative world ownership、Reliable lifecycle correctness、Network LOD 與 Protocol v6 wire contract。

本階段沒有拆 Cell Actor、沒有拆 mutable World ownership、沒有提高 UDP MTU、沒有放大 outbound queue，也沒有用降低玩家數或 tick rate 取得 PASS。

## S3-E.1 問題基線

500-client Gate Zerg 的 S3-E.1 最終基線：

```text
measurement                  10s
completed ticks              157
Tick avg                     61.96 ms
Tick p50                     95.34 ms
Tick p95                     99.25 ms
Tick p99                     153.13 ms
Simulation avg               0.127 ms
AOI avg                      13.81 ms
Replication Build avg        23.26 ms
Delivery avg                 1.26 ms
TotalAlloc                   約 1.29 GB
Mallocs                      約 1.97M
```

因此瓶頸不是 simulation owner，而是：

```text
Sessions × visible entities × repeated read-side work
```

## 最終架構

S3-E.2 將 snapshot pass 收斂成：

```text
World Owner tick
        ↓
ReplicationFrameBuilder
        ↓
immutable ReplicationFrame
├── Tick
├── stable EntityID order
├── immutable EntityState transforms
├── global transform generation
└── immutable spatial ReadFrame
        ↓
shared cell candidate view
        ↓
per-session exact AOI filtering
        ↓
per-session dense replication track
├── known / lifecycle
├── last delivered generation
├── cadence age
└── Network LOD
        ↓
overdue fairness + 64 transform budget
        ↓
existing Protocol v6 GameV1 chunks
```

### Immutable Replication Frame

每個 snapshot pass 只由 World Owner 建一次 immutable `ReplicationFrame`。

Entity 以 `EntityID` stable order 排列，Session replication 不再各自從 mutable simulation state materialize 一份 `[]EntityState`。

### Global Transform Generation

`ReplicationFrameBuilder` 對每個 Entity 每個 frame 最多做一次完整 transform equality comparison。

```text
transform changed
→ global generation 前進

Session dirty check
→ lastDeliveredGeneration != frame generation
```

因此 production replication hot path 不再為每個 Session 重複比較 Position / Yaw。

### Shared Spatial Candidate View

`spatial.ReadFrame` 在 frame 建立時建立 immutable cell membership。

同一個 snapshot pass 中，如果多個 Session 查詢相同 cell window：

```text
第一次
→ build candidate index list

後續 Session
→ reuse candidate index list
```

但 shared 的只有 coarse cell candidates。以下語意仍然逐 Session 執行：

- Layer filtering
- height delta filtering
- exact AOI radius
- future Session-specific visibility policy

因此 Gate Zerg 可以重用 hotspot work，但架構沒有永久假設所有 Session 都共享同一份 ViewList。

### Dense Per-Session Replication Track

shared frame 解掉 AOI 後，500-client 數據顯示下一個瓶頸仍在 `Replication Build`。

原本 steady-state 每個 Session / visible Entity 都需要查詢多個 map：

```text
known[EntityID]
lastDeliveredGeneration[EntityID]
lastSentBuild[EntityID]
```

S3-E.2 最終改成與 stable desired EntityID order 對齊的 dense track：

```text
entityTrack
├── EntityID
├── known
├── lastDeliveredGeneration
└── lastSentBuild
```

AOI membership 不變時，dirty / cadence / known 判斷直接走 slice。只有 membership 真正變化時才重建 track。

Reliable `known` map 仍保留為 lifecycle truth 與 `Knows` API；dense track 只是 scheduler read-side state，不改變 lifecycle contract。

### Vitals Dirty Fan-out

S3-E.2 量測過程另外找出一個原本未被 stage metrics 拆出的 O(N²) read-side path：

```text
每 tick
× all Sessions
× all Character states
```

即使 HP 沒有改變，舊 `replicateEntityVitals` 仍會反覆掃描全部 Character state。

現在改成兩種工作來源：

```text
Reliable Spawn success
→ queue initial vitals pending

Character vitals revision changed
→ global dirty entity fan-out
```

Backpressure contract 不變：

```text
TrySend success
→ delivered revision 前進

ErrBackpressure
→ 不前進
→ 下一 tick retry latest full state
```

Despawn 成功後也同步清除該 Session 的 vitals delivered / pending state。

## Reliable Lifecycle 不變量

S3-E.1 修正的 `desired != known` contract 完整保留：

```text
desired
→ AOI 希望 Client 看見

known
→ EntitySpawn 已成功 TrySend 進 outbound queue
```

Spawn backpressure 時不 ConfirmSpawn；Despawn backpressure 時不 ConfirmDespawn。

只有 lifecycle 成功排入 Reliable outbound queue 後，Server 才推進對應 lifecycle knowledge。

## Network LOD 不變

S3-E.2 沒有取消 S3-E.1：

- Near / Mid / Far
- 約 10Hz / 5Hz / 2Hz cadence
- dirty scheduling
- periodic full refresh
- 每 Session / build 最多 64 remote transforms
- normalized overdue fairness
- self transform 使用 `PositionCorrection`

`WorldSnapshot` 仍是 partial realtime transform update batch，absence 仍不代表 despawn。

## Protocol / Client

本階段完全沒有改 wire layout：

```text
Protocol                 v6
Realtime codec            GameV1
UDP datagram limit        1200 bytes
WorldSnapshot chunks      MTU-safe
EntityTransform layout    unchanged
```

因此 Astrahold Client 不需要 S3-E.2 對稱改動，也沒有另開 Client PR。

Client contract 繼續維持：

- SnapshotAssembler 收齊同 tick chunk set 才提交
- batch 可以是 partial entity updates
- snapshot absence != despawn
- SnapshotBuffer 是 per-Entity samples
- 不做 remote extrapolation
- Godot SceneTree 只在 main thread 修改

## Load Lab：24-client Vertical Siege

最終 run：

```text
clients                     24
measurement                 5s / 100 ticks
Tick avg                    0.105 ms
Tick p99                    0.291 ms
AOI avg                     0.004 ms
Replication Build avg       0.031 ms
Vitals Replication avg      0.001 ms
TotalAlloc                  5.75 MB
Mallocs                     34,796
Shared candidate reuse      91.67%
Physical candidate scans    2,400
Logical AOI candidates      28,800
Datagram too large          0
Delivery / network errors   0 / 0
Incomplete reset            0
```

24 人沒有用高負載換取退化；相較 S3-E.1 的 Tick avg 0.281 ms / Replication Build 0.079 ms / TotalAlloc 7.47 MB，仍有改善。

## Load Lab：100-client Gate Zerg

最終 run：

```text
clients                     100
measurement                 8s / 160 ticks
Tick avg                    0.626 ms
Tick p95                    1.250 ms
Tick p99                    4.893 ms
AOI avg                     0.037 ms
Replication Build avg       0.400 ms
Vitals Replication avg      0.002 ms
TotalAlloc                  27.94 MB
Mallocs                     221,386
Shared candidate reuse      98.00%
Physical candidate scans    16,000
Logical AOI candidates      800,000
Datagram too large          0
Delivery / network errors   0 / 0
```

S3-E.1 100-client 基線：

```text
Tick avg                    2.581 ms
Tick p99                    7.336 ms
AOI avg                     0.772 ms
Replication Build avg       0.852 ms
TotalAlloc                  71.06 MB
Mallocs                     285,900
```

## Load Lab：500-client Gate Zerg

最終 run：

```text
clients                     500
measurement                 10s
completed ticks             201
Tick avg                    9.38 ms
Tick p50                    14.10 ms
Tick p95                    18.72 ms
Tick p99                    32.71 ms
Tick max                    99.57 ms

Command avg                 0.138 ms
Simulation avg              0.166 ms
Dynamic replication avg     0.094 ms
Replication frame avg       0.068 ms
AOI avg                     0.824 ms
Replication Build avg       7.580 ms
Delivery avg                0.258 ms
Vitals replication avg      0.135 ms

Shared candidate reuse      99.60%
Logical AOI candidates      25.25M
Physical candidate scans    100,998

Snapshot candidates         2.294M
Snapshot transforms         2.221M
Snapshot deferred           73,449 / 3.20%
Transforms / session        43.98

TotalAlloc                  377.85 MB
Mallocs                     1.58M
```

### 500-client correctness gate

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

### S3-E.1 → S3-E.2 500-client A/B

| 指標 | S3-E.1 | S3-E.2 | 結果 |
|---|---:|---:|---:|
| completed ticks / 10s | 157 | 201 | +28% |
| Tick avg | 61.96 ms | 9.38 ms | -85% |
| Tick p99 | 153.13 ms | 32.71 ms | -79% |
| AOI avg | 13.81 ms | 0.824 ms | -94% |
| Replication Build avg | 23.26 ms | 7.58 ms | -67% |
| AOI + Replication Build | 37.07 ms | 8.40 ms | -77% |
| TotalAlloc / 10s | 約 1.29 GB | 377.85 MB | 約 -71% |
| Mallocs | 約 1.97M | 1.58M | 約 -20% |

20Hz 的 50ms tick budget 以本次 GitHub Hosted Runner Gate Zerg regression 的 p99 判定已通過：

```text
p99 = 32.71 ms < 50 ms
```

仍有單次 max 99.57ms spike，因此這代表 **目前 500-client Gate Zerg Load Lab capacity gate PASS**，不是宣告所有地圖、所有 gameplay workload 或 Internet production deployment 都已具備 500-player SLA。

## S3-E.2 結論

實測支持原先架構方向：

- Simulation / single mutable World Owner 不是瓶頸
- 不需要拆 Cell Actor
- 不需要跨 Cell two-phase commit
- shared immutable read-side work 可以在不改 gameplay ownership 的前提下大幅降低 hotspot CPU
- per-session lifecycle / LOD / fairness 仍可保持正確與可擴充

S3-E.2 的最重要成功條件「500-player hotspot 的 AOI + Replication Build 明顯下降」已達成，且 p99 已回到 20Hz budget 內。

## S3-E.3 決策

S3-E.2 後，剩餘最大單一 stage 仍是 `Replication Build` 約 7.58ms/tick，但目前 500-client Gate Zerg p99 已低於 50ms。

因此不需要因容量壓力立刻跳到 Cell Actor，也沒有數據要求立刻修改 wire。

下一階段若要繼續擴張，應以新的 workload / bandwidth / profiling 數據決定是否進入：

- Quantized transform payload
- baseline / delta payload
- shared serialization / encoding reuse
- 更大型或更複雜的 visibility policy

在沒有新數據前，不預先把 S3-E.3 定義成 Quantized Delta。
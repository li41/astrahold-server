# S3-C.5：Siege Load Lab

S3-C.5 的目的不是證明「Astrahold 已能承載 500 人」，而是在 Combat / Gate HP / Siege State 大量加入前，先建立一套**可重現、可比較、走真實網路路徑**的負載量測基線。

核心原則：

> Measure → Profile → Optimize。先量到瓶頸，再決定是否導入 Layer-aware bucket、AOI ViewList、binary codec、allocation reuse、worker jobs 或 Cell Actor。

## 為什麼 Load Server 與 Bot 分成兩個進程

Load Lab 使用：

```text
cmd/loadserver
    │
    │ 真 TCP / UDP
    │ Astrahold Protocol v2 + JSON v1
    │
cmd/loadbot
```

Bot 不直接呼叫 `WorldRuntime`，而是完整走：

```text
TCP connect
→ SessionWelcome
→ World Identity
→ Realtime Token
→ UDP ASTU/ASTR
→ ClientMoveInput
→ Gateway
→ Command Queue
→ World Tick
→ AOI / Replication
→ TCP / UDP outbound
→ Bot decode
```

Server 與 Bot 分進程還有另一個重要理由：Go `runtime.MemStats` 必須只量 Server。若 500 個 Bot 跟 Server 在同一個 process，Bot goroutine、JSON decode 與 socket buffer allocation 會污染 Server 的 GC / heap 結果。

## Measurement Window

Server 啟動後不立即計時。

```text
Load Server ready
      ↓
Bot ramp-up / connect
      ↓
ReadyPeerCount == expected clients
      ↓
ServerCollector.Reset()
      ↓
正式 measurement window
      ↓
JSON report
```

這避免 Session bootstrap、Entity Spawn 與連線 ramp-up 混入 steady-state Tick latency。連線建立延遲則由 Bot report 另外保存。

## 場景

### `distributed`

玩家分散在可走 Ground surface，用 deterministic 方向週期移動。

主要觀察：

- 一般 AOI candidate amplification
- Spatial cell 分布
- Replication 平均成本
- 每 Tick allocation

### `gate-zerg`

玩家集中在 Main Gate 前方並持續向 Gate 移動。

主要觀察：

- 單一 hotspot / cell 壓力
- Command Queue 深度
- 全互見 AOI 的最壞 replication 成本
- Full Snapshot 膨脹
- Gate blocker 造成的 movement rejection

### `vertical-siege`

玩家分布在：

```text
L0 = Siege Field / Ground
L1 = West / East Ramp
L2 = Front Wall Walk
```

主要觀察：

- XYZ + Layer 下的 AOI candidate 數
- Vertical siege replication
- Ramp / Wall movement
- 未來 Layer-aware Spatial Bucket 的收益空間

## Server Report

`cmd/loadserver` 輸出 JSON schema v1，包含：

### Tick latency

```text
average
p50
p95
p99
max
```

目前 20 Hz Tick budget 為 50ms。**不能只看 average**；是否接近 budget 應以 p95 / p99 與連續 spike 為主。

### Stage timing

- Simulation
- Dynamic World Replication
- AOI Query
- Replication Build
- Delivery / Outbox enqueue

注意：這些是單一 owner thread 內的 application-stage duration，不含 network writer goroutine 真正 encode / flush socket 的完整成本。

### Queue / AOI

- Command Queue max depth before / after
- Commands / Tick
- AOI query count
- Candidate entities
- Visible entities
- Candidate / Visible ratio

Candidate amplification 是判斷是否值得做 `CellX + CellZ + Layer` bucket 的重要依據。

### Memory / GC

- Total allocated bytes
- Malloc count
- Heap Alloc / Heap Sys
- GC count
- GC pause total
- Goroutine count

S3-C.5 不要求 zero-allocation。這些資料先用來找真正 hot path。

### Error counters

- Command errors
- Expected blocked movement
- Unexpected tick errors
- Delivery errors
- Network errors by operation
- `datagram_too_large`

`navigation.ErrBlocked` 在 Gate Zerg 等場景屬預期 gameplay 結果，不跟 unexpected tick error 混在一起。

## Bot Report

`cmd/loadbot` 另存：

- connected / ready / failed clients
- connection latency
- movement inputs sent
- TCP / UDP bytes received
- reliable / realtime message counts
- Spawn / Despawn
- Snapshot / Correction
- Dynamic World State
- decode / network errors

因此可以把「Server 產生了多少 replication」與「Client 實際收到多少」分開觀察。

## 1200-byte UDP Guard 不得為壓測放寬

S2-B 的 Realtime UDP datagram 上限仍維持 **1200 bytes**。

Load Lab 若遇到：

```text
ErrDatagramTooLarge
```

必須記錄在報表，不可為了讓壓測數字漂亮而提高 MTU 或依賴 IP fragmentation。

這是 compact binary、delta / chunk、quantization 與 Replication Tier 是否需要提前的直接輸入資料。

## 第一份成功基線：24-client Vertical Siege

環境：GitHub Actions hosted `ubuntu-24.04` runner，Go 1.26.5。

```text
Clients             24
Scenario            vertical-siege
Tick                20 Hz
Snapshot            10 Hz
Measurement         5 sec
Ticks               100
```

### Tick

```text
average     0.166 ms
p50         0.180 ms
p95         0.318 ms
p99         0.478 ms
max         0.620 ms
```

這表示 24 人時 single World owner 的 CPU Tick budget 完全不是瓶頸；現在沒有任何理由引入 Cell Actor。

### Stage average / Tick

```text
Simulation              0.024 ms
Dynamic replication     0.012 ms
AOI                     0.047 ms
Replication Build       0.047 ms
Delivery                0.015 ms
```

### AOI

```text
queries                 1,200
candidates              28,800
visible                 28,800
candidates/query        24
visible/query           24
candidate/visible       1.0
```

此小型 vertical 場景中 64m AOI 幾乎讓 24 人全互見，因此還看不出 Layer-aware bucket 的收益。

### Queue / allocation

```text
max command depth       25
commands total          2,401
commands/tick           24.01
TotalAlloc              18,620,288 bytes
Mallocs                 79,051
GC                      5
GC pause total          0.805 ms
```

目前 allocation 很明顯存在，但 24 人的 GC pause 仍很低；不能因此直接把所有資料結構換成 `sync.Pool`。

### 第一個真正的 scaling blocker：Full JSON Snapshot

正式 5 秒 measurement 中：

```text
Realtime Snapshot attempts = 1,200
ErrDatagramTooLarge         = 1,200
```

也就是 24 人全互見時，現有 **Full AOI + JSON v1 WorldSnapshot 已 100% 超過 1200-byte UDP datagram guard**。

Bot 在完整 run 中收到：

```text
Snapshots       37
Corrections     1,331
```

37 個 Snapshot 主要出現在 ramp-up / 尚未全員加入、payload 還較小的階段。steady-state 24 人 Snapshot 已無法送出。

這不是調高 UDP MTU 的理由；它直接證明正式攻城 Realtime replication 不能繼續使用 full JSON snapshot。

## 第二份基線：100-client Gate Zerg

環境同樣為 GitHub Actions hosted `ubuntu-24.04` runner、Go 1.26.5。

```text
Clients             100
Scenario            gate-zerg
Tick                20 Hz
Snapshot            10 Hz
Measurement         8 sec
Ticks               160
```

### Tick

```text
average     3.276 ms
p50         0.513 ms
p95         8.508 ms
p99         9.908 ms
max        10.754 ms
```

20 Hz 的每 Tick budget 是 50ms；即使在 100 人集中 Main Gate、全互見的 hotspot 下，p99 仍約 9.9ms。**目前數據再次證明 single World owner 還有明顯餘裕。**

p50 與 p95 / p99 的落差主要來自 Snapshot 每 2 Tick 才做一次；因此不能只看平均 Tick latency。

### Stage average / Tick

```text
Simulation              0.055 ms
Dynamic replication     0.026 ms
AOI                     1.479 ms
Replication Build       1.590 ms
Delivery                0.070 ms
```

Simulation 本身非常便宜，時間主要已轉移到 **AOI + Replication Build**。而 Stage timing 尚未包含 realtime writer goroutine 的完整 JSON encode / socket flush，所以不能把 3.276ms 當成整台 Server 的完整 CPU 成本。

### AOI / hotspot

```text
queries                  8,000
candidates             800,000
visible                800,000
candidates/query           100
visible/query              100
candidate/visible          1.0
```

Gate Zerg 本來就是全員集中且 64m 內全互見，因此 `candidate/visible = 1.0` 是合理結果。**Layer-aware Spatial Bucket 對這種同 Layer hotspot 幾乎不會解決核心成本**；真正要降的是 per-observer full replication 與更新頻率。

### Queue

```text
max depth before       102
max depth after         20
commands total      16,001
commands/tick        100.006
```

Queue 沒有逼近目前 4096 capacity，也沒有 command error，說明 100 人 × 20Hz input 尚未形成 ingress backpressure。

### Allocation / GC

```text
TotalAlloc          340,556,968 bytes / 8 sec
Mallocs                 534,920 / 8 sec
GC                           164 / 8 sec
GC pause total            33.611 ms
```

換算約：

```text
Allocation rate        ≈ 42.6 MB/sec
Malloc rate            ≈ 66.9k/sec
GC frequency           ≈ 20.5/sec
```

GC pause 總量目前仍沒有把 Tick p99 推近 50ms budget，但 **164 次 GC / 8 秒已足以把 allocation reduction 提升為近期工作**。

這不代表要立刻到處加入 `sync.Pool`。目前最可疑的 hot path 已相當明確：

```text
per-session AOI snapshot
→ visible slice
→ known/current map
→ transform slice
→ JSON encode buffer
```

應先在 Realtime replication redesign 時一起減少資料量與配置，再用下一輪 Load Lab 比較。

### Snapshot

正式 measurement：

```text
ErrDatagramTooLarge      8,001
udp_write errors         8,001
```

100 sessions × 10Hz × 8 秒理論上有約 8,000 個 full snapshot delivery cadence；數據顯示 steady-state full JSON snapshots 幾乎全部超過 UDP guard。

Bot 完整 run 收到：

```text
Snapshots       15
Corrections  9,047
```

15 個 Snapshot 只存在於 ramp-up 的較小 AOI 時期；100 人 steady-state 沒有可用 full snapshot。

## 壓測後的架構結論

目前數據足以排除幾個錯誤優化方向，也足以決定第一個真正的 scaling 修正：

1. **現在不拆 Cell Actor。** 100 人 Gate Zerg p99 約 9.9ms，single-owner mutation 尚未接近 50ms budget。
2. **現在不把 Layer-aware Spatial Bucket 當第一優先。** Gate Zerg 是同 Layer hotspot，candidate/visible 本來就是 1.0；改 bucket 也不會解決 100×100 replication。
3. **第一優先是 Realtime Replication Foundation。** Full JSON Snapshot 在 24 人就已超過 1200 bytes。
4. `WorldSnapshot` 與 self `PositionCorrection` 需要分離明確的 coalescing / priority semantics，不能長期共用一個沒有 message class 的 latest-state mailbox。
5. JSON v1 可以繼續服務 bootstrap / debug，但高頻 Transform replication 應演進為 compact binary payload。
6. Replication 應朝 delta / dirty state、quantized transforms、Tier / cadence、可組合 update block 演進，而不是提高 UDP MTU。
7. allocation 已是近期可量測的第二優先；先消除 full per-session snapshot churn，再視 pprof 決定 scratch reuse / pool。
8. 500 人 full baseline 現在沒有必要拿已知 100% 爆 MTU 的 wire path 硬跑出一個漂亮數字。先修已證實 blocker，再做 500+ 才有工程意義。

因此 roadmap 新增一個小型階段：

```text
S3-C.5  Siege Load Lab
        ↓
S3-C.6  Realtime Replication Foundation
        ├─ Snapshot / Correction stream semantics
        ├─ Compact transform payload baseline
        ├─ MTU-safe chunk / budget rule
        ├─ Allocation reduction
        └─ Load Lab regression
        ↓
S3-D    Gate HP / Attack / Destroy
```

S3-C.6 不會提前做完整 S3-E 的所有 Network LOD；只修正已由量測證實、會阻止攻城測試的底層問題。

## CI 與正式容量的界線

PR workflow 會跑：

- 24-client `vertical-siege` smoke
- 100-client `gate-zerg` 計算基線

另外提供 `workflow_dispatch`，可選 scenario / clients / duration / tick rate / snapshot rate，預設 500 clients。

GitHub hosted runner 的 CPU、VM noisy-neighbor、loopback network 與 production deployment 不同，所以結果只適合：

- 回歸比較
- 找相對瓶頸
- 驗證 scaling curve
- 驗證 payload / queue / allocation 問題

**不可把 GitHub Actions 上的「可跑 N 人」直接宣稱成正式 Server capacity。**

未來正式容量測試必須在固定硬體 / 固定 kernel / 固定 Go build / 固定 network topology 的 dedicated environment 重跑。

## S3-C.5 完成條件

- [x] 真 TCP/UDP Headless Bot
- [x] Load Server / Bot 分離進程
- [x] `distributed` / `gate-zerg` / `vertical-siege`
- [x] Tick p50 / p95 / p99
- [x] Command Queue metrics
- [x] AOI candidate / visible metrics
- [x] allocation / heap / GC metrics
- [x] Server / Bot JSON report
- [x] 24-client Vertical Siege CI smoke
- [x] 100-client Gate Zerg baseline
- [x] `workflow_dispatch` 500+ capability
- [x] 第一個 scaling blocker 已用數據定位
- [x] 後續優化順序已由數據決定

500+ full baseline 不列為本階段的假性勾選項：目前 wire format 已在 24 / 100 人明確 100% 超過 UDP payload budget。應先完成 S3-C.6，再用相同 Load Lab 對照修正前後 scaling curve；dedicated environment 的正式 500+ full-combat capacity test 則在 S5 前完成。

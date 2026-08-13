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

這避免 TLS/Session 以外的 bootstrap、Entity Spawn 與連線 ramp-up 混入 steady-state Tick latency。

連線建立延遲則由 Bot report 另外保存。

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

注意：這些是單一 owner thread 內的 application-stage duration，不含 kernel socket writer goroutine 真正 flush 到 NIC 的時間。

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

這是 S3-E 是否需要 binary codec、delta / chunk、quantization 與 Replication Tier 的直接輸入資料。

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

此小型 vertical 場景中 64m AOI 幾乎讓 24 人全互見，因此還看不出 Layer-aware bucket 的收益；需要 Gate Zerg / larger-world baseline 再判斷。

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

目前 allocation 很明顯存在，但 24 人的 GC pause 仍很低；應先看 100/500 scaling，而不是立刻把所有資料結構換成 `sync.Pool`。

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

這不是調高 UDP MTU 的理由；它直接證明 S3-E 的第一優先應是重新設計 Realtime replication payload。

## 目前得到的優化順序

在更大基線完成前，已可先鎖定：

1. **Snapshot transport / encoding 是第一個已證實的 scaling blocker。**
2. 不能把 `WorldSnapshot` 與 self `PositionCorrection` 當成沒有優先級差異的單一 realtime latest-state mailbox；兩者需要明確 coalescing / stream semantics。
3. Full JSON Snapshot 只適合作為早期 Thin Client bridge，不適合作為攻城 steady-state 格式。
4. 優先評估 compact binary transform block、delta / dirty mask、quantization 與 Replication Tier。
5. 目前沒有證據需要 Cell Actor / lock-free ring buffer。
6. allocation optimization 必須等待 100 / 500 scaling data 再決定 hot path。

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
- [x] 24-client CI smoke
- [x] `workflow_dispatch` 500+ capability
- [ ] 100-client Gate Zerg 基線記錄
- [ ] 修正第一個已證實的 Snapshot scaling blocker（S3-E 工作，不在本階段偷改）
- [ ] dedicated environment 的 500+ full baseline（S5 前必做）

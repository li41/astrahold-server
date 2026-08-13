# S3-C.6：Realtime Replication Foundation

S3-C.6 是由 S3-C.5 Siege Load Lab 的實測結果直接觸發的修正階段。

S3-C.5 已證明：

- 24-client Vertical Siege 時，steady-state Full AOI + JSON v1 `WorldSnapshot` 已 `1200 / 1200` 超過 1200-byte UDP guard。
- 100-client Gate Zerg 時，`8000 / 8000` Snapshot attempts 超過 UDP guard。
- 100 人 Tick p99 約 9ms，仍遠低於 20Hz 的 50ms budget，所以 single World owner 不是第一瓶頸。
- 100 人 8 秒約產生 340MB allocation、53.5 萬次 malloc、160 次 GC，Replication allocation 已值得優先處理。

因此本階段只處理兩件已被數據證實的問題：

1. Realtime Snapshot / Correction 的 semantic stream 不可互相覆蓋。
2. Realtime transform payload 必須符合 1200-byte UDP budget。

Gate HP、Combat、Replication Tier、dirty delta 與 500+ 正式容量不屬於本階段。

## Protocol v3

S3-C.6 將 Astrahold Protocol 升級為 **v3**。

```text
Reliable control path
SessionWelcome / Spawn / Despawn / Dynamic World
        ↓
JSON bridge（開發期保留）

Realtime path
Move / Snapshot / Correction
        ↓
GameV1 compact binary
```

這不是把整個 protocol 一次改成 binary。高頻 Realtime 先換掉，低頻 Reliable control message 繼續使用容易除錯的 JSON bridge。

## Realtime binary layout

### ClientMoveInput

```text
float32 direction_x     4 bytes
float32 direction_z     4 bytes
-------------------------------
payload                 8 bytes
```

完整 UDP datagram：

```text
ASTU header             24
ASTR frame              28
payload                   8
-------------------------------
total                    60 bytes
```

### PositionCorrection

```text
uint64 tick                 8
uint64 entity_id            8
float32 x                   4
float32 y                   4
float32 z                   4
float32 yaw                 4
uint16 layer                2
uint32 last_input_sequence  4
--------------------------------
payload                    38 bytes
```

完整 UDP datagram 為 **90 bytes**。

### WorldSnapshot chunk

Snapshot payload header：

```text
uint64 tick              8
uint16 chunk_index       2
uint16 chunk_count       2
uint16 entity_count      2
----------------------------
header                  14 bytes
```

每個 transform：

```text
uint64 entity_id         8
float32 x                4
float32 y                4
float32 z                4
float32 yaw              4
uint16 layer             2
----------------------------
transform               26 bytes
```

Protocol v3 固定：

```text
MaxSnapshotEntitiesPerChunk = 43
```

最大封包：

```text
ASTU header                         24
ASTR frame                          28
Snapshot payload header             14
43 × compact transform            1118
---------------------------------------
total                             1184 bytes
```

因此仍保留 16 bytes headroom，不提高既有 **1200-byte UDP guard**，也不依賴 IP fragmentation。

## Snapshot chunk contract

`WorldSnapshot` 現在是：

```text
Tick
ChunkIndex
ChunkCount
Entities[]
```

例如 100 個可見 Entity：

```text
chunk 0 = 43
chunk 1 = 43
chunk 2 = 14
```

Server 會對同一 authoritative tick 產生同一組 chunk。

Client **不得逐 chunk 套用到 interpolation buffer**。只有同一 tick 的 `ChunkCount` 全部收到後，才組成完整 snapshot 並提交 Presentation。

若 UDP loss 導致舊 tick 不完整，而更新 tick 已到達：

```text
old incomplete snapshot
        ↓
newer tick arrives
        ↓
discard whole old set
        ↓
assemble newer tick
```

這避免半張 snapshot 讓部分角色更新、部分角色停在舊位置。

## Realtime semantic streams

Protocol v2 的 `clientConnection` 只有一個 capacity=1 latest-state mailbox：

```text
WorldSnapshot ─┐
               ├─ same latest slot
Correction ────┘
```

因此兩種不同語意的 state 可以互相覆蓋。

Protocol v3 改為：

```text
Realtime Mailbox
├── latest PositionCorrection
└── current WorldSnapshot set
    ├── chunk 0
    ├── chunk 1
    └── ...
```

規則：

- `PositionCorrection` 永遠只保留最新一筆。
- `WorldSnapshot` 以 `Tick` 為 snapshot set。
- 新 tick 的 `chunk 0` 可以取代尚未送完的舊 snapshot set。
- Writer 優先送最新 correction，再送目前 snapshot chunks。
- Client 不能用單一全域 `Envelope.Sequence` 判斷所有 realtime packet freshness。

最後一點很重要：Correction 可能因優先級而先送，即使它的 Envelope sequence 比尚未送出的 Snapshot chunk 大。若 Client 用全域 sequence 丟棄所有較小序號，就會把合法 Snapshot chunk 誤判成 stale。

因此 Protocol v3 的 freshness 是 **semantic-stream scoped**：

```text
Snapshot
→ Tick + ChunkIndex / ChunkCount

PositionCorrection
→ authoritative Tick + LastProcessedInputSequence

ClientMoveInput ingress
→ Session input sequence
```

`Envelope.Sequence` 仍存在於 wire frame，供 diagnostics / ingress ordering 使用，但不能跨不同 realtime message semantic stream 做全域 latest-state 判定。

## Replication allocation 修正

S3-C.6 沒有試圖做 zero-allocation，而只修 S3-C.5 已證明的 hot path。

### per-session View Set 雙 buffer

舊版每個 snapshot tick 重新建立：

```text
current := make(map[EntityID]struct{}, ...)
```

現在每個 Session 保留：

```text
known
scratch
```

每 Tick `clear(scratch)` 後重用，完成後 swap：

```text
known, scratch = scratch, known
```

### 避免不必要的排序 copy

Spatial AOI 本身已回傳 EntityID 穩定排序。

Replication 先檢查是否已排序：

- 已排序：直接使用原 slice。
- 非排序 caller：才 defensive copy + sort。

因此不把 correctness 依賴藏在 caller 裡，但 hot path 不再固定付一次 copy/sort。

## Before / After：24-client Vertical Siege

兩次皆為 GitHub Actions hosted Ubuntu runner、Go 1.26.x、20Hz Tick、10Hz Snapshot、5 秒 measurement。

| 指標 | S3-C.5 | S3-C.6 | 變化 |
|---|---:|---:|---:|
| Tick p99 | 0.802 ms | 0.805 ms | 約持平 |
| TotalAlloc | 18,587,552 B | 10,352,952 B | **-44.3%** |
| Mallocs | 78,985 | 40,657 | **-48.5%** |
| GC | 5 | 4 | -20% |
| DatagramTooLarge | 1,200 | **0** | **-100%** |
| Bot completed snapshots | 舊格式無完整組裝概念 | **1,571** | 可正常持續同步 |
| Bot decode errors | 0 | 0 | 正常 |
| Bot network errors | 0 | 0 | 正常 |

24 人 CPU latency 沒有明顯改變，這符合預期；本階段主要目標是 wire correctness 與 allocation，而不是讓原本就只有不到 1ms 的 Tick 再追極小差異。

## Before / After：100-client Gate Zerg

兩次皆為 20Hz Tick、10Hz Snapshot、8 秒 measurement。

| 指標 | S3-C.5 | S3-C.6 | 變化 |
|---|---:|---:|---:|
| Tick average | 2.991 ms | 2.068 ms | **-30.9%** |
| Tick p99 | 8.979 ms | 8.292 ms | **-7.7%** |
| Tick max | 9.358 ms | 8.439 ms | 改善 |
| AOI average / Tick | 1.412 ms | 0.975 ms | **-31.0%** |
| Replication Build / Tick | 1.397 ms | 0.906 ms | **-35.1%** |
| TotalAlloc | 339,939,416 B | 179,654,016 B | **-47.2%** |
| Mallocs | 534,712 | 372,872 | **-30.3%** |
| GC | 160 | 80 | **-50%** |
| DatagramTooLarge | 8,000 | **0** | **-100%** |
| Bot completed snapshots | 舊格式幾乎無 steady-state Snapshot | **9,970** | 正常持續同步 |
| Incomplete snapshot resets | N/A | **0** | loopback 無 loss |
| Bot decode/network errors | 0 / 0 | **0 / 0** | 正常 |

100-client run 的 `outbound_messages` 從 16,199 增加到 32,199，是預期結果：一個 100-entity Snapshot 現在會拆成 3 個合法 UDP packets，而不是生成一個超過 MTU、最後被丟棄的大封包。

同樣地，Bot aggregate UDP receive bytes 從約 1.77MB / 11s 增加到約 27.86MB / 11s。這代表資料**真的送到了 Client**，不是 bandwidth regression 的單獨證據。

## S3-C.6 解決了什麼、還沒解決什麼

已解決：

- Full JSON Snapshot 的 1200-byte MTU correctness 問題。
- Snapshot / Correction 共用 latest slot 的 semantic collision。
- Client 收到半張 snapshot 就更新畫面的風險。
- 一部分已量到的 per-session Replication allocation / sorting churn。

尚未解決：

- 500 人全互見的 aggregate bandwidth。
- Entity dirty tracking / delta snapshot。
- Replication Tier / Network LOD。
- per-cell / per-tier serialized block reuse。
- 遠距角色 cadence 降頻。
- 正式 binary schema evolution / compatibility negotiation。

以 500 人全互見、10Hz、每 transform 26 bytes 粗估，光 raw transform payload 就會達到約 65MB/s，再加 frame/datagram overhead；因此 **chunking 只解決 MTU correctness，不代表 full-AOI bandwidth 已可商用**。

這些是 S3-E Siege Replication Scaling 的工作。

## CI Regression Gate

S3-C.6 起，24-client 與 100-client Load Lab 除了要求程序成功，也必須同時滿足：

```text
ready_clients == requested_clients
completed_snapshots > 0
decode_errors == 0
bot network_errors == 0
server datagram_too_large == 0
unexpected_tick_errors == 0
delivery_errors == 0
server network_errors == 0
```

另外 `internal/codec/**`、`internal/protocol/**`、`internal/replication/**` 的 PR 變更都會觸發 Siege Load Lab，避免未來 wire / replication 改動繞過負載回歸。

## S3-C.6 完成條件

- [x] Protocol v3 contract
- [x] Realtime compact binary codec
- [x] 43-entity / 1184-byte MTU-safe Snapshot chunk
- [x] Snapshot / Correction semantic mailbox 分流
- [x] Client complete-snapshot assembly contract
- [x] per-session view set reuse
- [x] 24-client `datagram_too_large = 0`
- [x] 100-client `datagram_too_large = 0`
- [x] 100-client 完整 Snapshot 持續到達 Client
- [x] allocation / GC 相對 S3-C.5 顯著下降
- [x] Load Lab regression gate
- [ ] Godot Client Protocol v3 PR 合併並通過 main runtime probe
- [ ] Server Protocol v3 PR 合併並通過 main CI

完成後下一個 gameplay milestone 才是 **S3-D Gate HP / Attack / Destroy**。

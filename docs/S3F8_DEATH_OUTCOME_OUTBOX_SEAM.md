# S3-F.8 Death Outcome Outbox Seam

## Scope

S3-F.8 建立 Server-owned **Death Outcome Outbox Seam**，把 S3-F.7 已成立的 authoritative death transaction轉成 immutable event，提供 future persistence / audit consumer一個不需要回頭修改 lethal gameplay path 的邊界。

本階段刻意不加入資料庫、Kafka、account/character durable storage、inventory/currency/durability/progression penalty，也不新增 Client protocol message。`worldd` 的第一個 consumer只做 process-local structured logging。

## Why an outbox seam first

目前 repo：

- `go.mod` 沒有外部 database / broker dependency；
- `internal/` 沒有 persistence/storage subsystem；
- S3-F.7 已明確把 durable account/character storage列為 deferred。

因此直接宣稱 persistence會是假證據。S3-F.8先把 event shape、ordering、delivery progress與failure semantics固定，後續 durable consumer可以替換 structured-log consumer，而不改 Character / respawn / penalty transaction。

## Immutable event

`deathoutcome.Event`包含：

- process-local monotonic `EventID`
- `EntityID`
- authoritative `DefeatRevision`
- `DeathContext`
- `DefeatedTick`
- respawn policy revision
- death penalty policy revision
- death-time bound respawn truth：scheduled、spawn point id/class、Position、DueTick
- penalty transaction是否 applied
- checkpoint是否真的 forfeited

Event在 `record revision -> bind respawn -> apply penalty`完成後才建立，因此 consumer看到的是該次 death transaction已固定的結果，不需要從後續 mutable checkpoint/pending state重新推導。

## Transaction ordering

Player lethal transition：

1. Character damage成立；
2. persistent movement立即歸零；
3. S3-F.7產生 DefeatRevision；
4. S3-F.4 respawn policy先綁定本次 destination / DueTick；
5. S3-F.7 exactly-once death penalty套用；
6. S3-F.8建立 immutable Death Outcome Event並 enqueue；
7. existing dirty vitals path繼續。

因此 PvE checkpoint forfeiture event會同時表達：

- 本次 respawn仍 bound到死亡前 checkpoint；
- penalty transaction已成立；
- checkpoint已被 forfeited，只影響後續死亡。

## Outbox truth

`deathoutcome.Outbox`是 bounded、thread-safe、process-local memory outbox。

- `Enqueue`由 World owner呼叫，只做 memory mutation，不做 DB / network I/O。
- `Pending(limit)` oldest-first回傳 snapshot，**不刪除**事件。
- `Confirm(EventID)`只允許 oldest-first；成功後才前進 delivery truth。
- same Entity incarnation的同一 `(EntityID, DefeatRevision)` + 同 payload為 idempotent no-op。
- same revision不同 payload為 conflict。
- revision regression為 invariant fault。
- `ResetEntity`在 `leave_world`只清 incarnation dedupe；已 pending的舊事件仍留給 consumer。

這些 semantics只保證 process內的 deterministic delivery progress，不保證 process restart後 durability。

## Capacity / failure policy

Production `worldd`預設 outbox capacity為 4096，可用 `-death-outcome-outbox-capacity`調整；非正值啟動失敗。

Outbox full時：

- `enqueue_death_outcome`回報 CommandError；
- `DeathOutcomeEventEnqueueFailures`增加；
- 已成立的 Character defeat不 rollback；
- 已綁定的 respawn schedule不 rollback；
- 已套用的 death penalty不 rollback。

這是刻意的 bounded safety policy：audit/output pressure不能把玩家永久卡在 gameplay transaction；但 event emission gap會被明確暴露，不會假裝成功。

## worldd consumer

`worldd`啟動獨立 goroutine，每100ms最多取64個 pending event：

1. 讀 `Pending(64)`；
2. 寫 structured key/value log；
3. log完成後 `Confirm(EventID)`；
4. shutdown時再 drain目前 pending batch直到空。

World owner不直接執行 logging或future external I/O。

startup log會明確說明此 outbox是 process-local structured logging，**不是 durable persistence**。

## Metrics

S3-F.8新增：

- `DeathOutcomeEventsEnqueued`
- `DeathOutcomeEventEnqueueFailures`

S3-F.7既有：

- `DeathOutcomesRecorded`
- `DeathPenaltyTransactionsApplied`
- `DeathPenaltyCheckpointForfeits`

因此可區分 authoritative death truth、penalty truth與event emission truth。

## Cleanup / EntityID reuse

S3-F.7仍在 `leave_world`清 Runtime DefeatRevision與death penalty applied revision。

S3-F.8額外呼叫 `Outbox.ResetEntity(EntityID)`，讓新的 EntityID incarnation可以從 revision 1重新開始；這不會刪除舊 incarnation尚未 Confirm的 pending event。process-local `EventID`仍可區分兩筆事件。

## Invariants preserved

- Protocol維持 v6。
- Client repo不修改。
- shared `gameplay.json` / Gameplay World identity不修改。
- S3-F.4 death context與respawn binding不修改。
- S3-F.5 resurrection不修改。
- S3-F.6 protection grace不修改。
- S3-F.7 checkpoint penalty與DefeatRevision contract不修改。
- input/action sequence與combat cooldown不 reset。
- snapshot cadence、lifecycle / Initial Vitals / Dirty Vitals budgets不修改。
- 不修改 S3-E workflow filters或thresholds。

## Tests

`internal/deathoutcome/outbox_test.go`：

- Pending non-destructive
- oldest-first Confirm
- same revision idempotency
- conflicting same revision rejection
- revision regression
- full capacity不前進 truth
- Confirm後retry enqueue
- EntityID reuse ResetEntity仍保留舊 pending event
- event shape validation

`internal/worldruntime/death_outbox_test.go`：

- event捕捉 death-time bound checkpoint / due tick
- event捕捉 policy revisions
- event捕捉 penalty applied / checkpoint forfeited
- outbox full不 rollback Character defeat
- outbox full不 rollback respawn schedule
- outbox full不 rollback checkpoint penalty

`cmd/worldd/main_test.go`：

- structured-log drain會 oldest-first Confirm pending events

## Deferred

後續 bounded stage才處理 durable event journal / persistence consumer、restart recovery、stable character/account identity，以及真正的 currency/inventory/durability/progression penalty。S3-F.8不能被解讀為這些已完成。

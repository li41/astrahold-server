# S3-F.9 Durable Death Outcome Journal

## Scope

S3-F.9 在 S3-F.8 process-local Death Outcome Outbox 後加入第一個真正可跨 `worldd` process restart 的 durable handoff：**append-only death outcome journal + durable consumer checkpoint**。

本階段只處理 death outcome event durability / recovery，不加入 account/character database、inventory、currency、durability、progression penalty、journal compaction/rotation、multi-process writer coordination或 Client protocol state。

## Durability boundary

S3-F.8 的 WorldRuntime仍只做 bounded O(1) in-memory outbox enqueue；world owner不直接碰 filesystem。

獨立 journal worker固定按 oldest-first順序處理 outbox：

1. `Pending(64)`取得 process-local event；
2. append一筆 framed journal record；
3. `fsync` journal file；
4. **只有 append + fsync成功後**才 `Confirm(EventID)` in-memory outbox。

因此：

- crash發生在 fsync前：outbox尚未 Confirm；如果磁碟只留下 incomplete trailing frame，restart會把該 torn tail截回最後完整 record；
- crash發生在 fsync後、outbox Confirm前：durable journal record已存在，restart可 recovery，即使 process-local outbox消失也不會失去已完成的 durable handoff；
- journal append/sync error：worker停止並取消 worldd context，不把 storage failure當成成功。

S3-F.7 / F.8 gameplay failure policy不反轉：Character defeat、respawn binding、death penalty先成立，journal故障不能 rollback已成立的 gameplay transaction；但 production server會 fail-closed停止繼續服務，避免長時間產生無 durability保證的新 outcome。

## Journal identity and record identity

每個 journal檔建立時產生128-bit random Journal ID並寫進 durable header。

每筆 record使用 journal-local、跨 restart單調的 `JournalRecordID = 1, 2, 3, ...`。這刻意**不使用** S3-F.8 process-local `EventID`作 durable identity，因為新的 `worldd` process會從 EventID 1重新開始。

Record仍保留原本 EventID，方便對照該 process內的 outbox delivery；真正 durable ordering / checkpoint使用 JournalRecordID。

## Framing / corruption detection

Journal schema v1每筆 frame：

- 4-byte big-endian JSON payload length；
- strict JSON payload（schema version + JournalRecordID + immutable Death Outcome Event）；
- 4-byte CRC32C (Castagnoli) of payload。

單筆 payload上限1 MiB。

startup會完整掃描 journal並驗證：

- header magic / Journal ID；
- frame length；
- CRC；
- strict JSON / schema version；
- JournalRecordID必須連續；
- death outcome event shape仍符合 S3-F.8 contract。

Corruption採 fail-closed。唯一可自動修復的情況是**最後一筆 frame不完整**：這被視為 crash-torn append，journal截回上一個完整 record的 end offset並 `fsync`。CRC mismatch、非法record、ID gap等不會被靜默截掉。

## Durable consumer checkpoint

Checkpoint schema v1保存：

- Journal ID；
- 最後成功 downstream-consumed JournalRecordID；
- 該 record的精確 end offset。

checkpoint只能指向同一 Journal ID，且 RecordID不能超過 journal durable tail、offset必須精確對應該 RecordID。stale/different journal、ahead checkpoint、offset mismatch都拒絕啟動。

checkpoint更新採：

1. create temp file in same directory；
2. write strict JSON；
3. `fsync(temp)`；
4. rename覆蓋 checkpoint；
5. `fsync(directory)`。

Consumer side effect先執行，checkpoint後更新，因此 recovery semantics是 **at-least-once downstream replay**：crash在 side effect後、checkpoint前可能重播同一 record，但不會因 checkpoint先走而跳過尚未處理的 record。

## Startup recovery

`worldd`在開放 network server之前：

1. open / validate journal；
2. repair only an incomplete final frame if necessary；
3. load / validate durable checkpoint；
4. replay所有 checkpoint後的 journal records到目前 structured-log consumer；
5. 每筆成功後原子更新 checkpoint；
6. recovery完成後才建立正常 runtime/network loop。

Production startup log會輸出 Journal ID、journal last record、checkpoint record與本次 recovered record數。

## Runtime worker / shutdown ordering

正常執行時worker每100ms最多：

- durable-ingest 64筆 outbox events；
- consume/checkpoint 64筆 journal records。

Journal worker error會取消主 worldd context。

正常 shutdown時 journal worker**不跟 world loop同時停止**：先讓 world loop停止產生新 outcome，再取消 journal worker。worker收到自己的 shutdown後：

1. 把剩餘 outbox全部 append + fsync並 Confirm；
2. 把所有尚未checkpoint的 durable journal records consume + checkpoint；
3. 才退出，最後由 main關閉 journal file。

這避免最後一個 world tick在 journal worker提早退出後留下 process-local-only event。

## S3-F.8 outbox capacity remains

S3-F.8 outbox仍是4096預設 capacity與WorldRuntime backpressure boundary。S3-F.9沒有把 filesystem I/O移進 world owner，也沒有改 outbox full時的 gameplay semantics。

Journal worker/storage故障是另一層 durability fault；production會停止server，而不是調高既有 replication / lifecycle budgets來掩蓋。

## Explicit non-goals

本階段沒有完成：

- character/account stable durable identity；
- SQL/NoSQL account storage；
- Kafka / external broker exactly-once delivery；
- journal compaction / retention / rotation；
- 多個worldd process共用同一journal的writer lock / lease；
- inventory/currency/durability/progression death penalty；
- Client-visible death history或penalty UI。

所以 S3-F.9 的「durable」只指本機 append-only journal與consumer checkpoint能跨正常 process restart恢復，不宣稱磁碟故障容忍、跨主機replication或external exactly-once。

## Invariants preserved

- Protocol維持 v6。
- Client repo不修改。
- shared `gameplay.json` / Gameplay World identity不修改。
- S3-F.4 death context / respawn binding不修改。
- S3-F.5 resurrection不修改。
- S3-F.6 protection grace不修改。
- S3-F.7 checkpoint penalty / DefeatRevision contract不修改。
- S3-F.8 immutable event / bounded outbox contract不修改。
- WorldRuntime tick不做 filesystem I/O。
- input/action sequence、combat cooldown、snapshot cadence不修改。
- lifecycle / Initial Vitals / Dirty Vitals budgets不修改。
- 不修改 S3-E workflow filters或thresholds。

## Tests

`internal/deathoutcome/journal_test.go`：

- append + reopen保留 Journal ID / JournalRecordID / event；
- checkpoint跨 reopen恢復，只讀 checkpoint後 records；
- incomplete final frame crash recovery只截 torn tail；
- CRC corruption fail-closed；
- checkpoint different-journal / ahead / wrong-offset拒絕；
- closed journal append不前進 truth。

`cmd/worldd/main_test.go`：

- journal append成功後才 Confirm outbox；
- journal failure時 outbox仍 pending；
- startup recovery只重播 durable checkpoint之後的 records；
- shutdown worker會把剩餘 outbox全部 durable-ingest並把checkpoint追到 journal tail。

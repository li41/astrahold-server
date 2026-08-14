# S3-F.11 Durable Character State Store

## Scope

S3-F.11 在 S3-F.10 durable CharacterID 上建立第一個 **trusted character core-state durable store** 與 optimistic revision save seam。

本階段只處理：

- trusted CharacterID 的 durable per-character record；
- HP / MaxHP / Defeated / authoritative Transform snapshot；
- Gameplay World provenance；
- optimistic revision / compare-and-swap；
- `leave_world` 時由 world owner capture immutable save intent；
- world tick 外的 persistence worker。

本階段 **不實作 reconnect restore**。尤其不在 persistence layer 猜測 defeated character 應如何重建 pending respawn、checkpoint、revive protection、input/action sequence 或 combat cooldown；那些 restore semantics 必須在後續 bounded stage 明確設計。

## Trusted identity only

Durable state store只接受 `characteridentity.AssuranceTrusted`。

S3-F.10 default development `tcpudp` 仍產生 fresh ephemeral CharacterID，因此一般未驗證 development connection：

- 可以正常 join / play / leave；
- 不會產生 durable character-state record；
- 不會因為本階段而突然變成 returning-character account。

這避免 process-local ephemeral identity 靜默污染 durable storage。

## Durable record

每個 record schema version 1 保存：

- `character_id`
- `revision`
- `world_id`
- `world_revision`
- `gameplay_sha256`
- `hp`
- `max_hp`
- `defeated`
- authoritative position `x/y/z/layer`
- authoritative `yaw`

CharacterID不直接拿來當 filename。Store以 CharacterID SHA-256 作為檔名，record本身仍保存完整 CharacterID並在 Load 時比對，避免 path interpretation 與 key/file mismatch。

### Snapshot validation

Store fail-closed驗證：

- WorldID / world revision不可為空；
- gameplay SHA-256必須是64字元 lowercase hex；
- MaxHP > 0；
- HP <= MaxHP；
- Defeated 必須對應 HP == 0；alive 必須 HP > 0；
- position與yaw不可為 NaN / Inf。

JSON decoder禁止 unknown fields與 trailing JSON/data；schema/version/key mismatch視為 corrupt record，不會靜默忽略。

## Optimistic revision

`Store.Save(identity, expectedRevision, snapshot)` 是 compare-and-swap：

- `expectedRevision = 0`：create-only；成功產生 revision 1；
- 已存在 record必須精確匹配 expected revision；
- 成功寫入只前進一個 revision；
- stale writer回 `ErrRevisionConflict`，不覆蓋 current truth；
- `uint64` revision overflow fail-closed。

同一 `Store` instance 以 mutex保護 read/check/write transaction，因此同 revision競爭只有一個 writer成功。

### Single-writer directory contract

S3-F.11 **不是 distributed database**。

Character state directory必須由單一 worldd / 單一 Store ownership寫入。本階段沒有跨 process filesystem lock、lease或distributed CAS；atomic rename不能被解讀為 multi-host transaction。

未來若要多 host共享同一 character store，應換成真正支援 conditional write / transaction 的 backend，而不是多個 process共同寫這個 directory。

## Durability boundary

成功 `Store.Save` 的順序：

1. encode完整 record；
2. write temp file；
3. temp file `fsync`；
4. atomic rename到 CharacterID hash record path；
5. directory `fsync`；
6. 才向 caller回報成功。

因此成功 Save後的 record是 durable truth；stale/invalid/corrupt/overflow失敗不前進 revision。

## World provenance

Snapshot保存：

- Gameplay World ID；
- Gameplay World revision；
- Gameplay SHA-256。

原因是 position / layer只在特定世界拓樸下有意義。後續 restore stage必須先驗證 provenance，再決定可否原位 restore、需要 migration，或 fail closed；S3-F.11不會把舊世界座標直接套到任意新世界。

## Runtime save seam

`WithCharacterStateOutbox(outbox, worldRef)`把 bounded save seam注入 WorldRuntime。

`leave_world` ordering：

1. remove Session registry entry；
2. **在任何 character/world cleanup之前** capture authoritative trusted-character snapshot；
3. enqueue immutable save intent；
4. 正常執行 replication/vitals/respawn/death state cleanup；
5. remove character identity / character state / world entity；
6. close connection。

因此 persistence worker不需要跨 goroutine讀 mutable world state。

### Failure policy

Character-state save seam不是 gameplay transaction的一部分：

- outbox full / invalid world provenance / internal capture error會記錄 `enqueue_character_state_save` command error；
- `CharacterStateSaveIntentFailures`增加；
- **leave仍繼續**，不把已要求離開的角色卡在world；
- successful enqueue增加 `CharacterStateSaveIntentsEnqueued`。

Ephemeral identity是預期 skip，不算 failure。

## Process-local save outbox

`characterstate.Outbox`：

- bounded；
- thread-safe；
- monotonically increasing process-local IntentID；
- `Pending` non-destructive；
- `Confirm`只允許 oldest-first。

它的目的只是把 disk I/O移出 world tick，**不是 durable queue**。突然 process crash可能丟失尚未交給 Store成功保存的 pending intent。本階段只宣稱已成功 `Store.Save` 的 record durable。

## worldd persistence worker

worldd新增：

- `-character-state-dir`，default `data/character-state`；
- `-character-state-outbox-capacity`，default 4096。

Worker每批：

1. `Pending` save intents；
2. Load current durable record；
3. 以 current revision作 expected revision；不存在則 expected=0；
4. CAS Save；
5. Save成功後才 Confirm intent。

同一 character若有多個 sequential intents，revision依序前進且最後 snapshot成為 durable current truth。

Worker在 shutdown時嘗試 drain目前 outbox。這不代表未被 world loop處理的 network disconnect一定會產生 save intent；S3-F.11的 durability boundary仍是「world owner已 capture並成功 Store.Save」。

## Tests

### `internal/characterstate`

- create / update / reopen；
- create-only與 stale revision conflict；
- concurrent same-revision writer只有一個成功；
- revision overflow；
- trusted-only / ephemeral rejection；
- invalid HP/Defeated/NaN/Inf/world provenance rejection；
- unknown field / trailing data / CharacterID mismatch fail-closed；
- hashed filenames與不同 character isolation；
- save outbox bounded / Pending / ordered Confirm。

### `internal/worldruntime`

- trusted leave capture authoritative HP/MaxHP/Defeated/position/yaw/world provenance；
- ephemeral leave不進 durable outbox；
- outbox full記 failure但不 rollback leave cleanup。

### `cmd/worldd`

- persistence worker Save-before-Confirm；
- 同 character sequential intents revision 1 -> 2；
- shutdown drain pending intent。

## Preserved invariants

- Protocol維持 v6；
- Client repo不修改；
- `gameplay.json` / Gameplay World identity不修改；
- S3-F.4～F.10 respawn / resurrection / revive protection / death penalty / death outcome / journal / CharacterID semantics不修改；
- no DB/network/file I/O inside world tick；
- lifecycle / Initial Vitals / Dirty Vitals budgets不修改；
- snapshot cadence、input/action sequence、combat cooldown不修改；
- 不修改 S3-E / Siege workflow filters或thresholds。

## Deferred

後續 bounded stage才處理：

- trusted CharacterID join時從 Store load；
- Gameplay World provenance compatibility / migration policy；
- HP/Defeated/transform restore transaction；
- defeated character pending respawn/checkpoint restore semantics；
- persistence revision與active session ownership整合；
- periodic/autosave policy；
- durable save-intent queue / crash recovery；
- multi-process / multi-host storage backend；
- inventory/currency/durability/progression。

S3-F.11不能被解讀為 reconnect restore或完整角色 persistence已經完成。

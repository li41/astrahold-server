# S3-F.12 Trusted Character State Restore Seam

## Scope

S3-F.12 在 S3-F.11 durable Character State Store 上建立第一個 **trusted character restore seam**。

本階段只處理已通過 Server-side trusted identity resolver 的 CharacterID：

- reconnect 前讀取 durable core-state record；
- restore 前先 flush 已經存在於 S3-F.11 save outbox 的 pending leave intents；
- exact Gameplay World provenance validation；
- alive character 的 HP / MaxHP / authoritative Transform restore；
- transport pre-welcome validation；
- WorldRuntime 單一 join transaction 套用 restore。

本階段不實作 login/auth protocol、session takeover、defeated pending respawn restore、inventory/currency/progression或跨 host storage。

## Why restore is not just Store.Load

S3-F.11 的 leave save 是：

`world owner capture -> process-local save outbox -> async durable Store.Save`

因此如果同一 trusted CharacterID 很快 reconnect，而 restore 直接 `Store.Load`，可能讀到前一個 durable revision，因為最新 leave snapshot 仍在 process-local outbox。

S3-F.12 在 worldd 新增 `characterStatePersistence` coordinator：

- persistence worker 的 batch Save；
- trusted restore read；

兩者共用同一 mutex serialization boundary。

`LoadRestore` ordering：

1. lock persistence coordinator；
2. durable flush **目前已經 pending** 的 save intents，仍維持 oldest-first；
3. 每筆仍是 `Load current revision -> CAS Save -> Confirm intent`；
4. outbox 已清空後才 Load target CharacterID；
5. convert durable Record -> immutable `worldruntime.CharacterRestore`；
6. exact validate identity/world/state；
7. unlock並把 immutable restore交給 transport。

這提供同一 process 內「已被 world owner capture 的 leave intent」對後續 restore 的 read-after-leave ordering。

### Important limit

如果新 connection 到達時，舊 session 的 `leave_world` command **尚未被 world owner處理**，該 save intent自然還不存在。S3-F.12 不新增同步 session takeover/admission lock。

S3-F.10 的 active CharacterID invariant仍是最終 defense：同一 CharacterID不能同時 active於兩個 EntityID。完整 graceful reconnect/session takeover是後續 bounded stage。

## Trusted-only restore

`tcpudp.CharacterRestoreFactory`只在：

`CharacterIdentity.Assurance == trusted`

時被呼叫。

Default development transport仍產生 fresh ephemeral CharacterID，所以：

- 不讀 Character State Store；
- 不執行 restore factory；
- 不因 S3-F.12 變成 returning-character authentication。

Client沒有 CharacterID欄位，Protocol仍是 v6。

## CharacterRestore contract

`worldruntime.CharacterRestore`包含：

- CharacterID；
- durable Store revision；
- exact Gameplay World identity：WorldID / revision / gameplay SHA-256；
- HP / MaxHP / Defeated；
- authoritative Transform：Position(X/Y/Z/Layer) + Yaw。

`ValidateCharacterRestore`同時由 transport與 Runtime使用，規則：

1. identity必須 valid + trusted；
2. restore CharacterID必須等於 session CharacterID；
3. durable revision必須非零；
4. stored Gameplay World identity必須 valid；
5. stored world必須與目前 server world **exact match**；
6. HP / MaxHP / Transform必須合法；
7. Defeated restore在本 stage fail closed。

沒有 world migration fallback，也沒有以 WorldID相同就忽略 SHA/revision差異。

## Why defeated restore fails closed

S3-F.11 record可以保存 `Defeated=true / HP=0`，但目前 durable character-state record沒有保存：

- death context（PvE/PvP/Siege）；
- death-time checkpoint binding；
- respawn destination/class；
- pending respawn DueTick；
- revive protection；
- combat cooldown history。

因此 S3-F.12 無法忠實重建 S3-F.3～F.7 的死亡/respawn transaction。

遇到 defeated durable record時回：

`ErrCharacterRestoreDefeatedUnsupported`

transport在 SessionWelcome之前終止該 connection。S3-F.12 **不會**：

- 自動滿血復活；
- 猜成 PvE default respawn；
- 把角色以 HP=0 丟進 world永久卡住；
- 重設 checkpoint/death penalty來製造可登入結果。

Defeated durable restore需要後續 stage先擴充可重建的 death/respawn durable truth。

## Transport pre-welcome boundary

`tcpudp.Config`新增：

`CharacterRestoreFactory func(characteridentity.Binding) (worldruntime.CharacterRestore, bool, error)`

Handle ordering：

1. allocate process-local SessionID / EntityID；
2. build bootstrap PlayerSpec；
3. resolve CharacterIdentity；
4. 若 trusted且 restore factory存在，執行 restore I/O；
5. factory有 record時，先用目前 `WorldIdentity` validate；
6. validation成功後才建立 Session / peer；
7. 才送 SessionWelcome；
8. EnqueueJoin帶 immutable restore。

因此 world mismatch / defeated / corrupt-load等 restore error不會先送出一個假的成功 welcome。

## WorldRuntime restore transaction

`JoinRequest`新增 optional `Restore *CharacterRestore`。

World owner ordering：

1. validate Session / EntityID / active CharacterID；
2. restore存在時再次 validate trusted identity、CharacterID與 exact world；
3. 用 restored Transform取代 bootstrap PlayerFactory Transform；
4. Spawn world entity；
5. 用 `character.Service.RegisterState`註冊 restored HP / MaxHP / Defeated；
6. 建立 vitals revision；
7. register Session；
8. bind active CharacterID；
9. register replication。

如果 restore validation或 health registration失敗，不留下 partial world entity / character state / session。

`RegisterState`只允許新 EntityID registration，不允許覆寫已存在 character，避免繞過正常 damage/revive transition。

## Session-local state after restore

S3-F.12 只恢復 S3-F.11 record真正保存的 state。

新 session仍然有新的 process/session-local state：

- input sequence從新 session開始；
- action sequence從新 session開始；
- transport outbound sequence重新開始；
- movement input初始為零；
- combat cooldown沒有從舊 session持久化；
- revive protection沒有持久化。

本 stage不虛構這些資料已經 durable。

## Persistence revision

Restore保留 durable Store revision於 `CharacterRestore.Revision`作為 evidence，但 S3-F.12 尚未把 revision變成 active-session lease/token。

S3-F.11 directory仍是 single-worldd/single-writer ownership；save worker仍會在每次 write前 Load current revision並CAS前進。

Cross-process writer、distributed lease與multi-host ownership仍不在 scope。

## Tests

### `internal/character`

- restored HP/MaxHP registration；
- duplicate EntityID拒絕；
- invalid HP/Defeated state拒絕；
- valid defeated representation仍可由底層 character service表達，但 S3-F.12 Runtime policy不允許 restore它。

### `internal/worldruntime`

- trusted alive restore套用 durable HP/MaxHP與Transform；
- bootstrap transform不覆蓋 durable transform；
- exact world mismatch在 Spawn前拒絕；
- defeated restore在 Spawn前拒絕；
- invalid restore不留下 partial Entity/Character/Session；
- trusted/ephemeral/CharacterID mismatch validation。

### `internal/netadapter/tcpudp`

- trusted restore factory結果進 `JoinRequest.Restore`；
- default ephemeral identity完全 skip restore factory。

### `cmd/worldd`

- pending leave intent先 durable Save/Confirm，再 Load restore；
- restore取得最新 revision/snapshot；
- world mismatch fail closed；
- defeated record fail closed；
- missing record回 fresh bootstrap path；
- persistence worker shutdown仍 drain pending intents。

## Preserved invariants

- Protocol維持 v6；
- Client repo不修改；
- `gameplay.json` / Gameplay World identity不修改；
- no DB/network/file I/O inside world tick；
- S3-F.4～F.11 respawn / resurrection / protection / penalty / death journal / CharacterID / durable save semantics不修改；
- lifecycle / Initial Vitals / Dirty Vitals budgets不修改；
- snapshot cadence、input/action sequence、combat cooldown不修改；
- 不修改 S3-E / Siege workflow filters或thresholds。

## Deferred

後續 bounded stage才處理：

- defeated character durable respawn/death context restore；
- graceful reconnect / session takeover / admission synchronization；
- active session與Store revision lease/token；
- periodic autosave；
- durable save-intent crash recovery queue；
- account DB / multi-host backend；
- inventory/currency/durability/progression。

S3-F.12 不能被解讀成完整 authentication、完整 reconnect或完整 MMORPG character persistence已完成。

# S3-F.7 Death Penalty Seam

## Scope

S3-F.7 在 S3-F.1~F.6 已建立的 Defeated / respawn / resurrection / protection lifecycle 上加入第一個 Server-owned death penalty transaction boundary。

本階段刻意只實作一種可逆、可測的 penalty：**PvE checkpoint forfeiture**。不加入 inventory drop、currency loss、durability、XP/progression penalty，也不新增 Client protocol state。

## Why checkpoint forfeiture

現有 Server 已有 authoritative checkpoint acquisition、death-time respawn destination binding與 context policy。checkpoint forfeiture因此不需要新增 persistence、economy或 simulation movement primitive：

- 玩家可重新靠近 checkpoint取得，penalty可逆；
- death-time schedule可先綁定本次 checkpoint，再消耗 checkpoint，ordering可被精確測試；
- 不改 HP primitive、不改 resurrection 30% policy、不改 post-revive protection；
- 不碰 shared `gameplay.json` / Gameplay World SHA。

Production `config/death-penalty.json` schema v1：

- revision `s3f7-001`
- `checkpoint_forfeit_contexts = ["pve"]`
- PvP / Siege第一版不清 checkpoint，避免跨模式死亡意外移除 PvE checkpoint。

## Defeat revision

每個 Player Entity 在 authoritative `alive -> Defeated` transition成功後，由 WorldRuntime owner產生單調 `DefeatRevision`：

`1, 2, 3, ...`

revision不由 Client提供，也不因 respawn / resurrection reset。`leave_world`會清掉 revision state，EntityID reuse從乾淨狀態開始。

## Transaction ordering

Player lethal transition固定走：

1. Character damage transition成立；
2. movement input立即清零；
3. `beginDeathOutcome`產生新的 DefeatRevision；
4. S3-F.4 respawn policy先綁定本次 `Context + SpawnPointID + Position + DueTick`；
5. S3-F.7 death penalty以 `(EntityID, DefeatRevision)` exactly-once套用；
6. vitals revision照既有路徑標 dirty。

因此若玩家在 PvE死亡時已有 checkpoint：本次 scheduled respawn仍使用該 checkpoint；penalty隨後清掉 checkpoint，只影響下一次死亡。玩家復活後可重新取得 checkpoint。

## Exactly-once semantics

`deathpenalty.Service`記錄每個 Entity最新已處理 revision：

- 新 revision：套一次 policy decision並記錄；
- 同 revision retry：no-op，不再次套 penalty；
- 較舊 revision：`ErrRevisionRegression`，視為 internal invariant fault；
- 即使該 context沒有 penalty effect，revision仍記為已處理，避免同一 death outcome日後因 policy path重入被重新判定。

這讓「重新取得 checkpoint後重放舊 death outcome」也不會再次把新 checkpoint清掉。

## Failure policy

Death penalty不是復活安全網的前置條件：若 DefeatRevision極端溢位，Runtime會回報 command error，但仍嘗試既有 respawn scheduling，避免角色因 penalty seam故障而永久卡在 Defeated。

respawn scheduling與 penalty transaction彼此不 rollback已成立的 lethal Character state。

## Cleanup

`leave_world`會清：

- Runtime `deathRevision`
- deathpenalty applied revision
- 既有 respawn checkpoint / pending schedule
- 既有 revive protection / vitals state

## Metrics

S3-F.7新增：

- `DeathOutcomesRecorded`
- `DeathPenaltyTransactionsApplied`
- `DeathPenaltyCheckpointForfeits`

transaction applied與實際 checkpoint effect分開計數；例如 PvP outcome會被 exactly-once處理，但 production policy不會清 checkpoint。

## Invariants preserved

- Protocol維持 v6。
- Client repo不修改。
- shared `gameplay.json` / Gameplay World identity不修改。
- S3-F.4 death context與death-time respawn binding不修改。
- S3-F.5 resurrection HP / cooldown不修改，resurrection不撤銷已發生的 death penalty。
- S3-F.6 protection grace不修改。
- input/action sequence與 combat cooldown不因 death outcome reset。
- lifecycle / Initial Vitals / Dirty Vitals budgets與 snapshot cadence不修改。

## Tests

`internal/deathpenalty/policy_test.go`：

- strict schema / unknown field
- duplicate / invalid context validation
- exactly-once same revision
- revision regression
- no-effect context仍consume revision
- remove / EntityID reuse cleanup

`internal/worldruntime/death_penalty_test.go`：

- PvE schedule先綁 checkpoint、penalty後清 checkpoint
- due respawn仍回已綁定 checkpoint
- 復活後重新取得 checkpoint
- 舊 revision retry不清新 checkpoint
- 第二次 authoritative death產生 revision 2並再次套 penalty
- PvP transaction不清 checkpoint
- leave清 death revision與 idempotency state

## Deferred

後續 death penalty slice才考慮 inventory、currency、durability、progression、persistent account/character storage，以及 Client-visible penalty presentation。這些不能從 S3-F.7 checkpoint seam推導為已完成。

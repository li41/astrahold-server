# S3-F.5 Resurrection Action Seam

## 目標

S3-F.4 已把 player defeat 分成 PvE / PvP / Siege respawn context，並在 defeat tick建立 server-owned pending auto-respawn。

S3-F.5 加入第一個 **player resurrection action seam**：在 pending auto-respawn尚未執行前，仍存活的玩家可以透過既有 `ClientUseAction` intent對倒地玩家施放 `resurrect`，由 Server權威驗證並原地復活。

本階段仍不加入：

- invulnerability grace；
- death penalty；
- resurrection item / resource cost；
- cast time / interrupt；
- Client UI或新的 wire message。

## 1. Wire contract維持 Protocol v6

Resurrection沿用既有：

```text
ClientUseAction
  ActionID
  TargetKind
  TargetID
```

第一版 action：

```text
ActionID: resurrect
TargetKind: entity
```

因此：

- 不新增 `ClientResurrect` message；
- 不改 Protocol version；
- 不需要 Client repo配合 schema變更；
- action sequence與 cooldown仍走既有 combat pipeline。

## 2. Combat action effect

`config/combat-actions.json` 升級為 server-side schema v2，ActionDefinition加入：

```text
effect = damage | resurrect
revive_hp_percent
```

`damage` action仍使用：

```text
base_damage
damage_type
```

`resurrect` action則要求：

```text
targets = [entity]
base_damage = 0 / omitted
damage_type = omitted
revive_hp_percent = 1..100
```

castle-sandbox第一版：

```text
resurrect
range = 4.5m
revive_hp_percent = 30
cooldown = 10s
```

Go runtime fixture若沒有填 `Effect`，零值仍 normalize成 `damage`，避免與本階段無關的大量 test fixture churn；正式 JSON則明確寫出 effect。

## 3. Character transition

Character service新增：

```go
RevivePercent(entityID, percent)
```

HP使用整數百分比與 ceil計算：

```text
ceil(MaxHP * percent / 100)
```

合法百分比必須是 `1..100`。

既有 `ReviveFull`保留並委派給 `RevivePercent(..., 100)`：

- S3-F.2 / S3-F.3 auto respawn仍是 full HP；
- S3-F.5 resurrection使用 action-owned 30%；
- 兩種 lifecycle policy不混用。

## 4. Target legality

Resurrection entity target必須同時符合：

1. 不是 self target；
2. authoritative entity存在；
3. Character state存在；
4. target kind是 `EntityPlayer`；
5. target目前 `Defeated == true`；
6. actor / target同 Layer；
7. 距離 <= action range；
8. authoritative dynamic world LOS成立。

普通 damage action維持原規則：Defeated target仍拒絕。

施法者本身若已 Defeated，仍由 S3-F.1 actor lock在 prepare前拒絕，並消耗 action sequence。

## 5. Resurrection transaction

成功 resurrection在單一 world-owner command phase中：

```text
validate target
→ Character.RevivePercent
→ Cancel pending auto-respawn
→ mark EntityVitals dirty
→ Commit action cooldown
```

它**不會**：

- teleport；
- 改 Layer；
- reset input sequence；
- reset action sequence；
- reset其他 combat cooldown；
- force snapshot；
- reset movement direction成新值。

Defeat transition在 S3-F.1已把 persistent movement input歸零，因此 resurrection後角色仍須收到新的 ClientMoveInput才會移動。

因 transform / AOI完全沒有改變，resurrection dirty Vitals可走一般 reliable Vitals pipeline，不需要 S3-F.2 respawn的 `respawnVitalsAwaitingAOI / desiredOnly` barrier。

## 6. Pending auto-respawn ordering

WorldRuntime Step順序維持：

```text
queued commands
→ applyDueRespawns
→ simulation
```

所以如果 `resurrect` intent在 pending schedule的 **exact DueTick** 被 drain：

1. resurrection action先執行；
2. pending被取消；
3. `applyDueRespawns`看不到該 schedule；
4. 玩家留在倒地位置，以30% HP復活；
5. 不會同 tick再被搬到 respawn point。

這個 ordering讓 resurrection window包含 DueTick本身，而且不需要額外 race / reservation state。

## 7. Sequence / cooldown semantics

Action sequence沿用 S3-F.1 contract：

```text
sequence = intent已被Server處理
```

因此下列情況都 consume sequence：

- target仍活著；
- resurrection cooldown中；
- actor已倒地；
- range / LOS rejection；
- resurrection成功。

但 combat cooldown只有 **成功 gameplay commit** 才開始：

- alive target rejection不吃 cooldown；
- range / LOS rejection不吃 cooldown；
- successful resurrection才 Commit 10秒 cooldown；
- cooldown rejection本身仍 consume新的 action sequence。

重送相同 sequence會得到 `session.ErrStaleAction`。

## 8. Replication

Resurrection只改：

```text
HP
Defeated
```

使用既有 Reliable `EntityVitalsState` full-state revision fan-out。

不新增 death / revive event packet，也不修改 lifecycle budgets、Dirty Vitals budget或 snapshot cadence。

## 9. Acceptance tests

### Character

- full revive仍恢復100%；
- partial revive按百分比 ceil；
- invalid percent不改 state。

### Combat

- schema v2 strict unknown-field handling；
- damage effect保持 server-owned damage source；
- resurrect effect不產生 Damage payload；
- resurrect禁止 gate target與 damage fields；
- cooldown仍只有 Commit後生效。

### WorldRuntime

- PvP defeat建立 pending；
- exact DueTick resurrection先於 policy respawn；
- 30% HP原地復活；
- pending成功取消；
- transform不變；
- defeated前 movement input不會在 resurrection後恢復；
- 新 move sequence才重新移動；
- alive-target rejection consume sequence但不 Commit cooldown；
- success Commit cooldown；
- cooldown rejection保留 target defeated + pending；
- rejection / success / cooldown rejection的 sequence replay均為 stale。

## 10. 不變量

S3-F.5 不修改：

- Protocol v6；
- Client repo；
- `gameplay.json` / Gameplay World identity；
- S3-F.4 respawn context policy；
- S3-F.3 non-destructive Due truth；
- S3-F.2 respawn AOI/Vitals barrier；
- S3-F.1 defeated movement/action lock；
- input/action sequence history；
- unrelated combat cooldowns；
- snapshot cadence；
- lifecycle / Initial Vitals / Dirty Vitals budgets。

## 11. 後續

S3-F.6 建議進入 **Post-Revive Protection Grace**：定義 resurrection / respawn後短暫 protection的 server-owned時間窗、damage legality與明確取消條件。Death penalty繼續留在再下一個 bounded slice。

# S3-F.2 Authoritative Respawn Seam

## 目標

S3-F.1 已把 `Defeated` 收斂成真正 actor capability lock：倒地角色不再移動、不再主動施放 gameplay action，而且倒地期間收到的 input / action sequence 仍會被消耗。

S3-F.2 在這個前提上建立第一個 **server-authoritative respawn transition**，但不急著加入自動倒數、Client respawn request、復活道具或墓地規則。

本階段回答五個 contract：

1. 誰可以觸發 respawn。
2. HP 如何恢復。
3. Position / Layer 從哪裡來。
4. movement / sequence / cooldown 如何處理。
5. 跨 AOI respawn 時 Vitals 與 lifecycle 如何排序。

## 1. Trigger ownership

S3-F.2 **沒有新增 ClientRespawn message**。

Runtime 提供：

```go
EnqueueRespawn(RespawnRequest{
    EntityID: entityID,
    Position: serverChosenPosition,
})
```

`RespawnRequest` 只給 server-side gameplay / admin / 未來 spawn-policy subsystem 使用。Client 不提供 respawn position，也不能直接改 Character state。

所有 mutable state 仍經過既有 bounded WorldRuntime command queue，維持：

```text
Network / DB / GM / gameplay policy
        ↓
bounded command queue
        ↓
single world owner
        ↓
Simulation + Character state
```

## 2. Transition legality / HP policy

只有 `Defeated=true` 的 Character 可以 respawn。

```text
Character missing
→ CommandError

Character alive
→ character.ErrCharacterNotDefeated
→ CommandError

Character defeated
→ allowed
```

第一版 policy 固定：

```text
HP = MaxHP
Defeated = false
```

Character package 只擁有 `ReviveFull` vitals transition；位置、movement 與 replication ordering 由 WorldRuntime 協調。

這不是最終 MMORPG respawn penalty policy。未來若要 30% HP、復活術、城鎮復活、死亡經驗處罰，應在更上層 gameplay policy 決定，不把 policy 塞回 Character health primitive。

## 3. Position / Layer ownership

S3-F.2 不在 Runtime 硬編碼城鎮座標或墓地。

目的地由可信任的 server subsystem 產生完整 authoritative `world.Position`：

```text
X / Y / Z / Layer
```

WorldRuntime 使用既有 `simulation.World.Teleport` gameplay-transition primitive：

- 更新 movement position
- 更新 authoritative Entity transform
- 更新 spatial index
- 清除 teleport 前 persistent movement direction

因此 respawn 不會在新位置沿用死亡前或倒地期間的方向。

## 4. Sequence / cooldown 不重置

Respawn 不是新 Session，也不是重新登入。

因此：

```text
lastProcessedInputSequence  保留
lastProcessedActionSequence 保留
Combat cooldown             保留
```

S3-F.1 已保證倒地期間收到的新 sequence 仍會 consume；S3-F.2 不把這些 history 清掉。

這避免：

- revive 後重播倒地期間舊 move intent
- revive 後重播舊 action intent
- 用死亡 / respawn 當 cooldown reset exploit

## 5. Why respawn needs an AOI/Vitals ordering barrier

Respawn 同時改變兩種 state：

```text
Character Vitals
HP=0, Defeated=true
→ HP=MaxHP, Defeated=false

World Position
old AOI
→ server chosen new Position / Layer
```

如果 respawn 發生在非 snapshot tick，而 Dirty Vitals 立即依舊的 `known` relationship fan-out：

```text
old observer still Knows(entity)
→ receives Defeated=false
→ next snapshot才收到 Despawn
```

這會把「已在另一個 AOI 復活」的狀態先洩漏給舊 observer。

### 第一層：等待下一次 normal AOI snapshot

Respawn 後 Vitals revision 立刻前進並保留 dirty truth，但標記：

```text
respawnVitalsAwaitingAOI
```

在下一次正常 snapshot 完整 rebuild 所有 Session 的 desired AOI 前：

- Initial Vitals 不送此 respawn entity
- Dirty Vitals 不送此 respawn entity
- 不額外 force snapshot
- 不改 SnapshotEveryTicks cadence

因此 S3-E 的 cadence / work staggering 不被 respawn 特例破壞。

### 第二層：stale-known observer protection

Snapshot rebuild 後可能出現：

```text
desired = false
known   = true
```

原因通常是 Reliable `EntityDespawn` backpressure，因為 lifecycle truth 只有 TrySend success 後才 ConfirmDespawn。

S3-F.2 新增兩個 replication query：

```text
Wants(session, entity)
HasKnownOutsideDesired(entity)
```

只要 respawn entity 仍有 known-but-not-desired observer，就暫時進入：

```text
respawnVitalsDesiredOnly
```

此時只有 `Wants=true` 的 Session 能收到該 entity 的 Initial / Dirty Vitals。即使 revived entity 又受到新傷害、Vitals revision 再前進，stale observer 仍被排除。

等所有 stale Despawn 成功確認後，guard 自動清除，該 entity 回到 S3-E.9 的一般 Dirty Vitals mirror hot path。

**重要：** `Wants` lookup 只存在於短暫 respawn guard，不重新放回所有 `(Session, Entity)` Dirty Vitals relationship，因此不回退 S3-E.9 已移除的 hot-path map lookup。

## 6. Replication / Protocol

S3-F.2 沿用：

```text
EntityVitalsState
├── EntityID
├── HP
├── MaxHP
└── Defeated
```

沒有新增 death / revive wire event。

位置仍由既有 realtime：

- WorldSnapshot
- PositionCorrection

以及 Reliable AOI lifecycle：

- EntitySpawn
- EntityDespawn

共同收斂。

Protocol 維持 **v6**，Client repo不需要修改。

## 7. Tests

### Character

`internal/character/revive_test.go`

- alive Character 不可 `ReviveFull`
- defeated Character 恢復 `HP=MaxHP`
- `Defeated=false`

### Replication

`internal/replication/respawn_membership_test.go`

鎖定：

```text
initial desired + confirmed known
→ Wants=true

AOI rebuild removes entity but Despawn未confirm
→ Wants=false
→ Knows=true
→ HasKnownOutsideDesired=true

ConfirmDespawn
→ stale knowledge clears
```

### WorldRuntime

`internal/worldruntime/respawn_test.go`

鎖定：

1. Defeated-only transition。
2. full HP restore。
3. authoritative Position / Layer relocation。
4. respawn 不重置 input sequence history。
5. non-snapshot respawn 不提前 fan-out revived Vitals。
6. old observer Despawn backpressure 時不收到 revived Vitals。
7. current desired self 仍收到 revived Vitals。
8. revive 後的新 HP revision仍不洩漏給 stale-known observer。
9. Despawn confirm 後 desired-only guard 自動清除。

## 8. Acceptance

S3-F.2 merge 前要求：

- Server CI `go test` PASS
- Server CI `go vet` PASS
- Server CI race detector PASS
- Siege Load Lab 24-client vertical smoke PASS
- Siege Load Lab 100-client Gate Zerg PASS
- dedicated Character / Replication / WorldRuntime respawn tests PASS
- Protocol 維持 v6
- Client repo無修改
- 不降低 S3-E lifecycle / Vitals budget
- 不 force 額外 snapshot
- 不把 respawn-specific `Wants` lookup放回一般 Dirty Vitals hot path

S3-E 500-client workflows若因既有 branch filter跳過，仍明確記為 skipped，不當成 PASS。

## 下一步

S3-F.3 才應決定玩家層 respawn policy，例如：

- 城鎮 / 城堡 / checkpoint spawn point registry
- respawn delay
- PvE / PvP / siege 不同規則
- resurrection action / item
- invulnerability grace
- death penalty

這些是 gameplay policy，不應反向污染 S3-F.2 建立的 authoritative transition primitive。

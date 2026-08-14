# S3-F.6 Post-Revive Protection Grace

## 目標

S3-F.6 在 S3-F.5 Resurrection Action Seam 與既有 authoritative respawn primitive 上加入 bounded、Server-owned 的 **Post-Revive Protection Grace**。

本階段只處理 revive 後短時間 damage legality，不加入 death penalty，也不新增 Client-visible buff/status protocol。

## 1. Protection duration

`worldd` 新增：

```text
-post-revive-protection-seconds
```

預設為 `3.0` 秒；`0` 可停用。

啟動時依 authoritative tick rate 轉換：

```text
ceil(seconds × tick_rate)
```

預設 20Hz 因此是 60 ticks。

Runtime fixture 的 `PostReviveProtectionTicks` 零值維持停用，避免既有 unit/integration fixtures 因新 policy 被隱性改語意。

## 2. Grant points

Protection 只在成功 revive transition 後授予：

- authoritative respawn，包括 manual server respawn 與 S3-F.3/S3-F.4 policy due respawn；
- S3-F.5 resurrection action。

失敗的 respawn / resurrection 不授予 protection。

Protection interval 為半開區間：

```text
[reviveTick, reviveTick + durationTicks)
```

因此 exact `untilTick` 已經可以受到傷害。

## 3. Damage legality

Protection 只阻擋對 `EntityPlayer` 的 `combat.EffectDamage`。

一般 target legality 仍先執行：

- entity exists；
- alive/defeated rule；
- same layer；
- range；
- line of sight。

若 target 在 protection window：

```text
ErrEntityReviveProtected
```

作為 gameplay ActionRejection。

此 rejection：

- 不改 HP；
- 不改 Defeated；
- action sequence 仍依既有 intent contract 被消耗；
- attacker action cooldown 不 Commit。

## 4. Aggression cancellation

Protection player 只有在 **成功造成 damage effect** 後才失去 protection。

成功 damage 可以是：

- entity damage；
- gate damage。

以下都不取消 protection：

- cooldown rejection；
- invalid/self target；
- wrong layer / out of range / no LOS；
- target 本身仍受 revive protection；
- resurrection 等 non-damage action。

取消發生在 action 成功並 Commit cooldown 的同一 world-owner command phase，因此後續同 tick command 可以立即對該 player 造成傷害。

## 5. Runtime ownership / scaling

Protection state：

```text
entityID -> protectedUntilTick
```

只由 WorldRuntime owner goroutine mutate。

Expiry 使用 lazy lookup：只有 damage legality查詢時判斷並刪除過期 entry，不做每 tick O(N) 全量 sweep。

`leave_world` 會清除 protection state，避免 EntityID reuse 殘留。

## 6. Replication / protocol

本階段不新增 Client-visible protection state。

理由：

- protection 是 authoritative damage legality，Server不依賴 Client知道 timer才能正確判定；
- HP / Defeated 仍走既有 Reliable `EntityVitalsState`；
- protection本身不改 transform / AOI；
- 不需要修改 Protocol v6、GameV1 codec或 Client。

未來若要加入 UI shield icon / remaining duration，應以獨立 replicated status state評估，而不是回頭改本階段 damage truth。

## 7. Metrics

`StepMetrics` 新增：

```text
ReviveProtectionsGranted
ReviveProtectionDamageBlocks
ReviveProtectionsCancelledByDamageAction
```

方便 correctness / load fixture觀察，但不改既有 lifecycle、Initial Vitals、Dirty Vitals budget。

## 8. Tests

`cmd/worldd/main_test.go`：

- disabled zero；
- 3s @ 20Hz = 60 ticks；
- fractional duration向上取整；
- negative / NaN / Inf拒絕。

`internal/worldruntime/revive_protection_test.go`：

- resurrection成功授予 protection；
- protection window內 damage被拒絕；
- rejected damage仍 consume action sequence；
- exact expiry tick可立即受到傷害；
- blocked attack不 Commit cooldown；
- respawn成功授予 protection；
- protected player成功 damage action後同 tick取消 protection；
- rejected damage action不取消 protection；
- leave清除 state。

## 9. 不變量

S3-F.6 不修改：

- Protocol v6；
- Client repo；
- shared `gameplay.json` / Gameplay World identity；
- S3-F.5 resurrection 30% HP / 10s cooldown；
- S3-F.4 PvE/PvP/Siege respawn context；
- S3-F.3 non-destructive Due truth；
- S3-F.2 respawn AOI / Vitals ordering barrier；
- lifecycle churn budget；
- Initial Vitals budget；
- Dirty Vitals 4,000/tick budget；
- snapshot cadence。

## 下一階段

S3-F.7 建議處理 **Death Penalty Seam**，先建立 Server-owned death outcome / penalty transaction boundary，再決定 inventory、currency、durability 或 progression 哪一種 penalty是第一個 bounded implementation；不要把多個經濟系統一次混入同一 stage。

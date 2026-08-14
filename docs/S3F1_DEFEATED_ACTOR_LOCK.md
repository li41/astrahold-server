# S3-F.1 Defeated Actor Lock

## 目標

S3-E 已完成 Siege Replication Scaling；S3-F 開始補齊 Character lifecycle correctness。

S3-D.3 已有 authoritative Character state：

```text
Character State
├── EntityID
├── HP
├── MaxHP
└── Defeated
```

但 S3-F.1 前，`Defeated` 只用在「target 已倒地時拒絕再次受擊」。Actor 自己仍可能：

- 沿用 lethal hit 前最後一筆 persistent movement input繼續移動。
- 繼續送新的 `ClientMoveInput`。
- 繼續送 `ClientUseAction` 攻擊 Entity 或 Gate。

這使 authoritative HP state與 movement/action capability分裂。

S3-F.1 將 `Defeated` 收斂成 actor capability lock，不新增 Client message、不改 Protocol v6，也不在本階段定義 respawn timer / corpse / resurrection。

## 起點

- Server `main`: `6044643502ddc3c629ffe6641d4fe91a80cb84ba`
- Protocol: v6
- Client 不需要修改

## 1. Lethal transition 必須立即清掉舊 movement input

Movement input是 Server simulation內的 persistent authoritative state，不是一次性 event。

因此只拒絕「下一筆」Client move還不夠：

```text
ClientMoveInput(direction=+X)
→ World stores +X
→ lethal damage
→ 如果不清 input
→ 下一個 simulation tick仍會沿 +X 移動
```

S3-F.1 在 Entity action造成 `HP=0 / Defeated=true` 的同一個 world-owner command phase執行：

```text
ReduceHP
→ Defeated transition
→ SetMoveInput(zero)
→ mark EntityVitals dirty
→ simulation Tick
```

因此 lethal hit與停止 authoritative movement之間沒有多一個 world tick的漂移。

若清 movement input發生不可能的 runtime error，仍記錄 `CommandError`；已套用的 damage不回滾，Combat cooldown仍照 action成功語意 commit。這個 error在 acceptance中必須為0。

## 2. Defeated move sequence semantics

倒地後收到新的 `ClientMoveInput`：

```text
ValidateInputSequence
→ Character state == Defeated
→ authoritative input維持 zero
→ MarkProcessedInput(sequence)
→ 不移動
```

這是正常 gameplay state，不是 Server fault，因此不新增 `CommandError`。

Sequence仍必須被消耗，原因是不能讓倒地期間收到的方向在未來 revive/respawn後被重播：

```text
Defeated 收到 sequence=42
→ consume 42

之後重送 42
→ ErrStaleInput
```

## 3. Defeated action sequence semantics

所有 `ClientUseAction` 在 target dispatch前先檢查 actor Character state。

```text
ValidateActionSequence
→ MarkProcessedAction(sequence)
→ actor exists
→ actor Character state
→ Defeated
→ ActionRejection(character.ErrCharacterDefeated)
```

因此這個 rule同時適用於 Entity / Gate / 未來新增的 action target，不需要在每個 target domain重複判斷。

倒地 action是 gameplay rejection：

- 不污染 `CommandErrors`
- 不套用 damage
- 不 commit cooldown
- action sequence已消耗
- 同 sequence重送為 `ErrStaleAction`

## 4. Protocol / replication不變

S3-F.1 沿用既有 Reliable full-state：

```text
EntityVitalsState
├── EntityID
├── HP
├── MaxHP
└── Defeated
```

沒有新增 death event。Client只需依既有 `Defeated=true` state呈現倒地狀態。

Spawn/Despawn、WorldSnapshot、PositionCorrection、Network LOD、MTU 1200、Protocol v6均不變。

## 5. Tests

`internal/worldruntime/defeated_actor_lock_test.go` 鎖定：

1. target在 lethal hit前已有 persistent movement input。
2. lethal transition同 tick不再產生額外位移。
3. Defeated actor後續 move不移動、沒有 command fault、input sequence前進。
4. 同一 move sequence重送為 `ErrStaleInput`。
5. Defeated actor主動 attack會得到 `character.ErrCharacterDefeated` gameplay rejection。
6. target HP完全不變、`EntityActionsApplied=0`。
7. 同一 action sequence重送為 `ErrStaleAction`。

## 6. Acceptance

S3-F.1 是 bounded correctness slice，不把 branch-filter skipped的500-client workflow宣稱為 PASS。

Merge前要求：

- Server CI：`go test` PASS
- Server CI：`go vet` PASS
- Server CI：race detector PASS
- Siege Load Lab：24-client vertical siege smoke PASS
- Siege Load Lab：100-client Gate Zerg PASS
- dedicated Defeated Actor integration tests PASS
- Client repo無修改

## 下一步 S3-F.2

在 actor capability lock成立後，再建立 server-authoritative revive/respawn seam。下一階段要明確定義：

- 誰可以觸發 revive / respawn
- full HP或指定HP policy
- respawn position / Layer來源
- movement input在 revive前後的清理
- Character vitals revision如何 fan-out
- 是否需要新的 Reliable gameplay state，或既有 `EntityVitalsState` 已足夠
- Client input enable/disable何時只依 authoritative Defeated state切換

在這些 contract定義前，不先加入任意自動倒數 respawn timer。

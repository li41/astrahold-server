# S3-D.2 Action Intent Integration

S3-D.2 把 S3-D 的 Gate-specific reliable attack path 收斂到可擴充的 Combat Action contract；本階段只開放 Gate target，不提前加入 PvP、Skill Effect、Buff 或 Projectile。

## 權威邊界

```text
Client
ClientUseAction(action_id, target_kind, target_id)
        ↓ ReliableOrdered
Gateway
        ↓
WorldRuntime bounded command queue
        ↓
combat.Service.Prepare
        ├─ action catalog
        ├─ target capability
        └─ cooldown
        ↓
target domain validation
        └─ Gate: alive / Layer / Range / LOS
        ↓
套用 HP / blocker transaction
        ↓
combat.Service.Commit
        └─ 成功後才開始 cooldown
```

Client 不得提供 damage、range、cooldown、命中結果或 destroyed 判定。

`config/combat-actions.json` 是新執行路徑中 damage / range / cooldown 的 Server authority。Gameplay World v2 仍保留舊 `gate.attack` 欄位，僅作 S3-D migration 相容；Protocol v5 Gateway / Runtime 不再讀它作為 action 數值。待第一個非 Gate action 接入後再移除 v4 migration API 與 schema 欄位，避免同一 PR 同時擴大 wire、runtime 與 world schema 風險。

## Protocol v5

Message type `2` 改為：

```json
{
  "action_id": "basic-attack",
  "target_kind": "gate",
  "target_id": "main-gate"
}
```

這是 wire-incompatible 變更，因此 Protocol Version 由 4 升為 5；v4 Client 會在 Frame version 邊界被拒絕，不做同 message type 的 payload 猜測。

目前 target kind：

```text
gate
```

新增 target kind 必須同時完成 Combat Catalog capability、target domain validator、Runtime dispatch 與測試，不能只擴 protocol enum。

## Sequence 與 rejection

Reliable action sequence 仍只有 `Envelope.Sequence` 一個來源，與 realtime movement input sequence 分流。

Runtime 一旦接受該 sequence 進入 gameplay validation，就視為「intent 已處理」。因此：

- out of range
- no LOS
- target destroyed
- cooldown
- target not allowed

都不可用相同 sequence 重播。

這與 action 是否成功是兩回事。

## Prepare / Commit transaction

Combat cooldown 採：

```text
Prepare
  ↓
Target validation
  ↓
Apply target mutation
  ↓ success only
Commit cooldown
```

如果 Gate 因距離、Layer、LOS 等原因拒絕，**不得消耗 cooldown**。這條規則未來必須延續到 PvP / NPC / Objective target。

## Gate domain

S3-D.2 新路徑使用 `siege.Service.ApplyActionDamage`。Gate domain 只擁有：

- Gate exists / destroyed
- Gate HP
- blocker mapping
- Layer / Range / LOS target validation
- HP 歸零與 blocker disable 的原子 transaction

`combat.Damage` 保留 `DamageSource{ActorEntityID, ActionID}`，未來 combat log、kill credit、threat、proc 與 audit 不需要從扣血結果反推來源。

## Migration shim

為了保留 S3-D.1 前既有 internal tests，Server 暫留：

- `Runtime.EnqueueAttackGate`
- `siege.Service.Attack`
- Gameplay World v2 `gate.attack`

v5 Gateway 不會呼叫它們。`worldd` 與 `loadserver` 都顯式載入 `config/combat-actions.json` 並在 `WithSiegeGates` 後注入 `WithCombatService`，所以 production/dev composition root 不會使用 legacy fallback。

Client 的 `SendAttackGateAsync()` 暫時作為 Gate debug presentation convenience；encoder 已轉成 v5 generic action wire。第一個非 Gate action 接入時應把 public send API 也完全 generic 化並移除 migration DTO。

## 驗證重點

- Protocol v5 JSON action round-trip
- Gateway Reliable-only whitelist
- TCP adapter generic action routing
- Combat Prepare / Commit cooldown contract
- Gate HP / blocker transaction
- Go 1.26 test / vet / race
- Siege Load Lab regression
- Client .NET build
- Godot headless runtime probe

## 下一步

S3-D.3 應優先接第一個真正的非 Gate target 或建立 Action Result / Combat Event replication，再移除 v4 migration shim。不要在還只有單一 target 時先做大型 effect graph 或技能腳本 VM。

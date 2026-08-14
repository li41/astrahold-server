# S3-F.3 Respawn Policy Core

## 目標

S3-F.2 已建立安全的 server-authoritative respawn transition primitive：Server 決定目的地，只有 Defeated Character 能轉換，成功後 full HP、Authoritative Teleport、movement input 清零，並透過 AOI/Vitals barrier 保證 lifecycle ordering。

S3-F.3 往上一層建立第一個 **player-facing respawn policy core**，但仍保持 bounded：

- Server-only spawn point registry
- default spawn point
- per-character checkpoint
- deterministic respawn delay
- defeat-time destination binding
- due-tick automatic respawn
- manual respawn / leave cleanup

本階段不加入 resurrection spell/item、invulnerability grace、death penalty，也不把 PvE/PvP/Siege 所有規則一次塞進同一個 PR。

## 1. Server-only policy config

新增：

```text
config/respawn-policy.json
```

第一版 schema：

```json
{
  "schema_version": 1,
  "revision": "s3f3-001",
  "respawn_delay_seconds": 5.0,
  "default_spawn_point": "field-camp",
  "spawn_points": [
    { "id": "field-camp", "x": 0, "y": 0, "z": -35, "layer": 0 },
    { "id": "courtyard-checkpoint", "x": 0, "y": 0, "z": 25, "layer": 0 }
  ]
}
```

這份檔案刻意不加入 shared `gameplay.json`。

理由是 respawn delay / checkpoint policy 是 Server gameplay policy，不是 Client navigation/world geometry contract。若只是新增 Server 私有出生規則就改 shared Gameplay World JSON，會不必要地改變 World Identity SHA 並要求 Client 同步資產。

因此：

```text
Gameplay World schema   維持 v2
Protocol                 維持 v6
World Identity SHA       仍只由 gameplay.json 決定
Client repo              不修改
Respawn policy revision  Server-only
```

## 2. Strict validation

`internal/respawnpolicy` 使用 strict JSON decode：

- unknown field 拒絕
- trailing JSON 拒絕
- schema version 必須正確
- revision 必填
- delay 必須 > 0 且 finite
- spawn point ID 必填且唯一
- default spawn point 必須存在
- X/Y/Z 必須 finite

worldd 啟動時再執行 `ValidateAgainstWorld`：

- spawn point 的 X/Z/Layer 必須落在 shared Gameplay World surface
- Y 必須與該 surface plane 高度一致
- 不可落在初始 enabled movement blocker 內

這讓 Server policy config 可以獨立版本化，又不允許設定出明顯不合法的出生座標。

## 3. Checkpoint ownership

Runtime 提供 server-side seam：

```go
EnqueueSetRespawnCheckpoint(entityID, spawnPointID)
```

這仍經 bounded WorldRuntime command queue。

Client Protocol **沒有**新增「自行指定 checkpoint」message。未來 checkpoint acquisition 可以由 server-trusted gameplay interaction、quest、region trigger、admin 或 persistence subsystem 呼叫此 seam。

空 `spawnPointID` 表示清除 checkpoint，回到 default spawn policy。

## 4. Defeat-time binding

最重要的 policy semantics 是：**目的地在 defeat transition 當下綁定**。

```text
alive
  ↓ lethal combat
Defeated=true
  ↓
read current checkpoint (or default)
  ↓
bind SpawnPointID + Position + DueTick
```

角色倒地後即使 checkpoint 被清除或改成另一個位置，本次已排定的 death outcome 也不會被改寫。

這避免：

- 死後切 checkpoint 逃離戰場
- persistence / gameplay update 競態改寫已成立死亡結果
- GM / quest 更新無意間搬動已等待 respawn 的角色

新的 checkpoint 只影響下一次死亡。

## 5. Deterministic delay

Policy 使用 Server tick，而不是 wall clock：

```text
delay_ticks = ceil(respawn_delay_seconds × tick_rate_hz)
due_tick    = defeated_tick + delay_ticks
```

castle-sandbox 預設：

```text
20 Hz × 5 seconds = 100 ticks
```

若 tick rate 改變，worldd 啟動時依實際 tick rate重建 delay ticks。

到期排程固定按：

```text
DueTick ASC
EntityID ASC
```

排序，保持 single-owner deterministic ordering。

## 6. Due selection 不等於 progress confirmation

`respawnpolicy.Service.Due(tick)` 只選出已到期項目，**不刪除 pending truth**。

```text
Due selection
    ↓
WorldRuntime applyRespawn
    ↓
Teleport + ReviveFull 成功
    ↓
Cancel pending  ← progress confirmed here
```

如果 authoritative transition 發生 fault，pending 仍保留，下一個 world tick 會重試。

只有以下情況清除：

- S3-F.2 respawn transition成功
- server-side manual respawn成功
- entity已由其他合法路徑變成 alive，stale schedule被清除
- entity leave/world removal

這和 Astrahold 既有 Reliable lifecycle 原則一致：**選中工作不等於語意完成，成功才前進 truth**。

## 7. World-owner ordering

每個 `Runtime.Step` 的順序是：

```text
1. drain bounded commands
   ├── move/action
   ├── checkpoint update
   └── manual respawn
2. apply due respawns
3. simulation Tick
4. dynamic / snapshot / lifecycle replication
5. Vitals replication
```

特別重要的是 due tick 同時收到 `ClientMoveInput` 時：

```text
move command先處理
→ Character仍是 Defeated
→ sequence被consume
→ authoritative input維持zero

then due respawn
→ full HP + teleport

then simulation
→ 不會把剛才的 defeated-period move套到新出生點
```

角色必須在 revive 後送新的 input sequence 才能再次移動。

Action sequence / Combat cooldown 同樣不因 respawn reset，沿用 S3-F.1 / S3-F.2 contract。

## 8. Reuse S3-F.2 AOI / Vitals barrier

S3-F.3 **沒有建立第二套 respawn transition**。

到期排程最後仍呼叫 S3-F.2：

```go
applyRespawn("respawn_policy", RespawnRequest{...})
```

因此自動 respawn直接繼承：

- full HP / Defeated=false
- authoritative Teleport
- persistent movement direction cleanup
- respawn Vitals revision
- next-normal-snapshot AOI barrier
- stale-known observer desired-only protection
- lifecycle TrySend success truth

沒有額外 force snapshot，也沒有改 Dirty Vitals hot path。

## 9. Lifecycle cleanup

### Manual respawn

如果 GM / server gameplay path在 delay 到期前合法呼叫 S3-F.2 `EnqueueRespawn`，成功 transition會取消原 pending auto-respawn，避免之後第二次觸發。

### Leave

Character真正 `leave_world` 時同步清除：

- pending respawn
- checkpoint assignment

避免 EntityID 未來被重用時繼承上一個角色 lifecycle state。

純 Session `unregister` 不清 Character policy，因為 entity仍存在於 world。

## 10. Tests

### `internal/respawnpolicy/policy_test.go`

鎖定：

- strict schema load
- default point存在性
- spawn point surface / blocker validation
- delay seconds → ticks
- defeat-time checkpoint binding
- deterministic Due ordering
- Due selection不提前刪 pending
- explicit Cancel / Remove cleanup

### `internal/worldruntime/respawn_policy_test.go`

鎖定：

1. lethal hit建立 pending schedule。
2. checkpoint在死亡當下綁定。
3. 倒地後清 checkpoint不改本次 schedule。
4. due tick前保持 Defeated。
5. due tick自動走 authoritative respawn。
6. full HP與目的地正確。
7. due-tick move sequence先consume，不會在出生點自動移動。
8. respawn後同 sequence仍是 stale。
9. manual respawn取消 pending auto-respawn。
10. leave清除 checkpoint / pending policy state。

原有 S3-F.2 tests仍負責 AOI/Vitals ordering regression。

## 11. Acceptance

Merge 前要求：

- `go test ./...` PASS
- `go vet ./...` PASS
- concurrency boundary race detector PASS
- Siege Load Lab 24-client smoke PASS
- Siege Load Lab 100-client Gate Zerg PASS
- dedicated respawn policy tests PASS
- Protocol維持 v6
- Client repo無修改
- Gameplay World schema / SHA contract不因 Server-only policy改變
- lifecycle / Initial Vitals / Dirty Vitals budgets不降低
- Snapshot cadence不改
- S3-F.2 AOI/Vitals barrier不繞過

S3-E 500-client / soak workflows若因既有 branch filter跳過，必須明確記為 skipped，不宣稱 PASS。

## 下一步

建議下一個 bounded slice 為 **S3-F.4 Respawn Context Rules**：

- PvE / PvP / Siege death context
- 依 context選擇 allowed spawn-point class / delay
- server-owned checkpoint acquisition validity
- siege期間禁止或覆寫某些 checkpoint

之後再獨立處理 resurrection action/item、invulnerability grace與 death penalty。這些機制的 transaction / exploit surface不同，不應和 S3-F.3 policy core綁在同一階段。

# S3-F.4 Respawn Context Rules

## 目標

S3-F.3 已建立 Server-owned respawn policy core：死亡時綁定目的地與 due tick，並重用 S3-F.2 authoritative respawn transition。

S3-F.4 把單一 respawn 規則拆成 **PvE / PvP / Siege death context**，並把 checkpoint 從「server caller 可直接指定」收斂成需要 authoritative world state 驗證的 gameplay acquisition。

本階段仍不加入 Client respawn request、resurrection action、invulnerability grace 或 death penalty。

## 1. Server-only schema v2

`config/respawn-policy.json` 升級為 schema v2，但它仍是 **Server-only policy file**：

- 不修改 `gameplay.json`。
- 不修改 Gameplay World SHA / handshake identity。
- Protocol 維持 v6。
- Client repo 不需要同步此檔案。

每個 spawn point 帶有 class：

```text
safe
checkpoint
siege
```

每個 death context 各自定義：

```text
respawn_delay_seconds
default_spawn_point
allowed_spawn_classes
```

castle-sandbox 第一版矩陣：

| Context | Delay | Default | Allowed classes |
| --- | ---: | --- | --- |
| PvE | 5s | field-camp | safe, checkpoint |
| PvP | 8s | field-camp | safe |
| Siege | 10s | siege-rally | safe, siege |

因此同一個已取得的 courtyard checkpoint：

- PvE death 可以使用。
- PvP death 不可使用，會 fallback 到 PvP default safe point。
- Siege death 不可使用，會 fallback 到 Siege default point。

## 2. Death context ownership

Client 不送 death context。

目前 Generic Entity Action 的 lethal transition 使用 authoritative entity kind 推導：

```text
attacker = EntitySiegeObject
→ Siege

attacker = EntityPlayer && target = EntityPlayer
→ PvP

otherwise
→ PvE
```

Respawn policy 只套用在 `target.Kind == EntityPlayer` 的 defeat lifecycle；NPC / Monster 的死亡不因為載入 player respawn policy 而自動排程。

這是第一個 bounded classifier。未來若 Siege participation / phase 能讓「Player 對 Player」在攻城期間也屬 Siege，應由 server-owned siege gameplay state 擴充 classifier，而不是加入 Client-provided context。

## 3. Context-specific scheduling

`respawnpolicy.Service.Schedule` 改為：

```go
Schedule(entityID, defeatedTick, deathContext)
```

在 defeat tick 立即綁定：

```text
DeathContext
SpawnPointID
SpawnClass
Position / Layer
DueTick
```

選點規則：

1. Character 有 checkpoint，且該 checkpoint class 被 death context 允許 → 使用 checkpoint。
2. 否則 → 使用 context 的 default spawn point。

Context delay 以：

```text
ceil(respawn_delay_seconds × tick_rate)
```

轉成 deterministic ticks。

與 S3-F.3 一樣，排定後修改 / 清除 checkpoint 不會改寫本次死亡結果。

## 4. Checkpoint acquisition validity

S3-F.3 的 `EnqueueSetRespawnCheckpoint` 是 server-side seam，但缺少 world-state validity；任何 gameplay subsystem 只要知道 point id 就能遠端指定。

S3-F.4 新增正式 gameplay seam：

```go
EnqueueAcquireRespawnCheckpoint(entityID, spawnPointID)
```

World owner 在 command phase 使用 authoritative state 驗證：

- Character 必須存在。
- Character 必須尚未 `Defeated`。
- Entity 必須存在於 simulation world。
- spawn point 必須是 `checkpoint` class。
- checkpoint 必須有正的 `checkpoint_activation_radius`。
- Entity 與 checkpoint 必須同 Layer。
- authoritative XYZ distance 必須在 activation radius 內。

Client 不提供 activation position，也不能宣稱自己已碰到 checkpoint。

為避免 S3-F.3 server caller 靜默繞過新規則，既有：

```go
EnqueueSetRespawnCheckpoint(entityID, nonEmptyPointID)
```

現在等價於 Acquire；空字串仍等價於 Clear。

## 5. Pending truth / ordering 不變

S3-F.4 不改 S3-F.3 的 progress-confirm 規則：

- `Due(tick)` 只 selection，不刪 pending。
- authoritative respawn 成功後才 `Cancel` pending。
- transition fault 時下一 tick重試。
- manual respawn 成功會取消 pending。
- entity leave 會移除 checkpoint + pending。

同一 due tick 的 queued move input 仍先以 Defeated 規則 consume / zero，之後才 respawn。

S3-F.2 的 AOI / Vitals ordering barrier完全沿用。

## 6. Replication / Protocol

沒有新增 wire message。

仍沿用：

```text
EntityVitalsState
EntitySpawn / EntityDespawn
WorldSnapshot / PositionCorrection
```

因此：

- Protocol = v6
- Client repo無修改
- snapshot cadence不變
- lifecycle / Initial Vitals / Dirty Vitals budgets不變

## 7. Tests

### Respawn policy package

`internal/respawnpolicy/policy_test.go`

鎖定：

- schema v2 strict decode。
- PvE / PvP / Siege 三個 context必須完整且不重複。
- context default point class必須在 allowed classes內。
- spawn point仍必須通過 Gameplay World surface / blocker validation。
- checkpoint取得需要 class / Layer / radius validity。
- PvE可使用 checkpoint。
- PvP checkpoint class不允許時 fallback safe default。
- Siege使用 siege default。
- context-specific delay ticks。
- death-time binding。
- Due selection仍非破壞式。

### WorldRuntime

`internal/worldruntime/respawn_policy_test.go`

鎖定：

- Monster → Player lethal action分類 PvE。
- Player → Player分類 PvP。
- SiegeObject → Player分類 Siege。
- PvE checkpoint respawn與 due-tick move ordering。
- PvP / Siege class fallback。
- checkpoint acquisition使用 authoritative position。
- non-checkpoint / out-of-range acquisition被拒絕。
- Defeated actor不能取得新 checkpoint。
- manual respawn與 leave cleanup regression。

## 8. Acceptance

S3-F.4 merge 前要求：

- Server CI `go test` PASS。
- Server CI `go vet` PASS。
- Server CI race detector PASS。
- Siege Load Lab 24-client vertical smoke PASS。
- Siege Load Lab 100-client Gate Zerg PASS。
- Protocol維持 v6。
- Client repo無修改。
- `gameplay.json` / Gameplay World identity不修改。
- S3-E lifecycle / Vitals budget與 snapshot cadence不降低或放寬。

S3-E 500 / E8 / E9 workflows若因既有 branch filter skipped，仍記為 skipped，不當作 PASS。

## 下一步

S3-F.5 建議進入 **Resurrection Action Seam**：在 auto-respawn due 前允許 server-authoritative resurrection取消 pending schedule，明確定義原地 revive、HP policy、action target legality與 sequence / cooldown semantics；invulnerability grace與 death penalty繼續拆成後續 bounded slice。

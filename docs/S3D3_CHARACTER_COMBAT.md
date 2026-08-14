# S3-D.3：Character Combat Target

S3-D.3 的目的，是讓 S3-D.2 建立的 generic `ClientUseAction` 第一次真正作用在非 Gate 目標，證明 Combat Action contract 可以被多個 target domain 共用，而不是把 Gate 特例重新包一層名稱。

## Protocol v6

S3-D.3 將 Protocol Version 升為 6。

`ClientUseAction` 結構不再變動；只新增可接受的 target kind：

```text
ClientUseAction
├── action_id
├── target_kind = gate | entity
└── target_id
```

Character full state 以新的 Reliable message 同步：

```text
EntityVitalsState
├── entity_id
├── hp
├── max_hp
└── defeated
```

Client 仍然不能提供 damage、range、cooldown、命中結果、HP 或 defeated 判定。

## Domain ownership

S3-D.3 明確維持三個不同 ownership：

```text
World / Navigation
→ Position / movement / Layer / LOS

Combat
→ Action catalog / Damage source / Range / Cooldown

Character
→ HP / MaxHP / Defeated
```

Character HP 不塞進 `world.EntityState`，也不混入 Navigation 或高頻 transform snapshot。

`internal/character` 是目前的 Character vitals owner。Runtime 在玩家 Join / Register 時建立 Character state，在 Leave 時移除；完整 death / respawn lifecycle 尚未在本階段處理。

## Action transaction

Entity target 延續 S3-D.2 的 transaction：

```text
ClientUseAction
      ↓
Session action sequence
      ↓
Combat.Prepare
├── action exists
├── target kind allowed
└── cooldown ready
      ↓
Entity target validation
├── target_id 可解析
├── target exists
├── target != self
├── Character exists / not Defeated
├── same Layer
├── XYZ range
└── Server LOS
      ↓
Character.ReduceHP
      ↓
mark EntityVitals dirty
      ↓
Combat.Commit
```

只有 target domain 成功套用 action 後才 `Commit` cooldown。Self target、距離、Layer、LOS、Defeated 等 gameplay rejection 都不消耗 cooldown。

Action sequence 則代表「這個 intent 已被 Server 處理」，即使 gameplay rejection 也不可使用相同 sequence 重播。

## Entity Vitals replication

`EntityVitalsState` 是 **Reliable full state**，不是一次性 combat event。

Server 只對已經透過 AOI `EntitySpawn` 知道該 Entity 的 Session 傳送 vitals：

```text
Entity enters AOI
→ EntitySpawn
→ current EntityVitalsState

Character HP changes
→ vitals revision++
→ observers receive latest full state
```

如果 Entity 離開 AOI，Server 會清掉該 Session 對此 Entity 的已送 revision；未來重新進入 AOI / Spawn 時會重新送完整 vitals，不依賴舊 combat event replay。

### Backpressure retry

100-client Gate Zerg regression 在第一版 S3-D.3 抓到一個重要問題：大量初始 Spawn 同步加入 `EntityVitalsState` 後，Reliable outbound queue 曾出現 36 次 backpressure。

這不是 CPU、Tick 或 MTU 瓶頸；當時 Tick p99 約 5.6 ms，`datagram_too_large = 0`。真正問題是第一版把 Vitals 當成一次性 `TrySend`。

修正後每個 Entity 有 authoritative vitals revision，每個 Session 保存成功送達的 revision：

```text
TrySend success
→ session revision = entity revision

TrySend ErrBackpressure
→ 不更新 session revision
→ 不視為 state loss
→ 下一 tick retry latest full state
```

沒有放大 Reliable queue，也沒有關掉 Load Lab regression gate。

這個 retry 語意與 `WorldDynamicState` 一樣重要：可靠狀態必須能在暫時 outbound pressure 後恢復，而不是只依賴 transport queue 當下有空位。

## Codec

S3-D.3 沒有建立新的 realtime codec。

```text
Reliable control/state
→ strict JSON v1
→ EntityVitalsState 也走此路徑

Realtime movement/snapshot/correction
→ GameV1 compact binary
```

Vitals 是低頻 Reliable state；目前不值得為 4 個固定欄位建立另一套 codec negotiation。Production Client 與 Siege Load Lab 因此仍走相同 Reliable payload contract。

## 驗證

Server integration tests 驗證：

- entity target basic attack 造成 Server-owned HP 下降
- cooldown rejection
- Defeated 終態
- 對已 Defeated target 再攻擊被拒絕
- Self target 被拒絕且不消耗 cooldown
- `EntityVitalsState` strict JSON round-trip
- unknown JSON field 被拒絕
- Reliable backpressure 不遺失 Vitals，下一輪可成功 retry

Siege Load Lab 最終 regression：

```text
24-client Vertical Siege   PASS
100-client Gate Zerg       PASS

datagram_too_large         0
unexpected tick errors     0
delivery regression gate   PASS
```

因此 S3-D.3 沒有提供拆 Cell Actor 或放大 queue 的新證據。

## 本階段刻意不做

以下不是 S3-D.3 scope：

- death animation / corpse lifecycle
- respawn / resurrection
- faction / PvP legality
- armor / defense / critical / hit rate
- skill effects / buffs / debuffs
- aggro / monster AI
- combat result event log
- Gameplay World `gate.attack` legacy schema cleanup
- S3-E Network LOD / dirty / delta replication

Gameplay World schema v2 目前仍保留 `gate.attack` 作 migration debt；production generic combat 的 damage / range / cooldown 已以 Combat Action Catalog 為正式來源。schema cleanup 應獨立處理，不與 Character Combat correctness 綁在同一 PR。

## 下一步

S3-D.3 完成後，generic Action / Damage source 已同時實際作用於 Gate 與 Character target。下一個主要 milestone 可進入 **S3-E Siege Replication Scaling**，以既有 24 / 100 / 500+ Load Lab 量測 Network LOD、dirty tracking 與 AOI fan-out，而不是繼續擴充更多 target-specific 特例。

# Skill Presentation / Gameplay Authority Boundary

這份文件是 Astrahold **目前技能系統的設計邊界與後續擴充準則**。它不是新的 milestone，也不是 Protocol v10 宣告。

目前 production baseline 仍是 **Protocol v9**。這份文件的目的，是在 Client 開始參考 `achrefelouafi` 的 Three.js skill / VFX sandboxes 擴充技能 presentation 時，先明確區分：

- 哪些只是 Client targeting / animation / VFX vocabulary；
- 哪些已經能由目前 Protocol v9 + Server combat runtime 表達；
- 哪些一旦具有 gameplay 意義，就必須由 Server 新增 authoritative state / lifecycle，而不能讓 Client 自己決定。

相關 Client 設計索引：

- `astrahold-client/docs/README.md`
- `astrahold-client/docs/ACHREFELOUAFI_REFERENCE_AUDIT.md`

外部設計參考：

- https://github.com/achrefelouafi/SamuraiThirdPersonTemplateThreeJS
- https://github.com/achrefelouafi/LinearAbiltyCastingThreeJS
- https://github.com/achrefelouafi/LinearAbilityExtThreeJS
- https://github.com/achrefelouafi/AvatarCastingAbilitiesThreeJS

> 核心規則不變：**Client sends intent; Server resolves gameplay truth.**
>
> Shader、particle、mesh、cast animation、telegraph style、camera shake、dissolve timing 都不是 gameplay authority。

## 1. Current Protocol v9 baseline

目前 `ClientUseAction` 已有三種 target kind：

```text
gate
entity
point
```

目前 action catalog 已包含：

```text
basic-attack
  target: entity
  range: 2.5m
  cooldown: 700ms
  damage: 22
  windup: 120ms

fireball
  target: point
  range: 12m
  cooldown: 1200ms
  damage: 18
  windup: 180ms
  hit radius: 0.9m

resurrect
  target: entity
  range: 3m
  cooldown: 5000ms
  windup: 450ms
  revive HP fraction: 0.35
```

這代表 Astrahold **已經不是只有 entity-target melee**。目前 `fireball` 已建立第一個 Server-authoritative point/linear skillshot seam。

## 2. Current Fireball semantics

Client 對 Fireball 只提交一個 ground-plane endpoint：

```text
Client actor
  + selected target point (X, Z)
        ↓
ClientUseAction(action_id=fireball, target_kind=point)
```

Server 使用自己的 actor position 與 action definition：

```text
Server actor position
        ↓
validated endpoint
        ↓
range check
        ↓
Server dynamic-world LOS
        ↓
line segment from actor → endpoint
        ↓
AOI combatant candidates
        ↓
first candidate intersecting Server-owned hit radius
        ↓
hit or miss
```

Client **不能**提交：

- range truth；
- hit radius；
- cooldown truth；
- damage；
- hit entity；
- LOS result；
- impact legality。

合法的 skillshot 即使 miss，也會消耗由 Server 管理的 cooldown，並由 Server 發出 resolved `CombatEvent(miss)`；Client 不需要自行猜測「看起來沒打到」是否算 miss。

因此目前 Fireball 的正確描述是：

> **point endpoint intent + Server-authoritative line intersection / first-hit resolution**

它不是 Client-side projectile collision，也不是 Client-authoritative ground AoE。

## 3. Current CombatEvent semantics

Protocol v9 的 `CombatEvent` 是 **已 resolve 的 presentation outcome**，不是 gameplay HP truth。

目前欄位包含：

```text
ActionInstanceID
ActorEntityID
ActionID
Result
TargetEntityID
ImpactX / ImpactZ
Damage
```

`EntityVitalsState` 仍是 HP / defeated 的 authoritative replicated state。

`ActionInstanceID` 的價值是讓 Client 可以把同一次 action 的 animation / projectile visual / impact VFX / audio 對起來，而不是用 local guesses 或單純 ActionID 合併事件。

目前 bounded foundation 的 `CombatEvent` **只 fan-out 給 action 直接參與者**：actor 與 resolved target。AOI observers 尚不接收完整 combat presentation event。這是未來要讓旁觀玩家看到精確 remote casting / impact presentation 時的重要擴充點，但不影響目前 gameplay truth。

## 4. 對應 LinearAbilityExt targeting vocabulary

`LinearAbilityExtThreeJS` 的價值是提供一套 Client presentation vocabulary，而不是可直接照搬的 gameplay contract。

### 4.1 Entity / Unit target

目前已支援。

```text
Client selects entity ID
→ Server validates target existence / range / state / action rules
→ Server resolves effect
```

適合：

- basic attack；
- heal；
- resurrect；
- targeted debuff；
- selected-target spell。

### 4.2 Point / Ground target

目前 Protocol v9 已有 `point`，但 **shape 必須由 Server action definition 決定**。

例如未來加入 ground circle：

```text
Client submits center X/Z only
Server action definition owns radius = 4m
Server resolves all entities in legal circle
```

Client 不應提交「我這次半徑是 8m」。

同理，range、minimum range、LOS、terrain legality、layer、team filter、max targets 都應由 Server definition / world state 決定。

### 4.3 Line skill

目前 Fireball 已經證明簡單 line skill **不一定需要新增 target kind**。

只要 gameplay geometry 可以完全由：

```text
Server actor origin
+ validated point endpoint
+ Server action definition width/radius
```

導出，就可沿用 `target_kind=point`。

未來的冰矛、劍氣、穿透波、直線雷擊，可以先評估是否沿用同一 contract，而不是為每個視覺形狀新增 wire schema。

### 4.4 Cone

若 cone 可完全由 actor origin → validated point direction，加上 Server-owned range / half-angle 導出，也 **不必因為 Client indicator 是 cone 就立刻新增 protocol target kind**。

Server 只需要自己的 action definition：

```text
range
cone_half_angle
max_targets / target policy
LOS policy
```

Client cone mesh 只是 telegraph / aiming preview。

### 4.5 Self-buff

Self-buff 不需要把 VFX aura 變成 network payload。

若 action semantic 明確定義為 actor-self，可由 Server action definition 決定 target semantics；不應讓 Client提交 arbitrary buff duration / multiplier / stack count。

是否需要新增 explicit `self` target kind，應以 protocol clarity / validation需求決定，而不是因為參考 demo 有一個 SelfBuff class 就新增。

### 4.6 Path / spline cast

`AvatarCastingAbilitiesThreeJS` 的自由路徑非常適合 Client VFX 或 designer authoring，但如果 spline **真的影響 gameplay**，目前 `point` contract 不足以安全描述。

分兩種：

#### Presentation-only path

例如：

- projectile VFX 故意繞弧線，但 Server gameplay 仍是已 resolve 的直線 hit；
- ground crack visual 沿曲線蔓延，但傷害只在 Server-known impact point；
- decorative boss telegraph。

這種路徑 **不應進 protocol**。所有 Client 可以依 ActionInstanceID / action preset 本地生成。

#### Gameplay path

例如：

- 火牆真的沿玩家畫出的曲線每 0.5m 造成傷害；
- 藤蔓路徑真的阻擋移動；
- dash 必須沿 spline 做碰撞；
- curve trap 的每段都具有 authoritative hit zone。

這時需要新的 **bounded, validated path intent**，至少必須規範：

- maximum point count；
- maximum encoded bytes；
- world-space quantization；
- maximum total path length；
- minimum/maximum sample spacing；
- actor-origin / endpoint constraints；
- Server-side terrain / layer / LOS / nav legality；
- canonical Server resampling；
- anti-self-intersection / abuse rules（若 gameplay 需要）；
- deterministic resulting gameplay geometry。

絕不能直接把滑鼠每 frame 的 raw points 當 authoritative path 上傳。

### 4.7 Gate cast

要特別區分兩個完全不同語意：

1. 現有 `target_kind=gate`：**選擇 world 已存在的 authoritative Gate ID**；
2. `LinearAbilityExtThreeJS` 的 gate cast：**在世界某個位置建立新的 portal / wall / structure**。

這兩者不可因名字相同而重用語意。

如果 player-created gate 只有 VFX，point + local presentation 就可能足夠。

如果它會：

- teleport；
- block movement；
- block projectile / LOS；
- have HP；
- be destructible；
- change navigation；
- persist across seconds / reconnect；

那它就是 Server-owned world object，需要 stable identity + lifecycle，不是一次 `CombatEvent` 就結束。

### 4.8 Ring / persistent area

Ring visual 本身可以完全在 Client 畫。

但如果 ring 代表：

- damage-over-time；
- heal-over-time；
- slow / silence；
- capture zone；
- blocker；
- protection area；

Server 必須持有：

```text
stable action/effect instance identity
canonical center / orientation / shape parameters
start tick
end tick or explicit retirement
team/target policy
effect cadence / state
```

Client 不能用 particle lifetime 當 gameplay zone lifetime。

### 4.9 Scribe / AirAnchor

如果只是空中的 portal / sigil presentation，Client 可依 canonical point / orientation 本地建立。

如果它具備 gameplay interaction（teleport endpoint、summon anchor、projectile reflector、airborne trigger volume），則 Server 必須有 canonical 3D anchor / layer / orientation / lifecycle。

目前 `point` 只有 X/Z ground-plane semantics，不應偷偷把高度或 orientation 塞進既有欄位。

## 5. Projectile：visual flight 與 gameplay flight 必須分開

目前 Fireball gameplay 是 Server 在 action resolve 時對 actor→endpoint 線段做 authoritative hit selection；Client 可以畫一顆飛行中的火球，但 **那顆 visual projectile 不是 gameplay collision body**。

這種模式很適合：

```text
Server resolves hit/miss
→ Client reconstructs travel VFX
→ CombatEvent drives impact presentation
```

不需要每 frame replication projectile position。

只有當遊戲設計真的要求以下行為時，才需要 Server projectile runtime：

- projectile 有飛行時間且期間目標可以躲開；
- projectile 會撞牆；
- projectile 可以被攔截 / 反射；
- projectile 可以穿透固定數量目標；
- projectile 本身可被其他技能影響；
- projectile lifetime / homing / ricochet 會改變 gameplay 結果。

此時 Server 應持有 projectile identity / position-or-deterministic trajectory / collision / lifetime，而不是信任 Client 回報「我撞到了誰」。

## 6. 未來可能需要的 additive presentation contracts

以下是 **design candidates，不是目前 Protocol v9 已實作欄位**。

### ActionStarted

當需要讓遠端 observer 精確看到 windup / cast animation，而不能等到 hit/miss 才播：

```text
ActionStarted
- ActionInstanceID
- ActorEntityID
- ActionID
- authoritative StartTick
- canonical target / aim data needed for presentation
```

用途：

```text
StartTick
→ remote cast animation
→ telegraph / projectile launch presentation
→ later CombatEvent or persistent-state update
```

不要傳 shader parameters；Client 用 `ActionID → local presentation preset` 找自己的美術資料。

### ActionPersistentState

只有當 action 產生長時間存在且 gameplay-relevant 的 zone / structure / portal 時才需要。

候選 semantic：

```text
ActionInstanceID / EffectInstanceID
kind
canonical anchor / orientation
state
start tick
end tick or explicit retirement
revision
```

實際 schema 應等第一個真實 gameplay slice 出現後再定，不要現在為假想十幾種技能過度設計。

### ActionEnded / Cancelled

若 channel、cast、persistent effect 可以被 Server 提前中斷，應由 Server 發明確 retirement/cancel truth；Client 不應只靠 animation 播完推論 action 已結束。

## 7. Observer fan-out

目前 `CombatEvent` bounded foundation 只送 actor + target。

若下一階段要支援：

- remote player cast animation；
- 大規模戰場技能可讀性；
- spectator；
- party member skill coordination；

應先建立 AOI-aware event fan-out policy，而不是把每個 VFX 物件加進 snapshot。

設計時要一起評估：

- encounter density；
- reliable event bandwidth；
- observer AOI count；
- event coalescing / drop policy；
- late join / snapshot recovery semantics；
- 哪些 presentation event 可丟、哪些 gameplay state 必須可靠收斂。

**原則：presentation event 可以在 backpressure 下失去細節，但 gameplay authoritative state 必須仍可由 replicated state 收斂。**

## 8. Client-side presentation contract

Client 可以自由演進：

```text
TargetingShape
CastAnimation
Telegraph
Travel / Channel
Impact
Persistent visual
Dissolve
Particles
Decals
Audio
Camera feedback
```

只要它們不變成 gameplay truth。

推薦 binding：

```text
ActionID
+ ActionInstanceID
+ authoritative ServerTick / canonical target
        ↓
Client PresentationPreset
        ↓
Godot AnimationTree / Mesh / Shader / GPU particles / audio
```

Server 不需要知道：

- shader 名稱；
- particle count；
- texture；
- animation resource path；
- bloom；
- camera shake；
- dissolve curve；
- local graphics quality。

## 9. 何時才應改 Protocol

只有當「新的 gameplay truth 無法由現有 contract deterministic 導出」時，才考慮新增 wire contract。

### 不需要改 Protocol 的例子

- 把 Fireball 換成更漂亮的火焰 / trail；
- line indicator 換材質；
- Client 增加 ring / portal 的 preview-only VFX；
- cast animation 換 clip；
- Client 依 action preset 改 projectile visual speed，但 gameplay 仍是 immediate resolved skillshot；
- point target + Server-owned radius 足以描述的新 ground effect。

### 可能需要改 Protocol / Server runtime 的例子

- gameplay-relevant freehand path；
- persistent damage/heal zone；
- player-created blocker / portal；
- gameplay projectile flight / collision；
- remote observer 必須收到 cast-start timing；
- 3D airborne anchor / orientation 成為 gameplay truth；
- channel / interrupt / cancel lifecycle。

## 10. 每個新技能的 acceptance questions

每加入一種新 ability，先回答：

1. Client 到底只需要提交什麼 **intent**？
2. Server 能否從 actor state + action definition + world state deterministic 導出 shape？
3. range / cooldown / resource / LOS / target legality 誰決定？答案必須是 Server。
4. hit / miss / damage / status / revive / death 誰決定？答案必須是 Server。
5. 這個效果是否跨 tick 持續存在？如果是，誰持有 lifecycle？
6. 它是否改 collision / nav / teleport / capture / blocker？如果是，必須有 Server world state。
7. Client 斷線 / late join 時，哪些 gameplay state 必須能重建？
8. observer 是否需要事件？需要的話 AOI fan-out / bandwidth 怎麼處理？
9. presentation preset 能否完全留在 Client？通常答案應該是可以。
10. 是否真的需要 protocol change？若現有 semantic 足夠，就不要新增欄位。

## 11. 近期建議順序

以目前 Astrahold 實作來看，最安全的演進順序是：

```text
1. 保留 Protocol v9 Fireball current contract
2. Client 擴充 targeting / VFX presentation vocabulary
3. 把 line / point / self 類技能做成 data-driven presentation presets
4. 若需要 remote readability，再做 ActionStarted + AOI observer fan-out decision
5. 第一個 persistent gameplay zone 出現後，再定 persistent-effect lifecycle schema
6. 第一個 gameplay projectile / freehand path 出現後，再建立對應 Server runtime
```

這個順序可以直接吸收外部 Three.js demo 的視覺與操作優點，但不會因此破壞 Astrahold 已建立的 authoritative Server boundary。

## 12. Reference-code license boundary

上述 `achrefelouafi` repository 主要作為設計與實作技術研究來源。即使 repository code 為 MIT，也不代表其中 Mixamo FBX、HDRI、PBR textures、Sketchfab/Blender source models 或其他 third-party assets 自動具有相同授權。

實際採用 code / asset 前仍需逐項確認 upstream license、third-party notices 與 attribution。Server protocol / gameplay design 不應依賴任何特定第三方 asset。
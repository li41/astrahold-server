# Map1／燼望新手區怪物掉落規劃

本文件定義 **map1 / Emberwatch starter region** 第一版正式怪物掉落內容。

上位怪物 roster 見 [Map1-怪物規劃](Map1-怪物規劃.md)。

本版已把「武器池／防具池」全部拆成 exact stable ID。實作時每一列都是獨立 Server roll；未寫入 production loot catalog 並實際驗證前，仍屬正式規劃而非已生效 gameplay truth。

## 1. Map1 金幣模型

Map1 V1 正式採：

```text
item_gold_coin
```

定義：

- stable ItemArchetypeID：`item_gold_coin`
- 可堆疊
- authoritative inventory quantity
- unit weight = 0
- 不可裝備
- 不可 item-use
- 可直接作 `shop.Offer.CostArchetypeID`
- 掉落／auto grant／pickup／persistence 都沿用既有 Server inventory authority
- Map1 怪物全部 100% 產生金幣，數量由 Server 在 authored min/max 內擲出
- `uint32` 只視為目前 storage representation，不升格為經濟上的 gameplay 上限

V1 不另建 wallet Protocol。若後續拍賣場、交易、郵件、跨角色銀行等經濟需求需要專用 currency ledger，再做 migration；Map1 不先開第二套 gameplay truth。

## 2. 共用非裝備道具

| ItemArchetypeID | 定義 |
| --- | --- |
| `item_gold_coin` | 金幣；零重量 stack |
| `item_gray_wolf_pelt` | 灰狼皮；現有 Emberwatch shop 已可 1:1 換 minor healing potion，因此有正式 sink |
| `item_minor_healing_potion` | 小型治療藥水 |
| `item_minor_mana_potion` | 小型魔力藥水 |
| `item_minor_speed_potion` | 小型加速藥水；規劃為 Server-authoritative 短時間移動速度增益，中文名稱只供文件／Client presentation；正式效果數值待 item-use 實作 slice 驗證 |
| `item_low_arrow` | 木箭 |
| `item_astrahold_weapon_enhancement_scroll` | 武器強化卷 |
| `item_astrahold_armor_enhancement_scroll` | 防具／盾牌強化卷 |

`item_silver_arrow` 不進 Map1 V1。

## 3. 掉落語義

除金幣 quantity 外，每個表格列出的機率都是 **independent roll**。

例如同一次蟻后擊殺可以：

- 只掉金幣；
- 金幣 + 藥水；
- 金幣 + 一件裝備；
- 少數情況金幣 + 多件裝備。

不使用「先抽一個 pool，再從 pool 選一件」的隱藏語義。

### Stack item

```text
monster defeated
-> Server roll
-> ItemArchetypeID + quantity
-> nearby auto grant or public ground stack
-> inventory
```

### TierMid unique equipment

```text
monster defeated
-> Server selects exact equipment archetype
-> Server creates ItemInstanceID
-> Server rolls exactly one affix
-> exact instance becomes loot
-> auto grant or public pickup transfers same instance
-> inventory / equipment / persistence / relogin keep same instance
```

不得在 pickup 時才重新擲 affix。

---

# 4. Exact loot tables

## 4.1 灰狼 — `monster_gray_wolf`

金幣：

| Drop | Chance | Quantity |
| --- | ---: | ---: |
| `item_gold_coin` | 100% | 1–3 |

其他：

| Drop | Chance | Quantity |
| --- | ---: | ---: |
| `item_gray_wolf_pelt` | 45% | 1 |
| `item_minor_healing_potion` | 4% | 1 |
| `item_minor_speed_potion` | 1% | 1 |
| `item_low_leather_helmet` | 1% | 1 |
| `item_low_leather_chest` | 1% | 1 |
| `item_low_leather_gloves` | 1% | 1 |
| `item_low_leather_legs` | 1% | 1 |
| `item_low_leather_boots` | 1% | 1 |

不掉 TierMid、不掉強化卷。

## 4.2 野豬 — `monster_wild_boar`

金幣：

| Drop | Chance | Quantity |
| --- | ---: | ---: |
| `item_gold_coin` | 100% | 2–4 |

其他：

| Drop | Chance | Quantity |
| --- | ---: | ---: |
| `item_minor_healing_potion` | 5% | 1 |
| `item_low_heavy_helmet` | 1% | 1 |
| `item_low_heavy_chest` | 1% | 1 |
| `item_low_heavy_gloves` | 1% | 1 |
| `item_low_heavy_legs` | 1% | 1 |
| `item_low_heavy_boots` | 1% | 1 |
| `item_militia_iron_spear` | 1% | 1 |
| `item_militia_battle_axe` | 1% | 1 |
| `item_iron_war_mace` | 1% | 1 |

## 4.3 枯柳逃兵 — `monster_witherwill_deserter`

金幣：

| Drop | Chance | Quantity |
| --- | ---: | ---: |
| `item_gold_coin` | 100% | 4–8 |

補給：

| Drop | Chance | Quantity |
| --- | ---: | ---: |
| `item_minor_healing_potion` | 8% | 1 |
| `item_minor_mana_potion` | 5% | 1 |
| `item_minor_speed_potion` | 3% | 1 |
| `item_low_arrow` | 12% | 4–8 |

武器：

| Drop | Chance |
| --- | ---: |
| `item_militia_iron_sword` | 1% |
| `item_light_guard_sword` | 1% |
| `item_iron_dagger` | 1% |
| `item_militia_battle_axe` | 1% |
| `item_militia_iron_spear` | 1% |
| `item_hunter_shortbow` | 1% |
| `item_hunter_light_crossbow` | 1% |

防具／盾：

| Drop | Chance |
| --- | ---: |
| `item_low_leather_helmet` | 1% |
| `item_low_leather_chest` | 1% |
| `item_low_leather_legs` | 1% |
| `item_low_heavy_helmet` | 1% |
| `item_iron_rim_round_shield` | 1% |

## 4.4 枯柳惡兵 — `monster_witherwill_enforcer`

金幣：

| Drop | Chance | Quantity |
| --- | ---: | ---: |
| `item_gold_coin` | 100% | 6–12 |

補給：

| Drop | Chance | Quantity |
| --- | ---: | ---: |
| `item_minor_healing_potion` | 10% | 1 |
| `item_minor_speed_potion` | 4% | 1 |
| `item_low_arrow` | 12% | 4–8 |

武器：

| Drop | Chance |
| --- | ---: |
| `item_gladiator_iron_sword` | 1% |
| `item_iron_warhammer` | 1% |
| `item_iron_morning_star` | 1% |
| `item_iron_war_mace` | 1% |
| `item_two_hand_iron_sword` | 1% |
| `item_two_hand_battle_axe` | 1% |
| `item_long_iron_spear` | 1% |
| `item_militia_dual_blades` | 1% |

防具／盾：

| Drop | Chance |
| --- | ---: |
| `item_low_heavy_helmet` | 1% |
| `item_low_heavy_chest` | 1% |
| `item_low_heavy_gloves` | 1% |
| `item_low_heavy_legs` | 1% |
| `item_low_heavy_boots` | 1% |
| `item_iron_rim_round_shield` | 1% |
| `item_guard_shield` | 1% |
| `item_runed_square_shield` | 1% |

## 4.5 枯柳頭目 — `monster_witherwill_captain`

金幣：

| Drop | Chance | Quantity |
| --- | ---: | ---: |
| `item_gold_coin` | 100% | 20–35 |

補給：

| Drop | Chance | Quantity |
| --- | ---: | ---: |
| `item_minor_healing_potion` | 20% | 1 |
| `item_minor_mana_potion` | 12% | 1 |
| `item_minor_speed_potion` | 12% | 1 |
| `item_low_arrow` | 25% | 8–16 |

Low equipment：

| Drop | Chance |
| --- | ---: |
| `item_gladiator_iron_sword` | 2.5% |
| `item_iron_warhammer` | 2.5% |
| `item_iron_morning_star` | 2.5% |
| `item_two_hand_iron_sword` | 2.5% |
| `item_two_hand_battle_axe` | 2.5% |
| `item_long_iron_spear` | 2.5% |
| `item_guard_shield` | 2.5% |
| `item_low_heavy_chest` | 2.5% |
| `item_low_leather_chest` | 2.5% |
| `item_low_cloth_chest` | 2.5% |

TierMid unique weapon；命中時建立 exact ItemInstance + 1 affix：

| Drop | Chance |
| --- | ---: |
| `item_mid_one_hand_sword` | 1% |
| `item_mid_dagger` | 1% |
| `item_mid_one_hand_axe` | 1% |
| `item_mid_one_hand_spear` | 1% |
| `item_mid_warhammer` | 1% |
| `item_mid_two_hand_sword` | 1% |
| `item_mid_bow` | 1% |
| `item_mid_crossbow` | 1% |

TierMid Garrison Steel unique；每件命中時建立 exact ItemInstance + 1 affix：

| Drop | Chance |
| --- | ---: |
| `item_garrison_steel_helm` | 1.2% |
| `item_garrison_steel_cuirass` | 1.2% |
| `item_garrison_steel_gauntlets` | 1.2% |
| `item_garrison_steel_greaves` | 1.2% |
| `item_garrison_steel_boots` | 1.2% |

強化：

| Drop | Chance | Quantity |
| --- | ---: | ---: |
| `item_astrahold_weapon_enhancement_scroll` | 2% | 1 |
| `item_astrahold_armor_enhancement_scroll` | 2% | 1 |

## 4.6 赤土工蟻 — `monster_redsoil_worker_ant`

金幣：

| Drop | Chance | Quantity |
| --- | ---: | ---: |
| `item_gold_coin` | 100% | 1–2 |

其他：

| Drop | Chance |
| --- | ---: |
| `item_minor_healing_potion` | 3% |
| `item_low_cloth_gloves` | 1% |
| `item_low_cloth_boots` | 1% |

## 4.7 赤土兵蟻 — `monster_redsoil_soldier_ant`

金幣：

| Drop | Chance | Quantity |
| --- | ---: | ---: |
| `item_gold_coin` | 100% | 2–5 |

其他：

| Drop | Chance |
| --- | ---: |
| `item_minor_healing_potion` | 5% |
| `item_minor_speed_potion` | 1% |
| `item_low_leather_helmet` | 0.5% |
| `item_low_leather_gloves` | 1% |
| `item_low_leather_boots` | 1% |
| `item_low_heavy_helmet` | 0.5% |
| `item_low_heavy_gloves` | 0.5% |
| `item_low_heavy_boots` | 0.5% |
| `item_militia_iron_spear` | 0.5% |
| `item_iron_war_mace` | 0.5% |
| `item_iron_rim_round_shield` | 0.5% |
| `item_guard_shield` | 0.5% |

## 4.8 赤土衛蟻 — `monster_redsoil_guard_ant`

金幣：

| Drop | Chance | Quantity |
| --- | ---: | ---: |
| `item_gold_coin` | 100% | 5–9 |

補給／Low equipment：

| Drop | Chance |
| --- | ---: |
| `item_minor_healing_potion` | 8% |
| `item_minor_speed_potion` | 3% |
| `item_low_leather_chest` | 1% |
| `item_low_leather_legs` | 1% |
| `item_low_heavy_chest` | 1% |
| `item_low_heavy_legs` | 1% |
| `item_guard_shield` | 1% |
| `item_runed_square_shield` | 1% |

赤土衛蟻 **不掉任何 TierMid unique／套裝部件**。Map1 中階套裝來源集中在枯柳頭目與赤土蟻后，避免可重複深層菁英怪成為套裝 farm 主來源。

## 4.9 赤土蟻后 — `monster_redsoil_queen`

金幣：

| Drop | Chance | Quantity |
| --- | ---: | ---: |
| `item_gold_coin` | 100% | 35–60 |

補給：

| Drop | Chance |
| --- | ---: |
| `item_minor_healing_potion` | 35% |
| `item_minor_mana_potion` | 20% |
| `item_minor_speed_potion` | 8% |

Low equipment；每件 2.5%：

| Drop | Chance |
| --- | ---: |
| `item_low_cloth_helmet` | 2.5% |
| `item_low_cloth_chest` | 2.5% |
| `item_low_leather_helmet` | 2.5% |
| `item_low_leather_chest` | 2.5% |
| `item_low_heavy_helmet` | 2.5% |
| `item_low_heavy_chest` | 2.5% |
| `item_guard_shield` | 2.5% |
| `item_runed_square_shield` | 2.5% |
| `item_two_hand_iron_sword` | 2.5% |
| `item_two_hand_battle_axe` | 2.5% |
| `item_hunter_shortbow` | 2.5% |
| `item_apprentice_wood_staff` | 2.5% |

TierMid weapon unique；**17 件全部各 0.7%**：

| Drop | Chance |
| --- | ---: |
| `item_mid_one_hand_sword` | 0.7% |
| `item_mid_dagger` | 0.7% |
| `item_mid_one_hand_axe` | 0.7% |
| `item_mid_one_hand_spear` | 0.7% |
| `item_mid_warhammer` | 0.7% |
| `item_mid_morning_star` | 0.7% |
| `item_mid_mace` | 0.7% |
| `item_mid_two_hand_sword` | 0.7% |
| `item_mid_two_hand_axe` | 0.7% |
| `item_mid_two_hand_spear` | 0.7% |
| `item_mid_knuckles` | 0.7% |
| `item_mid_claw` | 0.7% |
| `item_mid_dual_blades` | 0.7% |
| `item_mid_bow` | 0.7% |
| `item_mid_crossbow` | 0.7% |
| `item_mid_sling` | 0.7% |
| `item_mid_staff` | 0.7% |

TierMid armor unique；**15 件全部各 1%**：

| Drop | Chance |
| --- | ---: |
| `item_garrison_steel_helm` | 1% |
| `item_garrison_steel_cuirass` | 1% |
| `item_garrison_steel_gauntlets` | 1% |
| `item_garrison_steel_greaves` | 1% |
| `item_garrison_steel_boots` | 1% |
| `item_windchaser_cap` | 1% |
| `item_windchaser_armor` | 1% |
| `item_windchaser_gloves` | 1% |
| `item_windchaser_leggings` | 1% |
| `item_windchaser_boots` | 1% |
| `item_arcane_rune_hood` | 1% |
| `item_arcane_rune_robe` | 1% |
| `item_arcane_rune_gloves` | 1% |
| `item_arcane_rune_trousers` | 1% |
| `item_arcane_rune_boots` | 1% |

TierMid shield unique：

| Drop | Chance |
| --- | ---: |
| `item_mid_iron_rim_round_shield` | 1.67% |
| `item_mid_guard_shield` | 1.67% |
| `item_mid_runed_square_shield` | 1.67% |

強化：

| Drop | Chance |
| --- | ---: |
| `item_astrahold_weapon_enhancement_scroll` | 3% |
| `item_astrahold_armor_enhancement_scroll` | 3% |

所有 TierMid equipment 命中時建立 exact ItemInstance + 1 affix。

## 4.10 岩岸蟹 — `monster_shore_crab`

金幣：

| Drop | Chance | Quantity |
| --- | ---: | ---: |
| `item_gold_coin` | 100% | 2–4 |

其他：

| Drop | Chance |
| --- | ---: |
| `item_minor_healing_potion` | 5% |
| `item_iron_rim_round_shield` | 1.5% |
| `item_low_heavy_gloves` | 1% |
| `item_low_heavy_boots` | 1% |
| `item_low_heavy_helmet` | 0.5% |

---

# 5. 小型加速藥水掉落規劃

正式 stable ID：

```text
item_minor_speed_potion
```

中文顯示名：**小型加速藥水**。中文名稱只供規劃與 Client presentation；Server gameplay、loot、inventory、persistence 只使用 stable ID。

Map1 V1 掉落來源：

| 怪物 | 機率 | 數量 |
| --- | ---: | ---: |
| 灰狼 | 1% | 1 |
| 枯柳逃兵 | 3% | 1 |
| 枯柳惡兵 | 4% | 1 |
| 枯柳頭目 | 12% | 1 |
| 赤土兵蟻 | 1% | 1 |
| 赤土衛蟻 | 3% | 1 |
| 赤土蟻后 | 8% | 1 |

不掉來源：

- 野豬
- 赤土工蟻
- 岩岸蟹

設計目的：

- 普通怪可以偶爾取得，但不能成為高頻消耗品洪水。
- 枯柳人形敵人是主要一般來源，符合「攜帶補給品」的內容語彙。
- 赤土深層只給少量額外來源，不讓高密度工蟻成為藥水 farm。
- 枯柳頭目／赤土蟻后提供明顯較高機率，使 elite／boss 有補給價值。

效果 authority 邊界：

```text
Client use intent
-> Server validates owned item / cooldown / character state
-> Server consumes exact inventory quantity
-> Server applies timed movement-speed effect
-> Server movement step uses authoritative effective speed
-> Client only presents buff / movement result
```

V1 規劃為**短時間正向移動速度增益**，但加速百分比、持續秒數、cooldown group、與其他 speed modifiers 的 stacking rule 尚未在本掉落文件鎖死。這些必須在通用 movement/status effect 實作時一起定義與測試，不能由 Client 自行套 multiplier。

# 5. Map1 不掉

以下在 Map1 V1 **完全不進 loot table**：

- 所有 TierHigh equipment
- `item_silver_arrow`
- 技能書
- Skin
- Blessed / Cursed enhancement scroll
- 尚無 sink 的 generic crafting trash
- 任何 Client asset identity

## 7. 現有 Emberwatch shop 與狼皮

目前正式 shop 已有：

```text
item_gray_wolf_pelt x1
-> item_minor_healing_potion x1
```

因此 `item_gray_wolf_pelt` 保留在灰狼 45% drop。

這是目前 Map1 唯一已存在的 material sink；未來若商店改成 gold pricing，可保留 pelt barter 作 early-game alternate acquisition，不需要刪除。

## 8. Loot Catalog V2 需求

現行 `internal/loot.Drop` 只有：

```text
ItemArchetypeID
ChanceBasisPoints
```

要承接上述正式表，V2 至少需要：

```text
Kind
ItemArchetypeID
ChanceBasisPoints
QuantityMin
QuantityMax
```

其中：

- `Kind=stack`：金幣、藥水、箭、卷軸、low-tier archetype equipment
- `Kind=equipment_instance`：TierMid exact unique equipment

### Quantity

金幣與箭 bundle 必須由 Server resolve quantity。

不要用重複 N 個 Drop entries 模擬 8 支箭或 30 金幣，避免大量地面 entity。

### Equipment instance

`equipment_instance` 命中後：

1. resolve exact equipment archetype
2. `iteminstance.Create`
3. Server 擲 affix一次
4. exact instance進入 loot object
5. auto-grant／manual pickup轉移同一 instance
6. reconnect維持同一 ItemInstanceID / affix

## 9. 金幣與地面物

金幣 V1 建議使用 **單一 stack drop**，不是一枚一個 entity。

若 nearby contributor inventory 可接收：

- 直接 auto grant `item_gold_coin xN`

若無法接收：

- 地面只生成一個 gold stack drop，攜帶 quantity=N

`item_gold_coin` 的 UnitWeight 必須明確 override 為 0，避免每枚金幣吃 carry weight。

## 10. 第一輪經濟節奏

金幣的初始 target：

| 怪物 | Gold |
| --- | ---: |
| 灰狼 | 1–3 |
| 野豬 | 2–4 |
| 枯柳逃兵 | 4–8 |
| 枯柳惡兵 | 6–12 |
| 枯柳頭目 | 20–35 |
| 赤土工蟻 | 1–2 |
| 赤土兵蟻 | 2–5 |
| 赤土衛蟻 | 5–9 |
| 赤土蟻后 | 35–60 |
| 岩岸蟹 | 2–4 |

這是 map1 income baseline；正式商店售價、修理、交易手續費等 sink 要以實際 kills/hour 後再定。

## 11. 實作順序

1. **Common stack item / gold identity**
   - `item_gold_coin`
   - zero weight
   - inventory / persistence / shop-cost validation

2. **Loot Catalog V2**
   - quantity range
   - candidate kind
   - strict validation
   - Server-private deterministic test hooks

3. **Stack ground drop quantity**
   - gold
   - arrows
   - healing / mana / speed potions
   - scrolls
   - archetype equipment

4. **Unique equipment loot**
   - exact ItemInstanceID
   - one-time affix roll
   - auto-grant / public pickup
   - persistence continuity

5. **Map1 exact tables**
   - 10 monster archetypes
   - exact IDs and basis points from this document

6. **Runtime balance**
   - gold/hour
   - potion sustain
   - arrows/hour
   - low gear/hour
   - TierMid unique/hour
   - scroll/hour
   - inventory pressure
   - ground entity count

正式實作後若真 runtime economy 明顯過鬆／過緊，只調 authored quantities / basis points，不重寫 loot authority。

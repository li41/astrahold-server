# Map1／燼望新手區怪物掉落規劃

本文件規劃 **map1 / Emberwatch starter region** 第一版怪物掉落與裝備取得節奏。

上位怪物內容見 [Map1-怪物規劃](Map1-怪物規劃.md)。

本文件是 gameplay content plan；正式掉率在實作前仍需以真 Client TTK、平均擊殺速度、背包容量與裝備供給速度校正。沒有落入 Server loot catalog 並通過驗證前，不算 production gameplay truth。

## 1. 設計目標

Map1 掉落只解決三件事：

1. 玩家打怪後經常有「得到東西」的回饋。
2. 玩家能在新手區逐步取得 **低階裝備**，並第一次看到少量 **中階 unique 裝備**。
3. 木箭、藥水與少量強化資源能從 PvE 取得，但不讓 starter region 直接供應整個後期經濟。

V1 不追求完整經濟系統，不把每隻怪都塞滿材料／垃圾／貨幣。

## 2. 掉落層級

### A. 常用消耗品

- `item_minor_healing_potion`
- `item_minor_mana_potion`
- `item_low_arrow`（木箭）

用途：維持玩家繼續打怪的基本循環。

### B. 低階裝備

Map1 是低階 equipment 的主要 PvE 來源。

範圍：

- 低階武器
- 低階盾牌
- 低階 cloth / leather / heavy 五部位防具

低階裝備維持 archetype-only；不需要 unique affix。

### C. 中階 unique 裝備

只由：

- 枯柳頭目
- 赤土衛蟻
- 赤土蟻后

少量掉落。

中階裝備掉落時必須由 Server 建立新的 exact `ItemInstanceID`，並在生成當下擲出正式 1 affix；之後掉落、拾取、背包、裝備、持久化、relogin 都保留同一 instance。

### D. 高階裝備

**Map1 V1 不掉高階裝備。**

`TierHigh` 留給後續 Region / Dungeon，不讓 starter region 跳過 progression。

### E. 強化卷

Map1 不把普通怪做成強化卷 farm。

建議 V1：

- `item_astrahold_weapon_enhancement_scroll`：只由枯柳頭目／赤土蟻后極低機率掉落。
- `item_astrahold_armor_enhancement_scroll`：只由枯柳頭目／赤土蟻后極低機率掉落。

目的只是讓玩家在 map1 能第一次接觸正式強化 loop，不建立穩定量產來源。

### F. 銀箭

`item_silver_arrow` **Map1 V1 不掉**。

理由：

- map1 沒有 undead。
- 木箭已足以支援 bow/crossbow loop。
- 銀箭應在 undead 內容開始出現前後建立來源，避免新手區累積大量目前沒有用途的銀箭。

## 3. 目前 Server 技術邊界

現行 `internal/loot` 的 Drop 只有：

```text
ItemArchetypeID
ChanceBasisPoints
```

而現行 monster loot materialization 是：

```text
monster defeated
-> resolve Drop
-> spawn public item-drop entity
-> nearby damage-weighted auto grant or ground pickup
```

這條路目前適合：

- 藥水
- 箭矢
- 低階 archetype-only equipment
- 強化卷

但 **不適合中階 unique equipment**，因為目前 monster loot path 沒有建立／保存 `ItemInstance`。

因此正式實作順序必須是：

1. stack / archetype loot 先上線；
2. 補通用 unique-equipment loot materialization；
3. 再啟用中階 unique loot entries。

不得讓中階裝備先以 stack/archetype 形式掉落，再在拾取時重新擲詞綴；詞綴必須在 Server 掉落 instance 建立時決定一次。

## 4. V1 掉率哲學

掉率使用 independent candidate rolls，不採「每隻必掉 N 件」的固定寶箱模式。

建議基準：

- 普通怪：主要掉消耗品與低階裝備，單件 equipment 機率低。
- guard / enforcer：裝備率略高。
- elite / boss：有明顯裝備期待值，但不保證每次都出中階裝。
- Boss 的重要性來自較高的 loot pool 品質，不是一次噴十件垃圾。

所有百分比實作時轉成 `ChanceBasisPoints`。

## 5. 各怪物掉落表草案

以下為 **V1 balance target**；正式實作前可微調，但類別分工應保持。

### 5.1 灰狼 `monster_gray_wolf`

定位：最基礎野外狩獵怪。

建議：

- 小型補給：治療藥水 **6%**
- 低階 leather 防具池：**合計 5%**
- 低階 dagger / shortbow / sling 類輕武器池：**合計 2%**

不掉：

- 中階裝備
- 強化卷
- 銀箭

狼皮類材料先不列 production 掉落；在 crafting / vendor sink 未定前，不製造只會佔背包的垃圾素材。

### 5.2 野豬 `monster_wild_boar`

定位：較耐打、偏 heavy / melee acquisition。

建議：

- 治療藥水 **7%**
- 低階 heavy 防具池：**合計 5%**
- 低階 spear / axe / mace 類武器池：**合計 3%**

不掉中階裝備。

### 5.3 枯柳逃兵 `monster_witherwill_deserter`

定位：人形敵人，正式低階武器／裝備主要來源。

建議：

- 治療藥水 **8%**
- 魔力藥水 **5%**
- 木箭：**12%**
- 低階武器池：**合計 7%**
- 低階防具／盾牌池：**合計 5%**

人形怪是 map1 最主要的 low-tier gear farm，但單隻仍不應高機率噴裝。

### 5.4 枯柳惡兵 `monster_witherwill_enforcer`

定位：較硬的人形 guard。

建議：

- 治療藥水 **10%**
- 木箭 **12%**
- 低階武器池：**合計 8%**
- 低階 heavy armor / shield：**合計 8%**
- 中階裝備：V1 **不直接掉**

讓普通據點內容主要完成 low-tier build，不讓玩家刷惡兵就直接大量進中階。

### 5.5 枯柳頭目 `monster_witherwill_captain`

定位：map1 地表 elite acquisition checkpoint。

建議：

- 治療藥水 **20%**
- 魔力藥水 **12%**
- 木箭 **25%**
- 低階武器／盾牌／防具：**合計 25%**
- 中階武器 unique pool：**合計 8%**
- 中階 Garrison Steel armor unique pool：**合計 6%**
- 武器強化卷：**2%**
- 防具強化卷：**2%**

不掉高階裝備。

### 5.6 赤土工蟻 `monster_redsoil_worker_ant`

定位：數量多、低 HP；不能讓高密度直接膨脹裝備供給。

建議：

- 治療藥水 **3%**
- 低階 cloth / leather gloves / boots 等輕部位池：**合計 2%**

不掉武器、不掉中階、不掉強化卷。

### 5.7 赤土兵蟻 `monster_redsoil_soldier_ant`

定位：巢穴主力怪。

建議：

- 治療藥水 **5%**
- 低階 leather / heavy 防具池：**合計 4%**
- 低階 shield / spear / mace 類池：**合計 2%**

### 5.8 赤土衛蟻 `monster_redsoil_guard_ant`

定位：深層菁英，開始讓玩家看到中階 unique。

建議：

- 治療藥水 **8%**
- 低階防具：**合計 6%**
- 中階 armor unique pool：**合計 3%**

中階池以三套中階防具為主：

- `set_garrison_steel`
- `set_windchaser_huntgear`
- `set_arcane_rune_robes`

不要讓每種衛蟻固定綁一個職業／裝甲類；Astrahold 已是 classless。

### 5.9 赤土蟻后 `monster_redsoil_queen`

定位：map1 第一個地下 Boss，也是 starter region 最好的 loot source。

建議：

- 治療藥水 **35%**
- 魔力藥水 **20%**
- 低階裝備：**合計 30%**
- 中階武器 unique pool：**合計 12%**
- 中階 armor unique pool：**合計 15%**
- 中階 shield unique pool：**合計 5%**
- 武器強化卷：**3%**
- 防具強化卷：**3%**

這些是 independent rolls，所以同一場可能零件、單件或多件；但平均不應變成「每殺一次一定噴一套」。

### 5.10 岩岸蟹 `monster_shore_crab`

定位：探索支線怪，不應成為最佳 farm。

建議：

- 治療藥水 **5%**
- 低階 shield / heavy gloves / boots 類池：**合計 4%**

不掉中階裝備、不掉強化卷。

## 6. Equipment pool 分配

避免 107 件 equipment 全塞進同一區。

### Map1 low-tier weapon pool

Map1 可以覆蓋全部 low-tier weapon archetype，但依怪物 identity 分池：

**人形／據點**
- sword
- axe
- mace
- morning star
- warhammer
- spear
- two-hand weapon
- crossbow
- shield

**野外／狩獵**
- dagger
- bow
- sling
- spear

**蟻穴**
- 不以武器掉落為主；主要給 armor

這只是 loot pool grouping，不表示怪物真的「使用」該武器；AI weapon presentation／combat equipment若未來做，仍需獨立 Server-authored monster equipment。

### Map1 low-tier armor pool

全部 15 件 low-tier armor 都可由 map1 取得：

- cloth 5
- leather 5
- heavy 5

大致分工：

- 灰狼：leather
- 野豬：heavy
- 枯柳人形：三類都可
- 蟻穴：cloth / leather / heavy 混合，但以手套／鞋／頭盔等較輕部位較常見
- 岩岸蟹：heavy 小部位

### Map1 mid-tier unique pool

Map1 只啟用 **TierMid**，不啟用 TierHigh。

可包含：

- 全部 mid-tier weapons
- mid-tier shields
- 三套 mid-tier armor：
  - Garrison Steel
  - Windchaser Huntgear
  - Arcane Rune Robes

來源限制：

- 枯柳頭目
- 赤土衛蟻
- 赤土蟻后

這樣玩家有理由打 elite / dungeon，而不是只刷村門口狼。

## 7. 木箭掉落

現行 loot entity 一個 drop = 一個 item，因此沒有 Quantity field。

但箭矢如果每次只掉 1 支，體感與效能都不好。

V1 建議擴充 loot definition 支援：

```text
QuantityMin
QuantityMax
```

由 Server 一次 resolve 數量，再以 inventory stack grant / ground pickup quantity 表達。

在這個 extension 完成前：

- 不用複製 8–12 個 `Drop{item_low_arrow}` entries 偽裝成箭束。
- 不讓大量 item-drop entity 同時出現在地上。

建議木箭 bundle：

- 枯柳逃兵／惡兵：4–8 支
- 枯柳頭目：8–16 支

如果 V1 不想先改 quantity protocol，也可以暫時只讓人形怪掉少量單支箭，但這只是短期 playtest，不升格正式 balance。

## 8. Unique equipment loot semantics

這是 acquisition implementation 的核心。

正式流程應為：

```text
monster defeated
-> Server loot roll selects TierMid equipment archetype
-> Server creates exact ItemInstance
-> Server rolls affix exactly once
-> ground-drop owns exact instance identity
-> eligible auto grant OR public pickup transfers same instance
-> inventory snapshot / equipment / persistence keep same ItemInstanceID
```

禁止：

- 掉落只存 ItemArchetypeID，拾取時才創 instance
- 每次 reconnect 重擲 affix
- Client 產生 ItemInstanceID
- auto-loot 與 manual pickup 使用兩套 instance creation path

建議新增通用 loot candidate kind：

```text
stack
equipment_instance
```

而不是為「蟻后掉 unique」寫 boss-specific code。

## 9. Ground drop / auto-loot 規則沿用

現有政策可繼續使用：

- Server-private drop roll
- 玩家實際 damage contribution
- nearby contributor damage-weighted winner
- inventory 可容納則 auto grant
- 無法 auto grant則保留 public ground drop
- public drop 有 bounded expiry
- evade/reset 清舊 contribution

Map1 不新增 first-hit ownership、party-only loot 或 personal loot system。

隊伍 loot policy等 party system 正式設計後再做。

## 10. 暫不加入的掉落

Map1 V1 不掉：

- TierHigh equipment
- `item_silver_arrow`
- 未定義用途的怪物材料／vendor trash
- 技能書
- Skin
- Blessed / Cursed enhancement scroll
- currency（目前沒有正式貨幣 gameplay contract）

不要因為「怪物應該掉東西」而創造沒有 sink 的素材。

## 11. 預期 acquisition 節奏

第一輪 balance target：

- 玩家打普通怪應常看到消耗品，但裝備仍有期待感。
- 約每 10–20 隻普通怪出現一件 low-tier equipment 屬合理量級。
- elite / boss 明顯提高裝備品質，而不是只提高垃圾數量。
- TierMid 應主要來自枯柳頭目／赤土深層，不應在村門外普通野獸穩定 farm。
- Map1 可以讓玩家開始組中階 build，但不應在 starter region 輕易農齊全部 mid set。
- 強化卷在 map1 是「第一次接觸」，不是主要供給。

上述節奏要在真 runtime 以 kills/hour、drop/hour、inventory pressure 實測後再調掉率。

## 12. 實作切片

建議依序：

### Slice 1 — Loot catalog V2

- 支援 stack quantity range
- 支援 loot candidate kind
- validation / deterministic roll tests
- 保持目前 Server-private randomness

### Slice 2 — Unique equipment drop path

- item instance creation
- affix roll once
- exact instance ground drop
- auto-grant / manual pickup共用 transfer path
- inventory / persistence continuity

### Slice 3 — Map1 stack loot

先上：
- potions
- wood arrows
- low-tier equipment
- boss enhancement scrolls

真 runtime 驗證掉落密度。

### Slice 4 — Map1 TierMid unique loot

啟用：
- Witherwill Captain
- Redsoil Guard
- Redsoil Queen

驗證 exact ItemInstanceID / affix / pickup / reconnect。

### Slice 5 — Balance pass

用正式 Client gameplay camera與實際擊殺節奏量：

- kills/hour
- low equipment/hour
- mid unique/hour
- potion sustain
- arrow sustain
- enhancement scroll/hour
- ground-drop clutter
- inventory weight pressure

再調整 basis points，不在前面先過度精算。

## 13. 待後續區域

後續區域可以沿同一模型擴充：

- undead zone -> silver arrow source / undead-specific materials
- mine / cavern -> metal equipment / material identity
- larger dungeon -> high-tier equipment
- late region -> stable enhancement-scroll economy

Map1 只建立第一個完整 acquisition loop，不承擔全遊戲經濟。

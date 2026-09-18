# Map1／燼望新手區怪物規劃

本文件定義 **map1 / Emberwatch starter region** 的第一版 Server gameplay 怪物內容規劃。

Client world geography / presentation 以正式 Three.js Client 的 World Master / Emberwatch Region Master 為上位地理參照；本文件只定 Server gameplay content：stable monster identity、區域用途、遭遇節奏、BodySize、AI profile、生命週期與未來 loot hook。

固定 authority：

```text
Client world presentation
-> Client handoff authoritative geography facts
-> Server spawn / AI / combat / lifecycle / loot authority
-> Client presentation
```

Client asset path、scene node、raycast、視覺尺寸不決定 spawn、BodySize、AI 或 combat outcome。

## 1. Map1 gameplay 目標

map1 不做成均勻撒怪的新手草原。正式 PvE 節奏採：

```text
Emberwatch 安全村
-> Emberwatch Fields 原野野獸
-> Witherwill Stockade 人形據點
-> Redsoil Ant Nest 地下蟲巢
-> Whisperwood 西南林緣
-> 南農地／灰草溪／南岸探索
```

玩家離村後應在短時間內理解三種不同 PvE 語彙：

1. **野外狩獵**：狼／野豬，低壓力、可繞過、適合熟悉 target / attack / pickup。
2. **敵對據點**：枯柳逃兵，以較密集的人形近戰形成「攻進去」的感覺。
3. **地下巢穴**：赤土蟻群，密度更高、逐層提升，最後到蟻后。

南岸與林緣提供探索型 encounter，不把每個 POI 都變成戰鬥場。

## 2. V1 固定限制

Map1 V1 優先使用目前已存在的通用 Server foundation：

- server-owned `EntityMonster`
- `SpawnEntityRequest`
- `AutonomousMeleeAgentConfig`
- aggro / target / chase / leash / evade
- authoritative HP / defeat
- corpse -> despawn -> respawn
- threat cleanup
- ground-drop / pickup lifecycle hook
- `BodySize = small | large | giant`

V1 **不為單一怪物新增專用 gameplay system**。

因此第一版先不做：

- 弓箭手／投矛等 monster ranged combat
- 野豬 charge
- 蟻酸噴吐
- 蟻后召喚
- stun / poison / slow
- squad buff / captain aura
- pack coordination
- scripted boss phase

上述能力等通用 monster-ability / effect 模組成立後再配置回 archetype。

## 3. 正式 V1 monster roster

### 3.1 Emberwatch Fields／低語森林邊緣

| Stable ArchetypeID | 中文名 | BodySize | 定位 | V1 行為 |
| --- | --- | --- | --- | --- |
| `monster_gray_wolf` | 灰狼 | small | map1 基準野獸 | 快速近戰、較長追擊 |
| `monster_wild_boar` | 野豬 | large | 較耐打野獸 | 較慢近戰、較短追擊 |

現有 `wolf-gray-01` 只視為早期 playtest identity；正式 map1 content 應收斂到 stable gameplay ID `monster_gray_wolf`，不要讓 playtest naming 變成跨系統長期契約。

### 3.2 Witherwill Stockade／枯柳寨

枯柳寨正式採 **逃兵／潰散民兵** 路線，不採哥布林寨。

| Stable ArchetypeID | 中文名 | BodySize | 定位 | V1 行為 |
| --- | --- | --- | --- | --- |
| `monster_witherwill_deserter` | 枯柳逃兵 | small | 基本近戰 | 標準追擊／近戰 |
| `monster_witherwill_enforcer` | 枯柳惡兵 | small | 重裝近戰 | 較高 HP、較慢 |
| `monster_witherwill_captain` | 枯柳頭目 | small | map1 地表菁英 | 高 HP、長 respawn |

V1 不做弓手。未來 monster ranged foundation 完成後，可再增加 `monster_witherwill_marksman`，不阻塞第一版。

### 3.3 Redsoil Ant Nest／赤土蟻穴

| Stable ArchetypeID | 中文名 | BodySize | 定位 | V1 行為 |
| --- | --- | --- | --- | --- |
| `monster_redsoil_worker_ant` | 赤土工蟻 | small | 群體弱怪 | 低 HP、短 leash |
| `monster_redsoil_soldier_ant` | 赤土兵蟻 | large | 主力蟲怪 | 中 HP、中追擊 |
| `monster_redsoil_guard_ant` | 赤土衛蟻 | large | 深層菁英 | 高 HP、守巢 |
| `monster_redsoil_queen` | 赤土蟻后 | giant | map1 第一個地下 Boss | 極高 HP、核心巢定點 |

蟻后 V1 先是「大體型、高生命、高威脅的 melee boss」，不先發明召喚／產卵／毒霧 phase。

### 3.4 南部海岸

| Stable ArchetypeID | 中文名 | BodySize | 定位 | V1 行為 |
| --- | --- | --- | --- | --- |
| `monster_shore_crab` | 岩岸蟹 | large | 海岸探索怪 | 慢速、耐打、短 leash |

V1 不放水中怪物；Server 尚未定義游泳／水域 monster locomotion，不由 Client 水面 presentation 推導 gameplay movement。

## 4. 不放進 Map1 V1 的怪物

### Undead

Map1 V1 **不放 undead**。

理由：

- 第一大陸已有 Hollow Crypt／Sunken Abbey 等更合適的 undead 場域。
- 不因目前 silver +20% undead rule 已存在，就為了展示材質系統硬塞 undead 到新手區。
- `silver` gameplay 保持 dormant 直到正式 undead content 出現。

### 哥布林

枯柳寨不使用哥布林。World Master 已給出「舊巡防哨寨被流寇／逃兵佔據」的敘事方向，人形逃兵更符合地理與場景身份。

## 5. Encounter distribution

不做全圖 random scatter。使用 authored encounter pockets；exact spawn point 之後依 Server map1 navigation / collision representation落地。

### Emberwatch 安全村

- 村內 hostile concurrency：**0**
- 四個主要村口外保留明顯安全緩衝。
- 怪物不得 patrol 穿越安全村。
- leash 不得把野怪拖入村中心。

### Emberwatch Fields

目標同時存在約 **12–16 隻**普通野獸，拆成 6 個左右 encounter pockets：

- 灰狼：單隻或 2 隻。
- 野豬：多數單隻，少量 2 隻。
- 道路與農地保留 negative space，玩家可選擇繞怪。
- 不在每條道路節點強塞怪。

### Witherwill approach + Stockade

地表據點目標 concurrency：**8–11 隻**。

建議 composition：

- approach / 舊巡防殘跡：2–3 枯柳逃兵
- 主門／破口外：2–3 枯柳逃兵
- 寨內 open yard：3–4 逃兵／惡兵混合
- 頭目屋／中央區：1 枯柳頭目

頭目不是世界 Boss；它是 starter-region elite checkpoint。

### Redsoil surface

洞口地表保持可辨識入口，不做大群堵門：

- 2–4 工蟻
- 0–1 兵蟻

讓玩家能靠近洞口觀察後再決定進入。

### Redsoil L1 — 外圍工蟻層

目標 concurrency：**8–10**

- 工蟻為主
- 少量兵蟻
- 2–3 隻小群 encounter pockets
- 保留退路，不在入口後立刻形成不可逆圍毆

### Redsoil L2 — 食物／幼蟲層

目標 concurrency：**10–12**

- 工蟻／兵蟻混合
- 開始出現衛蟻
- encounter pocket 比 L1 更密，但仍留可恢復空間

### Redsoil L3 — 蟻后核心

目標 concurrency：**6–8 regular + 1 queen**

- 衛蟻為主
- 少量兵蟻
- 蟻后只在核心巢
- V1 不做無限增援；respawn 不能在正在打 Boss 時形成無限怪潮

### Whisperwood 西南入口

Starter Region 只涵蓋森林第一段，因此 V1 不建立完整 Whisperwood 生態。

目標 concurrency：**6–8**

- 灰狼較多
- 少量野豬
- 不新增 forest-only monster archetype，只為了「看起來有森林怪」而複製系統

完整 Whisperwood roster 等正式 Region production 時再定。

### 灰草溪／南岸

灰草溪本身以 traversal / scenery 為主，不塞水怪。

海岸目標 concurrency：**4–6 岩岸蟹**，分散在潮池／礫灘／岩岸 pocket；A5 海岸眺望附近保持主要視線與探索空間，不讓怪群堵住第一次看海的 presentation beat。

## 6. Map1 V1 怪物完整戰鬥數值

以下是 **Map1 V1 正式內容規劃值**。實作完成並經 runtime TTK／命中率／生存壓力驗證前，仍屬 authored tuning target，不得誤寫成已驗證 production balance。

`MonsterLevel` 是內容難度與未來 progression 對接用 stable metadata；它不直接替代 HP、攻擊、防禦、命中、閃避等 authoritative combat stats，也不自動套每級倍率。

Map1 V1 等級梯度：

- Lv.5–8：入門／野外。
- Lv.11–15：中段／深層主力。
- Lv.18：地表菁英頭目。
- Lv.22：Map1 地下 Boss。

### 6.1 核心戰鬥數值

| 怪物 | Lv | HP | 近戰 raw damage | 物防 | 魔防 | PhysicalHit | Evasion | CriticalRating | 暴擊率 | 攻擊間隔 | BaseXP |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| 灰狼 | 5 | 50 | 5–13 | 10 | 5 | 3 | 5 | 2 | 6% | 1.35 s | 20 |
| 野豬 | 7 | 75 | 6–17 | 20 | 5 | 1 | 1 | 0 | 5% | 1.55 s | 30 |
| 枯柳逃兵 | 8 | 85 | 7–19 | 15 | 10 | 3 | 3 | 1 | 5.5% | 1.30 s | 35 |
| 枯柳惡兵 | 12 | 170 | 9–27 | 35 | 15 | 4 | 2 | 1 | 5.5% | 1.45 s | 60 |
| 枯柳頭目 | 18 | 450 | 13–40 | 45 | 25 | 7 | 6 | 4 | 7% | 1.30 s | 180 |
| 赤土工蟻 | 7 | 55 | 5–14 | 5 | 5 | 1 | 4 | 0 | 5% | 1.10 s | 20 |
| 赤土兵蟻 | 11 | 135 | 8–24 | 20 | 10 | 3 | 3 | 1 | 5.5% | 1.30 s | 50 |
| 赤土衛蟻 | 15 | 220 | 11–33 | 40 | 20 | 5 | 2 | 2 | 6% | 1.40 s | 90 |
| 赤土蟻后 | 22 | 1200 | 18–55 | 50 | 40 | 6 | 0 | 4 | 7% | 1.60 s | 320 |
| 岩岸蟹 | 8 | 95 | 6–18 | 35 | 10 | 1 | 1 | 0 | 5% | 1.65 s | 30 |

數值語義：

- `近戰 raw damage` 是 critical 與 mitigation 前的 Server-owned `DamageMin..DamageMax`；每次命中由 Server 在閉區間內 authoritative roll，一次攻擊只 roll 一次。
- 這個傷害尺度參考 `li41/myriad-throne-server` 的同級怪物節奏：舊版怪物由 `Level + STR/3` 形成攻擊骰上限，再疊 STR damage bonus；Astrahold **只參考結果尺度，不搬舊 STR/DEX lookup table 或 Lua combat architecture**。
- 玩家基礎生命已改採舊專案相近尺度：`BaseMaxHP = 15 + 11 × (Level - 1)`。因此舊 60–150 固定 monster raw damage 與 200 HP 灰狼 playtest 尺度已 supersede，不得再作 Map1 正式 balance 依據。
- V1 十種怪的普通攻擊皆為 `physical`、`blockable=true`、`critical_eligible=true`、物防穿透 0%。
- `PhysicalHit` 與 `Evasion` 是 rating，不是百分比。
- 命中沿用正式公式：`clamp(90% + (attacker PhysicalHit - target Evasion) × 0.5%, 75%, 98%)`。
- 暴擊沿用正式公式：5% base + CriticalRating × 0.5 percentage point；本表已列出 V1 怪物在無額外 attribute bonus 時的實際基礎暴擊率。
- Map1 正式規劃改採 mitigation：`defense / (defense + 100)`。這是對現有裝備／強化尺度的修正；目前 production combat code 仍是 `+20` 曲線，實作 slice 必須同步 migration + tests 後才算 gameplay 生效。
- 這個尺度讓 50 Defense 約為 33% 減傷、100 Defense 為 50%、200 Defense 約為 67%，能容納 5 件防具 + 盾牌的基礎防禦、+N 強化、套裝 bonus 與 unique affix，而不會在 Map1 就過早接近高減傷區。
- `BaseXP` 是正式 reward target，但目前 character experience／level-up owner 尚未實作；在 progression slice 落地前不宣稱玩家已能取得 XP。

### 6.2 裝備尺度 sanity check

現行裝備資料證明 `+20` denominator 不適合正式 progression：

- 五件中階衛戍鋼基礎物防合計 12，`item_mid_guard_shield` 再給 6；六個防禦部位若各安全強化到 +4，再加衛戍鋼 5 件套 +2，合計可達 **44 PhysicalDefense**，尚未計 unique affix。
- 五件高階星鑄壁壘基礎物防合計 16，`item_high_guard_shield` 再給 8；六個防禦部位各 +4，加 2/5 件套物防 bonus 共 +3，已達 **51 PhysicalDefense**，尚未計 unique affix。
- 高階 armor／shield 本身還能 roll `affix_physical_defense`，單件最高 +5；強化亦沒有 gameplay hard max。因此防禦曲線必須能容納 50、100 甚至更高的長期數值。

注意：武器 +N 增加的是 PhysicalDamage；防禦強化來源是五件 armor + off-hand shield，共六個防禦部位。飾品目前不算在上述 sanity check。

### 6.3 移動、距離與生命週期

| 怪物 | BodySize | Move | AttackRange | Aggro / Leash | Corpse hold | Respawn | AI profile |
| --- | --- | ---: | ---: | --- | ---: | ---: | --- |
| 灰狼 | small | 4.5 m/s | 1.75 m | 9 / 18 m | 2 s | 25 s | `melee_roamer` |
| 野豬 | large | 3.6 m/s | 1.90 m | 7 / 16 m | 3 s | 30 s | `melee_roamer` |
| 枯柳逃兵 | small | 4.0 m/s | 1.80 m | 10 / 20 m | 2 s | 35 s | `melee_roamer` |
| 枯柳惡兵 | small | 3.5 m/s | 1.90 m | 9 / 18 m | 3 s | 45 s | `melee_guard` |
| 枯柳頭目 | small | 3.8 m/s | 2.00 m | 11 / 24 m | 5 s | 120 s | `melee_elite` |
| 赤土工蟻 | small | 4.0 m/s | 1.35 m | 7 / 14 m | 2 s | 25 s | `melee_roamer` |
| 赤土兵蟻 | large | 3.8 m/s | 1.70 m | 8 / 16 m | 2 s | 30 s | `melee_guard` |
| 赤土衛蟻 | large | 3.4 m/s | 1.80 m | 8 / 16 m | 3 s | 45 s | `melee_guard` |
| 赤土蟻后 | giant | 2.6 m/s | 2.40 m | 10 / 22 m | 5 s | 180 s | `melee_elite` |
| 岩岸蟹 | large | 2.8 m/s | 1.80 m | 6 / 13 m | 3 s | 35 s | `melee_roamer` |

`AttackRange` 是 AI 進入出手距離；Combat Action Catalog 必須再次驗證正式 range，不能只靠 AI steering 決定攻擊是否合法。

### 6.4 怪物個性

- **灰狼**：高移速、高閃避、偏高命中；本身防禦低，靠快速貼身與追擊形成壓力。
- **野豬**：較高 HP／物防、低閃避、慢攻擊；是第一個「硬但不靈活」的野獸。
- **枯柳逃兵**：平均型人形近戰，作為進入枯柳寨的基準。
- **枯柳惡兵**：高物防、較高單擊傷害、低閃避；靠重裝而不是速度。
- **枯柳頭目**：高命中、較高閃避、較高暴擊與較快攻擊節奏；地表最危險的單體近戰。
- **赤土工蟻**：低 HP／低防，但攻擊頻率快、數量多；危險來自群體。
- **赤土兵蟻**：中等 HP／物防與標準追擊，是蟻穴主力。
- **赤土衛蟻**：高物防、較高命中、低閃避，定位為深層守巢菁英；不靠套裝掉落維持價值。
- **赤土蟻后**：最高 HP／雙防、低閃避、較高命中與暴擊；V1 不做 phase／summon，強度來自穩定近戰與 Boss durability。
- **岩岸蟹**：高物防、低命中／低閃避、慢攻擊，是海岸耐打探索怪。

### 6.5 現行 Server 實作缺口

現有 Server foundation 已有 HP、BodySize、movement、Action damage、critical、physical/magic defense、physical hit/evasion 公式，但正式 monster content 尚未完整接入這些 stat。

Map1 實作不得只把表格寫成資料卻不生效，至少需要：

1. monster archetype 具備：
   - `MonsterLevel`
   - `MaxHP`
   - `PhysicalDefense`
   - `MagicDefense`
   - `PhysicalHit`
   - `Evasion`
   - `CriticalRating`
   - `BaseXP`
   - movement / lifecycle / AI profile
   - melee action identity + `DamageMin` / `DamageMax`

2. 通用 entity combat stat resolver：
   - 玩家仍從 character/equipment/passive truth 聚合。
   - 怪物從 monster archetype stats 聚合。
   - 不為每種怪物建立專用 combat path。

3. 通用 PvE melee hit resolver：
   - 目前玩家 equipped `basic-attack` 已有 hit/evasion roll。
   - 現有 `wolf-bite` 類 monster action 尚未消費 monster PhysicalHit／玩家 Evasion。
   - 實作時 monster physical melee 必須走同一正式 rating formula，不能維持「合法就必中」。

4. 通用 target defense resolver：
   - 玩家 defense 仍從 authoritative equipment／instance modifiers。
   - 怪物 defense 從 monster archetype stats。
   - `resolveIncomingDamage` 最終只接收已解析出的 authoritative defense，不讓 Client 傳入防禦值。

5. XP owner：
   - `BaseXP` 由 monster content author。
   - defeat／contribution owner 決定 reward recipient。
   - character progression owner 未建立前不得假裝 XP 已發放。
   - level-difference multiplier、組隊分配與 rested bonus 等都不在 Map1 V1 偷渡定義。

## 7. AI profile

V1 共用三種資料化 profile，不為 10 隻怪寫 10 套 AI：

### `melee_roamer`

灰狼、野豬、工蟻、普通逃兵、岩岸蟹。

- idle patrol
- acquire player in aggro range
- chase
- melee attack
- leash / evade / return home

### `melee_guard`

惡兵、兵蟻、衛蟻。

- patrol 半徑較小
- 較傾向守住 authored home area
- leash 比 roamer 緊
- 不追玩家跨越整個 POI

### `melee_elite`

枯柳頭目、赤土蟻后。

- 小 patrol 或定點
- 較高 HP
- 較長 respawn
- 較長但仍有限的 encounter leash
- V1 仍使用同一 authoritative melee legality，不建立 boss-only combat path

## 8. Spawn / lifecycle authority

正式實作必須保持：

```text
Map1 monster content data
-> Server spawn owner
-> Server AI
-> Server combat / threat
-> Server defeat
-> corpse
-> despawn
-> respawn
-> Client presentation
```

必要規則：

- stable `EntityID` 只代表當次 Server incarnation；respawn 必須清 threat / target-resource / stale replication。
- stable `ArchetypeID` 才是跨 spawn 的內容身份。
- exact spawn points、home positions、patrol paths 必須位於 Server-authoritative traversable map1 representation。
- Client POI anchor 可作設計參考，但不能直接當 authoritative spawn point。
- 沒有正式 Server map1 navigation/collision representation前，不把 Client world coordinate硬寫成 combat spawn truth。

## 9. Loot boundary

本文件不重複維護掉落表；正式 Map1 掉落規劃已移到 [Map1-怪物掉落規劃](Map1-怪物掉落規劃.md)。loot implementation 以本文件 stable monster archetype 作 `SourceArchetypeID`：

```text
monster ArchetypeID
-> Server loot table
-> ground drop / auto grant policy
-> inventory / unique item instance
```

不把目前 playtest `wolf-gray-01 -> item_gray_wolf_pelt 70%` 當成 map1 正式掉落；exact item、金幣、藥水、箭矢、裝備與強化卷來源以 [Map1-怪物掉落規劃](Map1-怪物掉落規劃.md) 為準。

## 10. 實作順序

Map1 monster implementation 建議切四個 slice：

1. **Map1 monster content model**
   - stable monster archetype definition
   - common melee AI profile
   - MonsterLevel / HP / physical & magic defense / PhysicalHit / Evasion / CriticalRating / BaseXP
   - per-archetype melee `DamageMin` / `DamageMax` / interval / range
   - movement / BodySize / lifecycle config
   - generic monster combat-stat / PvE hit / target-defense resolver
   - validation

2. **Emberwatch Fields + Witherwill**
   - 灰狼／野豬
   - 枯柳逃兵／惡兵／頭目
   - 真 Client visible PvE loop

3. **Redsoil Ant Nest**
   - 三層 spawn groups
   - 工蟻／兵蟻／衛蟻／蟻后
   - Boss lifecycle

4. **Whisperwood edge + South Coast**
   - reuse 灰狼／野豬
   - 岩岸蟹
   - 保留探索 negative space

掉落表在 monster spawn / combat / lifecycle 真 runtime 穩定後另開 acquisition slice，不與第一筆 map1 monster content 混成一個大型改動。

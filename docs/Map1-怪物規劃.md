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

**2026-09-18 決策：特殊攻擊另開後續規劃，不阻塞 Map1 普通近戰 V1。** 本輪只實作已定案的普通近戰、命中／閃避、暴擊、防禦、主動／非主動索敵、threat、同族 one-hop 援助與 lifecycle；charge、毒、slow、stun、遠程、召喚、aura、Boss phase 等均不得在這一輪自行補設計。

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

以下是 **Map1 V1 正式內容規劃值**。通用 monster combat foundation 已於 Server `7c401574...` 實作並由 Server CI run `35340474912` 驗證 Test／Vet／race detector；但完整 Map1 spawn placement、玩家正式 HP／MP progression 與真 runtime TTK／生存壓力尚未完成，因此表內數值仍是 authored tuning target，不得誤寫成最終 balance。

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
- Map1 正式 mitigation 為 `defense / (defense + 100)`。Server `7c401574...` 已將 physical／magic defense 共用曲線由舊 `+20` migration 到 `+100`，並更新 shield、affix、armor-ignore、critical、self-mitigation 等公式測試；Server CI run `35340474912` PASS。
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

### 6.5 現行 Server 實作狀態

Server checkpoint `7c401574b182f31e3965e48eaeb88aca985922d6` 已完成 Map1 普通近戰怪的通用 authoritative foundation，Server CI run `35340474912` 的 Test／Vet／race detector **PASS**。

已實作：

1. `internal/monstercatalog` 已正式收錄 10 個 Map1 stable monster archetype，以及本文件的：
   - `MonsterLevel`
   - `MaxHP`
   - `DamageMin / DamageMax`
   - `PhysicalDefense / MagicDefense`
   - `PhysicalHit / Evasion / CriticalRating`
   - `BaseXP` metadata
   - BodySize、movement、attack range／interval、aggro／leash、corpse／respawn
   - `AggroMode` 與同族 assist 欄位
   - shared `monster-melee` Action identity

2. runtime 已有通用 monster combat-stat resolver：
   - monster spawn 在 world-owner path 綁定 authored combat stats。
   - corpse despawn 清除當次 EntityID stats；respawn 由 lifecycle config 重新註冊，不沿用 stale incarnation state。
   - 玩家與怪物仍共用既有 combat／damage／vitals owner path，不建立第二套怪物 damage authority。

3. 普通近戰已真正消費 authored stats：
   - monster `DamageMin..DamageMax` 每次命中由 Server 在閉區間 authoritative roll。
   - monster PhysicalHit 對 target Evasion 使用正式 rating formula。
   - 玩家 equipped `basic-attack` 打 monster 時會消費 monster authored Evasion。
   - monster CriticalRating 走既有 5% base + rating 的正式 critical resolver。
   - monster physical／magic defense 走共用 incoming-damage resolver。
   - defense curve 已正式 migration 為 `defense / (defense + 100)`。

4. AI／threat：
   - `aggressive` 可依 AggroRadius 主動取得玩家。
   - `passive` 不因玩家靠近開戰，但受傷後會反擊。
   - `same_family` assist 依 encounter group、family、radius、LOS、home/leash legality 選最近援軍，再以 EntityID 穩定排序，受 `MaxAssist` 限制。
   - assist 是 one-hop；被叫來的援軍不再 rebroadcast，避免 chain aggro。
   - evade／defeat／respawn 會清 encounter threat／assist transient state。

5. playtest 灰狼已由歷史 `wolf-gray-01` cutover 到 `monster_gray_wolf`，普通攻擊由歷史 `wolf-bite` 收斂到 shared `monster-melee`，實際 HP／傷害／移速／攻速／命中／閃避／暴擊／防禦由正式 Map1 catalog 提供。

仍未完成，不得誤寫為已上線完整 Map1：

- 其餘 9 種怪的 exact Server spawn point／home／patrol／encounter group placement；沒有正式 Server map1 navigation/collision 座標前，不從 Client POI 猜。
- `BaseXP` 的實際發放、level-difference／party／contribution 規則；character progression owner 尚未建立。
- 玩家正式 `BaseMaxHP / BaseMaxMP` progression migration；runtime 目前仍保留 1000／100 playtest baseline。
- Map1 Loot Catalog V2、金幣 quantity、完整 exact loot／unique equipment materialization。
- 特殊攻擊／monster ability；依本文件決策留待後續獨立規劃。

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

### 7.1 主動／非主動與同族援助

怪物是否主動索敵與是否支援附近同族是兩個獨立的 Server-authored 欄位，不由 Client presentation 或怪物名稱推導。

`AggroMode`：

- `aggressive`：玩家進入 authored AggroRadius 且通過 Server targeting legality 後，怪物可主動建立 target／threat。
- `passive`：不因玩家接近自行開戰；受到合法傷害／敵對效果後才建立 threat。若本身允許接收 ally assist，也可以因同族求援加入戰鬥。

`AssistPolicy`：

- `none`：不呼叫附近同族，也不因一般同族求援加入。
- `same_family`：受到玩家傷害並正式進入戰鬥時，可通知同一 `AssistFamilyID` 的附近怪物加入。
- 援助只允許 **one-hop**：被叫來的援軍不得再次廣播援助，避免 chain aggro 把整個 POI／蟻穴拉進同一場戰鬥。
- 援助候選必須同屬 authored encounter pocket/group、在 `AssistRadius` 內、仍存活、未超出自己的 home/leash legality；不得隔牆跨 encounter pocket 或跨區域求援。
- `MaxAssist` 是一次 alert 最多加入的額外怪物數；選擇由 Server deterministic policy 決定（優先距離最近，再以 EntityID 穩定排序），Client 不決定誰來援助。
- 援助只建立對「觸發傷害的玩家／其合法敵對來源」的初始 threat，不直接複製完整 threat table。
- Boss 求援只會喚起**當下已存在的 authored nearby monsters**；V1 不因 assist 動態生成新怪，也不繞過「Boss 戰中不形成無限 respawn 怪潮」規則。

Map1 V1：

| 怪物 | AggroMode | AssistFamilyID | AssistPolicy | AssistRadius | MaxAssist | 玩家體感 |
| --- | --- | --- | --- | ---: | ---: | --- |
| 灰狼 | aggressive | `family_gray_wolf` | same_family | 6 m | 2 | 會主動追近距離玩家；狼群靠太近時可能有 1–2 隻加入 |
| 野豬 | passive | — | none | — | 0 | 玩家不打就不主動攻擊，受攻擊後單獨反擊 |
| 枯柳逃兵 | aggressive | `family_witherwill` | same_family | 6 m | 2 | 寨外／外圍可主動攻擊，附近同伙會小規模支援 |
| 枯柳惡兵 | aggressive | `family_witherwill` | same_family | 8 m | 3 | 守區型；攻擊其中一人容易帶動附近守軍 |
| 枯柳頭目 | aggressive | `family_witherwill` | same_family | 10 m | 3 | 頭目戰可帶入附近既存守軍，但不生成援軍 |
| 赤土工蟻 | passive | `family_redsoil_ant` | same_family | 6 m | 2 | 本身不主動；攻擊工蟻可能驚動附近蟻群 |
| 赤土兵蟻 | aggressive | `family_redsoil_ant` | same_family | 7 m | 2 | 蟻穴主力會主動迎擊並呼叫少量同巢單位 |
| 赤土衛蟻 | aggressive | `family_redsoil_ant` | same_family | 8 m | 3 | 深層守巢，較容易形成小型群戰 |
| 赤土蟻后 | aggressive | `family_redsoil_ant` | same_family | 10 m | 4 | 只喚起 Boss pocket 內既存蟻群；V1 無召喚／產卵 |
| 岩岸蟹 | passive | — | none | — | 0 | 探索型耐打怪，不主動追人、不互相幫忙 |

這些欄位與既有 `Aggro / Leash` 距離共同生效：

```text
AggroMode
+ AggroRadius
+ AssistPolicy / AssistFamilyID / AssistRadius / MaxAssist
+ encounter-group boundary
+ per-entity home / leash
-> Server authoritative target/threat decision
```

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
- 沒有正式 Server map1 navigation/collision representation前，不把 Client world coordinate硬寫成 combat spawn truth。2026-09-18 檢查時 repository 的正式 Gameplay World 只有 `worlds/castle-sandbox/gameplay.json` 與 `worlds/gm-room/gameplay.json`，尚無 Map1 gameplay proxy；因此其餘 9 種怪的 exact placement 保持未落地是 authority-safe 的刻意結果，不是用 Client 座標暫代。

## 9. Loot boundary

本文件不重複維護掉落表；正式 Map1 掉落規劃已移到 [Map1-怪物掉落規劃](Map1-怪物掉落規劃.md)。loot implementation 以本文件 stable monster archetype 作 `SourceArchetypeID`：

```text
monster ArchetypeID
-> Server loot table
-> ground drop / auto grant policy
-> inventory / unique item instance
```

playtest 灰狼已使用正式 `monster_gray_wolf` identity，現行 runtime fixture 只先接 `item_gray_wolf_pelt` 45% 作既有 loot path 驗證；這**不是完整 Map1 loot table**。金幣 quantity、藥水、箭矢、裝備、強化卷與 TierMid unique 等仍以 [Map1-怪物掉落規劃](Map1-怪物掉落規劃.md) 為正式 authored target，待 Loot Catalog V2／unique materialization slice 實作。

## 10. 實作順序

Map1 monster implementation 建議切四個 slice：

1. **Map1 monster content model — 已完成 foundation**
   - stable 10-monster archetype catalog
   - common melee AI／threat／passive-aggressive／one-hop assist
   - MonsterLevel / HP / physical & magic defense / PhysicalHit / Evasion / CriticalRating / BaseXP metadata
   - per-archetype melee `DamageMin` / `DamageMax` / interval / range
   - movement / BodySize / lifecycle config seam
   - generic monster combat-stat / PvE hit / target-defense resolver
   - defense `+100` migration
   - Server `7c401574...` / CI `35340474912` PASS

2. **Emberwatch Fields + Witherwill — placement/content rollout 待做**
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

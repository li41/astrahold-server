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

## 6. Relative combat tuning baseline

下列是 **第一輪 Server tuning target**，不是 character level 系統，也不是新的 Protocol field。

以正式 map1 灰狼作 1.0 基準。實作時先沿用現有 playtest wolf 約 200 HP 的量級，經真 Client TTK 再調整。

| Archetype | HP target | Damage pressure | Move | Aggro / Leash | Respawn |
| --- | ---: | ---: | ---: | --- | --- |
| 灰狼 | 200 | 1.00x | 4.5 m/s | 9 / 18 m | 25 s |
| 野豬 | 280 | 1.15x | 3.6 m/s | 7 / 16 m | 30 s |
| 枯柳逃兵 | 240 | 1.05x | 4.0 m/s | 10 / 20 m | 35 s |
| 枯柳惡兵 | 360 | 1.30x | 3.5 m/s | 9 / 18 m | 45 s |
| 枯柳頭目 | 700 | 1.50x | 3.8 m/s | 11 / 24 m | 120 s |
| 赤土工蟻 | 120 | 0.70x | 4.0 m/s | 7 / 14 m | 25 s |
| 赤土兵蟻 | 230 | 1.00x | 3.8 m/s | 8 / 16 m | 30 s |
| 赤土衛蟻 | 380 | 1.25x | 3.4 m/s | 8 / 16 m | 45 s |
| 赤土蟻后 | 1000 | 1.60x | 2.6 m/s | 10 / 22 m | 180 s |
| 岩岸蟹 | 300 | 0.90x | 2.8 m/s | 6 / 13 m | 35 s |

補充：

- Damage pressure 是相對 tuning target，不進 wire。
- 正式 action damage 要在實作 slice 對現有玩家武器、命中率與實際 TTK 做 checkpoint 校正後落數值。
- regular corpse hold 先以 2–3 秒量級；頭目／蟻后可 4–5 秒，確保 defeat presentation 可見。
- exact respawn tick 由 Server tick rate換算，Client 不持有 respawn truth。

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

本文件 **不決定正式掉落表／掉率**。

之後 loot slice 以本文件 stable monster archetype 作 `SourceArchetypeID`：

```text
monster ArchetypeID
-> Server loot table
-> ground drop / auto grant policy
-> inventory / unique item instance
```

不把目前 playtest `wolf-gray-01 -> item_gray_wolf_pelt 70%` 自動升格成 map1 正式掉落。

裝備、箭矢、強化卷、材料與一般雜物掉落等到下一個 acquisition 設計 pass再定。

## 10. 實作順序

Map1 monster implementation 建議切四個 slice：

1. **Map1 monster content model**
   - stable monster archetype definition
   - common melee AI profile
   - HP / movement / BodySize / lifecycle config
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

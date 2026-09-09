# Game Design Recovery — 2026-09-08

## 結果

已找回先前放在 `li41/astrahold-assets` 歷史規劃 branch 的完整 game-design snapshot。

原始來源：

- repository: `li41/astrahold-assets`
- branch: `content-gameplay-planning`
- exact commit: `aaf299a91bcb01082ae7f1c9ac49b5dc19f9eada`
- subtree: `docs/game-design/`
- preserved archive branch: `archive/game-design-2026-09-06`

重新以 source tree（`truncated=false`）計數後，共 **27 份 Markdown 設計文件**。中途曾口頭估算為 29 份，該數字不正確；正式清單以本文件與 source tree 為準。

## 為什麼不把 27 份全部直接當成現行 Server truth

Recovery snapshot 同時包含：

- 仍有效的核心職業／能力值／世界觀方向；
- 後來已 supersede 的技能名稱；
- 歷史 Unreal Client wording；
- 後來已撤回或重新標為未決定的裝備取得、掉落、配方構想；
- 尚未經 runtime 驗證的 balance 候選值。

因此採兩層處理：

1. **完整原稿不改內容保存**：`astrahold-assets/archive/game-design-2026-09-06@aaf299...`。
2. **正式 Server 只提升已核對的現行入口**：`docs/CLASS_PLAN_V1.md`；另把 canonical class source 原文保存於 `docs/recovered/game-design-2026-09-06/CLASS_CANONICAL_STATE_V1.md`。

歷史 snapshot 是設計證據，不自動覆寫最新 Server source、正式 Protocol、Project Instructions 或使用者後續明確決定。

## Current supersedes

- 正式 Client 是 `li41/astrahold-client-three.js`；recovered 文件中的 Unreal wording 屬歷史資料。
- Server 仍是唯一 gameplay authority。
- Server 不保存 Client asset path。
- 低階裝備正式取得方式仍是 **UNDECIDED**；歷史狼皮／王齒／配方描述不得直接恢復成目前 economy truth。
- 已撤回的「3 張灰狼皮 -> mana siphon staff」以及舊固定 siphon 行為不得因 recovery 復活。
- durable Character `ClassID` 尚未因 recovery 自動成為 runtime contract；真正實作前必須另外定義 stable IDs、persistence 與 Protocol consumption。

## Recovered source files

### Core / class / attribute

- `ATTRIBUTE_AND_COMPANION_SYSTEM.md`
- `ATTRIBUTE_SCALING_BASELINE_V1.md`
- `CLASS_AUXILIARY_SKILLS.md`
- `CLASS_BALANCE_BASELINE_V1.md`
- `CLASS_CANONICAL_STATE_V1.md`
- `CLASS_DESIGN.md`
- `CLASS_ROLE_OVERLAP_AND_COUNTER_AUDIT.md`
- `CLASS_SKILL_STANDARD.md`

### Content / world / progression

- `CONTENT_ROADMAP.md`
- `EMBERWATCH_REWARD_LOOP_V1.md`
- `EMBERWATCH_VERTICAL_SLICE.md`
- `EQUIPMENT_LOOT_BUILD_SYSTEM_V1.md`
- `EQUIPMENT_VISUAL_ASSET_PLAN_V1.md`
- `FREE_ASSET_COMMERCIAL_QUALITY.md`
- `GAME_VISION.md`
- `GRAYFANG_COMPANION_VERTICAL_SLICE.md`
- `GRAYVEIN_GEAR_PROGRESSION_V1.md`
- `README.md`
- `SIEGE_AND_GUILD_WAR.md`
- `STORY_AND_WORLD.md`
- `WORLD_MAP_AND_PLACES.md`

### Per-class originals

- `classes/BREAKER.md`
- `classes/OATHGUARD.md`
- `classes/OATHHEALER.md`
- `classes/RANGER.md`
- `classes/SHADOWBLADE.md`
- `classes/STARFIRE_MAGE.md`

## 使用方式

找職業 roster / V1 canonical 語義：先讀 `docs/CLASS_PLAN_V1.md`。

需要考古完整設計脈絡：讀 preserved archive branch 的 `docs/game-design/`，再依本文件的 supersede 規則判斷；不要直接把歷史 economy、Client 或 balance 候選值轉成 Server gameplay truth。

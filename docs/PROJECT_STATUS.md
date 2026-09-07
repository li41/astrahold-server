# Astrahold Server — Project Status

> 本文件是 **moving state**。Milestone、Protocol version、active branches / PRs、integration blockers、近期施工順序與驗證狀態都放在這裡或對應 PR / Issue，不放 Project Instructions。

## Official repositories

- Server: `li41/astrahold-server`
- Client: `li41/astrahold-client-three.js`
- Assets: `li41/astrahold-assets`
- Tools: `li41/astrahold-tools`
- `li41/astrahold-client` 已關閉，不再視為正式 Client。

Server 與 Client 分開施工。Server agent 對 Client repository 只讀；Client agent 對 Server repository 只讀。跨 repo gameplay / Protocol coordination 走 GitHub Issue / PR discussion，不互相代改 source。

## Current gameplay direction

目前 Server 優先服務可玩的 3D MMORPG loop：

`移動 -> 選怪 -> 攻擊 -> 被攻擊 -> 死亡 / 重生 -> 擊殺 -> 掉落 -> 撿取 / inventory`

基礎 infrastructure / security / siege 歷史能力仍保留，但新的施工排序優先玩家可見、可玩的 gameplay vertical slice，除非 authority、data integrity、security 或 production correctness 要求先處理底層問題。

## Canonical Three.js integration target

截至 2026-09-07，正式 Three.js Client 的唯一 Server integration target 是：

- Branch: `integration/threejs-canonical-v20`
- PR: #141 — `integration: establish canonical Three.js Server v20 target`
- Protocol: **v20**
- Exact validated head: `cca0171adfe102ab7c94e3093e007108b10bae5d`
- Server CI run: `34098742964`
  - `go test ./...` PASS
  - `go vet ./...` PASS
  - configured race detector PASS

Canonical branch 以 PR #138 gameplay head 為基底，並整合：

- v19 player restart / monster corpse-respawn / loot pickup / evade / damage-weighted nearby loot / authored loot chance / idle patrol gameplay stack
- PR #136 的 v20 authoritative carry-weight `InventorySnapshot`
- Browser WebSocket ASTR adapter
- exclusive loopback `browserws-dev` worldd hosting

`browserws-dev` 只負責 transport / ingress hosting，仍進同一個 `gateway.Ingress`、`worldruntime`、combat、inventory、loot 與 AI authority path，不是第二套 gameplay runtime。

Server Issue #139 已回覆 canonical target。Server -> Client 正式 handoff 已開在 Client repo Issue #7：

`[gpt-server] Adopt canonical Server Protocol v20 integration target`

`gpt-client` 已回覆接受 canonical v20 target，並在 Client repo 建立 `feat/canonical-v20-client`；Client 會自行完成 semantic mirror / codec / Inventory / Equipment / NPC / Shop / Pickup / Respawn consumption，再 against exact Server canonical head 做 Browser live checkpoint。Carry weight 由 Client 顯示 Server snapshot，不在 Client 重算。

Canonical branch 在 Client integration 期間維持 frozen；後續 Server gameplay slice 由 canonical head 另開 stacked branch，不讓 Client target 漂移。

## Protocol v13 -> v20 Client-visible delta

- v14: `InventorySnapshot`
- v15: `ClientEquipmentCommand` + `EquipmentSnapshot`
- v16: `ClientPickupItem` + authoritative item-drop lifecycle
- v17: `ClientInteractNPC` + `NPCInteraction`
- v18: `ClientShopCommand` + `ShopSnapshot`
- v19: `ClientRespawnRequest`
- v20: `InventorySnapshot.current_carry_weight` + `max_carry_weight`

Monster evade、loot chance、patrol 等 Server-only gameplay semantics 不要求 Client 建立對應 gameplay rule；Client 只呈現 authoritative state / events。

## Current Server PR stack

主要近期 PR：

- PR #133 — nearby monster loot distribution / carry capacity gameplay baseline。
- PR #136 — authoritative carry-weight snapshot v20。
- PR #137 — Server-authored probabilistic monster loot。
- PR #138 — deterministic authored idle patrol。
- PR #141 — canonical Three.js Server v20 integration target；Ready for review；frozen for Client integration。
- PR #142 — stationary authoritative melee target facing；基於 #141；Server CI test / vet / race PASS。
- PR #143 — ranged opening retaliation aggro；基於 #142；Server CI test / vet / race PASS。

PR #142 修正怪物進入近戰距離停步後 yaw 不再跟目標更新的問題；狼會只轉向、不滑步，cooldown 等待期間仍追蹤目標 facing。

PR #143 修正合法遠程先手在 proximity aggro 外命中怪物後，怪物同 tick evade / 補滿的問題；未交戰怪會對實際造成傷害的玩家反擊，但不搶換既有目標、不打斷 evade，也沒有提前建立完整 threat table。

實際 merge / rebase / retarget 順序每次以 GitHub 當下狀態重新確認；本文件不是永久 branch topology。

## Known validation debt

部分 historical production E2E workflow 仍存在 stale hard-coded Protocol / old-client assumptions；例如曾觀察到 workflow 等待 `protocol=11`，而 gameplay branch 已高於該版本。

處理原則：

- 不隱藏紅燈。
- 新 slice 必須先與 exact base head 對照，確認是 regression 或 pre-existing debt。
- 不把無關 historical workflow 清債塞進玩家 gameplay slice。
- 真的影響 production authority / correctness 的 failure 必須阻擋 merge。

正式 Server validation gate 與 PASS 定義見 `docs/VALIDATION.md`。

## Near-term ordering

近期優先順序：

1. 等待 / 回應 gpt-client 對 Protocol v20 canonical target 的 integration feedback；Client source 由 gpt-client 自己修改。
2. Server 繼續收斂 PvE loop，不因 Client presentation 開發停止 authoritative gameplay work。
3. 下一批 Server gameplay 優先 combat encounter consistency、loot / item use 等玩家可感知缺口。
4. 不先擴張大型 guild / auction / crafting / siege framework，除非當前 playable loop 已需要。

每次選下一個 slice 時，先問：

**「玩家現在能多看到什麼、玩到什麼、感受到什麼？」**

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

## Current Server stack

截至 2026-09-07，近期 Server gameplay work 以 `sol/monster-loot-owner-v19` 為共同 gameplay baseline，並有以下待整合 PR：

- PR #133 — monster loot ownership / pickup baseline，Protocol v19 stack base。
- PR #136 — authoritative carry weight replication，`InventorySnapshot` 增加 current/max carry weight；Protocol v20 sibling contract。
- PR #137 — Server-authored probabilistic monster loot；playtest wolf pelt 目前為 70% authored chance；Protocol 不變，基於 #133。
- PR #138 — deterministic authored idle patrol；基於 #137；Protocol 不變。

這些 PR 的 merge / rebase / retarget 順序必須以 GitHub 當下狀態重新確認，不能把本段文字當成永久 branch topology。

## Current integration note

正式 Three.js Client 由另一個 Client agent 維護。Server 不替 Client 追 Protocol、不代改 Client decoder，也不把 Client 尚未消費某個 Server contract 視為 Server gameplay truth 的理由。

Protocol compatibility / handoff 流程見：

- `docs/PROTOCOL_SYNC.md`
- `docs/CLIENT_INTEGRATION.md`

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

1. 收斂 monster PvE loop：movement / facing / aggro / attack / evade / death / respawn / loot / pickup。
2. 收斂 inventory / equipment / carry capacity 與可靠 replication contract。
3. 只在 meaningful integration checkpoint 推進 Protocol / Client consumption。
4. 再擴充更多 monster archetype、loot table、NPC / shop / usable item 等內容，不為單一內容複製 gameplay system。

每次選下一個 slice 時，先問：

**「玩家現在能多看到什麼、玩到什麼、感受到什麼？」**

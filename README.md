# Astrahold Server

Astrahold 的正式 Go authoritative MMORPG Server。

Official repositories：

- Server: `li41/astrahold-server`
- Client: `li41/astrahold-client-three.js`
- Assets: `li41/astrahold-assets`
- Tools: `li41/astrahold-tools`

`li41/astrahold-client` 已關閉，不再使用。

## Core contract

```text
Client Intent
    -> Go Server Validate / Decide
    -> Authoritative State / Events
    -> Client Presentation
```

Server 是唯一 gameplay truth，負責 movement / facing、combat、HP / MP、death / respawn、cooldown / resources、PvE AI、inventory / equipment、loot / pickup、PvP / siege、persistence、entity lifecycle 與 Protocol semantics。

Client 不決定 gameplay outcome，也不把 mesh / animation / VFX / UI / local prediction 當作 authority。

詳細穩定架構見 `docs/SERVER_ARCHITECTURE.md`。

## Documentation Routing

Project Instructions 只放長期穩定規則；會隨開發變動的狀態放 repository 文件、PR 或 Issue。

| 工作範圍 | 主要來源 |
|---|---|
| Repository 入口、技術概覽 | `README.md` |
| Current milestone、active work、branch / PR、blocker、近期順序 | `docs/PROJECT_STATUS.md` |
| Server architecture、world ownership、domain boundary | `docs/SERVER_ARCHITECTURE.md` |
| Client ↔ Server collaboration / Issue handoff | `docs/CLIENT_INTEGRATION.md` |
| Protocol synchronization / version compatibility | `docs/PROTOCOL_SYNC.md` + `internal/protocol/` / codec / ingress source |
| Build / tests / CI / PASS 定義 | `docs/VALIDATION.md` + `.github/workflows/` |
| Historical milestone / transport / security / gameplay evidence | `docs/S1*`、`docs/S2*`、`docs/S3*`、`docs/S4*` 等對應文件 |
| PR / Issue-specific 施工與驗證狀態 | 對應 GitHub PR / Issue |
| Go / dependency / tool versions | `go.mod`、`go.sum`、CI workflow |

一般資訊衝突優先：

1. 使用者最新明確指示
2. Project Instructions
3. 正式 Server source / contract
4. `docs/PROJECT_STATUS.md`
5. 對應專題文件
6. 歷史 milestone 文件

Gameplay / Protocol / authority 衝突時，以正式 Server source / tests / live contract 為準。

## Server / Client ownership

Server agent identity：**gpt-server**。

Client agent identity：**gpt-client**。

- gpt-server 可直接修改 `li41/astrahold-server`。
- gpt-server 對 `li41/astrahold-client-three.js` 只讀。
- 需要 gpt-client 處理的事項，一律到 Client repository 建 `[gpt-server]` GitHub Issue 或在既有 Issue 留 comment。
- 不跨 repo 代改彼此 source。

詳細規則見 `docs/CLIENT_INTEGRATION.md`。

## Stable engineering principles

- Mutable gameplay world 保持單一 authoritative owner path。
- Network / persistence / admin 不直接修改 world state。
- World tick 不做 blocking I/O。
- Gameplay feature 優先做成通用 module，再由 `ArchetypeID` / stats / policy / loot table / respawn config 擴充。
- Server 不儲存 Client asset path。
- Stable cross-system identity 使用 `EntityID`、`ArchetypeID`、`ItemArchetypeID`、`ActionID`。
- Presentation-only 改動不任意 bump Protocol；wire-incompatible contract 必須明確升 compatibility fence。
- 不用 infrastructure-first、framework-first 的工作取代玩家可玩的 MMORPG loop，除非 authority、data integrity、security 或 production correctness 需要先處理。

## Validation

基本 Server gate 以 `.github/workflows/server-ci.yml` 為準。目前主要包含：

```bash
go test ./...
go vet ./...
```

以及 workflow 指定的 race-detector package set。

不得在未實際執行對應驗證時宣稱 PASS。完整規則見 `docs/VALIDATION.md`。

## Current status

不要從 README 推測目前 Protocol version、active PR 或下一個 slice。

請直接讀：

`docs/PROJECT_STATUS.md`

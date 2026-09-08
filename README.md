# Astrahold Server

Astrahold／星壘的正式 Go authoritative MMORPG Server。

## 正式 repositories

- Server：`li41/astrahold-server`
- Client：`li41/astrahold-client-three.js`
- Assets / 資產工具：`li41/astrahold-assets`

`li41/astrahold-tools` 已併入 `astrahold-assets/tools/`，不再使用獨立 tools repository。

舊 `li41/astrahold-client` 已關閉，不再使用。

## 核心 authority

```text
Client Intent
-> Go Server Validate / Decide
-> Authoritative State / Events
-> Client Presentation
```

Server 是唯一 gameplay truth，負責 movement / facing、combat、HP / MP、death / respawn、cooldown / resources、PvE AI、inventory / equipment、loot / pickup、PvP / siege、persistence、entity lifecycle 與 Protocol semantics。

Client 不決定 gameplay outcome，也不把 mesh、animation、VFX、UI、camera、prediction 或 local raycast 當作 authority。

## 完整規劃從這裡開始

**[docs/規劃總覽.md](docs/規劃總覽.md)**

主文件全部以繁體中文整理：

| 內容 | 文件 |
|---|---|
| 完整規劃入口 | [規劃總覽](docs/規劃總覽.md) |
| current Protocol / PR / 驗證 / 近期順序 | [專案狀態](docs/專案狀態.md) |
| 武器、盾牌、怪物、掉落、combat、roadmap | [遊戲內容與系統規劃](docs/遊戲內容與系統規劃.md) |
| 場景、UI、VFX、音效、Three.js presentation | [視覺與客戶端呈現規劃](docs/視覺與客戶端呈現規劃.md) |
| 資產、授權、工具、購買原則 | [資產與授權規劃](docs/資產與授權規劃.md) |
| Server authority / ownership | [伺服器架構](docs/伺服器架構.md) |
| gpt-server ↔ gpt-client | [伺服器與客戶端協作](docs/伺服器與客戶端協作.md) |
| Protocol compatibility | [通訊協定同步](docs/通訊協定同步.md) |
| tests / CI / PASS | [驗證規範](docs/驗證規範.md) |

`docs/S1*`、`docs/S2*`、`docs/S3*`、`docs/S4*` 等舊 milestone 文件若仍存在，只是歷史證據，不是 current planning source of truth。

## Stable engineering principles

- Mutable gameplay world 維持單一 authoritative owner path。
- Network / persistence / admin 不直接修改 world state。
- World tick 不做 blocking I/O。
- Gameplay feature 優先做成通用 module，再由 stable ID / stats / policy / loot table / AI config / respawn config 擴充。
- Server 不儲存 Client asset path。
- Stable cross-system identity 使用 `EntityID`、`ArchetypeID`、`ItemArchetypeID`、`ActionID`。
- Presentation-only 改動不任意 bump Protocol；wire-incompatible contract 必須明確升 compatibility fence。
- 玩家可見、可玩的 gameplay loop 優先於無直接玩家價值的 infrastructure expansion。

## Validation

基本 Server gate 以 `.github/workflows/server-ci.yml` 為準，主要包含：

```bash
go test ./...
go vet ./...
```

以及 workflow 指定的 race-detector package set。

不得在未實際執行對應驗證時宣稱 PASS。完整規則見 [驗證規範](docs/驗證規範.md)。

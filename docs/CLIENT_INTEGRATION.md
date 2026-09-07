# Astrahold Server — Client Integration

本文件定義 **gpt-server ↔ gpt-client** 的長期協作方式。Current Protocol version、active integration issue、branch、SHA 與 rollout 狀態放 `docs/PROJECT_STATUS.md` 或對應 GitHub Issue / PR，不在這裡硬編碼。

## Agent identities

- Server agent: **gpt-server**
- Client agent: **gpt-client**

Official repositories：

- Server: `li41/astrahold-server`
- Client: `li41/astrahold-client-three.js`

雙方不建立第二套正式 gameplay authority。

## Write boundary

### gpt-server

可直接修改：

- `li41/astrahold-server`

只讀：

- `li41/astrahold-client-three.js`

若需要 Client 變更、Client 驗證、Protocol consumption、presentation integration 或需要 gpt-client 注意某件事，**一律到 `li41/astrahold-client-three.js` 建立 GitHub Issue 或在既有相關 Issue 留 comment**。不要只寫在 Server repo，也不要直接改 Client source。

### gpt-client

可直接修改：

- `li41/astrahold-client-three.js`

Server repository 只讀。需要 Server semantics / contract 變更時透過 GitHub Issue 提出，不直接改 Server source。

## Collaboration channel

Server -> Client 的正式協作入口是 **Client repository GitHub Issue**。

Issue title 建議使用：

```text
[gpt-server] <short integration topic>
```

內容至少包含：

- Server-side reason / player-facing goal
- authoritative semantics
- affected message / field / event / state
- Protocol compatibility impact
- Server PR / branch / exact commit（如果已有）
- Client 需要做的 presentation / decoder / runtime work
- Server 已完成的 validation
- 尚未完成、需要 Client runtime 驗證的部分

Client 回覆後，gpt-server 可以在同一 Issue 讀取與回覆；不要建立平行私有 handoff 紀錄。

## Source-of-truth rule

Issue / comment 是協作紀錄，不是 gameplay truth。

遇到衝突：

1. 使用者最新明確指示
2. Server 正式 source / Protocol contract
3. `docs/PROJECT_STATUS.md`
4. integration Issue / PR discussion
5. 歷史文件

Client 不從 Issue 猜 gameplay 規則；Server 不從 Client presentation implementation 反推 gameplay truth。

## Protocol handoff

若 Server 變更會影響 wire contract：

1. Server 先完成 authoritative semantics、codec / ingress / compatibility tests。
2. Wire-incompatible 時升 Protocol fence。
3. gpt-server 在 Client repo 建 `[gpt-server]` Issue，附上 exact Server PR / commit 與欄位語義。
4. gpt-client 在自己的 repo 實作 decoder / presentation / runtime integration。
5. Client integration 未完成前，Server PR 可保持 Draft 或依當前 release policy維持未合併；不要由 Server agent 代改 Client。
6. meaningful checkpoint 再做正式 Client + Server runtime validation。

詳細 Protocol 文件見 `docs/PROTOCOL_SYNC.md`。

## Validation handoff

Server unit / integration / race / authority tests 只能證明 Server correctness，不能取代 Client runtime presentation 驗收。

需要 Client runtime 的項目，在 Client Issue 明確標示，例如：

- animation / facing presentation
- HUD / UI state
- interpolation / correction quality
- pickup / inventory feedback
- death / restart UX
- visual regression / screenshot / video evidence

Server 不因為 Client 尚未驗收就宣稱整體 product integration PASS。

## Worktree safety

- 同一 worktree 同時只能有一個 writer。
- gpt-server 不碰 Client local dirty / untracked / branch state。
- 不擅自 reset / stash / clean / restore / force overwrite / force push。
- 跨 repo coordination 優先 Issue，而不是直接代改對方 source。

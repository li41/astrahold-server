# Astrahold Server — Validation

本文件定義 Server 的穩定 validation policy。Current CI run、failing workflow、benchmark number、exact SHA 與 temporary command 放 `docs/PROJECT_STATUS.md`、PR 或 Issue，不放 Project Instructions。

## PASS rule

不得宣稱 PASS，除非實際執行對應 validation。

無法執行時標記：

**NOT VALIDATED**

並留下可執行 handoff。

## Cheap Server gates

目前正式 Server CI 的基本 gate 以 `.github/workflows/server-ci.yml` 為準，包含：

```text
go test ./...
go vet ./...
go test -race ./internal/gameplayworld ./internal/navigation ./internal/worldruntime ./internal/netadapter/tcpudp ./internal/loadlab
```

實際 Go version / CI runner / package scope 若變更，以 workflow source 為準，不把版本號複製到 Project Instructions。

## What these gates prove

### `go test ./...`

應保護：

- deterministic gameplay semantics
- authority validation / rejection
- movement / facing
- combat / HP / MP / cooldown / resources
- PvE AI
- death / respawn
- inventory / equipment
- loot / drop / pickup
- persistence semantics
- Protocol / codec / ingress compatibility
- lifecycle / replication behavior

### `go vet ./...`

作為 cheap static correctness gate；不是 gameplay validation 的替代品。

### Race detector

保護 world/runtime/network concurrency boundary；尤其是 single-owner world 周邊、adapter / queue / load path。

Race PASS 不代表 gameplay PASS；gameplay tests 仍需獨立存在。

## Feature-level validation

每個 Server slice 應有 focused tests，直接鎖定該 feature 的 authoritative contract。

例如：

- rejected intent 不消耗 MP / cooldown
- defeated entity 不被非 respawn path revive
- evade 不保留舊 combat / loot contribution
- pickup 失敗不刪除 ground drop
- inventory weight / equipment ownership 不被 Client 重算
- Protocol strict decoder 對 incompatible shape fail closed

不要只靠大型 E2E 發現基本 gameplay rule regression。

## Base-head comparison

若 PR 上有 unrelated historical workflow 紅燈：

1. 先跑 / 查 exact feature head 的正式 gate。
2. 對照 exact base head 的同一 workflow。
3. 若 base 已同型失敗，標記 pre-existing debt，附證據。
4. 若只有 feature head 新失敗，視為 regression，不能用「舊 CI 很亂」帶過。

不要把所有歷史 infrastructure debt 塞進 gameplay PR 只為了看起來全綠；但任何真正影響 authority、data integrity、security 或 production correctness 的 failure 都必須阻擋 merge。

## Expensive validation

以下只在 meaningful checkpoint 執行，不需每個 cosmetic / small Server slice 都跑：

- full Server + Client runtime integration
- production ingress / recovery / proxy E2E
- high-player-count load / soak
- WAN impairment simulation
- persistence recovery drills
- siege-scale E2E

是否需要跑，依變更實際影響面判定。

## Client/runtime boundary

Server tests 可以證明 gameplay truth，不代表 Client presentation 已正確。

若 slice 需要玩家畫面驗收，例如：

- facing / animation
- interpolation
- HP / MP HUD
- death / restart UX
- pickup feedback
- inventory UI

由 gpt-server 到 `li41/astrahold-client-three.js` 建 Issue，交給 gpt-client 在自己的 repo / runtime 驗證。

Server 不直接改 Client 來完成驗收。

## Evidence

PR / Issue 應保存：

- exact head SHA
- 實際執行的 gate
- PASS / FAIL
- relevant run ID / failure summary
- base comparison（若適用）
- 尚未驗證的 runtime / Client 項目

「理論上會過」不是 evidence。

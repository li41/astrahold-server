# Astrahold Server — Architecture

本文件描述 **穩定的 Server architecture / ownership boundary**。Current milestone、Protocol version、branch、SHA、active PR 與近期施工順序不放在這裡，請看 `docs/PROJECT_STATUS.md`。

## Official role

`li41/astrahold-server` 是 Astrahold 唯一正式 gameplay authority。

正式 Client 是 `li41/astrahold-client-three.js`。Server 不依賴 Client asset path、rendering implementation 或 presentation state 來決定 gameplay truth。

固定資料流：

```text
Client Intent
    -> Server ingress / validation
    -> bounded world-owner command
    -> authoritative gameplay decision / mutation
    -> authoritative state / events
    -> Client presentation
```

## Server-owned gameplay truth

Server 決定並保存或重建：

- movement legality / authoritative position
- gameplay facing / yaw
- combat legality / hit / miss / damage
- HP / MP / death / respawn
- cooldown / resources
- PvE AI / aggro / leash / evade / patrol policy
- inventory / equipment truth
- loot / drop / pickup / ownership policy
- NPC / shop gameplay transactions
- PvP / siege / objective / ownership
- persistence
- authoritative entity lifecycle
- Protocol semantics

Client 只送 intent。Animation、VFX、UI、camera、root motion、prediction、local raycast、interpolation 或 presentation smoothing 都不能決定上述結果。

## Runtime ownership

Mutable gameplay world 必須保持單一 authority boundary。

```text
Network / login / persistence / admin
              |
              v
       validation / fences
              |
              v
      bounded command queue
              |
              v
       single world owner
       /      |       \
 movement   combat    gameplay systems
       \      |       /
              v
      replication / events
              |
              v
       bounded outbound
```

穩定不變量：

- Network goroutine、DB、GM、admin API 不直接修改 mutable world state。
- World tick 不做 blocking network / file / database I/O。
- 所有 gameplay mutation 必須經 authoritative service / world-owner path。
- Outbound queue 必須 bounded；不能用無限記憶體隱藏 backpressure。
- Client rejection / retry / prediction 不得成為 gameplay truth。

## Domain boundaries

Gameplay feature 優先做成通用能力，再由 archetype / policy / content data 擴充。

例如：

- Monster 使用 `ArchetypeID` + stats + AI config + loot table + respawn policy。
- Item 使用 `ItemArchetypeID` + authoritative inventory/equipment policy。
- Action 使用 `ActionID` + Server-authored legality / cost / cooldown / effect。
- Entity 使用穩定 `EntityID` 管理 lifecycle 與 replication identity。

不要為單一怪物、單一 item 或單一 map 複製 gameplay system。

## Stable identity

跨系統 identity 使用穩定 ID，而不是 presentation path：

- `EntityID`
- `ArchetypeID`
- `ItemArchetypeID`
- `ActionID`

Server 不儲存 Client mesh、texture、animation、icon 或其他 asset path。

## World and presentation boundary

Dynamic gameplay entities（玩家、NPC、怪物、掉落等）由 Server lifecycle / replication 決定。

Client 可以自行定義 static visual world presentation，但只要內容會影響：

- movement legality
- collision / navigation gameplay
- LOS
- combat range / targeting legality
- spawn / respawn
- objective / siege
- pickup legality

就必須有 Server-owned gameplay representation 或 validation source。

Visual mesh 不是 gameplay authority。

## Protocol boundary

Protocol DTO、message type、delivery class、codec 與 ingress semantics 必須和 gameplay domain 分離。

原則：

- Client intent message 只表達請求，不帶最終 gameplay outcome。
- Server state/event message 表達 authoritative result。
- Wire-incompatible contract 或會讓舊 Client/Server 產生語意歧義的變更必須升 Protocol fence。
- Presentation-only Server/Client 變更不任意 bump Protocol。
- 不在 Server domain model 塞 JSON / Client framework-specific presentation concern。

詳細 synchronization 流程見 `docs/PROTOCOL_SYNC.md`。

## Persistence boundary

Persistence 保存 authoritative gameplay state，不保存 Client presentation state。

Persistent mutation 必須具備可驗證 ordering / ownership / generation / revision semantics，不能讓 stale session 或 stale write 覆蓋較新的 gameplay truth。

Persistence implementation、schema 與 current migration 狀態屬 moving technical state，依 source、migration files 與 `docs/PROJECT_STATUS.md` 判定。

## Technology source of truth

實際 Go version、module dependency 與 toolchain constraint 以：

- `go.mod`
- `go.sum`
- CI workflow
- repository source

為準。

沒有明確相容性、安全、效能或 production 穩定性理由時，不因外部 reference 使用舊版本而降版，也不為單純追版製造無價值 dependency churn。

## Historical documents

`docs/S1*`、`docs/S2*`、`docs/S3*`、`docs/S4*` 等 milestone 文件保留當時設計與驗證證據。它們不是 current source of truth；遇到衝突時依 `README.md` 的 Documentation Routing 與最新正式 source 判定。

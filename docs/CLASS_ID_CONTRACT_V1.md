# Astrahold ClassID Contract V1

## 狀態

本文件鎖定 Astrahold 第一版六個核心職業的 stable Server `ClassID` vocabulary 與其 authoritative identity boundary。

這是 gameplay identity contract，不是 Client asset mapping。初始選職的 wire / durability contract 另見 `docs/INITIAL_CLASS_SELECTION_PROTOCOL_V25.md`。

## Canonical ClassIDs

| ClassID | 繁中名稱 | English |
|---|---|---|
| `class_oathguard` | 守誓者 | Oathguard |
| `class_breaker` | 破陣者 | Breaker |
| `class_ranger` | 巡獵者 | Ranger |
| `class_starfire_mage` | 星火術士 | Starfire Mage |
| `class_oathhealer` | 誓療師 | Oathhealer |
| `class_shadowblade` | 影刃者 | Shadowblade |

正式 source of truth：`internal/classid`。

上述 spelling 是穩定 identity；未來如果玩家可見名稱調整，不應因此改掉已持久化／跨系統使用的 ClassID。

## Unassigned semantics

空 `ClassID` 表示角色尚未被 authoritative Server assignment 流程分配職業。

它不是第七個職業，因此：

- 不建立 `class_unassigned`；
- 不把新／舊角色默認塞成守誓者；
- class-scoped gameplay policy 對空 ID 必須 fail closed；
- `ClassPolicyAll` 仍可在尚未分配職業時合法使用，避免現有低階通用裝備被職業基礎工程意外封鎖。

## Equipment policy

`equipmentcatalog.ClassPolicyAllowList` 的 authored `allowed_class_ids` 只接受上述六個 exact canonical IDs。

禁止：

- 未知 ClassID；
- 舊測試 ID，例如 `class_guard` / `class_knight`；
- 前後空白或大小寫自動正規化；
- Client asset path、model name 或 UI label 當作 ClassID。

初始 assignment 前，Server 會以目標 ClassID 重新驗證目前已裝備物品。若現有裝備對目標職業非法，assignment fail closed；不由 Client 決定、不自動脫裝。

目前 low-tier equipment 都是 `ClassPolicyAll`，所以這個 ClassID foundation 不自行改寫既有低階裝備 acquisition 或 combat semantics。

## ArchetypeID 與 ClassID 永久分離

```text
ArchetypeID = replicated entity / presentation archetype identity
ClassID     = character gameplay / profession identity
```

角色換裝、模型、外觀、體型或 Client presentation mapping 不得改變 ClassID。

職業 identity 也不得由 Client mesh、animation、UI selection 或本地 asset mapping 推導。

## Authoritative initial assignment

正式 Server 支援一次性的：

```text
empty / unassigned ClassID
    -> one canonical ClassID
```

初始 assignment 必須：

1. 由 Server 驗證只能是本文件六個 canonical IDs；
2. 使用 trusted CharacterIdentity / SessionOwnershipFence；
3. 與 character durable state 在同一 authoritative persistence owner path 保存；
4. restore 時先驗證 ClassID，再驗證 class-scoped equipment；
5. world tick 不做 blocking persistence I/O；
6. durable checkpoint 完成後，才由 world owner 把 ClassID commit 到 live character state；
7. network、DB、admin 或 Client 不得直接修改 mutable gameplay world truth；
8. 任何第二次 assignment / 任意 class swap 都 fail closed。

若角色在 durability completion 前離線，durable Store 仍是下一次 restore 的 truth；不需要為此建立第二個 live mutation path。

## Protocol v25

Protocol v25 提供正式 Client-facing 初始選職 handshake：

- Client `9` `ClientInitialClassSelection`
- Server `115` `CharacterClassState`
- Server `116` `InitialClassSelectionResult`

三者皆使用 `ReliableOrdered`。

Client 只送選職 intent。`committed` result 只能在 durable checkpoint 與 world-owner live ClassID commit 都成功後發出。

Trusted durable character 在 join / reconnect / authorized takeover 後取得 authoritative `CharacterClassState`。Ephemeral development identity 不具 durable profession identity，其 selection intent 會 fail closed。

完整 wire、rejection、backpressure、takeover semantics 見 `docs/INITIAL_CLASS_SELECTION_PROTOCOL_V25.md`。

## 本版本仍明確不做

- 轉職／轉換職業；
- respec / reset ClassID；
- 預設職業；
- 技能樹／專精；
- 六職業完整技能 runtime；
- Client-side class legality authority；
- 第二套 class persistence 或第二個 gameplay authority。

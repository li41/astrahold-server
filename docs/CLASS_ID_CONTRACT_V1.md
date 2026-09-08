# Astrahold ClassID Contract V1

## 狀態

本文件鎖定 Astrahold 第一版六個核心職業的 stable Server `ClassID` vocabulary。

這是 gameplay identity contract，不是 Client asset mapping，也不是轉職／選職流程規格。

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

Runtime 尚未有 authoritative character ClassID 時，既有 equipment path 對 allow-list 維持 fail closed；目前 low-tier equipment 都是 `ClassPolicyAll`，所以本 slice 不改變現有可玩裝備行為。

## ArchetypeID 與 ClassID 永久分離

```text
ArchetypeID = replicated entity / presentation archetype identity
ClassID     = character gameplay / profession identity
```

角色換裝、模型、外觀、體型或 Client presentation mapping 不得改變 ClassID。

職業 identity 也不得由 Client mesh、animation、UI selection 或本地 asset mapping 推導。

## 本 slice 明確不做

- 轉職／轉換職業；
- class selection UI；
- character creation 職業選擇；
- 預設職業；
- 技能樹／專精；
- 六職業技能 runtime；
- ClassID Protocol replication；
- Client-side class legality；
- 第二套 class persistence 或第二個 gameplay authority。

## 後續 authoritative assignment 原則

當正式 character creation / class assignment 開始施工時，ClassID 必須：

1. 由 Server 驗證只能是本文件六個 canonical IDs；
2. 與 character durable state 在同一 authoritative persistence owner path 保存；
3. restore 時先驗證 ClassID，再驗證 class-scoped equipment；
4. 不允許 network、DB、admin 或 Client 直接修改 mutable gameplay world truth；
5. 若未來真的設計轉職，另開正式設計與 mutation contract，不在本 V1 foundation 預埋任意 class swap。

## Protocol

本 foundation **不升 Protocol**；現有 Protocol v24 wire semantics 不變。

ClassID 尚未在此 slice 複製給 Client，因此 Client 不得自行宣告或推導 gameplay profession truth。

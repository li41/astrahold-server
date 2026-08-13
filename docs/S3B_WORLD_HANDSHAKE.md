# S3-B：World Identity 與 Dynamic World State

S3-B 解決兩個進入正式 Castle Blockout 前不能留下的問題：

1. Client / Server 必須證明自己載入**同一份 Gameplay Proxy**。
2. Gate 等動態世界狀態必須由 World Runtime 權威修改並可靠複寫，不能讓 Siege code 直接碰 Navigation。

## Protocol Version 2

S3-B 對 `SessionWelcome` 做 wire contract 變更，因此 Astrahold Protocol 從 v1 升到 **v2**。

原則：

> 任何會讓既有 Client 無法正確解碼／理解的 wire 變更，都必須升 Protocol Version；不要依賴「反正現在還在開發」而靜默破壞相容性。

Frame 層遇到不同 Protocol Version 直接拒絕，避免錯版 Client 進入世界後才產生難追的同步問題。

## World Identity

`SessionWelcome` 新增：

```text
world_id
world_revision
gameplay_sha256
```

Server 的來源只能是啟動時已 strict validate 的 `gameplay.json`：

```text
raw gameplay.json
      ↓
strict load / validate
      ↓
SHA-256 exact bytes
      ↓
protocol.WorldIdentity
      ↓
SessionWelcome
```

`tcpudp.Server.Open()` 若沒有合法 World Identity 直接失敗，不允許送出空白 Welcome。

Client 的正確流程是：

```text
TCP SessionWelcome
      ↓
載入 res://worlds/{world_id}/gameplay.json
      ↓
驗 world_id / revision / raw SHA-256
      ↓ OK
Activate Realtime UDP
```

也就是 **世界驗證成功前不能啟用 realtime movement**。

## Dynamic World State

低頻且關鍵的世界狀態使用：

```text
WorldDynamicState
├── revision
└── blockers[]
    ├── id
    └── enabled
```

它走 `ReliableOrdered`，不走 realtime snapshot。

Entity position / movement 仍走 realtime，不能把兩種生命週期混在同一訊息。

## Gate 修改流程

禁止：

```text
Siege goroutine
→ navigator.SetBlockerEnabled(...)
```

正確流程：

```text
Siege / GM / Gameplay Domain
        ↓
WorldRuntime.EnqueueSetBlocker(id, enabled)
        ↓
bounded Command Queue
        ↓
simulation owner tick
        ↓
DynamicWorld.SetBlockerEnabled
        ↓
world state revision++
        ↓
Reliable WorldDynamicState
        ↓
all sessions
```

這保留 S1 的核心不變量：只有 World Runtime owner 能修改 mutable world state。

## Revision 語意

- `revision = 1`：Gameplay World bake 初始 blocker state。
- 每次 blocker **實際發生狀態變更**才遞增。
- 重複設定相同值不遞增。
- 每個 Session 記錄最後成功送達 outbox 的 dynamic revision。
- Reliable outbox 若 backpressure，revision 不前進，下一個 tick 重試。
- 新 Session 的 known revision 視為 0，因此一定先拿到完整 dynamic snapshot。

S3-B 先送完整 blocker snapshot；資料量很小且狀態低頻。日後大型 Dynamic World 若需要 delta，再在有量測資料後加，不現在過早複雜化。

## Layering

```text
gameplayworld.BlockerState
       ↑ domain runtime state
navigation.GameplayNavigator
       ↑ implements DynamicWorld
worldruntime
       ↑ command/revision/replication owner
protocol.WorldDynamicState
       ↑ wire semantic
codec / transport
```

Navigation package 不依賴 Protocol。

## 下一步

S3-C / Castle Blockout：

- World Compiler 最小 exporter
- Godot Visual/Debug Proxy 對齊
- Gate visual/objective state
- 第一個可走的城牆、樓梯、主城門
- 兩個 Client 在 Ground / Wall 不同 Layer 同時移動
- LOS / Gate destruction E2E

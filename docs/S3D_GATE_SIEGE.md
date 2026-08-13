# S3-D：Gate Siege Interaction

S3-D 建立 Astrahold 王城 vertical slice 的第一個真正 gameplay interaction：**玩家攻擊主城門，Server 驗證並扣除 HP，城門摧毀時權威 Gameplay Proxy 同步開路。**

本階段刻意不建立完整 Combat / Skill / Guild Siege State Machine。目標是先證明 Siege objective 可以沿用既有 XYZ + Layer、WorldRuntime、Gameplay World、Protocol 與 Navigation 架構完成 E2E。

## Domain 邊界

Gate 與 Blocker 是不同概念：

```text
Siege Gate
├── ID
├── HP / MaxHP
├── attack profile（S3-D prototype）
└── blocker_id
        ↓
Gameplay Proxy Blocker
├── movement collision
└── LOS occlusion
```

Gate 是 Siege domain state；Blocker 是 Navigation / LOS proxy。未來完整 Combat damage source 可以替換 Gate 的 prototype attack profile，而不需要重寫 Navigation。

## Gameplay World schema v2

`castle-sandbox` 已升級為：

```text
schema_version = 2
revision       = s3d-001
```

主城門目前定義：

```text
id               main-gate
blocker_id       main-gate
max_hp           1000
attack_range     4.5 m
prototype_damage 100
cooldown         0.5 s
```

Client 不送 damage / range / cooldown；這些值由 Server 載入的 Gameplay World 決定。

Server 與 Godot Client 使用完全相同的 `gameplay.json` bytes，World Identity 仍以 `world_id + revision + SHA-256` fail-fast。

## Protocol v4

Protocol v4 在 v3 Realtime wire 不變的前提下新增低頻 Siege control message：

```text
ClientAttackGate
Delivery = ReliableOrdered
Payload  = { gate_id }
```

以及擴充：

```text
WorldDynamicState
├── Revision
├── Blockers[]
└── Gates[]
    ├── ID
    ├── HP
    ├── MaxHP
    └── Destroyed
```

`ClientAttackGate` 使用 TCP Reliable control path；Move / Snapshot / Correction 仍維持 S3-C.6 的 UDP compact binary。

## Action sequence

Movement 與 gameplay action 是不同 semantic stream：

```text
UDP Move
→ input sequence

TCP Gate Attack
→ action sequence
```

兩者不可共用 freshness state。

Server 對 action sequence 的語意是「intent 已處理」，不是「action 成功」。因此即使 Gate attack 因距離、LOS 或 cooldown 被拒絕，同一 sequence 也不可重播。

## Server validation

Gate attack 完全以 Server authoritative state 驗證：

```text
ClientAttackGate(gate_id)
        ↓
Reliable Gateway
        ↓
bounded WorldRuntime command queue
        ↓
session action sequence
        ↓
authoritative player Position
        ↓
Gate exists / alive
        ↓
Layer
        ↓
Range to nearest Gate blocker X/Z bounds
        ↓
Blocker enabled
        ↓
Server LOS
        ↓
Cooldown
        ↓
Server-side damage
```

距離不是量 Gate 中心，而是玩家到 blocker X/Z AABB 最近點的距離，避免寬城門產生不合理互動範圍。

### LOS 與目標 blocker

普通 LOS 仍會被關閉的 `main-gate` blocker 擋住。

攻擊 Gate 本體時，射線會忽略**目標 Gate blocker 自己**；否則射線終點落在 Gate AABB 時，Gate 會永遠遮蔽自己。其他 blocker 仍照常遮蔽。

## Destroy transaction

致命一擊在同一個 world-owner tick 中執行：

```text
Gate HP → 0
    ↓
SetBlockerEnabled(main-gate, false)
    ↓
commit Gate HP = 0
    ↓
bump WorldDynamicState revision
    ↓
Reliable broadcast
```

Blocker disable 失敗時不提交 HP=0，避免「畫面顯示門已破，但權威 collision 還在」的 split state。

Client 收到同一個 Dynamic revision 後，可同時呈現：

```text
Gate = DESTROYED
Blocker = disabled
```

## Gameplay rejection 與 Server fault

下列屬於預期 gameplay rejection，不應污染 Server fault 指標：

- unknown Gate ID
- Gate 已摧毀
- Layer 不符
- 超出距離
- LOS 被其他 blocker 阻擋
- cooldown 尚未結束

WorldRuntime 以 `ActionRejections` 與 `CommandErrors` 分開記錄。

Session 不存在、Entity 不存在、Siege service 未配置、Gate blocker state 與 HP 不一致等則仍屬 runtime/configuration fault。

## Client presentation

S3-D Godot debug presentation：

```text
F
↓
SendAttackGateAsync("main-gate")
↓
TCP ReliableOrdered

WorldDynamicState
↓
GameplayProxyDebugView
├── main-gate HP / MaxHP Label3D
├── DESTROYED label
└── blocker enabled / disabled material
```

這只是 vertical slice debug presentation。正式 Gate mesh、animation、VFX、hit feedback 與 Siege UI 不在 S3-D。

## 測試層級

Server：

- Gameplay World schema v2 / Gate reference validation
- Gate domain range / Layer / LOS / cooldown / lethal hit
- WorldRuntime Gate HP + dynamic revision + blocker opening
- Gateway delivery validation
- JSON reliable control wire round trip
- 真 TCP ingress `ClientAttackGate`
- S3-C.6 24 / 100 Load Lab regression

Client：

- .NET Release / Debug build
- Godot 4.7.1 .NET headless runtime probe
- Gameplay World v2 load / SHA handshake
- 真 TCP Reliable Gate attack frame
- Gate HP dynamic state
- Gate Destroyed + blocker disabled dynamic state
- 保留 S3-C.6 Snapshot / Correction semantic-stream probe

## 本階段不做

- Character combat stats
- weapon damage
- skill / cast / cooldown framework
- damage formula / armor
- attacker / defender permissions
- Guild / Siege phase
- Gate repair
- persistence
- S3-E Network LOD / dirty / delta

## 下一步

S3-D 完成後，下一個合理切點是把「prototype Gate attack profile」替換成最小 Combat Action / Damage pipeline，再接 Attacker / Defender 與 objective flow；S3-E replication scaling 仍保持獨立，避免 gameplay 與網路規模化同時擴張。

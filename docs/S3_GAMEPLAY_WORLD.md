# S3-A：Gameplay World v1

S3-A 的目標是讓 Astrahold Server 第一次脫離硬編碼平面地圖，改由 **版本化 Gameplay Proxy** 驅動權威 Movement、Layer transition、Gate blocker 與 LOS。

這一層不是 Visual Mesh，也不是最終 Recast/Detour 格式；它是 World Compiler 與 Gameplay Runtime 之間的穩定 contract。

## 資料流

```text
Blender / World Source
        ↓
World Compiler（後續）
        ↓
Gameplay World v1
`gameplay.json`
        ↓ strict load + validate + SHA-256
internal/gameplayworld
        ↓
GameplayNavigator
        ↓
Movement / LOS / Siege blocker
```

`movement.Service` 不知道 JSON、城門或樓梯細節，只依賴既有 `navigation.Navigator`。

## Schema v1

頂層欄位：

```text
schema_version
world_id
revision
units = meters
agent
surfaces
portals
blockers
```

### Surface

Surface 是某個 Layer 上的可走區域，S3-A 先採 axis-aligned XZ bounds + plane equation：

```text
y = base_y
  + (x - origin_x) * slope_x
  + (z - origin_z) * slope_z
```

因此同一套 schema 可以先表示：

- 平地
- 城牆走道
- 斜坡
- 簡化階梯

同一 Layer 的 Surface 不允許 XZ 內部重疊，避免 runtime 依賴陣列順序決定高度。

### Portal

Portal 描述 Layer 拓樸轉換：

```text
Ground → Stair → Wall
```

Portal 可以 bidirectional。只有角色從 trigger 外部進入／穿越 trigger 時才轉層，避免角色站在雙向 portal 內每 tick 來回切 Layer。

### Blocker

Blocker 有穩定 ID，例如：

```text
main-gate
front-wall-left
front-wall-right
```

並分開宣告：

```text
blocks_movement
blocks_los
enabled
```

`GameplayNavigator.SetBlockerEnabled(id, bool)` 是 Siege/Gate runtime 未來切換城門碰撞狀態的入口。攻城系統不應直接改 navigation internal map。

## LOS

LOS 使用真正 XYZ segment 對 blocker 3D AABB 檢查，不再假設「不同 Layer 一律看不到」。

因此未來可以表達：

- 城牆上射向地面
- 高塔射向庭院
- 關閉城門阻擋視線
- 城門摧毀後 LOS 打開

這仍是第一版 proxy geometry；複雜城堡後續可以把 LOS backend 換成 BVH / triangle proxy，而不改 Combat 呼叫介面。

## 動態 Gate

Gate 狀態不是重新 bake 世界：

```text
Gate HP > 0
→ main-gate enabled
→ movement blocked
→ LOS blocked

Gate HP <= 0
→ main-gate disabled
→ movement allowed
→ LOS allowed
```

S3-A 只建立 blocker control seam；Gate HP / Siege state machine 在後續里程碑接上。

## Castle Sandbox 範例

`worlds/castle-sandbox/gameplay.json` 目前包含：

- Ground Layer
- West Stair Layer
- Wall Walk Layer
- Ground ↔ Stair portal
- Stair ↔ Wall portal
- Main Gate dynamic blocker
- Front wall static blockers

這是 Gameplay 測試資料，不代表最終城堡美術比例。

## 嚴格載入規則

Server 啟動時：

1. JSON unknown field 拒絕
2. trailing JSON 拒絕
3. `schema_version` 必須等於 1
4. `units` 必須是 `meters`
5. ID 不可重複
6. Surface bounds / plane 必須是有限值
7. Portal 指向的 Layer 必須存在
8. movement blocker 的 Layer 必須存在
9. 計算原始 `gameplay.json` SHA-256

SHA-256 目前先用於 log/diagnostics；S3-B 會把 `world_id / revision / gameplay hash` 帶進 Client/Server world handshake，防止兩端載入不同 Gameplay Proxy。

## S3-A 完成條件

- [x] Gameplay World v1 schema
- [x] strict loader / validation
- [x] raw gameplay SHA-256
- [x] plane surface height
- [x] Layer portal transition
- [x] bidirectional portal anti-bounce rule
- [x] dynamic movement blocker
- [x] dynamic LOS blocker
- [x] cross-layer XYZ LOS
- [x] Castle Sandbox sample
- [x] `worldd -world` 載入 Gameplay World
- [x] unit tests
- [x] Server CI baseline

## 下一步 S3-B

- Client 載入對應 world package
- `SessionWelcome` 加入 world identity / revision / gameplay hash
- Client mismatch fail-fast
- 產生 debug geometry / Gameplay Proxy visualization
- Gate blocker state replication
- 第一個真正 Castle Blockout package

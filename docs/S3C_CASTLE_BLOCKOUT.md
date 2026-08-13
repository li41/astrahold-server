# S3-C：Castle Front Siege Blockout

S3-C 把 Castle Sandbox 從抽象的三塊測試 Surface，升級為第一個可以拿來驗證攻城拓樸的 **前門戰鬥切片**。

這仍然是 Gameplay Blockout，不是最終美術城堡。

## 空間配置

```text
                 Rear Wall  z≈35
        ┌──────────────────────────┐
        │                          │
        │        Courtyard         │
        │                          │
        │  West Ramp    East Ramp  │
        │      ╲          ╱        │
        ├────── Front Wall ────────┤  y=8
        │         Main Gate        │
        └────────────┬─────────────┘  z≈10
                     │
                     │
                Siege Field
                     │
                  z=-50
```

Gameplay World revision：

```text
world_id = castle-sandbox
revision = s3c-001
```

## Layer

```text
Layer 0 = Ground / Siege Field / Courtyard
Layer 1 = West + East Stair/Ramp
Layer 2 = Front Wall Walk
```

Layer 1 可以包含多個彼此分離、但拓樸語意相同的 Surface。Server spatial position 仍然保存真正 XYZ + Layer。

## Portal Contract 強化

S3-A 的 Portal transition 有一個重要問題：

```text
from.Y → target surface Y
```

若同一個 20 Hz tick 內角色先沿斜坡走了 0.3m，再跨了一個 0.5m 高差，舊邏輯會把兩者相加成 0.8m，再拿去和 `MaxStepHeight=0.5m` 比較，造成合法連接受 tick displacement 影響。

S3-C 改成：

```text
同一個 destination X/Z
source surface height
        ↕ MaxStepHeight
 target surface height
```

Simulation tick 位移與 Portal topology step 分開。

## Portal Geometry Validation

`gameplayworld.Validate()` 現在額外要求：

1. Portal trigger 完整位於某個 `from_layer` Surface 內。
2. Portal trigger 完整位於某個 `to_layer` Surface 內。
3. Trigger 四個角落的 source/target surface 高度差都不得超過 `agent.max_step_height`。

這讓不可能通行的 Portal 在 Server 啟動前就 fail-fast，而不是玩家走到樓梯頂端才卡住。

## Castle Sandbox v2 Blockout

### Ground

- X：-40 ～ 40m
- Z：-50 ～ 40m
- Siege Field 與 Courtyard 共用 Layer 0
- 城牆 Gameplay Blocker 決定能否穿越

### Main Gate

- X：-3 ～ 3m
- Front Wall 開口
- `main-gate` dynamic blocker
- Gate 關閉：Movement + LOS blocked
- Gate 開啟／摧毀：Movement + LOS allowed

### Front Wall

- Wall Walk Y = 8m
- Blocker body 高度先到 7.5m，讓站在 8m wall-walk 的角色可從牆頂取得 LOS，而不是出生點落在 LOS blocker 體積內。

### West / East Ramp

- Courtyard 內側進入
- Z 20m → 12m
- 高度 0m → 8m
- Ground ↔ Ramp portal 最大 surface gap = 0.5m
- Ramp ↔ Wall portal 最大 surface gap = 0.5m

## 測試

S3-C 增加：

- committed Castle Sandbox file strict load test
- impossible portal height gap rejection
- portal trigger 不完整位於 target surface 時 rejection
- Gate closed movement blocked
- Gate closed LOS blocked
- Gate disabled 後 movement / LOS 打開
- Ground → West Ramp → Front Wall traversal
- Front Wall → West Ramp bidirectional traversal

## 下一步

S3-D / Siege Interaction Prototype：

- Gate entity / Gate HP
- Attack Gate command
- Gate HP = 0 時由 WorldRuntime 自動 `EnqueueSetBlocker(main-gate, false)`
- Client Gate visual state / HP bar
- Objective state machine 的第一個最小版本
- 兩個 Client：攻方地面、守方城牆，同時驗證 LOS / Layer / Gate destruction

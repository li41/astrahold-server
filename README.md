# Astrahold Server

Astrahold 的全新權威 MMORPG 伺服器核心。

> 目標不是把天堂私服 Server 換名字，而是保留我們已經學會的 MMO 經驗，重新建立適合 **3D 王城、多人攻城與長期商用開發** 的底層。

## 現階段目標

第一階段只打世界模擬的地基，不急著搬帳號、商店、倉庫、任務或舊資料：

```text
World Position (XYZ + Layer)
        ↓
Spatial / AOI
        ↓
Authoritative Movement
        ↓
Navigation abstraction
        ↓
Astrahold Protocol semantics
        ↓
Godot 最小 Client 驗證
```

## 與 Myriad Throne 的關係

`myriad-throne-server` 是參考來源，不是 Astrahold 的基底。

我們會逐項評估真正值得保留的 domain 邏輯，例如角色、道具、技能、Buff、Party、Guild、持久化與資料驅動經驗；舊 Lineage protocol、2D 地圖座標、舊地圖格式與私服相容包袱不直接搬入。

詳細原則見 [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md)。

## 世界座標

Astrahold 的權威位置從一開始就是：

```go
type Position struct {
    X     float32
    Y     float32
    Z     float32
    Layer LayerID
}
```

- `X/Z`：水平世界位置
- `Y`：高度
- `Layer`：邏輯樓層／拓樸層

因此未來可以正確表達城牆、樓梯、地下層、橋面與高低差，而不必把 3D 世界硬壓回 2D Grid。

## Spatial / AOI

Grid 仍然存在，但只用來做高速空間索引：

```text
真實 XYZ Position
        ↓
32m Spatial Cell
        ↓
候選 Entity
        ↓
實際 radius / height / layer 過濾
```

**Grid ≠ 世界座標。**

## Server Authoritative Movement

Client 只送移動意圖：方向、input sequence 與時間片。

Server 自己決定：

- 移動速度
- 最長可接受時間片
- 碰撞
- 導航
- 高度
- Layer transition
- 最終權威 Position

這也是日後 client prediction、interpolation 與 reconciliation 的基礎。

## 目前目錄

```text
astrahold-server/
├── cmd/
│   └── worldd/            # 世界程序入口（目前只有 bootstrap）
├── internal/
│   ├── world/             # XYZ + Layer 與 Entity 基礎型別
│   ├── spatial/           # AOI spatial grid
│   ├── navigation/        # Navigation/LOS 抽象與測試平面
│   ├── movement/          # Server authoritative movement
│   ├── simulation/        # World state 組合層
│   └── protocol/          # Astrahold protocol 語意 DTO
└── docs/
    └── ARCHITECTURE.md
```

## 第一版刻意沒有的東西

- Lineage 3.80C protocol
- 舊加密／opcode
- 2D map `.txt/.s32`
- PostgreSQL
- Lua
- Login server
- Inventory / Item / Skill
- NPC AI
- Guild / Party
- Siege 規則

不是因為這些不重要，而是我們要先確定 **World → Movement → AOI → 新 Client** 的地基正確，再把上層 domain 一個一個接回來。

## 下一個里程碑

### S0 — World Core（目前）

- [x] XYZ + Layer
- [x] Entity 基礎型別
- [x] Spatial Grid / AOI
- [x] Authoritative Movement
- [x] Navigation abstraction
- [x] Protocol semantic DTO
- [x] 基礎單元測試

### S1 — World Loop + Realtime Transport

- [ ] 固定 tick world loop
- [ ] command queue
- [ ] connection/session abstraction
- [ ] Astrahold packet framing
- [ ] spawn/despawn/snapshot replication
- [ ] sequence / server tick / correction

### S2 — Godot Thin Client

- [ ] 連線
- [ ] 進入測試世界
- [ ] Capsule 玩家
- [ ] XYZ movement
- [ ] 第二個 client 可互相看到
- [ ] AOI enter/leave
- [ ] interpolation / reconciliation prototype

### S3 — Castle Sandbox

- [ ] World Compiler gameplay proxy
- [ ] 多高度導航
- [ ] 樓梯／Layer transition
- [ ] Gate blocker
- [ ] LOS
- [ ] 城牆上下同時有人

之後才開始 Combat / Skill / Siege 與大規模 VAT crowd 壓測。

## 開發

```bash
go test ./...
go run ./cmd/worldd
```

目前核心只使用 Go 標準函式庫，先保持依賴面最小。

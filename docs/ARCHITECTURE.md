# Astrahold Server 架構基線

## 核心決策

Astrahold Server 是全新專案。`myriad-throne-server` 只作為既有功能與資料模型的參考來源，不作為新專案的架構母體。

### 不直接沿用

- Lineage 3.80C 封包與加密相容層
- 2D `gx/gy` 作為唯一世界座標
- `.txt/.s32` 舊地圖格式
- 舊 client 相容處理
- 為私服資料格式而存在的 adapter
- 為舊遊戲規則硬編碼的特殊案例

### 可以逐項評估移植

- 帳號／角色的 domain 行為
- 道具、裝備、背包概念
- Buff / Skill 的資料驅動設計
- Party / Guild / Chat 的 domain 邏輯
- PostgreSQL migration 經驗
- Lua 是否仍適合作為內容腳本層
- 舊專案已驗證過的交易一致性與持久化概念

任何移植都必須先改成 Astrahold 的介面與資料模型，不直接複製舊 package tree。

## 世界模型

Astrahold 使用權威 3D/2.5D MMO 世界模型：

```text
Position
├── X (m)
├── Y / Height (m)
├── Z (m)
└── LayerID
```

`LayerID` 是邏輯拓樸，不是單純的高度。它讓城牆、橋面、地下層等在 X/Z 重疊時仍可以明確區分。

## Grid 的新角色

Grid 不再代表玩家站在哪一格，而只作為 Spatial/AOI acceleration structure：

```text
Entity 真實位置 XYZ
        ↓
Spatial Grid 32m x 32m（可調）
        ↓
快速取得候選 Entity
        ↓
以實際距離 / Layer / 高度差精確過濾
```

未來可替換為其他 spatial partition，而不改 gameplay position。

## 移動模型

Client 不提交最終座標，也不提供權威 delta time，只提交移動意圖：

```text
Client input
(direction, sequence)
        ↓
Server fixed tick + speed / state
        ↓
Navigator.ResolveMove
        ↓
Authoritative Position
        ↓
AOI / snapshot / correction
```

這是之後 prediction / reconciliation 的基礎。

## Navigation

目前只有 `navigation.Plane` 作為核心測試替身。

正式版本預計由 World Compiler 產出 server gameplay proxy / navigation data，導航層至少要能處理：

- 可走表面
- 高度與坡度
- 樓梯／坡道 transition
- Layer transition
- 動態 Gate blocker
- LOS
- path query
- siege objective topology

不要求伺服器執行完整剛體物理。

## Concurrency 原則

第一版假設每個 World/Zone 由單一 simulation goroutine 擁有 mutable gameplay state。

網路、DB、管理介面等透過 queue/message 與 simulation 溝通，避免在最核心的 Entity/Spatial state 上到處加 mutex。

等到有壓測數據後，再決定是否進一步拆 zone、shard 或 job system。

## Protocol 原則

Astrahold 不以 Lineage protocol 為新 client 的長期協定。

`internal/protocol` 目前只定義 message semantic DTO；wire format 暫不拍板。之後應以實際需求比較 binary encoding、schema evolution、頻寬與 CPU 成本，再決定 Protobuf / FlatBuffers / 自訂 binary 等方案。

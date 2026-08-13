# Astrahold Server 架構基線

## 核心決策

Astrahold Server 是全新專案。`myriad-throne-server` 只作為既有功能與資料模型的參考來源，不作為新專案的架構母體。

任何舊系統的移植都必須先符合 Astrahold 的世界模型、runtime 邊界與 protocol 語意，不能把 Lineage 私服的 package tree 或相容層原樣搬回來。

## 世界模型

Astrahold 使用權威 3D/2.5D MMO 世界模型：

```text
Position
├── X (m)
├── Y / Height (m)
├── Z (m)
└── LayerID
```

`LayerID` 是邏輯拓樸，不是單純高度。城牆、橋面、地下層等即使 X/Z 重疊，仍可被正確區分。

## Grid 的角色

Grid 只作為 Spatial/AOI acceleration structure，不是 gameplay position：

```text
Entity XYZ + Layer
      ↓
Spatial Grid
      ↓
候選 Entity
      ↓
距離 / Height / Layer 過濾
```

日後可更換 spatial partition，而不改 Entity Position 語意。

## 移動責任

Client 不提交最終座標，也不提供權威 delta time，只提交方向等操作意圖。

```text
ClientMoveInput
(direction + session-scoped sequence)
      ↓
Session sequence validation
      ↓
Command Queue
      ↓
Server fixed tick
      ↓
Movement + Navigator
      ↓
Authoritative XYZ + Layer
```

**Sequence 屬於 Session/Protocol 層，不屬於 Movement。** 玩家重新連線取得新 Session 後可以從新的 input sequence 空間開始，不會被舊 actor state 污染。

## Concurrency 不變量

Astrahold 第一階段採每個 World/Zone 一個 simulation owner goroutine 的模型。

必須維持兩條規則：

1. Network、DB、GM、管理 API 不得直接修改 World mutable state，只能 enqueue command。
2. World tick 不得直接做 blocking socket I/O，只能把 outbound envelope 送到非阻塞 connection/outbox。

因此核心 Entity、Spatial、Movement 不需要四處加 mutex。若未來壓測證明單 Zone 需要切 shard/job system，再在外層演進。

## S1 Runtime 資料流

```text
Network Reader / Future Transport Adapter
              ↓
        Session Boundary
              ↓
      Bounded Command Queue
              ↓
        World Runtime
    ┌─────────┴─────────┐
    ↓                   ↓
Command apply       Fixed 20 Hz Tick
                        ↓
                   Simulation
                        ↓
                AOI / Replication
                        ↓
              Protocol Envelope
                        ↓
            Non-blocking Outbox
                        ↓
              Network Writer
```

Command Queue 與 Outbox 都必須 bounded。壓力不能靠無限配置記憶體吸收；Queue full / outbound backpressure 必須成為可觀測錯誤，日後接 metrics 與 disconnect/drop policy。

## Protocol 分層

```text
Gameplay Message DTO
        ↓
protocol.Envelope
(type / delivery / sequence / server tick)
        ↓
PayloadCodec
(Protobuf / FlatBuffers / custom binary 可替換)
        ↓
Astrahold Frame
        ↓
Transport Adapter
(UDP / QUIC / TCP 等，尚未綁死)
```

`internal/protocol` 不知道 socket；`internal/transport` 不知道 gameplay rule；World/Simulation 不知道 serialization。

### Delivery class

- `ReliableOrdered`：spawn、despawn、重要狀態切換等不能任意遺失的事件。
- `RealtimeSequenced`：snapshot、position correction 等「最新狀態優先」的即時資料。

跨 channel 不保證到達順序，因此 Client 必須允許 snapshot 比 spawn 先到。Snapshot 不負責建立未知 Entity；spawn 本身攜帶初始 transform。EntityID 在同一 server lifetime 應避免快速重用，防止舊 realtime packet 誤套到新實體。

## Packet Frame v1

固定 header 28 bytes：

```text
0   uint32  Magic = ASTR
4   uint16  Protocol Version
6   uint16  Header Size
8   uint16  Message Type
10  uint8   Delivery Class
11  uint8   Flags
12  uint32  Sequence
16  uint64  Server Tick
24  uint32  Payload Length
28  bytes   Payload
```

Frame 有最大 payload 限制；decoder 必須檢查 magic、version、header size 與 payload length，避免把不可信網路資料直接帶進 gameplay layer。

## Replication

S1 採明確且容易驗證的基線：

- AOI 進入：Reliable `EntitySpawn`
- AOI 離開：Reliable `EntityDespawn`
- 週期狀態：Realtime `WorldSnapshot`
- 本機玩家修正：Realtime `PositionCorrection`
- Correction 攜帶 `LastProcessedInputSequence`，供 Godot Client reconciliation

初版可先使用 full AOI snapshot。等 S2/S3 有實際玩家數與頻寬數據後，再加入 delta compression、bit packing、priority/dirty masks；不要在沒有測量前把 replication 做成難維護的位元魔法。

## Navigation

`navigation.Plane` 只是測試替身。正式 World Compiler 需要輸出 server gameplay proxy/navigation data，至少支援可走表面、高度/坡度、Layer transition、Gate blocker、LOS、path query 與 siege topology。

伺服器不需要完整剛體物理，但所有會影響 gameplay 結果的導航、碰撞與 LOS 必須由 Server 可重現地判定。

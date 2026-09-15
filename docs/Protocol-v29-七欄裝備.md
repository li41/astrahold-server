# Protocol v29 — 通用七欄裝備契約

本文件記錄 Astrahold Server Protocol v29 的正式裝備契約。Server 為唯一 gameplay authority；Client 僅送出 intent 並呈現 Server state。

## Protocol fence

- `protocol.Version = 29`
- v28 Client 不得與 v29 Server 混用；裝備 slot 語義已擴充，必須 hard fence。
- 既有 message ID 不重編號：
  - Type 3 `ClientEquipmentCommand`
  - Type 111 `EquipmentSnapshot`
  - Type 119 `ClientEquipmentInstanceCommand`
  - Type 120 `InventoryInstanceSnapshot`
  - Type 121 `EquipmentInstanceSnapshot`
  - Type 122 `AppearanceSnapshot`

## 正式七欄

順序固定為：

1. `main_hand`
2. `off_hand`
3. `helmet`
4. `chest`
5. `gloves`
6. `legs`
7. `boots`

此順序是 wire presentation policy；裝備合法性仍由 Server catalog 與 worldruntime 驗證。

## 權威規則

- Client 的 equip/unequip 只是 intent。
- Server 決定 item 是否存在、是否可裝備、item kind、tier、slot、雙手／副手衝突、裝備屬性與最終戰鬥結果。
- `EquipmentSnapshot` 與 `EquipmentInstanceSnapshot` 是 authoritative state；Client 不得以本地 UI 狀態覆寫。
- unique instance 仍以 stable `ItemInstanceID` 操作；archetype 裝備仍以 stable `ItemArchetypeID` 操作。
- Server 不保存 Client mesh、icon、GLB path 或其他 presentation asset path。

## 持久化

Server durable inventory 使用通用 slot encoding 保存七欄 archetype／unique-instance 裝備。

舊資料中的 `MainHand`、`OffHand`、`MainHandInstanceJSON`、`OffHandInstanceJSON` 僅作讀取相容；canonicalization 會遷移到通用 slot encoding。新存檔不再以兩個手部欄位作為裝備 truth。

重登後七欄裝備必須由 durable state restore，再由正式 equipment snapshots 重建 Client state。

## 低階防具

v29 正式加入五個防具欄位，每個欄位各有三類低階散裝，共 15 件：

- 布甲 `cloth`
  - `item_low_cloth_helmet`
  - `item_low_cloth_chest`
  - `item_low_cloth_gloves`
  - `item_low_cloth_legs`
  - `item_low_cloth_boots`
- 皮甲 `leather`
  - `item_low_leather_helmet`
  - `item_low_leather_chest`
  - `item_low_leather_gloves`
  - `item_low_leather_legs`
  - `item_low_leather_boots`
- 重甲 `heavy`
  - `item_low_heavy_helmet`
  - `item_low_heavy_chest`
  - `item_low_heavy_gloves`
  - `item_low_heavy_legs`
  - `item_low_heavy_boots`

防具固定屬性沿用既有 authoritative equipment stat aggregation，會進入正式物理／魔法防禦計算；沒有第二套 armor combat system。

本切片不定義掉率、商店價格、任務、製作或其他經濟來源。

## Client consumption

Client v29 應：

- 把 Protocol hard fence 升至 29。
- Type 3／119 允許送出七個正式 slot。
- Type 111／121 store/UI 接受並 complete-replace 七欄 state；空 snapshot 仍代表清除 stale state。
- 以 stable item IDs 自行映射名稱、icon、mesh、材質等 presentation；不得把 presentation mapping 送回 Server 當 gameplay truth。
- 驗證 equip、unequip、錯欄拒絕、雙手／副手衝突，以及 reconnect/relogin 後防具仍由 Server snapshot 還原。

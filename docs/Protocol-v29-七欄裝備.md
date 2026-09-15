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
- Server 決定 item 是否存在、是否可裝備、item kind、tier、slot、雙手／副手衝突、base-stat requirement、裝備屬性、詞綴、套裝效果與最終戰鬥結果。
- `EquipmentSnapshot` 與 `EquipmentInstanceSnapshot` 是 authoritative state；Client 不得以本地 UI 狀態覆寫。
- unique instance 以 stable `ItemInstanceID` 操作；archetype 裝備以 stable `ItemArchetypeID` 操作。
- 高階 requirement 只使用 durable character base stats；裝備、詞綴、套裝、被動與狀態加成不得反向滿足 requirement。
- Server 不保存 Client mesh、icon、GLB path 或其他 presentation asset path。

## 持久化

Server durable inventory 使用通用 slot encoding 保存七欄 archetype／unique-instance 裝備。

舊資料中的 `MainHand`、`OffHand`、`MainHandInstanceJSON`、`OffHandInstanceJSON` 僅作讀取相容；canonicalization 會遷移到通用 slot encoding。新存檔不再以兩個手部欄位作為裝備 truth。

重登後七欄裝備由 durable state restore，再由正式 equipment snapshots 重建 Client state。restore 會重新驗證已裝備 unique instance 的 base-stat requirement；不合法 durable state fail closed，不以自動脫裝或裝備加成補足門檻。

## 防具 catalog

正式防具共 **45 件**，覆蓋 `helmet`、`chest`、`gloves`、`legs`、`boots` 五欄。

### 低階散裝 15 件

低階為 archetype 裝備、0 詞綴、無套裝、無 base-stat requirement。

- 布甲：`item_low_cloth_helmet`、`item_low_cloth_chest`、`item_low_cloth_gloves`、`item_low_cloth_legs`、`item_low_cloth_boots`
- 皮甲：`item_low_leather_helmet`、`item_low_leather_chest`、`item_low_leather_gloves`、`item_low_leather_legs`、`item_low_leather_boots`
- 重甲：`item_low_heavy_helmet`、`item_low_heavy_chest`、`item_low_heavy_gloves`、`item_low_heavy_legs`、`item_low_heavy_boots`

### 中階套裝 15 件

中階每件為 unique `ItemInstance`，由 Server 產生 **1 條合法詞綴**，沒有 base-stat requirement。

- 衛戍鋼甲 `set_garrison_steel`
  - `item_garrison_steel_helm`
  - `item_garrison_steel_cuirass`
  - `item_garrison_steel_gauntlets`
  - `item_garrison_steel_greaves`
  - `item_garrison_steel_boots`
  - 2 件：MaxHP +40；5 件：Strength +2、PhysicalDefense +2
- 逐風獵裝 `set_windchaser_huntgear`
  - `item_windchaser_cap`
  - `item_windchaser_armor`
  - `item_windchaser_gloves`
  - `item_windchaser_leggings`
  - `item_windchaser_boots`
  - 2 件：PhysicalHit +2；5 件：Agility +2、CriticalRating +1
- 秘紋法衣 `set_arcane_rune_robes`
  - `item_arcane_rune_hood`
  - `item_arcane_rune_robe`
  - `item_arcane_rune_gloves`
  - `item_arcane_rune_trousers`
  - `item_arcane_rune_boots`
  - 2 件：MaxMP +20；5 件：Intelligence +2、MagicPower +1

### 高階套裝 15 件

高階每件為 unique `ItemInstance`，由 Server 產生 **2 條不同合法詞綴**。

- 星鑄壁壘 `set_starforged_bastion`：base Constitution >= 18
  - `item_starforged_bastion_helm`
  - `item_starforged_bastion_cuirass`
  - `item_starforged_bastion_gauntlets`
  - `item_starforged_bastion_greaves`
  - `item_starforged_bastion_boots`
  - 2 件：MaxHP +80、PhysicalDefense +1；5 件：Strength +3、PhysicalDefense +2
- 星影追獵 `set_starshadow_huntgear`：base Agility >= 18
  - `item_starshadow_hunter_cap`
  - `item_starshadow_hunter_armor`
  - `item_starshadow_hunter_gloves`
  - `item_starshadow_hunter_leggings`
  - `item_starshadow_hunter_boots`
  - 2 件：PhysicalHit +4、Evasion +2；5 件：Agility +3、CriticalRating +3
- 星輝法衣 `set_starlight_robes`：base Intelligence >= 18
  - `item_starlight_hood`
  - `item_starlight_robe`
  - `item_starlight_gloves`
  - `item_starlight_trousers`
  - `item_starlight_boots`
  - 2 件：MaxMP +40、MagicDefense +2；5 件：Intelligence +3、MagicPower +3

套裝 activation 由目前 authoritative equipped pieces 即時計算，不另存第二份 mutable set-active state。固定屬性、unique affix 與 set bonus 共用 authoritative equipment stat aggregation；沒有第二套 armor combat system。

本契約不定義掉率、商店價格、任務、製作或其他經濟來源。

## Client consumption

Client v29 應：

- Protocol hard fence 固定為 29。
- Type 3／119 接受七個正式 slot；中高階 unique armor intent 使用 exact `ItemInstanceID`。
- Type 111／121 store/UI complete-replace 七欄 state；空 snapshot 仍代表清除 stale state。
- 顯示 Server 提供的 stable item identity、詞綴、requirement 與套裝資訊時，不在 Client 重算 equip legality 或 gameplay outcome。
- 以 stable item IDs 自行映射名稱、icon、mesh、材質等 presentation；不得把 presentation mapping 送回 Server 當 gameplay truth。
- 驗證 equip、unequip、錯欄拒絕、高階 base-stat requirement、雙手／副手衝突，以及 reconnect/relogin 後七欄狀態由 Server snapshot 還原。

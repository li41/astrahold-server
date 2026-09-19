# Protocol v31 — 十一欄裝備與飾品

本文件記錄 Astrahold Server 的正式十一欄裝備 contract。Server 是唯一 gameplay authority；Client 只送 intent 並呈現 authoritative snapshot／result。

## 正式欄位

Protocol v31 的固定欄位順序：

```text
main_hand
-> off_hand
-> helmet
-> chest
-> gloves
-> legs
-> boots
-> necklace
-> ring_1
-> ring_2
-> belt
```

v31 保留既有七欄的 identity 與順序，只在尾端追加四個飾品欄：

- `necklace`：項鍊
- `ring_1`：戒指 1
- `ring_2`：戒指 2
- `belt`：腰帶

`ring_1` 與 `ring_2` 是兩個不同的 authoritative equipment location；Client 不得用 UI index、左右顯示順序或本地裝備狀態合併兩欄。

## 飾品分類

四個新欄位都屬於 `accessory` gameplay kind。

catalog slot identity：

- 項鍊：`necklace`
- 戒指：共用 `ring` item slot kind；同一戒指 archetype 可以合法裝在 `ring_1` 或 `ring_2`
- 腰帶：`belt`

實際裝在哪一個戒指欄仍由 Server authoritative inventory／equipment snapshot 明確保存。

本 slice 只建立欄位與類別 contract，不自行新增 production 飾品 archetype、數值、掉落、商店、價格、任務或製作來源。

## 強化卷軸

飾品不可使用目前兩種正式強化卷軸：

- `item_astrahold_weapon_enhancement_scroll`：只允許 weapon。
- `item_astrahold_armor_enhancement_scroll`：只允許 armor／shield。

因此 `accessory` 對兩種卷軸皆 fail closed；Server 回 `wrong_scroll` rejection，不能消耗卷軸、不能改變飾品、不能進入強化 RNG。

## Wire contract

v31 不改既有 equipment message type ID：

- Type 3 `ClientEquipmentCommand`
- Type 111 `EquipmentSnapshot`
- Type 119 `ClientEquipmentInstanceCommand`
- Type 121 `EquipmentInstanceSnapshot`

Type 3／119 可指定十一欄中的合法 slot；Type 111／121 仍是 complete replacement snapshot，Client 必須以 Server snapshot 覆蓋本地 equipment state。

Protocol v30 的 Type 123／124 裝備強化 contract 保持不變；v31 只是擴充合法 equipment locations，並明確將新四欄歸類為不可使用現有武器／防具強化卷軸的飾品。

## Persistence

正式 durable inventory 使用 generic equipment slot encoding 保存十一欄。`ring_1` 與 `ring_2` 分別保存，不合併。

歷史 `MainHand`／`OffHand` persistence 欄位只保留 legacy read compatibility；新存檔持續以 generic equipment encoding 為 canonical truth。

## Authority invariants

- Client intent 不直接修改 equipment world truth。
- equip／unequip legality 由 Server catalog + slot mapping 驗證。
- unique equipment 以 exact `ItemInstanceID` 裝備與保存。
- reconnect／relogin 以 Server durable state 還原十一欄。
- Client UI、拖拉、slot highlight、animation 不得決定裝備結果。
- Server 不保存 Client icon、mesh、asset path 或 presentation-only metadata。

## 驗證 checkpoint

Code checkpoint：`599530b6184123afb0d1a5cea5850028579e3395`

Server CI run `35172371627` / Server CI #1403：PASS。

- `go test ./...`：PASS
- `go vet ./...`：PASS
- configured race detector：PASS

測試涵蓋十一欄固定順序、雙戒指 distinct identity、四個 accessory slot mapping、catalog legality、durable canonical round-trip，以及現有武器／防具強化卷軸對 accessory 的拒絕。

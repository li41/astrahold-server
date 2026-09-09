# Astrahold 六職業規劃 V1

## 狀態

本文件是從歷史 game-design 規劃救回後，重新收斂到正式 Server repository 的六職業規劃入口。

來源證據：

- source repository: `li41/astrahold-assets`
- source branch: `content-gameplay-planning`
- source snapshot: `aaf299a91bcb01082ae7f1c9ac49b5dc19f9eada`
- preserved archive ref: `archive/game-design-2026-09-06`
- recovered canonical source: `docs/recovered/game-design-2026-09-06/CLASS_CANONICAL_STATE_V1.md`

這份文件確認的是**職業 roster、職業幻想與 canonical V1 技能語義方向**，不是宣告目前 runtime 已實作 durable `ClassID`、全部技能或完整職業系統。

若未來實作職業 identity，`ClassID` 必須由 Server 定義與持久化，Client 只消費正式 Protocol 做 presentation；不得使用 Client asset path 當 gameplay identity。

## 六個核心職業

### 1. 守誓者（Oathguard）

核心問題：**我怎麼讓戰線不要崩？**

定位：前線承壓、保護、打斷、控制位置。

主要武器方向：單手劍 + 盾。

核心資源：`決意 0–100`。

Canonical V1 技能：

1. 劍擊
2. 破勢斬
3. 盾擊
4. 震地擊
5. 堅守
6. 盾鋒突進
7. 護衛誓約
8. 不退陣線
9. 護衛挑戰
10. 盾下反擊（被動）

重要 supersede：舊稿中的「破甲斬」已被「破勢斬」取代；守誓者不再是全隊護甲降低來源。

### 2. 破陣者（Breaker）

核心問題：**我什麼時候把戰線打穿？**

定位：正面爆發、裂甲、打散陣型、突破。

主要武器方向：雙手重武器；V1 原稿優先雙手大劍。

核心資源：`戰勢 0–100`。

Canonical V1 技能：

1. 重刃斬
2. 裂甲重劈
3. 崩勢斬
4. 橫掃
5. 破陣突進
6. 鋼意
7. 處決重擊
8. 破軍之勢
9. 震陣怒吼
10. 乘破追擊（被動）

裂甲重劈是六職業 V1 中主要的全隊護甲降低來源。

### 3. 巡獵者（Ranger）

核心問題：**現在最該處理哪個目標？**

定位：中遠距點殺、遠距打斷、陷阱、追擊、偵查。

主要武器方向：長弓。

核心資源：`獵勢 0–100`。

職業機制：`鎖定獵物`，不佔 10 技。

Canonical V1 技能：

1. 獵弓射擊
2. 弱點箭
3. 穿甲箭
4. 繫足陷阱
5. 震弦箭
6. 撤步射擊
7. 鷹眼偵察
8. 狩獵時刻
9. 示警信標
10. 追獵本能（被動）

重要 supersede：穿甲箭只讓巡獵者自己的該次攻擊忽略部分護甲；鷹眼偵察偏個人追蹤／世界偵查，團隊小範圍反側翼／反隱由示警信標負責。

### 4. 星火術士（Starfire Mage）

核心問題：**哪一塊地現在不能站？**

定位：區域壓制、路線封鎖、遠距法術、逼迫移動。

主要武器方向：長法杖。

核心資源：`星熱 0–100`；強力施法增加星熱，不是普通 Mana 消耗模型。

Canonical V1 技能：

1. 火矢
2. 星焰爆
3. 熾地
4. 星火環
5. 灼星鎖
6. 星步
7. 冷星引流
8. 燼雨
9. 星燼護幕
10. 餘燼回流（被動）

### 5. 誓療師（Oathhealer）

核心問題：**下一個會出事的人是誰？**

定位：預判防護、治療、淨化、救援、維持戰線。

主要武器方向：權杖／短杖類支援武器；具體正式 weapon archetype 待 Server content contract 定義。

核心資源：`誓印 0–3`；Server 可維護隱藏誓印進度。

Canonical V1 技能：

1. 誓光擊
2. 回誓
3. 護誓
4. 淨誓
5. 誓環
6. 退邪震印
7. 續命誓約
8. 不滅誓域
9. 守望祝福
10. 誓光回響（被動）

單人普通野外擊殺速度的歷史設計目標約為純輸出職的 85–90%，以高續航補足差距；此比例仍屬 balance target，不是 runtime guarantee。

### 6. 影刃者（Shadowblade）

核心問題：**哪個側面／後排現在露出破綻？**

定位：側翼、後排干擾、短爆發、追擊、撤離。

主要武器方向：雙短刃。

職業機制：目標 `破綻`，最多 3 層，不與其他影刃者共享。

Canonical V1 技能：

1. 雙刃連擊
2. 裂隙刺
3. 封喉
4. 影步
5. 斷筋刃
6. 煙幕撤離
7. 破綻處決
8. 獵影時刻
9. 擾亂飛刃
10. 伺機破綻（被動）

不做永久隱身，也不做滿血零反應一套秒殺。

## 職業文件 precedence

若 recovered 歷史文件互相衝突，讀取順序為：

1. `CLASS_CANONICAL_STATE_V1.md`
2. `CLASS_BALANCE_BASELINE_V1.md`
3. `CLASS_ROLE_OVERLAP_AND_COUNTER_AUDIT.md`
4. `ATTRIBUTE_SCALING_BASELINE_V1.md`
5. `CLASS_AUXILIARY_SKILLS.md`
6. `classes/*.md` 的 fantasy / loop / scenario 描述

Project Instructions、正式 Server source / current contract 與使用者最新明確決定仍高於 recovery snapshot。

## 能力值方向

歷史 V1 使用六個 Build 軸：

- STR 力量
- DEX 敏捷
- CON 體魄
- INT 智識
- WIL 意志
- CHA 魅力

主要縮放方向：

- 守誓者：STR 主；WIL 保護向次要
- 破陣者：STR
- 巡獵者：DEX
- 星火術士：INT
- 誓療師：攻擊 INT；治療／護盾 WIL
- 影刃者：DEX
- CON：跨職業生存軸
- CHA：跨職業寵物／夥伴 Build 軸，不直接免費提高本職技能傷害

寵物不是第七職業，也不得建立第二套 gameplay authority。

## Runtime 邊界

目前不要因為救回設計文件就假定以下內容已存在：

- durable Character `ClassID`
- Class selection / class change persistence
- 60 個職業能力的完整 Server runtime
- 全部職業武器與裝甲限制
- 專精／技能樹

真正實作時維持：

```text
Client Intent
-> Go Server Validate / Decide
-> Authoritative State / Events
-> Client Presentation
```

Server 決定職業 identity、技能 legality、資源、冷卻、命中、傷害、治療、控制與 persistence；Client 不得從動畫、asset、UI 或本地 class selection 直接決定 gameplay truth。

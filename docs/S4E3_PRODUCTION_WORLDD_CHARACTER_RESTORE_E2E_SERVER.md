# S4-E.3 — Production `worldd` Trusted Character Crash/Restore E2E (Server)

> **Historical milestone document.** 本文記錄 S4-E.3 當時的 production restore proof。當時 wire contract 是 Protocol v8，trusted preface 只允許受控 loopback；後續 S4-E.4 已加入 TLS 1.3 trusted ingress，S4-E.5～E.7 已演進到 Protocol v9 authenticated realtime。Current production contract 請以 root `README.md` 與 `docs/ARCHITECTURE.md` 為準。

## Scope

S4-E.3 把先前 S3-F durable/trusted restore seam 提升成真正的 **production-process Go Server ↔ Godot Client crash/restore E2E**。

本階段證明：

- production `cmd/worldd` 使用 Server-owned trusted credential map 選出 CharacterID；
- Client 只提供 opaque credential，不提交 CharacterID、HP、position 或 durable revision；
- authoritative HP / Transform 由正常 gameplay mutation 產生；
- periodic autosave 將 live state 寫入 durable character store；
- production `worldd` 被 `SIGKILL` 後，新的 process 可從 durable state 恢復；
- fresh Godot Client 用同一 trusted identity reconnect 後，收到的 authoritative vitals / correction / snapshot 與 durable record 一致。

## Server composition

E2E 使用真正的 `cmd/worldd` composition root，而不是 `cmd/e2eserver` 或 mock transport。

```text
Trusted credential
→ worldd trusted identity resolver
→ Server-selected CharacterID
→ WorldRuntime join
→ authoritative movement / combat
→ bounded autosave / durable character store
```

Restart 後：

```text
fresh worldd process
→ same durable state directory / journal / checkpoint
→ same trusted auth map
→ credential resolves same CharacterID
→ durable CharacterRestore load + validation
→ WorldRuntime join transaction
→ Reliable vitals + realtime correction / snapshot
```

## Real-process scenario

Paired Client workflow 啟動兩個 real Godot processes：

- `e3-keeper`
- `e3-damager`

Keeper 透過普通 `ClientMoveInput` 移離 spawn；Damager 透過普通 `ClientUseAction` 對 Keeper 造成 Server-authoritative damage。Client 不注入 damage、HP、position 或 restore state。

Workflow 等待 production periodic autosave 產生 alive keeper durable record，並要求：

- exact Gameplay World provenance = `castle-sandbox` / `s3d-001`；
- durable revision > 0；
- HP < MaxHP 且 HP > 0；
- Z 已離開 spawn（E2E gate 使用 Z >= 2.0）。

該 durable JSON record 是 restart 後的 expected-value oracle。

接著在 Clients 仍連線時對 production `worldd` 送出 `SIGKILL`，避免把 graceful disconnect save 誤當成 crash recovery 證據。

新的 `worldd` process 使用同一 durable storage / auth map 啟動，新的 Godot keeper 再以相同 opaque credential 連線。E2E 要求 authoritative `EntityVitalsState`、`PositionCorrection` / `WorldSnapshot` 恢復到 durable HP 與 Z（允許小幅位置 tolerance）。

## Authority boundary

S4-E.3 明確保留：

- CharacterID 由 Server credential map 選定；
- Client 不提交 CharacterID 或 team；
- Client 不提交 HP / MaxHP / damage；
- Client 不提交 restore transform 或 durable revision；
- mutation 只走既有 `ClientMoveInput` / `ClientUseAction`；
- restore expected truth 來自 Server durable record；
- restore 套用仍經 WorldRuntime single-owner join transaction。

## Durability boundary

S4-E.3 證明的是 **已被 periodic autosave durably committed 的 alive character core state** 可跨 process crash/restart 恢復。

它不宣稱：

- 任意 crash 時點都能保存最後一個未 commit tick；
- inventory / currency / equipment / progression 已 durable；
- multi-host distributed writer / lease 已完成；
- formal account login / token lifecycle 已完成。

底層 durable/trusted restore seam 的 transaction 與 validation 細節仍由 S3-F 文件定義，例如：

- `S3F11_DURABLE_CHARACTER_STATE_STORE.md`
- `S3F12_TRUSTED_CHARACTER_STATE_RESTORE_SEAM.md`
- `S3F15_DURABLE_CHARACTER_STATE_SAVE_INTENT_JOURNAL.md`
- `S3F16_BOUNDED_TRUSTED_CHARACTER_AUTOSAVE_SEAM.md`

## Transport boundary at S4-E.3

E.3 當時的 trusted credential preface 是 possession proof，不是 encryption，因此只允許受控 loopback/local path；它不是 Internet-facing authentication boundary。

這個限制在後續階段被明確演進：

- **S4-E.4**：same-process TLS 1.3 trusted ingress + duplicate/takeover policy；
- **S4-E.5**：Protocol v9 public RoutingID + HMAC authenticated realtime；
- **S4-E.6**：connection-generation route/key revocation；
- **S4-E.7**：authenticated same-IP NAT rebind + WAN / long-session readiness。

因此讀取本文件時，不應把 Protocol v8 / plaintext trusted preface 當成 current deployment guidance。

## Validation relationship

Server-side focused tests負責 durable store / restore validation / autosave / transaction correctness；paired Client S4-E.3 production workflow負責 real `worldd` + real Godot 的 process-level proof。

對應 Client 文件：

- `astrahold-client/docs/S4E3_PRODUCTION_WORLDD_CHARACTER_RESTORE_E2E.md`

## Preserved contracts

- Go Server gameplay-authoritative；
- Client intent-only；
- Gameplay World identity exact validation；
- no blocking persistence I/O inside world tick；
- durable restore 不繞過 WorldRuntime ownership；
- snapshot absence 不代表 despawn；
- E.3 沒有引入新的 gameplay authority 或 Client restore truth。

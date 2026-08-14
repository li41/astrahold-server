# S3-F.10 Durable Character Identity Seam

## Scope

S3-F.10 在 S3-F.9 durable Death Outcome Journal 上建立 **Server-owned durable character ownership key**。

目標只有一個：Death Outcome 經歷 session reconnect、World EntityID reuse與 process restart後，仍能明確回答「這筆 durable record 屬於哪個 character」。本階段不實作 account database、login protocol、currency、inventory、durability或 progression。

## Why SessionID / EntityID are not durable identity

目前 development `tcpudp.Server` 的 SessionID 與 EntityID 都由 process-local counter產生；process restart後會重新開始，而且 Client handshake沒有 account/character selector。因此：

- SessionID 是 transport/session identity；
- EntityID 是目前 world incarnation identity；
- IP / realtime token 也不是 character identity；
- 以上都不能被寫入 durable storage後冒充 returning-character ownership。

S3-F.10 新增獨立 `characteridentity.Binding`。

## Character identity contract

`characteridentity.Binding`包含：

- opaque `CharacterID`；
- assurance：`trusted` 或 `ephemeral`。

格式限制為 bounded ASCII identifier；`ephemeral:` prefix保留給 Server-generated identity。

### trusted

`trusted` 只代表此 binding 由 Server-side trusted integration提供。`tcpudp.Config.CharacterIdentityFactory` 是正式 injection seam；未來 authenticated account/character resolver可以在完成驗證後回傳 trusted binding。

Client protocol沒有 CharacterID欄位，因此 Client不能自行宣稱某個 trusted CharacterID。

S3-F.10 **不提供 authentication實作**；把未驗證的 Client字串直接放進 `NewTrusted`/factory仍是部署錯誤，不會因本階段而自動變安全。

### ephemeral

既有 `session.New`與 default `tcpudp` factory會以 `crypto/rand`產生 128-bit ephemeral identity：

`ephemeral:<32 lowercase hex>`

它可以讓同一 character incarnation的 durable records具有不依賴 EntityID的 ownership key，但它不是 returning-character authentication。下一次未驗證連線會得到新的 ephemeral CharacterID。

## Runtime active binding

WorldRuntime維護：

- `EntityID -> characteridentity.Binding`
- `CharacterID -> active EntityID`

規則：

1. Session join/register前必須有合法 binding；
2. 同一 active CharacterID不能同時綁到兩個 EntityID；
3. 同一 EntityID不能被不同 CharacterID重新解釋；
4. `leave_world`才釋放 world ownership binding；
5. `unregister_session`只移除 connection/session，不清仍存在 world中的 character ownership。

因此 returning character未來可以在舊 entity離開後，以同一 trusted CharacterID綁到新的 SessionID / EntityID。

## Death outcome semantics

Player alive -> defeated時，S3-F.7既有 ordering維持：

`DefeatRevision -> bind respawn -> apply penalty -> enqueue immutable event`

S3-F.10只在 `beginDeathOutcome`快照當下 active CharacterID與assurance，並寫入 `deathoutcome.Event`。

`DefeatRevision`仍是 **world/entity incarnation-local** revision；leave後新的 EntityID incarnation可以重新從1開始。S3-F.10不把它宣稱成 account-global sequence。

Durable consumer的唯一穩定 delivery cursor仍是 S3-F.9 `JournalID + JournalRecordID`。

若內部 invariant破壞而 player沒有 character identity，lethal / respawn / penalty truth不 rollback；strict outbox validation會使 `enqueue_death_outcome`明確失敗並使用既有 failure metric。這和 S3-F.8 failure policy一致。

## Journal record schema v2

S3-F.10把 **record schema**從 v1升到 v2，新增：

- `character_id`
- `character_identity_assurance`

S3-F.9 container/header magic仍維持 `ASTRAHOLD-DEATH-OUTCOME-JOURNAL-V1`；frame length、CRC32C、Journal ID與checkpoint格式皆不變。

因此既有 S3-F.9 journal可原地繼續使用：

- scanner接受 record schema v1與v2；
- v1 record保持 CharacterID/assurance空值，明確視為 legacy identity-less history；
- 不從 EntityID、SessionID、position或其他欄位反推假 identity；
- 新 append一律寫 record schema v2；
- 同一 journal可為 v1 + v2 mixed records，JournalRecordID仍連續。

## Protocol / Client

Protocol維持 v6。

`SessionWelcome`沒有 CharacterID或assurance欄位。Client repo不修改。Character identity在此 slice只屬於 Server ownership / durable history boundary。

## Tests

- `internal/characteridentity/identity_test.go`
  - trusted / ephemeral validation
  - crypto-random ephemeral uniqueness
  - reserved prefix與 malformed identity rejection
- `internal/session/character_identity_test.go`
  - default constructor發 ephemeral identity
  - explicit trusted binding preservation
- `internal/worldruntime/character_identity_test.go`
  - same CharacterID不能同時 active於兩個 Entity
  - leave後可在新 EntityID reuse同一 trusted CharacterID
  - durable event history保留同一 CharacterID而 DefeatRevision依 incarnation重置
  - unregister不釋放 world ownership
- `internal/deathoutcome/journal_identity_test.go`
  - v2 roundtrip / reopen保留 CharacterID
  - legacy v1 read不虛構 identity
  - existing v1 journal可繼續 append v2
- `internal/netadapter/tcpudp/server_test.go`
  - default factory為 ephemeral
  - trusted factory結果到達 Runtime JoinRequest

## Preserved invariants

- Protocol v6。
- Client repo不修改。
- `gameplay.json` / Gameplay World identity不修改。
- S3-F.4～F.9 respawn / resurrection / protection / death penalty / outbox / journal durability semantics不修改。
- lifecycle、Initial Vitals、Dirty Vitals budgets不修改。
- snapshot cadence、input/action sequence、combat cooldown不修改。
- 不修改 S3-E / Siege workflow filters或thresholds。

## Deferred

後續 bounded stage才適合處理：

- authenticated account / character resolver；
- durable character state store與 optimistic revision；
- trusted CharacterID reconnect真正載入相同角色狀態；
- currency/inventory/durability/progression penalty；
- account-level or character-level global sequence；
- journal retention/compaction與跨host durability。

S3-F.10不能被解讀為上述能力已完成。

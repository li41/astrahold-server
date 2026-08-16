# Astrahold Server

Astrahold 是全新設計的 Go authoritative MMORPG Server Core，目標是支援 **3D 王城、多人攻城、可驗證的 Server authority、持久化角色狀態，以及可量測的網路／Replication 擴充**。

> `myriad-throne-server` 只作為經驗參考；Astrahold 不沿用舊 Lineage protocol、2D `gx/gy` 世界模型或舊私服相容包袱。

## 目前狀態

Server runtime 主線已完成到 **S4-F.16 — TLS Certificate Rotation / Runtime Reload**；paired Godot Client 維持 **S4-F.11 — Client Recovery UX / Provider-Neutral Reset Flow**。

目前 production vertical slice 已具備：

- XYZ + Layer authoritative 3D world model
- Server authoritative movement、Navigation、LOS、AOI、Combat、Siege
- Reliable Spawn / Despawn、Vitals、Dynamic World State、Siege Match State
- Network LOD、per-session transform cap 與 lifecycle work budget
- Durable Character state / castle ownership recovery
- Trusted TLS 1.3 game ingress、duplicate-session fence、Server-authorized takeover
- Formal TLS 1.3 login control plane與短生命週期 opaque issued session credential
- Protocol v9 authenticated realtime UDP
- connection-generation realtime key rotation / revocation
- authenticated same-IP NAT-like UDP source-port migration
- WAN-like latency / jitter / loss / burst-loss / reorder / duplication recovery
- Account-authentication provider seam
- schema-v1 high-entropy SHA-256 compatibility account map
- schema-v2 Argon2id human-password reference provider
- schema-v3 durable account lifecycle compatibility
- **schema-v4 durable account store + recovery generation contract**
- `accountctl` account create / password rotation / disable / enable lifecycle
- `accountctl migrate / issue-recovery / reset-password / rehash-password`
- schema-v3 / v4 `SIGHUP` account reload with live issued-bearer revocation
- stale pre-reload password proof cannot mint a post-reload bearer
- bounded source-IP login throttling before password KDF work
- F.8 normal Godot `Main.tscn` product login / reauthentication integration
- F.9 operator recovery reset → live retirement → product reauthentication E2E
- **F.10 pluggable verified-recovery provider seam + TLS public recovery request/reset exchange**
- F.10 known/unknown request同 status / field set，沒有 explicit account-existence欄位
- F.10 successful public reset在同一 account/session mutation fence內立即退休舊 bearer/live peer，**不需要 SIGHUP**
- **F.11 normal Godot `Main.tscn` provider-neutral forgot-password / proof / new-password UX**
- F.11 Client只保存opaque recovery challenge於process memory；不顯示、不持久化、不寫log
- F.11 successful reset回normal login並要求fresh password submit；不自動replay replacement password
- F.11 production gate同時驗證generic unknown request、generic `invalid_recovery`、`recovery_throttled + Retry-After`與13/13 Client workflows
- **F.12 provider-neutral `DeliveryAdapter` seam + schema-v2 delivered recovery reference provider**
- F.12 destination只由Server/provider config決定；public request仍只送`login_id`且不回destination/account existence
- F.12 filesystem reference adapter以0600 proof handoff完成真實F.11 `Main.tscn` reset/fresh-login E2E；delivery failure維持generic accepted public shape但challenge不可兌換
- **F.13 vendor-isolated `https-json-v1` recovery delivery adapter + operational hardening**
- F.13 relay要求HTTPS/TLS 1.3、禁止redirect，Bearer credential由獨立owner-only file載入；raw public `request_id`不送relay
- F.13同一delivery以derived idempotency key做bounded transient retry，分類2xx / transient / permanent outcome並只輸出secret-safe observability metadata
- **F.14 schema-v2 recovery provider / relay credential / private CA `SIGHUP` runtime generation reload**
- F.14 reload完整等待old-generation in-flight `Begin`/delivery完成後再cutover；pre-cutover challenge仍路由回原verifier，new `Begin`只使用新generation
- F.14 invalid replacement保留last-known-good；retired provider立即清除proof key與HTTPS Bearer credential，最多保留4個仍有challenge的retired generations
- **F.15 optional bounded durable recovery delivery outbox + challenge restart recovery**
- F.15 pending HTTPS delivery先以0700 outbox root / 0600 atomic record落盤，再由單一worker以F.13 stable delivery/idempotency identity送出；transient delivery可跨`worldd` restart重播
- F.15 successful/terminal delivery會將raw proof與destination從durable record原子scrub；unexpired verifier/account-generation/attempt state可跨restart恢復，successful reset consume後durable record刪除並directory fsync
- **F.16 session-login / trusted game-ingress TLS certificate/key `SIGHUP` runtime generation reload**
- F.16 replacement keypair完整load、key-match、X.509 validity與server-auth EKU驗證後才publish；invalid replacement保留listener各自的last-known-good generation
- F.16 已建立的TLS connection維持原negotiated state；cutover後新handshake才使用新certificate generation，TLS最低版本仍為1.3

核心 production contract：

```text
Account proof / Recovery proof
    │
    └── HTTPS / TLS 1.3
          ↓
    Account Auth / Recovery Provider
          ↓
Server-owned account generation / CharacterID / takeover grant
          ↓
opaque short-lived issued session credential
          ↓
TLS 1.3 ASTRAH1 game bootstrap
          ↓
Session / Ownership Fence
          ↓
Single-owner World Loop
          ↓
Movement / Combat / Siege
          ↓
Reliable state + Protocol v9 realtime UDP
```

## 核心不變量

Astrahold 的 Server authority 不因 Client、Transport、Account UI 或 Presentation 需求而放寬：

1. **Go Server authoritative**：Client 只送 intent，不送 position truth、damage、HP、team、winner 或 castle ownership。
2. **單一 world owner**：Network / DB / GM 不直接修改 mutable world state，只能提交 bounded command。
3. **World tick 不做 blocking I/O**：outbound 只能進非阻塞 connection / outbox。
4. **Snapshot absence != despawn**：lifecycle 只由 Reliable Spawn / Despawn 管理。
5. **Spawn / Despawn confirm only after `TrySend` success**。
6. **Self correction authoritative**：Client prediction 必須收斂到 `PositionCorrection`。
7. **Gameplay Proxy 是真相**：Visual Mesh 不得反過來成為 Navigation / LOS / Gate authority。
8. **Account proof / recovery proof != gameplay authority**：登入或 recovery 成功不代表 Client 可以指定 CharacterID、takeover、team、HP 或 ownership。
9. **Measure → Profile → Optimize**：沒有數據前不拆 world actor、不做激進 quantization / delta compression。

## Gameplay World

目前 production world：

```text
schema_version = 2
world_id       = castle-sandbox
revision       = s3d-001
main gate id   = main-gate
main gate HP   = 1000
```

權威位置：

```go
type Position struct {
    X     float32
    Y     float32
    Z     float32
    Layer LayerID
}
```

`worlds/castle-sandbox/gameplay.json` 是 Gameplay World authoritative proxy。Surface、Portal、Blocker、Gate 與 agent 規格由 Server strict validate；Visual geometry 只負責呈現。

## Movement / Combat / Siege

Movement：

```text
ClientMoveInput
→ Envelope.Sequence
→ fenced Session ingress
→ bounded Command Queue
→ fixed 20 Hz World loop
→ Navigation / collision
→ authoritative XYZ + Layer
→ WorldSnapshot + PositionCorrection
```

Combat Client intent 只有：

```text
ClientUseAction
├── action_id
├── target_kind
└── target_id
```

Damage、range、cooldown、HP、Defeated 與 hit result 皆由 Server 決定。

Siege authoritative flow：

```text
Round start
→ Attacker / Defender role
→ main-gate HP 1000
→ Gate breach / blocker disabled
→ Throne capture / contest
→ Completed
→ winner + castle owner
→ durable ownership
→ next-round role rotation
```

`SiegeMatchState` 是 Reliable full state；Client 只保存與呈現，不自行推進 phase、winner 或 castle ownership。

## Protocol v9 / Transport

目前 wire contract 為 **Protocol v9**。

### Reliable Ordered / TCP

主要承載：

- `SessionWelcome`
- `EntitySpawn` / `EntityDespawn`
- `WorldDynamicState`
- `EntityVitalsState`
- `ClientUseAction`
- `SiegeMatchState`
- 其他低頻且不可遺失的 authoritative state

### Realtime Sequenced / UDP

承載：

- `ClientMoveInput`
- `WorldSnapshot`
- `PositionCorrection`

Realtime payload 使用 GameV1 compact binary；UDP MTU 上限維持 **1200 bytes**，`WorldSnapshot` 每 chunk 最多 **43 entities**。

### Authenticated ASTU

Protocol v9 不把 bearer realtime secret 放在 UDP packet 上：

```text
Session realtime secret
       │
       ├── one-way public RoutingID
       │
       └── HMAC-SHA256
             └── truncated 128-bit auth tag
```

- ASTU header 只帶 public per-session RoutingID。
- C2S / S2C 使用不同 HMAC domain。
- Server 先以 route 找 peer，再驗 HMAC，通過後才信任 frame / sequence / endpoint。
- stale / replay movement 同時受 transport sequence gate 與 world-runtime sequence gate 約束。
- UDP **提供 authenticity / integrity，不提供 confidentiality**。

### NAT-like endpoint migration

Server 只有在現有 connection generation 的 packet 已經通過：

```text
live RoutingID
→ valid C2S HMAC
→ valid ClientMoveInput
→ strictly newer realtime sequence
→ same source IP
```

之後才更新觀察到的 UDP source port。Unauthenticated、tampered、stale / replay、retired generation 或 cross-IP packet 都不能搶走 S2C route。

## Trusted Identity / Login / Account Lifecycle

Production `worldd` 可選擇互斥的 trusted credential source：

1. `-trusted-character-auth-file` static trusted credential mode；或
2. `-session-login-account-file` issued-session login mode。

兩種模式最後都進入既有 `sessioncredential.Provider`、ASTRAH1 trusted game admission 與 Session / Ownership Fence。

Issued-session flow：

```text
login_id + login_secret
→ HTTPS / TLS 1.3
→ Server account-authentication provider
→ Server-owned CharacterID / takeover grant
→ 32-byte CSPRNG opaque session credential
→ TLS 1.3 trusted game ingress
→ ASTRAH1 credential preface
→ Session Credential Provider
→ authoritative Character session
```

### Account schema compatibility

```text
schema v1
└── login_secret_sha256
    └── high-entropy machine/bootstrap secret only

schema v2
└── password_argon2id PHC verifier
    └── restart-only reference human-password provider

schema v3
└── durable account lifecycle compatibility
    ├── stable account_id
    ├── login_id / password_argon2id
    ├── credential_version
    ├── lifecycle timestamps
    ├── Server-owned character_id
    └── optional allow_active_takeover

schema v4
└── current durable account contract
    ├── all schema-v3 account fields
    └── recovery_grants
        ├── recovery_id
        ├── account_id
        ├── credential_version binding
        ├── token_sha256 only
        └── issued_at / not_before / expires_at
```

Schema v1/v2 保留 compatibility；schema v3 仍可讀取與 reload。**Schema v4 是目前 durable recovery contract**，而 schema v3 不會 silent accept `recovery_grants`。

### Argon2id policy

目前 Argon2id verifier envelope：

```text
version       v=19
memory        64..128 MiB
passes        3..10
parallelism   1..8
salt          16..64 bytes
digest        32 bytes
```

`accountctl` 新增／輪替／recovery reset密碼預設使用：

```text
m = 65536 KiB
t = 3
p = 4
random salt = 16 bytes
digest = 32 bytes
```

Unknown `login_id` 仍執行同成本 dummy Argon2id derivation後回相同 `invalid_credentials` shape；單一 `worldd` 最多同時執行四個 password KDF，以限制 memory pressure。

**Running durable login仍要求所有帳號使用同一 Argon2 cost policy。** `rehash-password` 的 multi-account migration 必須在 maintenance / 未 publish partial state的情況下先收斂所有帳號，再一次 start / SIGHUP。若 partial mixed-cost store 被送給 `worldd`，reload fail-closed並保留 last-known-good snapshot。

### Durable account operations

`cmd/accountctl` 提供 bounded operator surface：

```bash
accountctl init
accountctl migrate
accountctl create
accountctl set-password
accountctl issue-recovery
accountctl reset-password
accountctl rehash-password
accountctl disable
accountctl enable
```

Password 只從 `-password-stdin` 讀取，不要求把 plaintext password 放入 command-line argument。

Durable store write：

```text
validate
→ temp file in target directory
→ chmod 0600
→ write + fsync
→ atomic rename
→ chmod 0600
→ directory fsync
```

Store 使用 monotonic revision；mutation 依 expected revision fail-closed。這仍是 **single-writer durable JSON backend**，不宣稱 multi-host distributed transaction / consensus。

### F.9 operator recovery / password reset

F.9 recovery proof由 operator / verified out-of-band process發行：

```text
accountctl issue-recovery
→ 32-byte OS CSPRNG bearer proof
→ new exclusive 0600 handoff file
→ durable store只保存 SHA-256(token)
→ proof綁定發行當下 credential_version
```

Recovery grant使用 UTC RFC3339時間窗，規則為：

```text
issued_at <= not_before < expires_at
maximum lifetime = 24h
exact expires_at = rejected
```

同帳號新 proof會 supersede舊 outstanding grant。發 proof只推進 store revision，**不推進 `credential_version`**，因此單純發 recovery proof不應踢掉 live session。

Password reset：

```text
read recovery token from file
→ SHA-256 lookup
→ require not_before <= now < expires_at
→ require current credential_version == grant credential_version
→ hash new password with current Argon2id default
→ credential_version++
→ password_changed_at update
→ consume all account recovery grants
→ store revision++
→ durable SaveIfRevision
```

Password變更與 recovery proof consumption在同一 durable update完成，因此 successful proof是一次性。任何 password rotation、disable/enable、reset或 KDF rehash只要先改變 credential generation，舊 recovery proof即使尚未到期也會 stale而 fail-closed。

Reset disabled account不會順便 enable；enable/disable仍是獨立 Server lifecycle authority。

### F.10 verified recovery provider / public reset exchange

F.10 在既有 TLS 1.3 login control plane加入：

```text
POST /v1/account/recovery/request
POST /v1/account/recovery/reset
```

`accountrecovery.Provider` 負責 challenge與proof verification。內建 F.10 compatibility reference provider為：

```text
sha256-high-entropy-recovery-code
```

Provider file只保存 high-entropy recovery code的 SHA-256 digest；raw code不由 `worldd`持久化。這不是 email/SMS delivery provider，也不適合 human-memorable低熵 recovery phrase。

對 syntactically valid `login_id`，known/unknown/disabled/provider-unconfigured subject使用相同 public request contract：

```text
HTTP 202
{"request_id":"...","expires_at":"..."}
```

Public response不含 account existence、account_id、credential_version、CharacterID或eligibility。F.10保證**沒有 explicit account-existence欄位／status差異**；不宣稱不同 provider、host load或外部 delivery path具有形式化 constant-time網路行為。

Challenge bounds：

```text
default TTL                    10 minutes
default attempts / challenge   5
max active challenges          4096
default recovery POST/IP       10 / minute
exact expires_at               rejected
```

成功 reset會再次驗證 Server-owned `account_id + credential_version`，用目前 online uniform Argon2id policy產生新 verifier，並在既有 issuance/account mutation lock內：

```text
require disk revision == live account revision
→ credential_version++
→ replace password verifier
→ remove legacy F.9 recovery grants
→ durable SaveIfRevision
→ remove issued bearers from old account generation
→ publish reduced transport revocation scopes
→ retire old live peer
→ publish new account authenticator
→ consume provider challenge
```

**這條 public reset成功路徑不需要 SIGHUP。** `Issue` 與 reset共用 mutation lock且會重新驗證 account generation，因此 reset前已完成的舊 password verification不能在 reset commit後偷發 bearer。

若 disk revision已被外部 mutation推進、但 live authenticator尚未 publish該 revision，public reset fail-closed為 unavailable，不覆寫較新的 disk state。Recovery-enabled `worldd`也拒絕 SIGHUP downgrade到 schema v3。

F.9 的 single-writer operational contract仍成立：啟用 public recovery時，running `worldd`應被視為 active account writer；`accountctl` mutation應放在 controlled maintenance/reload procedure，不與 online reset競爭。這不是 multi-writer CAS或distributed DB。

完整 F.10 contract：[`docs/S4F10_VERIFIED_RECOVERY_PUBLIC_RESET.md`](docs/S4F10_VERIFIED_RECOVERY_PUBLIC_RESET.md)。

### F.11 Client recovery acceptance

F.11 不改 Server wire或authority contract；它讓 normal Godot產品 Client正式消費 F.10 public endpoints：

```text
forgot password
→ Client sends login_id only
→ F.10 returns opaque request_id + expires_at
→ Client shows generic provider-proof UI
→ Client submits request_id + recovery_proof + new_password
→ Server validates provider proof / generation / durable CAS
→ successful reset = 204 + immediate old-generation retirement
→ Client discards challenge
→ normal login screen
→ user performs fresh password login
```

Client不接收account existence、AccountID、credential version、CharacterID或takeover authority；opaque `request_id`只存在process memory，不顯示、不持久化、不寫log。Recovery proof與new-password欄位在submit前清空。

F.11 product E2E證明：unknown-account request使用generic accepted UX、wrong proof使用generic `invalid_recovery` UX、correct proof成功reset、舊密碼失效、新密碼完成既有issued-session/game bootstrap、explicit logout成功，且`recovery_throttled + Retry-After`不會被誤顯示為invalid credentials。

F.11 Client exact final head通過13/13 workflows；這個Stage沒有新增GameV1、Protocol v10或任何Client gameplay/account authority。

### F.12 verified recovery delivery adapter

F.12 保留 F.10 public API與 `accountrecovery.Provider` authority boundary，新增只負責transport的 `accountrecovery.DeliveryAdapter`。Client仍不能選destination；destination ownership完全來自Server/provider config。

既有 `-session-recovery-provider-file` 維持單一入口：schema v1仍相容F.10 digest-only recovery code；schema v2加入 delivered reference provider，使用32-byte proof key產生domain-separated HMAC-SHA256 proof並綁定Server-owned `login_id + account_id + credential_version`。

F.12 reference transport為 `filesystem-reference-v1`：inbox root必須是無group/other權限的真實directory，每次delivery只原子發布 `<destination>.proof` 且檔案mode為0600；不把opaque `request_id`寫進handoff file或一般log。這是deterministic / CI-friendly reference adapter，**不是 production email/SMS vendor integration**。

Enumeration-safe failure mapping保持public contract不變：unknown/ineligible account不delivery；per-subject transient/permanent delivery failure仍回同一`202 + request_id + expires_at` shape，但reserved challenge維持non-authorizing。Wrong、expired、inactive、consumed或stale-generation proof都回generic `401 invalid_recovery`；source-IP throttle仍回`429 recovery_throttled + Retry-After`。

F.12 production E2E固定paired Client F.11 main，證明unknown no-delivery、known delivery、forced delivery failure不洩漏existence、delivered proof可由真實normal `Main.tscn`完成reset/fresh-login、舊密碼失效、stale generation/replay fail-closed、throttle與secret/log gate。既有F.10 exact-head E2E同時持續證明successful reset立即退休舊bearer/live peer且不需要SIGHUP。

完整 F.12 contract：[`docs/S4F12_VERIFIED_RECOVERY_DELIVERY.md`](docs/S4F12_VERIFIED_RECOVERY_DELIVERY.md)。

### F.13 production recovery delivery provider

F.13 延伸F.12 schema-v2 delivery config，新增vendor-isolated `https-json-v1` adapter；core Server不依賴任何email/SMS vendor SDK。Relay endpoint必須是absolute HTTPS、TLS最低1.3、禁止redirect，並可選擇額外private CA root。

Relay Bearer credential不放provider JSON，而是從獨立owner-only regular file於process start載入。每次delivery只送`schema_version + delivery_id + destination + proof + expires_at`；raw public `request_id`不送relay。`delivery_id`由domain-separated SHA-256衍生為128-bit opaque id，並同時作為`Idempotency-Key`/correlation key，所有retry保持相同body/key。

Transport error / attempt timeout、HTTP 408/425/429與5xx分類為transient；其他3xx/4xx分類為permanent。單次timeout允許100ms..2s、最多3 attempts、backoff上限500ms。無論per-subject delivery transient/permanent failure，F.12 enumeration-safe mapping仍回generic accepted public shape且challenge保持non-authorizing。

普通Server log只記adapter/revision/outcome/attempt/status class，不記destination、proof、public request ID、derived delivery ID、Bearer credential或response body。F.13 production gate以本機TLS 1.3 fake relay證明400 permanent不retry、503→202使用同一idempotency key/body retry、unknown不delivery、credential與secret不洩漏，並將relay取得的proof直接交給未修改F.11 normal `Main.tscn`完成reset/fresh-login。

完整 F.13 contract：[`docs/S4F13_PRODUCTION_RECOVERY_DELIVERY_PROVIDER.md`](docs/S4F13_PRODUCTION_RECOVERY_DELIVERY_PROVIDER.md)。

### F.14 recovery delivery runtime generation reload

F.14 將schema-v2 delivered provider包成runtime generation router。每次成功`SIGHUP`都發布新的monotonic recovery generation，即使provider文字`revision`不變也可只輪替proof key、relay credential、endpoint或private CA；schema-v1 digest-only compatibility provider仍維持restart-only。

Cutover以generation read/write barrier序列化完整`Begin`：已在舊generation開始的delivery會先完成並註冊opaque challenge route，才允許新generation publish。Cutover後所有新`Begin`只使用新provider；舊challenge仍可在TTL內路由到原challenge verifier，所以不會出現「proof已送達但request_id突然失效」的窗口。

Retired provider不再需要delivery secrets來驗證既有challenge，因此publish新generation時會清除舊HMAC proof key；`https-json-v1`同時清除舊in-memory Bearer credential並關閉idle HTTP connections。Invalid config、credential permissions、CA或schema downgrade一律fail-closed並保留last-known-good。Router最多保留4個仍有challenge的retired provider generations；超過上限時最舊challenge安全失效並沿既有generic `invalid_recovery` path。

Durable account reload與recovery-provider reload在同一`SIGHUP`上獨立驗證／報告；recovery-only credential/CA rotation不要求account file revision前進，而任何account generation變更仍由durable reset writer做最後`account_id + credential_version` re-check。

F.14 production E2E以兩個不同TLS 1.3 fake relays、credential與CA證明relay A→B無restart cutover、old challenge跨reload可兌換、new request只使用B、invalid credential replacement保留generation-2 LKG，並由未修改F.11 normal `Main.tscn`使用generation-2 proof完成reset/fresh-login。Protocol仍v9，schema仍v4。

完整 F.14 contract：[`docs/S4F14_RECOVERY_DELIVERY_RUNTIME_RELOAD.md`](docs/S4F14_RECOVERY_DELIVERY_RUNTIME_RELOAD.md)。

### F.15 durable recovery delivery outbox / restart reliability

F.15 在不改 `accountrecovery.Provider` public authority boundary的前提下，為schema-v2 `https-json-v1`加入optional bounded durable outbox。Durable record同時承擔pending delivery retry state與challenge restart ledger，因為只重播relay payload、卻沒有原challenge verifier，restart後proof仍無法兌換。

Durable enqueue順序是：

```text
reserve process-local challenge
→ durable write pending record
→ activate challenge
→ register F.14 opaque request route
→ publish record to the single outbox worker
```

Worker只有在route已註冊後才送，避免fast permanent delivery outcome與`Begin` activation競態。F.13 adapter仍負責單一transport cycle的bounded attempts；F.15另外對跨時間／跨restart delivery cycles做bounded exponential retry，並沿用同一F.13 `delivery_id` / `Idempotency-Key`。

Outbox storage要求預先存在的owner-only directory，live record為exact `0600` regular file；create/rewrite採temp→file fsync→atomic rename→directory fsync，delete也會directory fsync。Strict JSON、filename/delivery-id、proof/verifier一致性、size、symlink與permission都在startup fail-closed；expired record cold start直接drop。Backpressure或enqueue persist failure仍沿F.12 generic accepted public shape，但reserved challenge維持non-authorizing。

Pending record為了restart replay會暫時保存raw recovery proof與Server-owned destination。成功delivery會原子改寫成不含proof/destination的`delivered` challenge-only record；permanent/exhausted failure則改寫成non-authorizing `failed` record並同樣scrub secret material。這是0700/0600 application permission boundary，**不是application-layer disk encryption**；需要media-at-rest confidentiality的部署應使用encrypted filesystem/volume。

Restart會恢復unexpired verifier、`account_id + credential_version` binding、active state與verification attempt count；pending record由worker用相同idempotency identity replay，delivered record不重送。Restored challenge會seed進F.14 generation-1 route，因此restart後再`SIGHUP`也不會把原request orphan。F.14 reload期間只有一個process-global outbox worker，cutover只交換validated HTTPS transport/provider target並退休old credential；pending record本身保持原proof/destination/expiry/idempotency identity。

Successful public reset仍走既有durable schema-v4 CAS與old-generation bearer/live-peer retirement，最後`Consume`會刪除outbox challenge record並directory fsync，因此proof不會因另一個restart重新出現。

F.15 production E2E證明503 transient→durable pending→`SIGKILL worldd`→same outbox restart→same delivery identity replay→202 delivery→proof/destination scrub→使用restart前原`request_id`完成reset→consume delete→old password 401 / new password 200；schema仍v4，Protocol仍v9。Unit/race coverage另外鎖定backpressure、permanent failure、attempt persistence、expiry/drop、exact 0600、corrupt storage、F.14 transport swap與restart後再reload的old-challenge routing。

完整 F.15 contract：[`docs/S4F15_DURABLE_RECOVERY_DELIVERY_OUTBOX.md`](docs/S4F15_DURABLE_RECOVERY_DELIVERY_OUTBOX.md)。

### F.16 TLS certificate runtime generation reload

F.16 把session-login/public-recovery HTTPS control plane與trusted ASTRAH1 game ingress最後的process-start-only certificate/key限制改成各自獨立的runtime generation。兩個listener都維持TLS 1.3 minimum，且不改任何HTTP/GameV1 public contract。

每個listener持有一個immutable current certificate snapshot；TLS handshake透過`tls.Config.GetCertificate`取得當下generation。SIGHUP會先完整讀取candidate cert/key，再驗證PEM/key match、X.509 leaf、`NotBefore <= now < NotAfter`以及若存在EKU則必須允許serverAuth/any。只有全部通過才publish下一個monotonic generation。

```text
read candidate certificate + key
→ validate complete pair
→ failure: keep last-known-good generation
→ success: atomically publish generation N+1
→ new ClientHello uses N+1
```

已完成或已選定certificate的TLS handshake不會被reload改寫；established TLS connection也不會因certificate generation切換而被踢掉。新的handshake在cutover後才拿到新certificate。Retired key material只是不再可由新handshake取得；Go runtime管理其memory lifetime，F.16不宣稱private-key RAM可deterministic zeroize。

Session-login TLS、durable account snapshot、F.14 recovery provider/relay credential/private CA、trusted ingress TLS都由同一process `SIGHUP`觸發，但仍是**獨立last-known-good domains**，不是跨檔案transaction。一個invalid TLS replacement不會阻止另一個合法runtime reload成功。

F.16 production E2E以real `worldd`證明login與trusted listener都從certificate A→B無restart切換；cutover前建立的TLS 1.3 connection持續可用，cutover後新handshake只看到B；unchanged `/v1/session/login`仍成功；故意放入C certificate + B private key時兩個listener都拒絕reload並持續以generation 2 / certificate B服務。

完整 F.16 contract：[`docs/S4F16_TLS_CERTIFICATE_RUNTIME_RELOAD.md`](docs/S4F16_TLS_CERTIFICATE_RUNTIME_RELOAD.md)。

### KDF migration

```bash
printf '%s\n' 'current password' | accountctl rehash-password \
  -path /secure/accounts.json \
  -login alice \
  -password-stdin \
  -memory-kib 65536 \
  -time 3 \
  -threads 4
```

`rehash-password` 會先驗證現有密碼，再以新 random salt產生目標 policy verifier；若 policy真的改變，會推進 credential generation與 store revision，但刻意保留 `password_changed_at`，因為 human password沒有改變。已在目標 policy時是 no-op。

KDF migration publish後，舊 verifier generation所發出的 bearer會沿既有 account-generation fence退休。

### SIGHUP account / recovery / TLS reload

Schema v3 / v4 account snapshot可在 durable store 更新後對 `worldd` 發 `SIGHUP`；recovery provider啟用時account schema必須維持v4。Schema-v2 delivered recovery provider也可由同一`SIGHUP`建立新的runtime generation；schema-v1 recovery provider維持restart-only。F.16起session-login與trusted game ingress certificate/key也由同一process signal觸發各自獨立的certificate generation reload。

Account reload有效時要求store revision嚴格前進。Recovery provider reload不要求文字revision前進，因為credential/private-CA-only rotation本來就不一定修改該revision。TLS certificate reload只要求candidate完整valid，不要求certificate identity一定改變。這些reload各自validate、各自last-known-good，不形成跨檔案distributed transaction。

Account安全順序：

```text
load + validate new account snapshot
→ serialize with issuance mutation lock
→ clone current issued-bearer provider
→ prune expired bearers
→ remove bearers whose account generation is no longer active
→ publish reduced transport revocation-scope allow-set
→ retire affected live peers
→ publish reduced bearer provider
→ publish replacement account authenticator
```

Recovery provider安全順序：

```text
load + fully validate replacement schema-v2 provider / credential / CA
→ wait for old-generation Begin + delivery completion
→ register old challenge route
→ publish new generation
→ clear old proof key / relay credential
→ keep bounded old verifier routes until consume / expiry / generation cap
```

TLS certificate安全順序：

```text
load candidate certificate + private key
→ validate key match / leaf validity / server-auth usage
→ publish immutable generation
→ existing TLS connections unchanged
→ new handshakes resolve new generation
```

F.15 outbox啟用時，recovery generation reload不建立第二個worker，而是把同一process-global outbox背後的HTTPS transport/provider target在F.14 barrier內替換；pending records仍保留原delivery identity。Cold restart恢復的challenge先seed成generation-1 routes，再參與後續F.14 cutover。

因此 password rotation、account disable、password reset、KDF verifier generation或其他 account proof-generation 變更可讓舊 issued bearer 與 live game session立即失效；recovery credential/CA rotation與TLS certificate rotation則不會把已建立且仍合法的game/login session無條件作廢。

Argon2id verification刻意在 issuance lock外執行；`Issue` 會在同一 serialization boundary重新檢查 `AuthenticationSubject + AuthenticationGeneration`。即使舊 password verification在 reload/reset前已完成，只要新的 account generation已 commit，stale grant就不能再 mint bearer。

F.9 production recovery E2E證明 operator流程；F.10 production public recovery E2E證明 no-SIGHUP public reset；F.11由normal Client產品UX直接覆蓋public request/reset/fresh-login/throttle；F.12把proof取得路徑接到Server-owned delivery adapter；F.13把delivery transport收斂成可部署HTTPS relay、bounded retry/idempotency與secret-safe observability；F.14加入provider/credential/CA fail-closed runtime generation reload；F.15把delivery/challenge可靠性推進到single-host durable restart recovery；F.16再把login/game TLS certificate/key推進到fail-closed runtime generation。Recovery proof、password、opaque request metadata、delivery credential與issued bearer持續有log-leak fail-fast檢查。

### Login / recovery abuse control

Login listener 在 JSON decode / password KDF 前套用 bounded fixed-window source-IP guard。

Login預設：

```text
30 login POST attempts / 1 minute / observed source IP
max tracked source entries = 4096
```

Recovery另有獨立 source-IP attempt guard與per-challenge attempt/TTL bounds。

來源身份目前只相信 TLS socket 的實際 `RemoteAddr`；**不信任 `X-Forwarded-For`**。F.17預計在明確allowlist trusted reverse proxy的前提下加入proxy-aware attribution；未被信任的socket peer仍必須忽略所有forwarding metadata，而不能讓任意header繞過來源throttle。

### Issued-session lifecycle

- login request 只接受 `login_id` + `login_secret`；Client 不送 CharacterID / takeover bit。
- successful login response 只包含 `session_credential` + `expires_at`。
- 每個 bearer 使用 32-byte `crypto/rand` entropy；Server provider map只保存 digest。
- TTL 預設 15 分鐘，可設定 1 分鐘到 24 小時。
- `POST /v1/session/logout` 會撤銷 bearer；known / unknown well-formed bearer都回 204。
- logout / expiry / account-generation revocation 都沿既有 F.3 transport fence退休 live peer。
- bearer 仍是 process-local；`worldd` restart 後要求重新 login。
- `allow_active_takeover` 只由 Server account / credential grant提供；Client沒有 takeover bit。

完整身份／登入文件：

- [`docs/S4F16_TLS_CERTIFICATE_RUNTIME_RELOAD.md`](docs/S4F16_TLS_CERTIFICATE_RUNTIME_RELOAD.md)
- [`docs/S4F15_DURABLE_RECOVERY_DELIVERY_OUTBOX.md`](docs/S4F15_DURABLE_RECOVERY_DELIVERY_OUTBOX.md)
- [`docs/S4F14_RECOVERY_DELIVERY_RUNTIME_RELOAD.md`](docs/S4F14_RECOVERY_DELIVERY_RUNTIME_RELOAD.md)
- [`docs/S4F13_PRODUCTION_RECOVERY_DELIVERY_PROVIDER.md`](docs/S4F13_PRODUCTION_RECOVERY_DELIVERY_PROVIDER.md)
- [`docs/S4F12_VERIFIED_RECOVERY_DELIVERY.md`](docs/S4F12_VERIFIED_RECOVERY_DELIVERY.md)
- [`docs/S4F10_VERIFIED_RECOVERY_PUBLIC_RESET.md`](docs/S4F10_VERIFIED_RECOVERY_PUBLIC_RESET.md)
- [`docs/S4F9_ACCOUNT_RECOVERY_KDF_MIGRATION.md`](docs/S4F9_ACCOUNT_RECOVERY_KDF_MIGRATION.md)
- [`docs/S4F7_DURABLE_ACCOUNT_LIFECYCLE.md`](docs/S4F7_DURABLE_ACCOUNT_LIFECYCLE.md)
- [`docs/S4F6_ACCOUNT_AUTH_PROVIDER.md`](docs/S4F6_ACCOUNT_AUTH_PROVIDER.md)
- [`docs/S4F4_FORMAL_LOGIN_SESSION_ISSUANCE.md`](docs/S4F4_FORMAL_LOGIN_SESSION_ISSUANCE.md)
- [`docs/S4F3_RUNTIME_CREDENTIAL_REVOCATION.md`](docs/S4F3_RUNTIME_CREDENTIAL_REVOCATION.md)
- [`docs/S4F2_CREDENTIAL_LIFECYCLE.md`](docs/S4F2_CREDENTIAL_LIFECYCLE.md)
- [`docs/S4F1_SESSION_CREDENTIAL_PROVIDER_SEAM.md`](docs/S4F1_SESSION_CREDENTIAL_PROVIDER_SEAM.md)
- [`docs/S4E4_SECURE_TRUSTED_INGRESS_TAKEOVER.md`](docs/S4E4_SECURE_TRUSTED_INGRESS_TAKEOVER.md)

## Replication / Scaling

目前重要 scaling gates：

```text
UDP MTU                         1200 bytes
Snapshot entities / chunk      43
Per-session transform cap      64
Lifecycle churn max            6000 / snapshot
Dirty Vitals max               4000 / tick
```

S3-E 已包含 Network LOD / tier cadence、shared AOI work、encode/buffer ownership、lifecycle convergence與 churn budget。24-client Vertical Siege、100-client Gate Zerg持續作為 correctness / performance gate；500-client soak用於較重 scaling evidence。

## Production E2E Matrix

| Stage | 驗證內容 | 狀態 |
|---|---|---|
| S4-E.1 | 真實 Go Server + 2 Godot Clients；movement / PvP / death / respawn / gate / throne / result | ✅ |
| S4-E.2 | Server SIGKILL + restart + fresh Clients；durable ownership / round recovery | ✅ |
| S4-E.3 | Production `worldd` trusted identity；HP / position autosave + crash restore | ✅ |
| S4-E.4 | TLS 1.3 ingress；duplicate fail-closed；authorized takeover / cooldown | ✅ |
| S4-E.5 | UDP HMAC、tamper / replay rejection、loss / reorder / duplicate impairment recovery | ✅ |
| S4-E.6 | Connection-generation realtime route rotation + old-generation revocation | ✅ |
| S4-E.7 | NAT-like endpoint migration、spoof protection、WAN impairment、long-session health | ✅ |
| S4-F.5 | TLS login → issued bearer；duplicate/takeover；logout/expiry live retirement | ✅ |
| S4-F.6 | Schema-v2 Argon2id human-password provider + real-Godot login / takeover / logout | ✅ |
| S4-F.7 | Schema-v3 durable store；password rotation / disable live revocation；enable/relogin；source-IP throttle | ✅ |
| S4-F.8 | Normal Godot `Main.tscn` login / live-revocation reauth / logout / 401 / 429 UX | ✅ |
| S4-F.9 | Schema-v4 operator recovery；digest-only proof；issue不踢人；one-time reset後live retirement | ✅ |
| S4-F.10 | Provider seam + public recovery request/reset；same known/unknown response schema；no-SIGHUP immediate retirement；normal Main reauth | ✅ |
| S4-F.11 | Normal Godot Client recovery UX；generic unknown/wrong-proof；reset→fresh login；recovery throttle；secret/challenge no-persistence | ✅ |
| S4-F.12 | DeliveryAdapter seam；Server-owned destination；filesystem reference delivery；enumeration-safe failure；F.11 Main delivered-proof reset/fresh-login | ✅ |
| S4-F.13 | HTTPS/TLS 1.3 relay；owner-only credential；stable idempotent retry；outcome classification；secret-safe logs；F.11 Main relay-proof reset/fresh-login | ✅ |
| S4-F.14 | schema-v2 provider/credential/CA SIGHUP generation reload；in-flight cutover fence；old challenge routing；invalid reload LKG；F.11 Main post-rotation reset/fresh-login | ✅ |
| S4-F.15 | bounded durable HTTPS recovery outbox；0700/0600 atomic storage；503→SIGKILL→restart same-id replay；challenge restore；proof/destination scrub；original request reset；consume delete | ✅ |
| **S4-F.16** | **login + trusted-ingress TLS certificate/key SIGHUP generation reload；A→B cutover；old TLS connection survives；new handshake uses B；mismatched replacement LKG** | **✅** |

Server runtime contract現在是F.16；paired Client runtime仍是F.11。F.16 final exact product head `d90d819f7f3065a449e88cd08a4ef4ef23476bb3`通過8/8 workflows：Server CI、Production Account Recovery E2E、Production Public Recovery E2E、Production Recovery Delivery E2E、Production Recovery Delivery Provider E2E、Production Recovery Delivery Reload E2E、Production Recovery Delivery Outbox E2E與Production TLS Certificate Reload E2E；Protocol仍v9，Client product code未為F.16增加任何account/game/certificate authority。

## 文件入口

### Current production contract

- 本 `README.md` — current Server / Protocol v9 / account lifecycle / recovery / TLS lifecycle / known limitations
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — Current Architecture Baseline
- [`docs/S4F16_TLS_CERTIFICATE_RUNTIME_RELOAD.md`](docs/S4F16_TLS_CERTIFICATE_RUNTIME_RELOAD.md) — login/game TLS certificate generations / validation / established-connection / last-known-good contract
- [`docs/S4F15_DURABLE_RECOVERY_DELIVERY_OUTBOX.md`](docs/S4F15_DURABLE_RECOVERY_DELIVERY_OUTBOX.md) — single-host durable delivery/challenge restart recovery / storage / retry / scrub contract
- [`docs/S4F14_RECOVERY_DELIVERY_RUNTIME_RELOAD.md`](docs/S4F14_RECOVERY_DELIVERY_RUNTIME_RELOAD.md) — recovery provider / credential / CA runtime generations / cutover / last-known-good contract
- [`docs/S4F13_PRODUCTION_RECOVERY_DELIVERY_PROVIDER.md`](docs/S4F13_PRODUCTION_RECOVERY_DELIVERY_PROVIDER.md) — vendor-isolated HTTPS relay / credential / retry / idempotency / observability contract
- [`docs/S4F12_VERIFIED_RECOVERY_DELIVERY.md`](docs/S4F12_VERIFIED_RECOVERY_DELIVERY.md) — provider-neutral verified recovery delivery adapter / reference transport / failure mapping
- [`docs/S4F10_VERIFIED_RECOVERY_PUBLIC_RESET.md`](docs/S4F10_VERIFIED_RECOVERY_PUBLIC_RESET.md) — verified recovery provider / public reset / immediate revocation
- Client `docs/S4F11_CLIENT_RECOVERY_UX.md` — provider-neutral normal Client recovery / fresh-login / secret-handling acceptance
- [`docs/S4F9_ACCOUNT_RECOVERY_KDF_MIGRATION.md`](docs/S4F9_ACCOUNT_RECOVERY_KDF_MIGRATION.md) — schema-v4 operator recovery / one-time reset / KDF migration
- [`docs/S4F7_DURABLE_ACCOUNT_LIFECYCLE.md`](docs/S4F7_DURABLE_ACCOUNT_LIFECYCLE.md) — durable schema-v3 lifecycle / SIGHUP / abuse control
- [`docs/S4F6_ACCOUNT_AUTH_PROVIDER.md`](docs/S4F6_ACCOUNT_AUTH_PROVIDER.md) — account-auth provider seam / Argon2id reference backend
- [`docs/S4F4_FORMAL_LOGIN_SESSION_ISSUANCE.md`](docs/S4F4_FORMAL_LOGIN_SESSION_ISSUANCE.md) — TLS login / opaque issuance / logout / expiry
- [`docs/S4F3_RUNTIME_CREDENTIAL_REVOCATION.md`](docs/S4F3_RUNTIME_CREDENTIAL_REVOCATION.md) — runtime credential revocation
- [`docs/S4E5_AUTHENTICATED_REALTIME_BINDING.md`](docs/S4E5_AUTHENTICATED_REALTIME_BINDING.md) — Protocol v9 authenticated ASTU
- [`docs/S4E6_REALTIME_KEY_LIFECYCLE.md`](docs/S4E6_REALTIME_KEY_LIFECYCLE.md) — realtime generation rotation / revocation
- [`docs/S4E7_WAN_NAT_SECURE_DEPLOYMENT_READINESS.md`](docs/S4E7_WAN_NAT_SECURE_DEPLOYMENT_READINESS.md) — NAT / WAN readiness

### Production E2E / durability evidence

- [`docs/S4E1_TRUE_GO_GODOT_E2E_SERVER.md`](docs/S4E1_TRUE_GO_GODOT_E2E_SERVER.md)
- [`docs/S4E2_RESTART_RECOVERY_HARNESS.md`](docs/S4E2_RESTART_RECOVERY_HARNESS.md)
- [`docs/S4E3_PRODUCTION_WORLDD_CHARACTER_RESTORE_E2E_SERVER.md`](docs/S4E3_PRODUCTION_WORLDD_CHARACTER_RESTORE_E2E_SERVER.md)
- [`docs/S4D4A_SIEGE_RESULT_CONTRACT.md`](docs/S4D4A_SIEGE_RESULT_CONTRACT.md)

### Historical milestone documents

`S1*` / `S2*` / `S3*` / early `S4*` 文件刻意保留該 milestone 當時的 protocol、schema、transport與量測結果；不要把舊 Protocol / raw bearer / Gameplay World schema v1 / Full AOI內容當成目前 production contract。

常用歷史文件：

- [`docs/S2_PROTOCOL.md`](docs/S2_PROTOCOL.md)
- [`docs/S2B_TRANSPORT.md`](docs/S2B_TRANSPORT.md)
- [`docs/S3_GAMEPLAY_WORLD.md`](docs/S3_GAMEPLAY_WORLD.md)
- [`docs/S3C6_REALTIME_REPLICATION.md`](docs/S3C6_REALTIME_REPLICATION.md)
- [`docs/S3E1_REPLICATION_TIER_CADENCE.md`](docs/S3E1_REPLICATION_TIER_CADENCE.md)
- [`docs/S3E6_LIFECYCLE_WORK_BUDGET.md`](docs/S3E6_LIFECYCLE_WORK_BUDGET.md)
- [`docs/S3E9_LONG_MIXED_GAMEPLAY_SOAK.md`](docs/S3E9_LONG_MIXED_GAMEPLAY_SOAK.md)

## 開發 / Deployment

一般驗證：

```bash
go test ./...
go vet ./...
go run ./cmd/worldd
```

Account lifecycle / recovery tool：

```bash
go run ./cmd/accountctl --help
```

預設 development transport：

```text
TCP             127.0.0.1:7777
UDP             127.0.0.1:7778
World Tick      20 Hz
Snapshot        10 Hz
Protocol        v9
Gameplay World  castle-sandbox@s3d-001
```

Production static trusted deployment：

```text
-trusted-character-auth-file
-trusted-tls-listen
-trusted-tls-cert
-trusted-tls-key
```

Production issued-session deployment：

```text
-session-login-account-file
-session-login-tls-listen
-session-login-tls-cert
-session-login-tls-key
-session-credential-ttl
-session-login-ip-attempt-window
-session-login-ip-max-attempts
-session-recovery-provider-file
-session-recovery-challenge-ttl
-session-recovery-challenge-max-attempts
-session-recovery-ip-attempt-window
-session-recovery-ip-max-attempts
-session-recovery-outbox-dir
-session-recovery-outbox-max-records
-session-recovery-outbox-max-delivery-attempts
-session-recovery-outbox-retry-min
-session-recovery-outbox-retry-max
-trusted-tls-listen
-trusted-tls-cert
-trusted-tls-key
```

Static trusted mode 與 issued-session mode互斥。Login/recovery control plane與 trusted game ingress都要求 TLS 1.3。Realtime UDP仍是 Protocol v9 authenticated plaintext datagram。

Static trusted credential schema-v2可用原有 SIGHUP runtime reload；issued-session account schema-v1/v2為 restart-only compatibility，durable schema-v3/v4支援 SIGHUP account-generation reload。Public recovery provider啟用時要求 durable schema v4。Schema-v2 recovery provider可選F.12 `filesystem-reference-v1`或F.13 `https-json-v1`，並自F.14起支援`SIGHUP` provider generation reload；HTTPS relay credential/private CA/endpoint可隨generation輪替。F.15 `https-json-v1`可另外啟用single-host durable outbox，outbox root需由部署預先建立為owner-only directory，F.14 cutover會在同一shared worker後方替換validated relay transport。Schema-v1 recovery provider仍restart-only。F.16起session-login與trusted game ingress的certificate/key也支援`SIGHUP` fail-closed runtime generation reload；兩個listener各自保留LKG，既有TLS connection不因certificate rotation被強制斷線。

## 目前刻意保留的限制

- Realtime UDP尚未加密；目前只有 authenticity / integrity。
- Schema-v4仍是 **single-writer durable JSON account backend**；public recovery啟用時running `worldd`是 active writer，尚未有 distributed account DB或 multi-writer recovery CAS。
- F.15 已提供bounded single-host durable recovery delivery/challenge outbox、restart replay與stable F.13 idempotency identity；這不是distributed broker、multi-host consensus、cross-host recovery ownership或exactly-once vendor delivery。
- F.15 pending record為了restart replay會短暫以plaintext保存recovery proof與Server-owned destination；application只提供owner-only 0700/0600 permission boundary與terminal scrub，**不提供application-layer disk encryption**。需要media-at-rest confidentiality時應使用encrypted filesystem/volume。
- F.14 provider/credential/private-CA runtime generation reload、in-flight cutover fence與last-known-good仍適用；F.15只有一個shared outbox worker，pending records會跨transport generation保持原delivery identity。
- F.16 已有login/game TLS certificate/key runtime generation reload；不包含Client trust-store/CA hot reload、ACME/PKI自動化、OCSP lifecycle、mTLS identity或multi-host certificate atomic cutover。Retired private key的RAM lifetime由Go runtime管理，不宣稱deterministic zeroization。
- Core Server刻意不綁特定email/SMS vendor SDK；實際vendor仍應位於F.13 HTTPS relay後方。
- 尚未加入 breached-password corpus、MFA / TOTP / WebAuthn / passkeys / OIDC external IdP adapter。
- Login/recovery都有 direct-listener source-IP fixed-window throttling，但尚未有 trusted reverse-proxy attribution、distributed rate limit、IP reputation、credential-stuffing intelligence或 CAPTCHA。
- Issued session credential仍為 process-local short-lived proof；Server restart強制重新 login，尚無 refresh token、remembered-device session、durable bearer recovery或 cross-server revocation propagation。
- F.11 Client刻意不持久化human password、recovery proof或opaque recovery challenge；尚未加入 OS keychain / secure remembered-session storage，因目前沒有refresh token/remember-session contract。
- NAT-like migration目前只允許 same-IP UDP source-port change；跨 IP migration fail-closed。
- 尚未做 multi-server distributed ownership / failover。
- 尚未加入 periodic in-session realtime rekey、DTLS或 QUIC；目前 evidence沒有證明 MVP需要它們。
- 500 rendered Godot actors、VAT / MultiMesh與 final commercial art不是目前 Server MVP gate。

## 下一個 bounded focus

S4-F.16 已把兩個Server TLS termination point最後的process-start-only憑證限制收斂成fail-closed runtime generation：candidate certificate/key完整validate後才publish，invalid replacement維持各listener LKG；cutover前已建立的TLS state不被強制重建，新handshake才取得新generation。F.9/F.10/F.12/F.13/F.14/F.15 gates與Server Test/Vet/Race也在同一exact head持續全綠。

下一個 bounded stage 建議進入 **S4-F.17 — Trusted Reverse-Proxy Source Attribution / Edge Trust Boundary**：維持public login/recovery API、Client F.11、schema-v4與Protocol v9不變，讓login/recovery source-IP throttling在明確部署為trusted reverse proxy時可安全取得原始client address；只有socket `RemoteAddr`屬於operator allowlist的proxy peer才允許解析bounded forwarding metadata，direct/untrusted peer仍完全忽略`X-Forwarded-For`/`Forwarded`，並明確定義multi-hop選擇、malformed header fail-closed、IPv4/IPv6 normalization與observability。Distributed rate limit、IP reputation、WAF/CDN vendor integration與proxy authentication仍保持獨立decision gate。

Public registration、MFA/WebAuthn/passkeys/OIDC、distributed account DB、refresh-token / remember-session、ACME/PKI automation與Protocol v10仍保持獨立 decision gate。

Astrahold 的原則保持不變：**Server State 是真相；先證明 correctness，再用量測決定複雜度。**

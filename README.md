# Astrahold Server

Astrahold 是全新設計的 Go authoritative MMORPG Server Core，目標是支援 **3D 王城、多人攻城、可驗證的 Server authority、持久化角色狀態，以及可量測的網路／Replication 擴充**。

> `myriad-throne-server` 只作為經驗參考；Astrahold 不沿用舊 Lineage protocol、2D `gx/gy` 世界模型或舊私服相容包袱。

## 目前狀態

Server runtime 主線已完成到 **S4-F.27 — Rollout Observation Evidence / Convergence Timing**；paired Godot Client 維持 **S4-F.11 — Client Recovery UX / Provider-Neutral Reset Flow**。

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
- **F.17 optional trusted reverse-proxy source attribution for login/recovery abuse control**
- F.17 direct/untrusted TLS socket peer仍只以實際`RemoteAddr`計數且完全忽略forwarding headers；只有operator allowlist中的proxy peer才可啟用選定的`X-Forwarded-For`或`Forwarded`
- F.17 對trusted proxy的forwarding field套用1024-byte / 16-hop bounds、IPv4/IPv6 normalization與right-to-left trusted-hop stripping；missing/malformed metadata在password KDF / recovery provider之前generic `400 invalid_request` fail-closed
- **F.18 optional trusted-proxy upstream mTLS identity on top of F.17**
- F.18 只有F.17 allowlisted socket peer才切到`RequireAndVerifyClientCert`；direct/untrusted Client仍維持既有TLS 1.3 server-auth-only行為，不要求Godot Client certificate
- F.18 strict schema-v1 edge trust policy以bounded CA bundle + exact DNS SAN allowlist驗證proxy identity，支援獨立`SIGHUP` generation / invalid replacement LKG；old authenticated proxy TLS connection不因CA cutover被強制重建，新handshake使用新trust generation
- **F.19 atomic trusted-proxy edge-policy generation + network↔identity binding**
- F.19 新的strict schema-v1 edge-policy file把selected forwarding mode、client CA、trusted prefixes與per-binding exact DNS SAN identity一次validate後原子publish；legacy F.17/F.18 flags仍相容但與F.19 authority file互斥
- F.19 trusted proxy TLS connection綁定握手時完整edge generation，因此SIGHUP後existing connection不會混用new header/prefix rules；new handshake完整使用new network/CA/identity/header generation，invalid replacement保留整個edge-policy LKG
- **F.20 optional immediate retirement of old F.19 trusted-proxy TLS generations**
- F.20 啟用`-session-login-trusted-proxy-edge-retire-old-connections`後，只有successful edge publication才會同步close舊edge generation的trusted proxy connections；invalid candidate不觸發retirement，direct/untrusted Client connections也不受edge fence影響
- F.20 保留未啟用flag時的F.19 graceful pinned-generation行為；old-generation handshake若在cutover後才完成，也會在ConnState觀察到stale authority後fail-closed
- **F.21 effective edge-authority no-op detection / change-aware retirement**
- F.21 仍完整load/validate F.19 candidate，但只在forwarding mode、實際CA certificate DER set或normalized prefix→exact DNS identity mapping真的改變時才publish新generation；revision、JSON/binding order、等價prefix spelling、DNS case/duplicates與同一CA的PEM重排/重複都不會製造新authority
- F.21 semantic no-op保留current snapshot、revision與generation，因此F.20會retire 0條proxy connections；real authority change仍沿F.19 publish + F.20 immediate cutover，F.16 Server certificate等其他SIGHUP domain可獨立成功而不再造成無謂edge reconnect
- **F.22 binding-aware trusted-proxy retirement / selective edge cutover**
- F.22 在F.20 opt-in retirement上改用connection pinned snapshot對current snapshot的peer-specific compatibility：global forwarding mode、實際CA root set或normalized trusted-prefix topology一變仍全域fail-closed；三者都相同時只退休自身prefix→exact DNS identity set真正改變的binding
- F.22 unchanged binding的old keep-alive與late old-generation handshake可跨其他binding的identity-only rotation繼續使用原pinned snapshot；不silent promote到new generation，之後若再發生global authority cutover仍會立即被判定stale並close
- **F.23 authenticated proxy identity-aware retirement / partial binding rotation**
- F.23 在TLS handshake時保存leaf DNS SAN與原binding allowlist交集形成的bounded normalized matched identity set；同一binding identity-only rotation時，只要至少一個原本就被授權的exact identity仍存在，old connection可保留原pinned snapshot
- F.23 multi-SAN certificate不會因reload後新allowlist加入一個原握手未授權的SAN而retroactive取得存活authority；原授權matched identity全數移除就退休，fresh handshake才可依new generation重新取得identity authority
- **F.24 trusted proxy leaf credential revocation / certificate instance fence**
- F.24 維持F.19 schema v1不變，新增獨立Server-owned revocation generation；唯一credential identifier為leaf `RawSubjectPublicKeyInfo`的SHA-256，設定檔使用64字元lowercase hex，因此同一DNS identity下可只撤銷被compromise的key，健康的另一把key仍可存活
- F.24 fresh trusted-proxy handshake會查current revocation generation並把SPKI identifier連同F.23 matched identity一起pin；F.20啟用時successful revocation publication只退休currently-revoked credential的既有connection，request-time attribution另做current revocation fence以封住publication→socket-close race；direct/untrusted Client TLS仍是server-auth-only
- **F.25 multi-instance trusted-proxy revocation distribution fence**
- F.25 在F.24 revocation authority上加入strict schema-v1 distribution manifest：monotonic epoch + F.24 semantic authority SHA-256 + 最長24小時`valid_until` lease；revocation candidate與manifest digest不一致、epoch rollback或same-epoch conflicting reuse一律fail-closed並保留LKG
- F.25 每個`worldd`使用stable operator-owned instance ID與local durable 0600 ack作restart epoch floor與convergence evidence；漏掉新distribution的instance最晚在lease到期時失去全部trusted-proxy forwarding authority，ack寫入失敗則維持新revocation但fence proxy authority直到same-epoch ack重試成功；direct/untrusted Client不受影響
- **F.26 revocation rollout orchestration / required-ack gate**
- F.26 新增Server/deployment-side `revocationctl publish|wait|rollout`：strict schema-v1 plan明列單一target epoch/lease、F.24 source與explicit required instance set；controller使用與`worldd`完全相同的F.24 semantic digest，先對全部target stage canonical revocation，再以F.25 manifest作per-target commit marker
- F.26 rollback/same-epoch conflict在任何write前reject；mid-manifest failure採fail-forward且允許identical epoch/digest/lease idempotent retry。Required member只有在durable F.25 ack的`instance_id + epoch + revocation_revision + digest + valid_until`完全命中時才converged；missing/older ack為pending，timeout/lease expiry回`incomplete`/exit 2，direct Client與F.25 authorization fence均不變
- **F.27 rollout observation evidence / convergence timing**
- F.27 不新增remote executor或新的authorization fence；`revocationctl wait|rollout`在既有F.26 all-required gate上加入controller-owned `observation` evidence，以單一controller timing domain記錄rollout `started_at/completed_at/elapsed_ms`與每個exact ack第一次被觀察到的`first_observed_at/observed_elapsed_ms`
- F.27 不把各`worldd`自行寫入的`acknowledged_at`當成跨主機latency truth；production timeout依controller elapsed/monotonic clock，F.25 `valid_until`仍是absolute UTC security boundary。Delayed/missing activation仍只表現為pending/incomplete，不把SIGHUP/restart、service discovery或process-control authority塞進controller或Client

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

### F.17 trusted reverse-proxy source attribution

F.17 只改login/recovery abuse-control的**來源選擇**，不改fixed-window guard本身，也不改HTTP API。預設未配置proxy attribution時，Server行為與F.16以前完全相同：只以TLS socket的實際`RemoteAddr`計數，任意`X-Forwarded-For`/`Forwarded`都不具authority。

Trusted proxy模式必須同時配置：

```text
-session-login-trusted-proxy-cidrs
-session-login-forwarded-header=x-forwarded-for|forwarded
```

最多接受64個normalized trusted IP/CIDR prefixes。Server永遠先檢查實際socket peer；peer不在allowlist時完全不解析forwarding metadata。只有allowlisted peer才要求恰好一個選定header field，且上限1024 bytes / 16 hops。

Multi-hop選擇從nearest advertised hop向左走：trusted intermediary會被剝除，遇到第一個untrusted address即成為attributed source；一旦遇到untrusted boundary，更左側的內容不再有authority。IPv4-mapped IPv6會先normalize成canonical IPv4，避免`::ffff:198.51.100.30`與`198.51.100.30`形成不同bucket。

Trusted peer若提供missing、duplicate、oversized、malformed、obfuscated `Forwarded for=`或超過hop上限的metadata，Server在login password KDF / recovery provider之前回既有generic `400 invalid_request`，不silent fallback到proxy IP。Direct/untrusted peer即使送 malformed header仍只依socket peer行為處理。

Login與recovery guard仍彼此獨立，原本4096 tracked sources、window、`429 login_throttled` / `429 recovery_throttled + Retry-After`語意不變。普通log只輸出`source_attribution` mode、trusted prefix count與hop bound，不輸出client IP或attacker-controlled header value。

F.17 production E2E使用real `worldd`與TLS reverse-proxy harness，讓proxy upstream socket明確bind到allowlisted `127.0.0.2`；同一gate證明direct spoofed header不能換bucket、proxied clients有獨立bucket、multi-hop trust stripping、IPv4-mapped normalization、trusted malformed metadata fail-closed、recovery attribution與secret-safe logs。

完整 F.17 contract：[`docs/S4F17_TRUSTED_PROXY_SOURCE_ATTRIBUTION.md`](docs/S4F17_TRUSTED_PROXY_SOURCE_ATTRIBUTION.md)。

### F.18 trusted proxy upstream mTLS edge identity

F.18 在F.17 trusted-proxy source attribution上加入**可選**的TLS client-certificate identity。未設定F.18 policy時，既有F.17 IP/CIDR-only deployment保持相容；設定：

```text
-session-login-trusted-proxy-mtls-file=/secure/proxy-mtls.json
```

則該flag必須與F.17 allowlist/header pair一起使用。新的TLS handshake先看真實socket peer：未命中F.17 allowlist的direct/untrusted Client維持既有TLS 1.3 server-auth-only config，不要求client certificate；命中allowlist的peer才切換到`RequireAndVerifyClientCert`。

F.18 policy為strict schema-v1 JSON，包含`revision + client_ca_file + dns_names`。Policy <=64 KiB、CA bundle <=256 KiB、最多16 roots / 32 DNS identities；CA必須是目前有效的X.509 CA且可簽證書。Proxy leaf在正常`clientAuth` chain verification成功後，還必須有至少一個**exact** allowlisted DNS SAN；不接受Common Name fallback、IP literal或wildcard identity。

Proxy trust以獨立immutable generation發布。SIGHUP會先完整load/validate candidate policy + CA + identity set，成功才publish下一generation；invalid JSON/PEM/CA/identity保留LKG。新trusted-proxy handshake snapshot當下generation；已建立且已驗證的proxy TLS connection保留原握手state，因此CA A→B cutover不會強制踢掉舊A connection，但A的新handshake會依B generation重新判斷。

F.16與F.18是同一listener上的相反trust direction：F.16讓Server向Client/proxy證明身分；F.18讓reverse proxy向Server證明身分。兩者同受process `SIGHUP`觸發，但各自validate、各自LKG，不形成跨檔案transaction。F.18 production gate故意讓F.16 server cert/key candidate mismatch，同時成功把proxy CA A→B切到generation 2，以證明獨立publication。

F.18不改F.17後續source-selection：mTLS驗證成功後仍只解析指定的一種forwarding field，維持1024-byte / 16-hop bound、IPv4/IPv6 normalization與right-to-left trusted-hop stripping。Login/recovery fixed-window guard、Client F.11 API/UX、schema-v4與Protocol v9都不變。

完整 F.18 contract：[`docs/S4F18_TRUSTED_PROXY_MTLS_EDGE_IDENTITY.md`](docs/S4F18_TRUSTED_PROXY_MTLS_EDGE_IDENTITY.md)。

### F.19 trusted proxy edge-policy runtime reload

F.19 收斂F.17/F.18原本分離的network/header與certificate identity authority。新的F.19 mode使用：

```text
-session-login-trusted-proxy-edge-policy-file=/secure/edge-policy.json
```

此flag與`-session-login-trusted-proxy-cidrs`、`-session-login-forwarded-header`、`-session-login-trusted-proxy-mtls-file`互斥。Legacy F.17/F.18 mode仍完整保留；F.19 mode則只有一份authoritative edge policy，不做兩種設定的merge或precedence。

F.19 strict schema-v1 generation一次包含：

```text
revision
forwarded_header = x-forwarded-for | forwarded
client_ca_file
bindings[]
  ├── prefixes[]
  └── dns_names[]
```

Policy <=64 KiB、最多32 bindings、總trusted prefixes最多64、總DNS identities最多64；CA維持F.18的256 KiB / 16 roots bound與X.509 CA validation。Prefixes沿用F.17 normalization，exact DNS SAN沿用F.18 normalization。不同bindings的prefix不得overlap，因此一個真實socket peer在同一generation最多只對應一個identity binding，不需要longest-prefix或config-order規則。

新的trusted proxy handshake先以真實`Conn.RemoteAddr`在current generation選binding。沒有match時仍走ordinary TLS 1.3 server-auth Client path；有match時必須`RequireAndVerifyClientCert`，chain通過generation CA pool後，leaf還必須匹配該**特定network binding**的exact DNS SAN。合法certificate不能自動跨另一個proxy network取得forwarding authority。

成功mTLS handshake後，Server會把該connection綁定到完整immutable edge snapshot。該connection後續HTTP request會用握手generation的header mode、trusted prefix union、identity binding與F.17 right-to-left source selection，而不是每個request重新讀current policy。因此：

```text
old proxy connection established under generation A
→ SIGHUP publishes generation B
→ old connection完整保留A的identity + header + prefix semantics
→ new handshake完整使用B的network + CA + identity + header semantics
```

這避免「TLS identity是舊generation、HTTP attribution卻突然讀新generation」的混代trust state。HTTP connection close/hijack會清除binding；若current policy視某peer為trusted但request沒有已驗證的connection binding，則fail-closed，不silent fallback成socket-only attribution。

SIGHUP會先完整validate forwarding mode、CA、所有prefix、non-overlap、exact identities與bounds；F.21再比較effective authority，只有真正變更才publish下一edge generation。任何invalid replacement保留整個edge-policy LKG。F.19/F.21與F.16 Server certificate、account reload、F.14 recovery reload仍各自獨立validate與publish，不形成跨檔案transaction。

F.19 production gate把generation 1的XFF + CA A + `.2/.3/10/8` bindings一次切成generation 2的`Forwarded` + CA B + `.2/.4/172.16/12` bindings，同時保留一條generation-1 keep-alive connection，證明old connection仍完整使用A；new handshake只接受B。Gate也驗證cross-binding cert reject、removed prefix回direct/socket attribution、new/removed trusted-hop behavior、recovery同一source contract、invalid overlap LKG，以及F.16 intentionally-invalid certificate replacement不會阻止F.19 valid publication。

完整 F.19 contract：[`docs/S4F19_TRUSTED_PROXY_EDGE_POLICY_RUNTIME_RELOAD.md`](docs/S4F19_TRUSTED_PROXY_EDGE_POLICY_RUNTIME_RELOAD.md)。

### F.20 trusted proxy connection revocation / immediate edge cutover

F.20 保留F.19 edge-policy schema與graceful compatibility behavior，另外提供明確opt-in：

```text
-session-login-trusted-proxy-edge-policy-file=/secure/edge-policy.json
-session-login-trusted-proxy-edge-retire-old-connections
```

第二個flag只允許搭配F.19 mode。未啟用時，F.19 established trusted proxy connection仍保留其握手generation直到自然close；啟用後，每次**真正publish**新的edge generation都會在`reload applied` log前同步執行F.22 peer-specific retirement。Global forwarding mode / CA root set / trusted-prefix topology變更仍退休所有old trusted-proxy connections；只有純binding identity rotation才可保留effective authority未變的old connections。

Server透過`http.Server.ConnState`追蹤F.20模式下的live listener connections，並從F.19已驗證connection binding取得握手generation與pinned immutable snapshot。Direct/untrusted connection沒有F.19 authenticated generation，因此不會被edge fence關閉。F.22不再只以generation age判定：old connection / late handshake會把自己的pinned snapshot直接與current snapshot對actual socket peer比較；相容就保留原snapshot，不相容就close並要求fresh handshake。

F.20/F.21/F.22的安全順序是：

```text
load + fully validate F.19 candidate
→ invalid: keep complete edge-policy LKG; retire 0 connections
→ valid + effective authority unchanged: keep generation N; retire 0 connections
→ valid + authority changed: publish generation N+1
→ F.20 disabled: existing authenticated connections keep pinned snapshot
→ F.20 enabled + global header/CA/prefix topology changed: retire all older trusted proxies
→ F.20 enabled + only binding identity mapping changed: retire incompatible peer bindings only
→ report reload applied + retired_connections metadata
```

這個fence是transport authority revocation，不是跨handler transaction rollback。已經通過source attribution並進入password KDF、recovery provider或account mutation boundary的in-flight request可能完成Server-side work；F.20只保證真實cutover後舊keep-alive connection不能再發下一個trusted forwarding request。既有account-generation / recovery mutation fence仍是最終資料一致性authority。

F.16 Server certificate與F.20/F.21 edge lifecycle仍為獨立domains。F.21 production gate讓F.16 Server certificate A→B成功reload，同時以revision、等價prefix spelling、DNS case/duplicates與重複同一CA PEM製造representation-only edge candidate；edge generation維持1且現有proxy keep-alive繼續可用。之後真正切換CA/identity/header才publishgeneration 2並觸發F.20 retirement。

完整 F.20 contract：[`docs/S4F20_TRUSTED_PROXY_CONNECTION_REVOCATION.md`](docs/S4F20_TRUSTED_PROXY_CONNECTION_REVOCATION.md)。

### F.21 edge-policy no-op reload detection / change-aware retirement

F.21 不改F.19 schema，也不新增deployment flag。每次SIGHUP仍先完整讀取並validate candidate，但比較的是**effective edge authority**，而不是source-file bytes或operator label。

Authority fingerprint只包含：

```text
selected forwarding mode
actual client CA certificate DER set
normalized trusted prefix -> exact allowed DNS identity set mapping
```

因此以下representation-only變更不會前進generation：

```text
revision-only edit
JSON / binding order
等價的binding grouping
127.0.0.2 與 127.0.0.2/32 這類normalize後相同的prefix spelling
DNS identity大小寫、順序或重複項
client_ca_file文字path本身
相同CA certificates的PEM順序、格式或重複block
```

CA authority用parsed X.509 certificate的raw DER做SHA-256後排序/去重比較；不同certificate即使path、subject、serial與root count相同，仍會被判定為real authority change。Digest只作process內比較，不對Client公開，也不寫ordinary log。

Semantic no-op會保留current immutable snapshot，因此running revision metadata也維持目前已publish的revision；candidate revision只有在伴隨真實authority change並publish時才成為current metadata。這避免單純operator label變更間接成為connection revocation command。

F.21 compare/publish在既有edge-policy mutex內完成，因此concurrent reload不會先對generation N比較、再錯誤相對於另一個generation publish。Invalid candidate仍在comparison前fail-closed並保留完整LKG。

Production F.21 gate使用real `worldd` + TLS 1.3 + F.19 mTLS + F.20 retirement，證明：representation-only reload保持generation 1、retired_connections=0、existing proxy connection可繼續送XFF；同一SIGHUP的F.16 Server certificate reload可獨立成功；真正CA/identity/header變更才publishgeneration 2、退休old keep-alive，new proxy cert + `Forwarded`成功，而old cert的新handshake失敗，direct Client仍維持server-auth-only路徑。

完整 F.21 contract：[`docs/S4F21_EDGE_POLICY_NOOP_RELOAD.md`](docs/S4F21_EDGE_POLICY_NOOP_RELOAD.md)。

### F.22 binding-aware trusted proxy retirement / selective edge cutover

F.22 不改F.19 schema、不新增flag，也不改F.21 publication判斷；它只收斂F.20在**已經真的publish新generation之後**要關哪些old authenticated proxy connections。

每條trusted proxy TLS connection本來就由F.19保存完整pinned snapshot。F.22直接比較該snapshot與current snapshot：

```text
same forwarding mode
AND same actual CA certificate DER set
AND same normalized trusted-prefix set
AND old/current都仍把actual socket peer映射到binding
AND old/current該peer binding的exact DNS identity set相同
```

前三項是global authority。任一改變會讓所有old trusted proxy connections fail-closed退休，因為header interpretation、TLS trust anchor或F.17 trusted-hop stripping語意可能全域改變。只有前三項都相同時，identity-only rotation才可做binding-local retirement。

被保留的old connection仍使用原握手時已驗證的TLS state與pinned forwarding snapshot，**不會被silent promote或重新標記成new generation**。這之所以安全，是因為對該actual socket peer而言global與binding-local effective authority都相同；若更後面的generation再改global authority，Server會直接拿old pinned snapshot與最新snapshot比較並立即退休它，不需要維護額外generation-change history。

Late handshake採同一規則：若handshake在cutover前選到old snapshot、cutover後才完成，changed binding會close，unaffected binding可存活；global cutover時所有old handshakes都close。Invalid candidate仍不publish、不進retirement；F.21 no-op仍generation不變、retire 0。

F.22 production gate以real `worldd` + TLS 1.3 + 同一edge CA建立`.2=edge-a`與`.3=edge-b`兩條persistent proxy connections。Generation 2只把`.2` exact identity輪替成`edge-a2`，證明`retired_connections=1`、old `.2`失效而old `.3`仍可用XFF；fresh old A cert reject、fresh A2/B成功。Generation 3只把global header XFF→`Forwarded`，再證明剩下old `.3`失去舊header authority；之後invalid overlapping-prefix replacement保留generation-3 LKG，direct Client仍維持server-auth-only TLS。

F.22的global與binding safety boundary繼續成立；F.23只把同一binding內的retirement decision收斂到每條connection握手時真正被原generation接受的identity，而不是整個allowlist set。

完整 F.22 contract：[`docs/S4F22_BINDING_AWARE_TRUSTED_PROXY_RETIREMENT.md`](docs/S4F22_BINDING_AWARE_TRUSTED_PROXY_RETIREMENT.md)。

### F.23 authenticated proxy identity-aware retirement / partial binding rotation

F.23 不改F.19 schema、不新增deployment flag，也不改F.21 publication或F.22 global cutover；它只在F.20 retirement啟用且global authority仍相容時，把connection compatibility細化到**握手當下真正被授權的exact DNS identity**。

TLS verify成功時，Server將leaf certificate的normalized DNS SAN與該pinned generation binding allowlist取交集、去重、排序後保存成connection-local matched identity set。後續identity-only publication只要current peer binding仍允許至少一個這個原授權identity，old connection即可保留原snapshot；所有原授權identity都被移除時才close。

```text
leaf DNS SANs = {edge-a, edge-future}
old binding   = {edge-a, edge-canary}
pinned match  = {edge-a}
new binding   = {edge-future, edge-next}
=> old connection retires
=> fresh handshake may now authenticate through edge-future
```

這個multi-SAN rule刻意禁止retroactive promotion：certificate裡雖然早已有`edge-future`，但它沒有被原握手generation授權，因此不能在reload後拿來替舊connection續命。若原握手確實同時匹配多個allowlisted SAN，則其中任一仍被current binding接受即可保留connection。Missing/invalid pinned identity state一律fail-closed。

Late old-generation handshake使用同一matched-identity比較。Header mode、實際CA root set或trusted-prefix topology任何變更仍沿F.22全域退休；F.21 no-op仍generation不變、retire 0；未啟用F.20 flag時仍維持F.19 graceful pinned-generation behavior。

F.23 production gate在同一`127.0.0.2/32` binding建立`edge-a`、`edge-canary`與`{edge-a, edge-future}` multi-SAN三條persistent mTLS connection。Generation 2由`{edge-a, edge-canary}`切到`{edge-a, edge-next}`時只退休canary，兩條原本透過edge-a取得authority的connection維持同一keep-alive socket；generation 3切到`{edge-future, edge-next}`時兩條old edge-a authority都退休，包括原certificate已有edge-future的multi-SAN connection，而fresh multi-SAN handshake才可透過newly-authorized edge-future成功。Invalid overlap replacement維持LKG，direct Client仍是server-auth-only TLS。

完整 F.23 contract：[`docs/S4F23_AUTHENTICATED_PROXY_IDENTITY_RETIREMENT.md`](docs/S4F23_AUTHENTICATED_PROXY_IDENTITY_RETIREMENT.md)。

### F.24 trusted proxy leaf credential revocation / certificate instance fence

F.24 解決F.23無法在「相同edge CA、socket prefix、exact DNS identity」下只撤銷單一被compromise proxy credential的缺口。Server使用 `SHA-256(leaf RawSubjectPublicKeyInfo)` 作為唯一canonical credential identifier；這是key-level SPKI fence，而不是certificate-DER或serial fence，所以同一compromised key即使重簽certificate仍持續被revoked，而同DNS identity但不同healthy key不受影響。

`-session-login-trusted-proxy-leaf-revocation-file` 是獨立於F.19 schema v1的strict schema-v1 LKG domain：64 KiB file bound、最多256 entries、exact 64 lowercase hex syntax、unknown/trailing fields reject、duplicates deterministic dedupe。Effective revoked SPKI set不變時屬semantic no-op，不增加generation也不觸發retirement；invalid replacement保留LKG。

SIGHUP ordering固定為Server TLS certificate → F.19 edge-policy → F.24 leaf revocation，各自validate/publish且不組成cross-file distributed transaction。Fresh handshake與late-handshake bind都查current revocation set；已建立connection則pin credential identifier，F.20開啟時revocation publication只close revoked credential。Request attribution同時查current revocation authority，避免socket close完成前仍保有forwarding authority。Direct/untrusted Client完全不進proxy credential map。

F.24 final exact product head `7fa0cd77bef86ebf2185c0c69749c4ace3ca24a2`通過16/16 workflows；Server CI的Test、Vet、Race detector及新的Production Trusted Proxy Leaf Credential Revocation E2E全部success。Product以squash merge進main，merge SHA為`a1c2b098c4a23421bcb811bface1c20b4710a07e`。

### F.25 multi-instance trusted proxy revocation distribution fence

F.25 不把F.24改造成distributed PKI，而是用bounded lease把『某個`worldd`漏掉revocation更新後可以無限期繼續信任舊set』收斂成可證明的上限。Opt-in distribution manifest使用strict schema v1，包含monotonic `epoch`、對F.24 effective revoked-SPKI set計算的semantic `revocation_authority_sha256`，以及canonical UTC RFC3339 `valid_until`；lease最多可向前24小時。

Revocation file與distribution manifest是paired candidate：manifest digest必須精確對上F.24 candidate semantic digest。Lower epoch、same epoch但digest/lease不同、expired lease或超過24小時上限都reject，且不修改current LKG。Higher epoch可同時發布新revocation authority，也可只renew lease而不製造新的F.24 generation。

每個instance另外配置stable `instance-id`與獨立local ack file。成功startup/reload後Server以0600 atomic write記錄`instance_id + epoch + revocation_revision + semantic digest + valid_until + acknowledged_at`；既有ack同時是restart epoch floor，避免process重啟時silent rollback到已acknowledge epoch之前。若新authority已安全publish但ack write失敗，Server不rollback revocation，而是將該instance的trusted-proxy credential authority整體fail-closed；same epoch且digest/lease完全相同時可重試ack並恢復authority。

Fresh proxy handshake與existing keep-alive request-time attribution都要求current F.24 credential state加上F.25 matching digest、healthy ack與unexpired lease。漏掉新distribution的member只可沿既有lease存活，lease一過即拒絕所有trusted-proxy authority；direct/untrusted Client仍維持TLS server-auth-only。

F.25 final exact product head `d0896c73442e13ac8f2e8af58998bc0fbcaa6ee4`通過17/17 workflows；Server CI的Test、Vet、Race detector及新的Production Trusted Proxy Revocation Distribution Fence E2E全部success。Production gate以兩個real `worldd`證明delayed member lease expiry fail-closed、healthy Leaf B preservation、epoch/digest/lease ack convergence、rollback fence與direct Client unchanged。Product以squash merge進main，merge SHA為`a87dfbe93f5e19989b6466183b8cbb0a0d86da4c`。

### F.26 revocation rollout orchestration / required-ack gate

F.26 維持F.25 `worldd` authorization fence原封不動，新增獨立Server/deployment-side `revocationctl`。Strict schema-v1 rollout plan指定單一target `epoch + valid_until`、F.24 revocation source與最多64個explicit required instances；每個member明列revocation/distribution/ack path，relative path以plan directory為基準，並沿用F.24最多256個SPKI與F.25最長24小時lease界線。

Controller會完整validate/canonicalize F.24 source，使用與`worldd`相同的`astrahold/session-leaf-revocation-authority/v1\x00` domain separator與sorted raw SPKI digest計算semantic SHA-256。Publish先preflight全部targets的newer-epoch / same-epoch conflict，再對全部unique target pair atomic stage canonical F.24 revocation；只有全部stage成功後才逐一atomic publish F.25 manifest作commit marker。若manifest commit中途失敗不做可能重新授權compromised key的rollback，而是回partial並允許identical target idempotent retry。

`revocationctl wait` / `rollout`只在所有required members的durable F.25 ack精確命中`instance_id + epoch + revocation_revision + revocation_authority_sha256 + valid_until`時回`converged`/exit 0。Missing或older ack保持pending；timeout或lease expiry回`incomplete`/exit 2；malformed/conflicting/superseded ack與invalid publication回exit 1。F.26不發明remote process-control protocol，SIGHUP/restart仍由deployment supervisor負責。

F.26 final exact product head `9e777aad8bea033acba919e196102d14dcfc1717`通過18/18 workflows；Server CI Test/Vet/Race全部success，新增Production Trusted Proxy Revocation Rollout E2E以兩個real TLS 1.3 `worldd`證明required A+B gate、delayed member阻止convergence、timeout incomplete、後續`wait`可續收斂、rollback fence、healthy Leaf B preservation與direct Client unchanged。Product以squash merge進main，merge SHA為`453e237863e64442a2e68e4efdee39627fb0bacf`。

### F.27 rollout observation evidence / convergence timing

F.27 不改F.25 `worldd` security fence，也不改F.26 explicit required-member / exact durable-ack convergence定義。它只為`revocationctl wait`與`rollout`的`converged`或`incomplete`結果加入controller-owned `observation`：同一controller invocation記錄`started_at`、`completed_at`、總`elapsed_ms`，以及每個已命中exact F.25 ack的`instance_id + first_observed_at + observed_elapsed_ms`。一開始就已存在的exact ack在本次window記為elapsed 0；後續重新執行`wait`會建立新的observation window，F.27不是durable telemetry database。

F.27刻意不使用ack file內由各`worldd`寫入的`acknowledged_at`推導跨host latency，避免不同machine clock被誤當一致時間源。`ack_timeout`改以controller elapsed time判定；production `time.Now`相減在可用時保留Go monotonic clock。F.25 `valid_until`仍是absolute UTC security deadline，因此wait會在controller elapsed timeout或lease expiry兩者先到者停止；非monotonic injected clock若倒退則fail hard，而不是輸出負的或誤導的timing evidence。

F.27 exact product head `7a7d91dae1840768d1a3db0babd3d5a82ea63ee0`通過19/19 workflows。Server CI Test/Vet/Race全部success；新增Production Trusted Proxy Revocation Rollout Observation E2E沿用兩個real TLS 1.3 `worldd`的F.26 acceptance，刻意讓A先activation/ack、約350ms後才讓B activation，證明controller觀察到A早於B且只有B ack後才converged；另一個2秒case只reload A並驗證`incomplete`保留A timing、B pending，之後同target可重新`wait`完成。既有rollback fence、revoked Leaf A、healthy Leaf B與direct Client unchanged也持續成立。Product以squash merge進main，merge SHA為`e67371a47e24c883c9f8b5ead1df6907a6a0ffdc`。

完整 F.27 contract：[`docs/S4F27_ROLLOUT_OBSERVATION_EVIDENCE.md`](docs/S4F27_ROLLOUT_OBSERVATION_EVIDENCE.md)。

完整 F.26 contract：[`docs/S4F26_REVOCATION_ROLLOUT_ORCHESTRATION.md`](docs/S4F26_REVOCATION_ROLLOUT_ORCHESTRATION.md)。

完整 F.25 contract：[`docs/S4F25_TRUSTED_PROXY_REVOCATION_DISTRIBUTION_FENCE.md`](docs/S4F25_TRUSTED_PROXY_REVOCATION_DISTRIBUTION_FENCE.md)。

完整 F.24 contract：[`docs/S4F24_TRUSTED_PROXY_LEAF_CREDENTIAL_REVOCATION.md`](docs/S4F24_TRUSTED_PROXY_LEAF_CREDENTIAL_REVOCATION.md)。

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

### SIGHUP account / recovery / TLS / edge-policy reload

Schema v3 / v4 account snapshot可在 durable store 更新後對 `worldd` 發 `SIGHUP`；recovery provider啟用時account schema必須維持v4。Schema-v2 delivered recovery provider也可由同一`SIGHUP`建立新的runtime generation；schema-v1 recovery provider維持restart-only。F.16起session-login與trusted game ingress certificate/key也由同一process signal觸發各自獨立的certificate generation reload。Legacy F.18 mode的proxy CA/exact identity policy可獨立reload；F.19 mode由單一edge-policy generation一起管理network/header/CA/identity binding，F.21先做effective-authority no-op detection，F.20可選擇在真實publication後啟用immediate retirement，F.22把identity-only cutover收斂成binding-aware peer-specific retirement，F.23再以握手實際授權的matched exact identity set決定同一binding內哪些connection可保留；F.24另以獨立revocation generation對握手leaf SPKI做key-level revoke/preserve fence。

Account reload有效時要求store revision嚴格前進。Recovery provider reload不要求文字revision前進，因為credential/private-CA-only rotation本來就不一定修改該revision。TLS certificate與legacy F.18 proxy trust仍依各自既有規則reload；F.21只為F.19 edge-policy mode加入semantic no-op detection。這些reload各自validate、各自last-known-good，不形成跨檔案distributed transaction。

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

Server TLS certificate安全順序：

```text
load candidate certificate + private key
→ validate key match / leaf validity / server-auth usage
→ publish immutable generation
→ existing TLS connections unchanged
→ new handshakes resolve new generation
```

F.19/F.20/F.21/F.22 edge-policy安全順序：

```text
load strict edge policy + client CA bundle
→ validate forwarding mode + all prefixes + non-overlap + exact DNS identities
→ failure: keep complete edge-policy LKG; do not retire connections
→ build effective authority fingerprint
→ unchanged: keep current snapshot/generation/revision; retire 0
→ changed: atomically publish network/header/CA/identity generation
→ F.20 disabled: existing authenticated proxy connection keeps pinned generation
→ F.20 enabled: compare each old connection pinned snapshot with current authority
   → header / CA / trusted-prefix topology changed: global retirement
   → otherwise: compare each old connection's handshake-authorized matched identities
      → any originally-authorized identity still allowed: preserve pinned connection
      → none remain: retire connection
→ new handshakes resolve the new generation
```

F.15 outbox啟用時，recovery generation reload不建立第二個worker，而是把同一process-global outbox背後的HTTPS transport/provider target在F.14 barrier內替換；pending records仍保留原delivery identity。Cold restart恢復的challenge先seed進generation-1 routes，再參與後續F.14 cutover。

因此 password rotation、account disable、password reset、KDF verifier generation或其他 account proof-generation 變更可讓舊 issued bearer 與 live game session立即失效；recovery credential/CA rotation與Server TLS certificate rotation不會把已建立且仍合法的game/login session無條件作廢。F.19 edge-policy可維持graceful existing-proxy semantics；若operator需要立即撤銷舊edge authority，則可啟用F.20 retirement fence；F.21避免unchanged edge authority因shared SIGHUP產生新generation；F.22在real identity-only cutover時保留其他effective authority未變的bindings；F.23進一步保留同一binding中仍具有原握手授權identity的healthy connections；F.24再讓operator可只撤銷其中被compromise的leaf key，而不必移除整個DNS identity或輪替整個edge CA。

F.9 production recovery E2E證明 operator流程；F.10 production public recovery E2E證明 no-SIGHUP public reset；F.11由normal Client產品UX直接覆蓋public request/reset/fresh-login/throttle；F.12把proof取得路徑接到Server-owned delivery adapter；F.13把delivery transport收斂成可部署HTTPS relay、bounded retry/idempotency與secret-safe observability；F.14加入provider/credential/CA fail-closed runtime generation reload；F.15把delivery/challenge可靠性推進到single-host durable restart recovery；F.16把login/game TLS certificate/key推進到fail-closed runtime generation；F.17為reverse-proxy deployment建立不信任public forwarding header的bounded source-attribution boundary；F.18加入proxy client-certificate chain + exact DNS SAN identity；F.19把network/header/CA/per-binding identity收斂成同一atomic edge-policy generation；F.20提供explicit old-generation connection retirement fence；F.21加入effective-authority no-op detection；F.22把identity-only cutover改成binding-aware selective retirement；F.23再把同一binding細化到handshake-authorized identity-aware retirement；F.24加入獨立SPKI revocation generation與same-identity healthy-key preservation。Recovery proof、password、opaque request metadata、delivery credential、issued bearer與proxy credential identifier持續維持secret-safe observability。

### Login / recovery abuse control

Login listener 在 JSON decode / password KDF 前套用 bounded fixed-window source-IP guard。

Login預設：

```text
30 login POST attempts / 1 minute / observed source IP
max tracked source entries = 4096
```

Recovery另有獨立 source-IP attempt guard與per-challenge attempt/TTL bounds。

未配置proxy attribution時，來源身份只相信TLS socket實際`RemoteAddr`，完全忽略`X-Forwarded-For`/`Forwarded`。Legacy F.17/F.18 mode仍先以process-start allowlist判定socket peer，並可再要求F.18 mTLS identity。F.19/F.21/F.22/F.23/F.24 mode則由current effective edge generation決定哪些socket prefixes需mTLS、可使用哪一種forwarding field，以及各network binding允許哪些exact DNS identity；若配置F.24 revocation source，trusted-proxy credential還必須通過current SPKI revocation fence。

F.19 trusted connection的forwarding parser仍維持F.17 1024-byte / 16-hop / normalization / right-to-left stripping語意。Direct/untrusted connection永遠不取得forwarding authority；trusted connection malformed/missing selected metadata仍在password KDF / recovery provider之前fail-closed。F.20只決定是否啟用real cutover後的immediate close；F.21決定candidate是否真的需要新generation；F.22建立global/binding compatibility；F.23再以原握手matched identities決定同一binding內哪些old authenticated proxy connections仍相容；F.24只再限制該connection的pinned proxy credential key是否被revoked。這些stage都不改source parsing與throttle bucket contract。

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

- [`docs/S4F23_AUTHENTICATED_PROXY_IDENTITY_RETIREMENT.md`](docs/S4F23_AUTHENTICATED_PROXY_IDENTITY_RETIREMENT.md)
- [`docs/S4F22_BINDING_AWARE_TRUSTED_PROXY_RETIREMENT.md`](docs/S4F22_BINDING_AWARE_TRUSTED_PROXY_RETIREMENT.md)
- [`docs/S4F21_EDGE_POLICY_NOOP_RELOAD.md`](docs/S4F21_EDGE_POLICY_NOOP_RELOAD.md)
- [`docs/S4F20_TRUSTED_PROXY_CONNECTION_REVOCATION.md`](docs/S4F20_TRUSTED_PROXY_CONNECTION_REVOCATION.md)
- [`docs/S4F19_TRUSTED_PROXY_EDGE_POLICY_RUNTIME_RELOAD.md`](docs/S4F19_TRUSTED_PROXY_EDGE_POLICY_RUNTIME_RELOAD.md)
- [`docs/S4F18_TRUSTED_PROXY_MTLS_EDGE_IDENTITY.md`](docs/S4F18_TRUSTED_PROXY_MTLS_EDGE_IDENTITY.md)
- [`docs/S4F17_TRUSTED_PROXY_SOURCE_ATTRIBUTION.md`](docs/S4F17_TRUSTED_PROXY_SOURCE_ATTRIBUTION.md)
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
| S4-F.16 | login + trusted-ingress TLS certificate/key SIGHUP generation reload；A→B cutover；old TLS connection survives；new handshake uses B；mismatched replacement LKG | ✅ |
| S4-F.17 | trusted-proxy source attribution；direct spoofed header ignored；per-client proxied buckets；multi-hop trust stripping；IPv4-mapped normalization；trusted malformed metadata fail-closed；recovery attribution | ✅ |
| S4-F.18 | trusted proxy upstream mTLS；direct Client unchanged；no-cert/wrong-SAN/future-CA reject；CA A→B runtime generation；old proxy connection survives；new A reject/new B accept；invalid CA LKG；F.16 reload independence | ✅ |
| S4-F.19 | atomic edge-policy generation；network↔exact identity binding；XFF→Forwarded + CA/prefix/identity cutover；old connection generation pinning；new-handshake cutover；removed/new trusted-hop behavior；invalid overlap LKG；F.16 independence | ✅ |
| S4-F.20 | optional immediate old-edge-connection retirement；old keep-alive revoked after successful real policy publication；late old handshake fail-closed；fresh generation handshake succeeds；invalid replacement keeps LKG connection；direct Client/recovery unchanged；F.19 graceful gate remains green | ✅ |
| S4-F.21 | effective edge-authority no-op detection；revision/order/prefix/DNS/duplicate-CA representation changes保持generation；existing proxy survives；F.16 TLS rotation independent；real CA/identity/header change才generation++並觸發F.20 retirement；old cert reject/new cert accept；direct Client unchanged | ✅ |
| S4-F.22 | binding-aware selective retirement；identity-only `.2` rotation只退休changed binding、unaffected `.3` old keep-alive存活；late handshake同peer-specific規則；global header cutover仍fail-closed退休remaining old connection；invalid replacement LKG；direct Client unchanged | ✅ |
| **S4-F.23** | **authenticated identity-aware partial binding rotation；同一binding中still-authorized `edge-a` keep-alive保留、removed `edge-canary`退休；multi-SAN原未授權`edge-future`不得retroactive續命；fresh handshake可依new generation授權；invalid replacement LKG；direct Client unchanged** | **✅** |
| **S4-F.24** | **independent trusted-proxy SPKI revocation generation；same exact DNS identity下Leaf A revoked/retired、healthy Leaf B存活；fresh revoked key reject；invalid replacement LKG；semantic no-op retire 0；direct Client server-auth-only unchanged** | **✅** |
| **S4-F.25** | **multi-instance revocation distribution fence；monotonic epoch + F.24 semantic digest + bounded lease；per-instance durable ack/restart floor；delayed member lease expiry fail-closed；ack convergence與rollback fence；healthy Leaf B/direct Client unchanged** | **✅** |
| **S4-F.26** | **Server/deployment `revocationctl`；explicit required instance set；exact F.24 semantic digest；all-revocation-stage-before-manifest-commit；rollback/conflict preflight；fail-forward idempotent retry；all-required exact durable-ack gate；incomplete exit 2/resumable wait；worldd/Client contract unchanged** | **✅** |
| **S4-F.27** | **controller-owned rollout observation evidence；per-required-instance first-observed exact-ack timing；controller elapsed timeout + absolute F.25 lease boundary；staggered/incomplete convergence timing；no remote executor / no new Client or worldd authority** | **✅** |

Server runtime contract現在是F.27；paired Client runtime仍是F.11。F.27 final exact product head `7a7d91dae1840768d1a3db0babd3d5a82ea63ee0`通過19/19 workflows：既有18個exact-head workflows全部success，新增的Production Trusted Proxy Revocation Rollout Observation E2E亦success；Server CI的Test、Vet、Race detector全部success。Protocol仍v9，`worldd` F.25 authorization semantics與F.26 all-required acknowledgement gate未改，Client product code未為F.27增加任何edge-policy/network/certificate/revocation/distribution-epoch/lease/ack/required-membership/observation-timing/activation-control authority。

## 文件入口

### Current production contract

- 本 `README.md` — current Server / Protocol v9 / account lifecycle / recovery / TLS lifecycle / edge trust / known limitations
- [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) — Current Architecture Baseline
- [`docs/S4F23_AUTHENTICATED_PROXY_IDENTITY_RETIREMENT.md`](docs/S4F23_AUTHENTICATED_PROXY_IDENTITY_RETIREMENT.md) — handshake-authorized matched DNS identity set / partial binding rotation / multi-SAN anti-promotion / identity-aware late-handshake contract
- [`docs/S4F22_BINDING_AWARE_TRUSTED_PROXY_RETIREMENT.md`](docs/S4F22_BINDING_AWARE_TRUSTED_PROXY_RETIREMENT.md) — peer-specific pinned-snapshot compatibility / binding-local identity rotation / global cutover / late-handshake contract
- [`docs/S4F21_EDGE_POLICY_NOOP_RELOAD.md`](docs/S4F21_EDGE_POLICY_NOOP_RELOAD.md) — effective authority fingerprint / semantic no-op / change-aware F.20 retirement / F.16 independence contract
- [`docs/S4F20_TRUSTED_PROXY_CONNECTION_REVOCATION.md`](docs/S4F20_TRUSTED_PROXY_CONNECTION_REVOCATION.md) — immediate old-proxy connection retirement / handshake race / invalid-LKG / F.16 independence contract
- [`docs/S4F19_TRUSTED_PROXY_EDGE_POLICY_RUNTIME_RELOAD.md`](docs/S4F19_TRUSTED_PROXY_EDGE_POLICY_RUNTIME_RELOAD.md) — atomic edge-policy generation / network↔identity binding / per-connection generation pinning / LKG contract
- [`docs/S4F18_TRUSTED_PROXY_MTLS_EDGE_IDENTITY.md`](docs/S4F18_TRUSTED_PROXY_MTLS_EDGE_IDENTITY.md) — proxy client CA / exact DNS identity / per-peer mTLS / SIGHUP generation / LKG contract
- [`docs/S4F17_TRUSTED_PROXY_SOURCE_ATTRIBUTION.md`](docs/S4F17_TRUSTED_PROXY_SOURCE_ATTRIBUTION.md) — trusted socket-peer allowlist / XFF-Forwarded parsing / multi-hop / fail-closed source-attribution contract
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
-session-login-trusted-proxy-cidrs                         # optional legacy F.17; pair with next flag
-session-login-forwarded-header                            # optional legacy F.17: x-forwarded-for|forwarded
-session-login-trusted-proxy-mtls-file                     # optional legacy F.18; requires F.17 pair
-session-login-trusted-proxy-edge-policy-file              # optional F.19/F.21/F.22/F.23/F.24/F.25; mutually exclusive with all three legacy flags
-session-login-trusted-proxy-edge-retire-old-connections   # optional F.20/F.22/F.23/F.24/F.25; requires F.19 edge-policy mode
-session-login-trusted-proxy-leaf-revocation-file          # optional F.24/F.25; requires F.19 edge-policy mode; SHA-256 SPKI revocation generation
-session-login-trusted-proxy-leaf-revocation-distribution-file # optional F.25; strict epoch/digest/lease manifest
-session-login-trusted-proxy-leaf-revocation-instance-id   # optional F.25; stable per-worldd acknowledgement identity
-session-login-trusted-proxy-leaf-revocation-ack-file      # optional F.25; local durable 0600 ack + restart epoch floor
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

Static trusted credential schema-v2可用原有 SIGHUP runtime reload；issued-session account schema-v1/v2為 restart-only compatibility，durable schema-v3/v4支援 SIGHUP account-generation reload。Public recovery provider啟用時要求 durable schema v4。Schema-v2 recovery provider可選F.12 `filesystem-reference-v1`或F.13 `https-json-v1`，並自F.14起支援`SIGHUP` provider generation reload；HTTPS relay credential/private CA/endpoint可隨generation輪替。F.15 `https-json-v1`可另外啟用single-host durable outbox。F.16起session-login與trusted game ingress的certificate/key支援`SIGHUP` fail-closed runtime generation reload。Legacy F.17/F.18 proxy mode仍保留；F.19另提供單一authoritative edge-policy file，把network/header/CA/exact identity binding一起做SIGHUP generation reload，且與legacy flags互斥。F.20 retirement flag只在F.19 edge-policy mode有效；F.21自動對validated candidate做semantic no-op detection，F.22在flag啟用時建立global/binding selective retirement，F.23再以握手matched exact identities做connection-level preservation；F.24可選擇加入獨立SPKI revocation source並在retirement flag啟用時只close revoked credential。未設定retirement flag時F.19既有generation cutover仍維持graceful semantics，但fresh revoked credential handshake仍fail-closed。

## 目前刻意保留的限制

- Realtime UDP尚未加密；目前只有 authenticity / integrity。
- Schema-v4仍是 **single-writer durable JSON account backend**；public recovery啟用時running `worldd`是 active writer，尚未有 distributed account DB或 multi-writer recovery CAS。
- F.15 已提供bounded single-host durable recovery delivery/challenge outbox、restart replay與stable F.13 idempotency identity；這不是distributed broker、multi-host consensus、cross-host recovery ownership或exactly-once vendor delivery。
- F.15 pending record為了restart replay會短暫以plaintext保存recovery proof與Server-owned destination；application只提供owner-only 0700/0600 permission boundary與terminal scrub，**不提供application-layer disk encryption**。需要media-at-rest confidentiality時應使用encrypted filesystem/volume。
- F.25 已以monotonic epoch、F.24 semantic digest、最長24小時lease與per-instance durable ack/restart floor收斂stale revocation consumption；F.26已提供explicit required-member rollout controller與all-required ack decision，F.27再加入controller-local convergence timing evidence。仍未提供service discovery、dynamic/quorum membership、multi-writer consensus、central online revocation publisher或remote SIGHUP/restart process execution。
- F.14 provider/credential/private-CA runtime generation reload、in-flight cutover fence與last-known-good仍適用；F.15只有一個shared outbox worker，pending records會跨transport generation保持原delivery identity。
- F.16 已有login/game TLS certificate/key runtime generation reload；不包含Client trust-store/CA hot reload、ACME/PKI自動化、OCSP lifecycle或multi-host certificate atomic cutover。Retired private key的RAM lifetime由Go runtime管理，不宣稱deterministic zeroization。
- F.24 已用SHA-256 SPKI做到同CA + 同exact DNS identity下的key-level selective revocation，F.25再把stale multi-instance consumption用lease/ack bounded fail-closed；仍未實作CRL/OCSP ingestion、ACME/PKI自動化、HSM attestation、自動compromise detection或centralized revocation publisher。
- Login/recovery仍是單process fixed-window guards；尚未有distributed rate limit、IP reputation、credential-stuffing intelligence、WAF/CDN vendor integration或 CAPTCHA。
- 尚未支援PROXY protocol；F.17–F.25的HTTP forwarding boundary不應被視為PROXY protocol parser或L4 load-balancer identity contract。
- Core Server刻意不綁特定email/SMS vendor SDK；實際vendor仍應位於F.13 HTTPS relay後方。
- 尚未加入 breached-password corpus、MFA / TOTP / WebAuthn / passkeys / OIDC external IdP adapter。
- Issued session credential仍為 process-local short-lived proof；Server restart強制重新 login，尚無 refresh token、remembered-device session、durable bearer recovery或 cross-server revocation propagation。
- F.11 Client刻意不持久化human password、recovery proof或opaque recovery challenge；尚未加入 OS keychain / secure remembered-session storage，因目前沒有refresh token/remember-session contract。
- NAT-like migration目前只允許 same-IP UDP source-port change；跨 IP migration fail-closed。
- 尚未做 multi-server distributed ownership / failover。
- 尚未加入 periodic in-session realtime rekey、DTLS或 QUIC；目前 evidence沒有證明 MVP需要它們。
- 500 rendered Godot actors、VAT / MultiMesh與 final commercial art不是目前 Server MVP gate。

## 下一個 bounded focus

S4-F.27 已把F.26可觀察的pending/incomplete狀態變成同一controller timing domain的量測 evidence：每個required member第一次出現exact ack的時間與整體convergence elapsed現在可直接記錄，因此可以區分「gate能否發現未收斂」與「實際部署到底多常、多久未收斂」兩個問題。F.25 `worldd` security fence、F.26 all-required gate、Direct Godot Client、Protocol v9與gameplay authority都沒有擴權。

**下一個 bounded focus 仍是 deployment evidence decision gate，而不是自動進F.28。** F.27 Production E2E中的約350ms stagger與2秒timeout是correctness驗證用的synthetic evidence，不能替代真實部署量測。應先收集實際rollout的`observation`結果；只有當真實資料顯示publish-success後的activation miss或過高activation lag是material operational risk，才考慮 **S4-F.28 — Activation / Supervisor Handoff Contract**。若沒有這類證據，就不加入remote executor、service discovery、consensus或更大的PKI control plane。CRL/OCSP、ACME/PKI automation、HSM、自動compromise detection、distributed rate limit、WAF/CDN與PROXY protocol仍保持獨立decision gate。

Public registration、MFA/WebAuthn/passkeys/OIDC、distributed account DB、refresh-token / remember-session與Protocol v10仍保持獨立 decision gate。

Astrahold 的原則保持不變：**Server State 是真相；先證明 correctness，再用量測決定複雜度。**

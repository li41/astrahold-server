# S4-F.15 Durable Recovery Delivery Outbox / Restart Reliability

S4-F.15 closes the process-local reliability gap left after S4-F.13 HTTPS delivery and S4-F.14 runtime credential/provider rotation. It adds an optional bounded durable outbox for schema-v2 `https-json-v1` recovery delivery and makes the corresponding recovery challenge survive a `worldd` restart.

The public recovery API, S4-F.11 Client contract, durable account schema v4, Protocol v9, gameplay authority, and F.14 provider-generation semantics do not change.

## Why delivery replay alone is insufficient

A recovery proof is useful only while the Server still owns the matching challenge verifier and account-generation binding. Before F.15 both the delivery retry state and the public recovery challenge were process-local. Replaying only the relay payload after restart would send a proof that the restarted Server could no longer redeem.

F.15 therefore treats one durable record as two related pieces of state:

1. while delivery is pending, it is a durable delivery outbox item; and
2. after delivery succeeds, it becomes a challenge-only restart record containing the verifier and Server-owned account-generation binding but no raw proof or destination.

This keeps the existing `accountrecovery.Provider` authority boundary intact instead of creating a second recovery authority inside the transport layer.

## Opt-in configuration

The durable outbox is optional and applies only to schema-v2 `https-json-v1` delivery:

```text
-session-recovery-outbox-dir
-session-recovery-outbox-max-records
-session-recovery-outbox-max-delivery-attempts
-session-recovery-outbox-retry-min
-session-recovery-outbox-retry-max
```

Defaults and bounds:

```text
max live records             1024, allowed 1..4096
delivery retry cycles        8, allowed 1..64
initial durable retry        1s, allowed 100ms..5m
maximum durable retry        30s, >= retry-min and <=5m
```

A durable retry cycle invokes the configured F.13 HTTP adapter once. That adapter can itself perform its existing bounded HTTP attempts, so the two limits have different meanings: F.13 bounds one transport cycle; F.15 bounds how many completed transport cycles may be replayed across time and process restarts.

The outbox directory must already exist and be a real owner-only readable/writable/searchable directory. `filesystem-reference-v1` remains the deterministic F.12 reference path and is not wrapped by the durable outbox.

## Durable record contract

Each live challenge is stored as one file named by the existing F.13 opaque delivery identity:

```text
<delivery_id>.json
```

The file schema is version 1. The identity fields are strict and include:

```text
schema_version
delivery_id
request_id
login_id
account_id
credential_version
verifier_sha256
active
expires_at
verification_attempts
delivery_state
delivery_attempts
```

A `pending` record additionally contains `destination`, `proof`, and `next_attempt_at`. A `delivered` or `failed` record must not contain those fields.

`delivery_id` must equal the existing F.13 derivation from the opaque public request ID. The same delivery ID therefore survives durable replay and remains the relay `Idempotency-Key` / `X-Astrahold-Delivery-ID`; restart does not create a new vendor-send identity.

## Durable write ordering

Every create/rewrite follows the same local durability boundary:

```text
validate record
→ create owner-only temp file
→ write complete JSON
→ fsync file
→ atomic rename over final record
→ chmod 0600
→ fsync containing directory
```

Record deletion removes the file and fsyncs the containing directory before the deletion is treated as durable.

Startup is fail-closed. Live record files must be real regular files with exact mode `0600`, bounded size, strict JSON fields, valid delivery identity, internally consistent proof/verifier state, and a filename matching `delivery_id`. Unexpected/corrupt entries reject outbox startup rather than being silently skipped. A crash-left owner-only `.tmp` file is removed before live records are restored.

Expired records are dropped during recovery and are not restored as active challenges.

## Enqueue and activation ordering

For an eligible schema-v2 subject:

```text
reserve process-local challenge
→ durable enqueue pending record
→ mark provider challenge active
→ register F.14 request route
→ publish record to outbox worker
```

The worker deliberately does not send a newly enqueued item until the F.14 router has registered the opaque request route. This prevents a fast permanent relay result from racing with `Begin` and being overwritten by a later challenge activation.

Once durable enqueue succeeds, the public request can return the existing generic `202 + request_id + expires_at` without waiting for the external relay.

If the durable outbox is full or enqueue persistence fails, the existing F.12 enumeration-safe behavior remains authoritative: the public request keeps the generic accepted shape, but the reserved challenge remains non-authorizing and is not presented as a successful durable delivery.

## Worker, retry, and terminal outcomes

The worker processes a bounded batch of due pending records. It reuses the exact original request ID, destination, proof, and expiry so the F.13 adapter derives the same delivery ID and relay idempotency key on every cycle.

Transient F.13 results schedule a durable exponential retry bounded by the configured F.15 retry interval. A record becomes terminal when:

- delivery succeeds;
- F.13 returns a permanent result;
- the configured durable retry-cycle bound is exhausted; or
- the recovery challenge expires.

On success, the record is atomically rewritten to `delivered`, retaining only the verifier/account-generation challenge data. On permanent/exhausted failure, it is atomically rewritten to `failed`, marked non-authorizing, and raw proof/destination are removed.

The in-memory challenge is not retired until the terminal non-authorizing state is successfully durable. If that state rewrite fails, the older pending record remains the disk authority; the Server does not create a restart window where disk says active while memory already assumes terminal retirement.

## Restart recovery

At process start F.15 restores every unexpired valid record into the schema-v2 recovery provider:

- `pending` restores an active challenge and is eligible for replay;
- `delivered` restores an active challenge but is not delivered again;
- `failed` restores a non-authorizing challenge;
- the persisted verification-attempt counter is restored so restart does not reset the per-challenge proof-attempt budget.

The F.14 generation router seeds restored challenges into generation 1 routes. Therefore a `SIGHUP` after restart can rotate the current provider/credential/CA without orphaning a still-valid pre-restart challenge.

Successful public reset still calls the existing provider `Consume`. F.15 additionally deletes and directory-fsyncs that durable challenge record, so a consumed proof cannot return after another restart.

## F.14 credential / CA rotation interaction

There is exactly one process-global outbox worker. Provider reload does not create a second worker.

During F.14 cutover:

```text
wait for old-generation Begin barrier
→ validate new schema-v2 provider + HTTPS adapter
→ atomically swap outbox transport/provider target
→ retire old HTTP adapter credential
→ publish new recovery generation
```

Already-pending records remain in the same durable outbox and can be retried through the newly published HTTPS transport. Their request ID, proof, destination, expiry, and relay idempotency identity do not change.

Old challenge verifiers remain routed according to the existing F.14 generation rules. The old HMAC proof key is still cleared at generation retirement because the durable record already contains the verifier required for reset verification.

## Secret-at-rest boundary

F.15 does **not** claim application-layer disk encryption.

A pending record necessarily contains the raw recovery proof and Server-owned destination so it can be replayed after process loss. The application boundary is therefore deliberately explicit:

- dedicated outbox directory: owner-only `0700`-style access;
- live and temporary record files: exact `0600`;
- no symlink acceptance;
- no ordinary log output of request ID, proof, destination, account ID, delivery ID, or relay Bearer credential;
- raw proof and destination are scrubbed from the durable record immediately after success or terminal failure;
- successful reset or expiry removes the record entirely.

Deployments requiring confidentiality against filesystem/media readers must place the outbox on an encrypted filesystem/volume or equivalent platform storage. F.15's file-permission contract is not a substitute for disk encryption.

The relay Bearer credential remains outside the outbox in the existing F.13 owner-only credential file and remains governed by F.14 runtime rotation.

## Observability

Outbox logs expose only bounded operational metadata such as outcome, record/pending counts, and delivery-cycle count. They do not log per-user delivery identity or secret material.

Representative outcomes include:

```text
outcome=enqueued
outcome=restored
outcome=delivered
outcome=retry_scheduled
outcome=permanent
outcome=exhausted
outcome=expired
outcome=backpressure
outcome=state_persist_failed
```

## Public / Client contract

Nothing is added to either public endpoint:

```text
POST /v1/account/recovery/request
POST /v1/account/recovery/reset
```

The S4-F.11 Client still sees only opaque `request_id + expires_at`, asks the user for a provider-neutral proof, and submits only `request_id + recovery_proof + new_password`.

The Client does not receive outbox status, retry count, delivery ID, destination, account ID, credential version, provider generation, relay endpoint, or storage metadata.

## Acceptance

The F.15 exact-head gate must prove, in addition to all prior F.9/F.10/F.12/F.13/F.14 gates, that:

- a recovery request durably creates a 0600 pending record before external delivery succeeds;
- transient relay failure updates durable retry state with the F.13 delivery identity unchanged;
- `worldd` can be killed and restarted with that pending record;
- startup restores the original challenge and worker replays the same delivery identity;
- a successful post-restart relay send atomically scrubs raw proof and destination from the record;
- the original pre-restart public request ID plus delivered proof can complete reset after restart;
- successful consume removes the durable record;
- old password fails and the new password succeeds under durable schema v4;
- Server/relay logs do not expose password, proof, destination, request ID, delivery ID, or relay credential;
- Protocol remains v9.

Unit/race coverage additionally fences backpressure, permanent failure, verification-attempt persistence, unsafe/corrupt storage, F.14 transport replacement, and cold-restart challenge routing across a subsequent provider generation change.

## Non-goals

S4-F.15 does not add a distributed broker, multi-host outbox consensus, exactly-once vendor delivery, cross-host recovery ownership, vendor SDKs, public delivery-status APIs, public registration, MFA/WebAuthn/passkeys/OIDC, refresh tokens, TLS certificate hot reload, Protocol v10, or gameplay/realtime changes.

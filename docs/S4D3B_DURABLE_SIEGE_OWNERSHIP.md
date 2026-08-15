# S4-D.3B — Durable Castle Ownership Persistence & Recovery

## Scope

S4-D.3B turns the process-local castle owner introduced in D.3A into durable single-writer Server state. The stage keeps Siege winner/ownership details off the network and does not change the Godot Client.

Astrahold Protocol remains **v7**. `SiegeMatchState` message 106 is unchanged.

## Durable key and record

The current MVP has one authoritative castle per gameplay world, so the durable ownership key is the Gameplay World `world_id` (for the checked-in world: `castle-sandbox`). The on-disk filename is the SHA-256 of that key; the world ID is also stored inside the strict JSON record and must match on load.

Ownership record schema v1 contains:

- `schema_version`
- `world_id`
- `revision`
- `owner_id`
- `previous_owner_id` when an ownership transfer has occurred
- `last_transfer_match_id` when an ownership transfer has occurred

The first startup creates revision 1 with the configured Siege defender as owner. Later startups load durable ownership first; they do not overwrite it with the config default.

This is deliberately a separate `internal/siegeownership` persistence package instead of extending trusted character-state storage with unrelated world ownership fields.

## Optimistic revision / CAS

`Store.Commit` accepts a `CastleOwnershipTransfer` containing:

- expected ownership revision
- expected previous/current owner
- requested next owner
- source Siege match ID

The Store fails closed on stale revision or owner mismatch. A successful real transfer advances the ownership revision exactly once.

A replay of the exact already-committed transfer returns the current durable snapshot without another write or revision bump. This covers the crash/error window where the atomic ownership file has already been replaced but the in-memory Match has not yet published `Completed`.

A transfer whose requested owner already equals the durable owner is a no-op and does not advance ownership revision.

## Durability mechanics

Ownership writes follow the same local durability pattern already used by trusted character state:

1. encode strict JSON;
2. write a temporary file;
3. `fsync` the temporary file;
4. atomic rename over the world record;
5. `fsync` the containing directory.

Unknown JSON fields, trailing JSON values, schema mismatch, world-ID mismatch, zero revision, or malformed transfer provenance fail closed.

This stage guarantees process-restart durability for a **single writer** sharing one ownership directory. It does not claim distributed CAS or multi-process locking. Multiple authoritative world processes for the same castle require a later storage/lease design.

## Completion durability barrier

D.3A changed `Throne -> Completed` and ownership in one in-memory world-owner operation. D.3B strengthens that ordering:

1. authoritative throne capture reaches `ReadyForResolution`;
2. Siege prepares a deterministic ownership CAS transfer;
3. the durable ownership Store commits that transfer;
4. Siege validates the returned durable snapshot;
5. only then does Match publish `Completed`, attacker winner, and the restored/committed ownership snapshot;
6. existing reliable `SiegeMatchState` replication may then expose the already-supported `phase=completed`.

If the durable commit fails, Match remains `Throne`, winner remains unset, in-memory ownership remains unchanged, and the D.2B readiness latch remains set. A later world tick retries the same deterministic transfer.

This prevents a client-visible completion from getting ahead of durable castle ownership.

## WorldRuntime / startup integration

`worldd` adds `-siege-ownership-dir` (default `data/siege-ownership`). Startup:

1. loads Gameplay World and Siege Match config;
2. opens the ownership Store keyed by Gameplay World ID;
3. creates the first defender-owned record or restores the existing durable record;
4. configures `WorldRuntime` with the restored snapshot and durable commit callback before the loop starts.

The persistence callback is installed only after `WithSiegeMatch`; invalid restored state fails startup rather than silently substituting process-local truth.

## Performance boundary

No session scan, replication loop, snapshot layout, lifecycle budget, or per-tick storage write is added.

The only blocking filesystem durability work on the world-owner path is the rare ownership transfer barrier when a throne capture actually resolves. A normal tick, capture progress tick, Gate attack, movement tick, and replication tick perform no ownership file I/O. Exact replay after an ambiguous durable write may perform a record read but does not rewrite the file.

This explicit one-shot latency tradeoff is chosen so `Completed` cannot be published before durable ownership. A future distributed authority layer can move the barrier behind a lease/consensus-backed command without changing the Siege domain CAS intent.

## Preserved boundaries

S4-D.3B does not change:

- Protocol v7 or Message 106 fields;
- GameV1 codec;
- Client repository;
- Gameplay World schema 2 / `castle-sandbox` revision `s3d-001`;
- Siege config schema 3 / revision `s4d2b-001`;
- 10-second configured throne capture duration;
- trusted roster/team assignment rules;
- UDP MTU 1200;
- WorldSnapshot chunking;
- Network LOD / transform max64;
- lifecycle desired-vs-known / Confirm-after-`TrySend` behavior;
- lifecycle churn max 6000/snapshot;
- Initial Vitals budgets;
- dirty Vitals max 4000/tick.

## Acceptance coverage

Focused tests cover:

- first ownership record creation;
- ownership CAS transfer and revision advance;
- stale CAS rejection without overwriting durable truth;
- exact transfer replay idempotence;
- reopen/startup recovery of the committed owner;
- strict corruption rejection;
- no-op same-owner transfer without revision churn;
- persistence failure leaving Match in `Throne` with readiness latched;
- retry completing Match only after durable commit succeeds;
- restored attacker ownership completing without a redundant durable transfer;
- WorldRuntime same-tick barrier and next-tick retry behavior.

## Explicitly deferred

D.3B does not persist the full Siege round (Gate HP, Match phase, capture progress, participant presence) and does not schedule a new round after restart. It persists only the castle ownership authority established by D.3A.

Also deferred:

- distributed/multi-process ownership locking or consensus;
- automatic attacker/defender role rotation from the recovered owner;
- Siege round scheduling/reset policy;
- winner/owner/capture-progress wire contract;
- Godot Siege HUD;
- Protocol v8 evaluation;
- true Go Server ↔ Godot Client S4-E end-to-end test.

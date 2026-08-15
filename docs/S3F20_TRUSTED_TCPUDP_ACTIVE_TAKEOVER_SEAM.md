# S3-F.20 — Trusted TCP/UDP Active Takeover & Old-Peer Eviction Seam

## Scope

S3-F.20 connects the process-local trusted ownership primitives from S3-F.17 through S3-F.19 to the production TCP/UDP transport.

The stage allows a newly authenticated trusted connection for an already-active `CharacterID` to replace the current network `Session` without despawning/re-spawning the authoritative Entity or restoring stale durable state.

This is a bounded transport takeover seam, not a general seamless-session protocol.

## Why this stage exists

S3-F.18 made stale trusted `Move`, `Action`, and `Leave` commands reject against an immutable `SessionOwnershipFence`.

S3-F.19 added the atomic world-owner ownership transfer transaction, but the production TCP/UDP path still always used S3-F.17 inactive admission and therefore rejected an active trusted `CharacterID` before it could reach the transfer primitive.

S3-F.20 closes that transport gap while preserving the existing authoritative-state and scaling boundaries.

## Trusted connection-plan barrier

`AwaitCharacterConnectionPlan` reuses the existing S3-F.17 `characterAdmissionCommand` FIFO barrier.

For one authenticated trusted `CharacterID`, the same world-owner command does exactly one of two things:

1. **Inactive character** — reserve and return the existing `CharacterAdmissionLease` generation.
2. **Active character** — return the exact current S3-F.18/S3-F.19 `SessionOwnershipFence` as the takeover CAS expectation.

The active branch is a successful connection plan rather than `ErrCharacterIdentityActive`.

Normal callers of `AwaitCharacterAdmission` are unchanged: they still reject an active `CharacterID`. The takeover behavior exists only when the connection-plan operation explicitly supplies an ownership result slot.

This prevents a transport composition race between a separate active lookup and inactive admission reservation.

## TCP/UDP connection flow

For each accepted TCP connection:

1. Allocate a new `SessionID` and a provisional fresh `EntityID`.
2. Resolve `CharacterIdentity` through the existing server-trusted `CharacterIdentityFactory` seam.
3. Ephemeral identities keep the legacy join path.
4. Trusted identities call `AwaitCharacterConnectionPlan`.
5. If inactive:
   - keep the newly allocated EntityID;
   - run the existing durable restore lookup if configured;
   - commit through `AwaitJoinOwned` with the admission lease.
6. If active:
   - use the current ownership fence's existing EntityID;
   - skip durable restore completely;
   - build a replacement `Session` for that same EntityID;
   - commit through `AwaitOwnershipTransfer(expected, replacement)`.
7. Validate the returned ownership fence against the new SessionID, existing EntityID, and trusted CharacterID.
8. Publish the new immutable peer ownership and peer-bound fenced ingress.
9. Mark the replacement peer joined.
10. For takeover, retire the old peer only after the F.19 transfer has committed.
11. Send the normal Protocol v6 `SessionWelcome` for the new SessionID and existing EntityID.
12. Mark the replacement peer ready and continue normal TCP/UDP routing.

The provisional allocated EntityID is intentionally unused when the connection becomes a takeover. EntityID allocation is monotonic; gaps are acceptable and avoid changing the existing `CharacterIdentityFactory` signature.

## Old-peer eviction

After transfer commit, worldruntime has already removed the old Session and activated a strictly newer ownership epoch.

The transport retirement path therefore:

- disables/removes the old peer and closes its connection;
- consumes the old peer's one-shot Leave callback without enqueueing a normal Leave;
- never asks worldruntime to save/despawn the transferred Entity.

If a concurrent old-peer close already enqueued its fenced Leave, F.18 still makes that delayed command stale after the successful transfer. The deterministic takeover retirement path itself does not create that expected stale-command noise.

If transfer fails, the replacement peer is closed before it is marked joined and the old peer is not retired.

## Durable-state ordering

Active takeover deliberately does **not** call `CharacterRestoreFactory`.

The already-active world Entity and character state are newer authoritative truth than the last durable autosave. F.19 preserves that live Entity, HP/Defeated state, respawn/revive state, combat/entity-scoped state, and CharacterID↔EntityID binding in memory.

No Character State save is emitted by transfer itself. Normal final persistence still occurs when the current owner later performs the existing fenced Leave.

## Session semantics

Takeover creates a new network Session:

- new `SessionID`;
- new realtime token;
- same authoritative `EntityID`;
- same trusted `CharacterID`;
- newer internal ownership epoch;
- fresh Session-scoped input/action sequence state;
- fresh replication/Vitals/dynamic delivery state through S3-F.19.

The ownership epoch remains server-internal and is not added to Protocol v6.

## Authentication boundary

S3-F.20 does not invent reconnect credentials.

Takeover is possible only when the configured trusted `CharacterIdentityFactory` returns the same `AssuranceTrusted` CharacterID for the new connection. Production authentication/account/character authorization remains an upstream integration responsibility.

The default development identity factory still emits a fresh ephemeral identity per connection and therefore cannot trigger trusted takeover.

## Failure behavior

- Invalid or conflicting connection plans fail closed before join/transfer.
- Admission reservation conflicts retain the existing F.17 behavior.
- Transfer CAS failure does not evict the old peer.
- A transfer that has already committed is irreversible at the transport layer; the old peer is retired before the new Welcome write. If that Welcome then fails, the new joined peer performs its current fenced Leave cleanup rather than reviving the stale old owner.
- Old TCP/UDP credentials are removed from the peer map when the old peer is retired.

## Preserved boundaries

S3-F.20 does not change:

- Protocol v6 wire schema;
- Client code;
- GameV1 codec;
- UDP MTU 1200;
- `WorldSnapshot` chunking;
- Network LOD or transform batch max64;
- lifecycle desired-vs-known truth;
- Spawn/Despawn Confirm-after-`TrySend` semantics;
- lifecycle churn max 6,000/snapshot;
- Initial Vitals budgets;
- dirty gameplay Vitals max 4,000/tick;
- Character State schema/store/journal/autosave format;
- death/respawn/penalty/outcome persistence semantics;
- F.17 60-second process-local admission lease lifetime;
- process-local nature of ownership epochs.

No filesystem or network I/O is added to the world-owner tick.

## Explicit non-goals

- no Client-visible reconnect/ownership token;
- no reconnect grace timer;
- no reconnect proof or credential protocol beyond the trusted identity factory seam;
- no preservation of the old SessionID;
- no preservation of old input/action sequence counters;
- no distributed or multi-worldd fencing;
- no durable ownership epoch;
- no cross-process takeover;
- no rollback to the old peer after transfer commit;
- no claim of zero-packet-loss seamless roaming;
- no Go↔Godot end-to-end claim.

## Focused acceptance tests

Worldruntime tests verify:

- inactive connection-plan reserves an admission lease;
- active connection-plan returns the exact current ownership fence without a command error;
- a second inactive candidate still gets `ErrCharacterAdmissionReserved` and cannot replace the first reservation.

TCP/UDP tests verify:

- a second trusted connection reuses the existing EntityID with a new SessionID and realtime token;
- active takeover does not call durable restore;
- the transfer receives the exact old ownership fence and replacement Session;
- old TCP peer is closed;
- old UDP token no longer routes input;
- new TCP Action and UDP Move route through the new Session;
- takeover eviction does not enqueue the old normal Leave;
- closing the new owner enqueues its new ownership-fenced Leave;
- transfer failure produces no Welcome for the candidate and leaves the old peer operational.

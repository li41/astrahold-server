# S3-F.17 — Trusted Character Admission Lease / Fencing Seam

## Goal

S3-F.17 adds a process-local trusted-character admission lease that serializes the pre-join restore window and provides a generation fence for later takeover work.

S3-F.14 intentionally used a read-only barrier: multiple inactive candidates could pass admission, run restore work concurrently, and race at the final world-owner join. That remained correct because only one join could commit, but it did not provide a reservation/fencing primitive suitable for later old-session eviction or controlled session handoff.

S3-F.17 supersedes that **no-reservation behavior** for the production trusted tcpudp flow without adding takeover itself.

## Admission lease

`AwaitCharacterAdmission` still enters the existing world-runtime FIFO, preserving the S3-F.14 leave-before-restore ordering. The operation now creates a small process-local reservation when the trusted CharacterID is inactive.

The lease contains only server-internal coordination truth:

- trusted `CharacterID`;
- monotonic process-local `Generation`;
- wall-clock `ExpiresAt` deadline.

It is never sent to the Client, never persisted, and is not a distributed lock.

The default lease lifetime is 60 seconds. The lease is intentionally long relative to the local durable-restore path while still bounding a leaked reservation if a transport path cannot enqueue its explicit release.

## FIFO ordering with leave

The existing ordered reconnect chain remains:

`old leave already enqueued -> world-owner leave capture -> old active binding released -> admission lease issued -> pending Character State flush -> durable load -> reserved join`

If the old session is still active because its leave has not been ordered, admission continues to fail closed with `ErrCharacterIdentityActive`.

S3-F.17 does not evict, close, steal, or replace the active session.

## Single candidate reservation

While a live lease exists for a trusted CharacterID:

- another `AwaitCharacterAdmission` fails with `ErrCharacterAdmissionReserved`;
- a trusted join without the matching lease fails with `ErrCharacterAdmissionLeaseRequired`;
- a join carrying a wrong, stale, mismatched, or expired lease fails closed;
- only the exact current generation may pass the reservation fence.

Therefore only one candidate owns the restore/join admission intent at a time.

This is stronger than S3-F.14's prior behavior where several inactive candidates could all pass the read-only barrier and rely solely on the final active-identity check.

## Join commit and lease consumption

The existing world-owner join transaction remains the active-character ownership commit point.

For a reserved trusted join:

1. validate the lease against the trusted CharacterID and current generation;
2. run the existing join validation/restore/spawn/character/session transaction;
3. bind the active CharacterID to the committed session/entity;
4. consume the matching admission lease;
5. return successful join completion to tcpudp;
6. only then may tcpudp send `SessionWelcome`.

If any join step fails, the reservation is not consumed. The tcpudp caller releases it through the world-owner FIFO.

No existing rollback semantics are weakened.

## Release and stale-generation fencing

`ReleaseCharacterAdmission` is ordered through the same world-owner FIFO.

Release is intentionally idempotent:

- the exact current generation clears its reservation;
- an already-consumed generation is a no-op;
- an expired generation is a no-op;
- an older/stale generation is a no-op and **cannot remove a newer lease**.

That final rule is the fencing prerequisite needed before any later stage can safely introduce old-session eviction or handoff races.

## TTL fallback

Normal tcpudp failure paths explicitly release the lease when restore or join fails.

The reservation also has a 60-second TTL. On a later admission/join check, an expired reservation is discarded before making a decision. This bounds availability damage if explicit release cannot be enqueued, for example under extreme command-queue pressure.

A restore that runs beyond the lease deadline fails closed at reserved join instead of silently committing with an expired ownership intent.

The lease clock is coordination wall time, not gameplay simulation time. It does not affect respawn timers, autosave intervals, combat, or deterministic world state.

## tcpudp flow

For a trusted identity, production tcpudp now performs:

1. server-owned CharacterIdentity resolution;
2. world-owner admission lease acquisition;
3. existing durable Character State restore lookup and validation;
4. peer/session construction;
5. `AwaitJoin` with the exact admission lease;
6. world-owner lease validation + join transaction + lease consumption;
7. mark peer joined;
8. write `SessionWelcome`;
9. mark peer ready and start normal ingress/egress.

A deferred cleanup releases the lease if the handler exits before successful join consumption.

The default ephemeral development identity path does not acquire a lease.

## Existing persistence semantics

S3-F.15 and S3-F.16 remain unchanged.

Admission leasing does not perform disk I/O and does not alter the Character State durability boundary:

`capture -> process-local outbox -> journal append + fsync -> Store CAS -> checkpoint`

Trusted reconnect still depends on existing persistence coordination to flush already-produced save intents before durable restore read.

The autosave RPO behavior from S3-F.16 is unchanged.

## Scaling boundaries

The lease is one small map entry per in-flight trusted admission attempt. It does not add per-tick scans.

Expiration is checked lazily only when the same CharacterID is involved in admission/join coordination. No timer goroutine, global lease sweep, filesystem I/O, network I/O, or distributed coordination is added to the world owner.

S3-F.17 does not change:

- Protocol v6;
- GameV1 codec;
- UDP MTU 1200;
- WorldSnapshot chunking;
- Network LOD or transform batch max 64;
- lifecycle desired-vs-known truth;
- Spawn/Despawn Confirm-after-TrySend behavior;
- lifecycle churn ceiling of 6,000/snapshot;
- Initial Vitals budgets;
- Dirty Vitals ceiling of 4,000/tick;
- Character State autosave capture budget;
- workflow filters or acceptance thresholds.

## Explicit non-goals

S3-F.17 does **not** add:

- seamless reconnect or session takeover;
- old-session eviction or forced close;
- active-session ownership transfer;
- stale old-session leave fencing after an ownership transfer;
- Client-visible lease/token protocol;
- account authentication or Client CharacterID selection;
- distributed leases or cross-worldd coordination;
- synchronous fsync in the world owner;
- zero-window Character State durability;
- journal compaction/rotation;
- inventory/currency/equipment/progression/XP/loot/guild persistence;
- world actor split, quantization, or delta compression.

A later takeover stage must still design active ownership transfer and stale old-session leave/input handling. This stage only supplies the pre-join admission reservation and generation-fencing primitive.

No Go↔Godot E2E claim is made.

## Test coverage

The stage covers:

- active trusted identity still rejects admission;
- an earlier queued leave captures state before the following lease is issued;
- concurrent inactive candidates yield exactly one live admission lease;
- a live lease blocks an unreserved trusted join;
- the matching lease allows join and is consumed only after commit;
- explicit release allows a newer generation;
- stale release cannot clear the newer generation;
- expired reservations can be replaced;
- trusted restore failure releases its lease;
- trusted join failure releases its lease and does not enqueue leave for an uncommitted session;
- existing close-after-join race behavior remains covered.

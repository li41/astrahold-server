# S3-F.14 — Trusted Reconnect Admission Barrier Seam

## Goal

S3-F.14 closes two server-side ordering gaps around the trusted character restore seam without adding session takeover or changing Protocol v6.

Before this stage, a trusted connection could start `CharacterRestoreFactory` before an older `leave_world` command had reached the world owner, so the S3-F.12 persistence coordinator had no new save intent to flush yet. The transport also wrote `SessionWelcome` before the queued world join transaction had actually committed.

This stage adds two process-local world-owner completion boundaries:

1. a read-only trusted character admission barrier before durable restore lookup;
2. a synchronous join completion before `SessionWelcome` is written.

## Admission ordering

`AwaitCharacterAdmission` enters the existing world-runtime command FIFO. It performs no persistence I/O and does not mutate world or session ownership.

If an older connection has already enqueued `leave_world` before the new admission command, FIFO processing guarantees that the world owner handles leave first. The existing leave path captures authoritative trusted character state into the S3-F.11 process-local outbox before releasing the active `CharacterID` binding. Only then can the admission command succeed.

After admission succeeds, transport calls the existing `CharacterRestoreFactory`. The worldd `characterStatePersistence.LoadRestore` coordinator still owns persistence ordering: it flushes save intents that already exist, then reads the durable Store under the same coordinator lock.

Therefore the bounded read-after-leave chain is:

`old leave already enqueued -> world-owner leave capture -> admission succeeds -> pending flush -> durable load -> join`

If the old leave has not been enqueued yet, the old `CharacterID` is still active and admission fails closed with `ErrCharacterIdentityActive`. S3-F.14 never closes, evicts, or steals the old session.

The existing S3-F.11 outbox durability boundary is unchanged. If authoritative leave capture itself fails, for example because the bounded process-local save outbox is full, S3-F.14 does not invent a missing save intent or claim durable freshness. That existing failure remains observable through the world-runtime command error/metrics path.

## No admission reservation

The barrier is intentionally not a long-lived ownership reservation. Two new candidates may both pass admission while a trusted `CharacterID` is inactive.

The actual world join remains the ownership commit point. The existing active `CharacterID` invariant is checked again by `applyJoin`, so serialized world-owner processing allows at most one candidate to commit; later candidates fail closed before spawning a second active entity.

This keeps the stage bounded and avoids introducing session takeover, leases, fencing tokens, or distributed coordination.

## SessionWelcome ordering

The tcpudp transport now waits for the world owner to complete the existing join transaction before it writes `SessionWelcome`.

A failed queue admission, identity validation, restore validation, world spawn, character registration, defeated restore installation, or session registration therefore cannot produce a successful-looking Welcome first.

The existing join rollback remains authoritative for partial world-state failures.

The transport peer distinguishes:

- `joined`: the world-owner join transaction committed and a matching leave is now required on connection teardown;
- `ready`: SessionWelcome was written and realtime ingress may be accepted.

These are separate because a connection can close after world join commits but before Welcome finishes. `closePeer` has an independent one-shot leave path, so a close that races with join completion cannot strand a committed world entity. Realtime UDP remains blocked until `ready` becomes true.

## Cancellation boundary

Admission is read-only, so a caller may stop waiting if its context is cancelled; a later execution of the abandoned barrier cannot mutate world state.

Join is different. Once the mutating join command has been queued, `AwaitJoin` waits for the world-owner result even if the transport context becomes cancelled. Returning early would make the outcome ambiguous and could strand a late successful join without a transport owner.

## Durable defeated restore

S3-F.13 semantics are unchanged. Defeated v2 records still restore immutable death-time context, bound respawn destination, remaining world ticks, and settled checkpoint truth. S3-F.14 only orders admission and join around that existing restore path.

It does not change the S3-F.13 restart timer representation, death penalty semantics, resurrection ordering, or respawn policy validation.

## Protocol and performance boundaries

Protocol v6 is unchanged. No Client change is required.

The admission barrier is one process-local command and performs no storage access on the world tick. Character State disk I/O remains in the existing worldd persistence coordinator outside the world owner.

Lifecycle, Initial Vitals, dirty Vitals, Siege Load Lab, and S3-E scaling thresholds/filters are unchanged.

## Explicit non-goals

S3-F.14 does not add:

- seamless session takeover;
- old-session eviction;
- admission leases or fencing tokens;
- synchronous distributed locks;
- account authentication or Client CharacterID selection;
- SQL, PostgreSQL, MySQL, Redis, Kafka, or distributed persistence;
- autosave or journal compaction;
- inventory, currency, equipment, progression, XP, loot, or guild state;
- world actor split, quantization, or delta compression;
- protocol changes;
- scaling threshold or workflow-filter changes.

## Test coverage

The stage covers:

- active trusted identity admission fails closed;
- an already-enqueued leave is processed and captures its save intent before a following admission succeeds;
- synchronous join returns the actual duplicate-identity failure from the world owner;
- two candidates that both pass the non-reserving barrier still commit at most one active trusted character;
- rejected trusted admission does not run durable restore lookup and does not send SessionWelcome;
- world-owner join failure does not send SessionWelcome;
- a close racing with a late successful join still enqueues exactly one leave;
- existing restore, respawn, persistence, replication, and scaling suites remain regression coverage.

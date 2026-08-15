# S3-F.18 — Trusted Session Ownership Fence Seam

## Goal

S3-F.18 adds the active-session fencing prerequisite required before any future trusted session takeover or old-session eviction policy can be introduced safely.

S3-F.17 serializes trusted admission attempts with a short-lived `CharacterAdmissionLease`, but that lease is consumed when join commits. After join, the existing network commands still identified their sender only by `session.ID`. That is sufficient while ownership never transfers, but it is not sufficient for a later handoff: a delayed command from an older transport owner must be distinguishable from a command issued by the current owner.

This stage therefore introduces an independent, process-local `SessionOwnershipFence` for active trusted tcpudp sessions.

It does **not** transfer ownership, evict an old session, or make reconnect seamless.

## Ownership fence

A `SessionOwnershipFence` contains:

- `SessionID`
- `EntityID`
- trusted `CharacterID`
- monotonic process-local `Epoch`

All four fields must match the world owner's current active ownership record.

The ownership epoch is intentionally independent from the S3-F.17 admission lease generation. Admission reservations and active ownership have different lifetimes and future transition rules; reusing one counter/token as both would couple two correctness boundaries unnecessarily.

The epoch is server-internal and is never serialized into Protocol v6 or exposed to the Client.

## Join ordering

For a trusted join, the world owner allocates the candidate ownership epoch after admission/session validation but before any world mutation.

A failed join may therefore leave an unused epoch gap. Epochs are fencing identities, not a dense sequence, so gaps are harmless.

The ownership fence becomes active only after the existing join transaction has completed its successful world/session ownership binding. The order is:

`admission validation -> ownership epoch allocation -> restore/world/session transaction -> CharacterID bind -> admission lease consume -> ownership fence activate`

`AwaitJoinOwned` returns the committed fence to tcpudp only after the world-owner completion boundary. Ephemeral joins return a zero fence and keep the existing development path.

## Fenced trusted network commands

After a trusted tcpudp join succeeds, the peer stores the immutable ownership fence and swaps from the shared legacy gateway ingress to a peer-bound gateway sink.

The following trusted network commands carry the fence into the existing bounded world-runtime command queue:

- realtime `ClientMoveInput`
- reliable `ClientUseAction`
- connection teardown `leave_world`

The network goroutines never read or mutate active ownership maps. They only copy the immutable fence value into commands. Current ownership validation remains inside the single world owner.

A command is accepted only when its `CharacterID + SessionID + EntityID + Epoch` matches the current ownership record.

## Stale command behavior

A stale fenced command fails closed with `ErrCharacterOwnershipFenceStale` before it can mutate current-owner state.

In particular:

- stale Move does not validate or consume the active session's input sequence and cannot change movement input;
- stale Action is rejected before combat availability, action-sequence validation, `MarkProcessedAction`, or gameplay dispatch;
- stale Leave is rejected before session removal, Character State save capture, replication cleanup, identity removal, character removal, or world entity removal.

This ordering is the central correctness seam for a later handoff stage. Once ownership transfer exists, an older peer can be disconnected asynchronously without allowing its delayed ingress or teardown to destroy the newer owner.

S3-F.18 itself does not create that newer owner or perform the transfer.

## Current leave behavior

A current trusted tcpudp peer closes through `EnqueueFencedLeave` using its exact active fence. The existing authoritative leave order remains unchanged after the fence check:

`validate current ownership -> remove session -> authoritative Character State capture -> ownership cleanup -> autosave bookkeeping cleanup -> replication/vitals/gameplay cleanup -> CharacterID release -> world entity removal`

The existing S3-F.11/F.15/F.16 persistence boundaries are therefore unchanged.

## Defensive ownership cleanup

Ownership cleanup is generation-fenced defensively. Removing ownership by an older session deletes the CharacterID ownership record only if that record still exactly matches the same fence.

No F.18 production path installs a newer owner while an old owner remains active; this defensive rule is preparation for a later bounded transfer stage rather than a hidden takeover implementation.

## Legacy in-process APIs

Existing transport-neutral/in-process APIs remain available:

- `EnqueueLeave(session.ID)`
- `EnqueueMove(session.ID, ...)`
- `EnqueueUseAction(session.ID, ...)`

Commands produced by those legacy APIs contain a zero ownership fence and preserve their existing behavior. This avoids forcing load tools, unit fixtures, and non-network runtime embeddings into tcpudp ownership semantics.

The S3-F.18 stale-owner guarantee therefore applies to the F.18-aware trusted tcpudp network path and to callers that explicitly use the new fenced APIs. It does not claim that arbitrary legacy in-process callers are cryptographically or structurally fenced.

## No distributed ownership claim

The epoch counter and ownership maps are process-local to one `worldruntime.Runtime`.

S3-F.18 does not add:

- database-backed leases;
- Redis/etcd/ZooKeeper coordination;
- cross-worldd fencing;
- crash-persistent ownership epochs;
- a distributed lock or consensus protocol.

This matches the existing single-worldd/single-writer Character State ownership boundary.

## Persistence and autosave boundaries

S3-F.15 journal and S3-F.16 autosave semantics are unchanged.

A current fenced Leave still captures authoritative trusted Character State into the existing process-local outbox before world cleanup. Durability still begins only after the existing journal append + fsync path.

This stage does not close the remaining process-local capture-to-journal crash window and does not claim zero-window durability.

## Protocol and scaling boundaries

Protocol v6 is unchanged. No Client change is required.

The fence adds four small scalar values to selected in-process commands only; it does not change network packet format or UDP size.

No filesystem or network I/O is added to the world owner.

The following existing constraints remain unchanged:

- lifecycle truth remains desired vs known;
- Spawn/Despawn confirmation still occurs only after successful `TrySend`;
- snapshot absence is not despawn;
- self correction remains `PositionCorrection`;
- lifecycle churn ceiling remains 6,000 messages/snapshot;
- Initial Vitals budgets are unchanged;
- dirty gameplay Vitals remains capped at 4,000/tick;
- Network LOD remains unchanged;
- transform batch remains max 64;
- UDP MTU remains 1200;
- `WorldSnapshot` remains chunked;
- no world actor split, quantization, or delta compression is added;
- workflow filters and acceptance thresholds are unchanged.

## Explicit non-goals

S3-F.18 does not add:

- trusted active-session takeover;
- old-session eviction;
- ownership transfer to a second live session;
- reconnect retry/wait policy;
- admission of a new trusted candidate while the old CharacterID is active;
- seamless transport handoff;
- Client-visible ownership epochs;
- protocol changes;
- distributed ownership or persistence;
- synchronous filesystem acknowledgement in the world tick;
- journal compaction/rotation;
- broader inventory/equipment/currency/progression persistence.

A later stage may use this fence as the correctness prerequisite for bounded active ownership transfer, but that policy is deliberately not part of F.18.

## Test coverage

S3-F.18 adds coverage for:

- a committed trusted `AwaitJoinOwned` returning and activating a valid ownership fence;
- stale fenced Move being rejected without consuming input sequence;
- stale fenced Action being rejected before action-sequence consumption;
- stale fenced Leave being rejected without removing the session or entity;
- a current fenced Leave retaining the existing cleanup behavior;
- trusted tcpudp TCP Action, UDP Move, and teardown Leave all carrying the exact same ownership fence;
- existing F.17 admission/restore/join failure tests remaining on their intended failure path after tcpudp switches to `AwaitJoinOwned`.

No Go↔Godot E2E claim is made. The transport regression remains a Go tcpudp test, not a Godot client integration test.

# S3-F.19 Trusted Active Ownership Transfer Seam

## Scope

S3-F.19 adds the smallest world-owner primitive needed after S3-F.18 before tcpudp can safely implement active trusted-session takeover. It does **not** make reconnect seamless yet.

The stage introduces an optimistic compare-and-swap ownership transfer for an already-active trusted CharacterID. The existing authoritative world Entity and gameplay state remain in place; only the owning Session and ownership epoch change.

## Why this stage exists

S3-F.18 made old-session Move, Action, and Leave commands generation-fenced, but deliberately did not provide an operation that advances active ownership. Without an atomic handoff primitive, a transport takeover implementation would have to compose session removal, replacement registration, ownership advancement, and replication cleanup outside the world owner. That would reopen the stale-owner races S3-F.18 was designed to prevent.

S3-F.19 therefore creates one world-owner FIFO boundary:

`old-owner commands before transfer -> atomic ownership transfer -> old-owner commands after transfer become stale`

## Ownership lookup

`AwaitCharacterOwnership` inserts a world-owner read barrier and returns the exact active `SessionOwnershipFence` for a trusted CharacterID.

The returned fence is an optimistic CAS expectation, not a reservation or lock. A later transfer must still match that exact `SessionID + EntityID + CharacterID + Epoch`.

Only trusted identities may use this seam. Inactive CharacterIDs fail closed.

## Atomic transfer

`AwaitOwnershipTransfer(expected, replacement)` queues a mutating world-owner command.

A transfer succeeds only when:

- `expected` is a valid current S3-F.18 ownership fence;
- the replacement Session is valid and trusted;
- replacement CharacterID exactly matches the expected CharacterID;
- replacement EntityID exactly matches the current EntityID;
- replacement SessionID is different and not already registered;
- the expected active Session, world Entity, and character state still exist;
- ownership has not advanced since the caller obtained `expected`.

Two candidates that race with the same expected fence cannot both win. The first committed transfer advances the ownership epoch; later attempts using the older fence fail with the existing stale-ownership error.

## State preserved across transfer

The transfer does not despawn or recreate the Entity. It preserves all EntityID-scoped authoritative state already owned by the world runtime, including:

- Transform and world presence;
- HP / MaxHP / Defeated state;
- pending respawn policy truth;
- revive protection and death state;
- combat/entity-scoped gameplay state;
- CharacterID <-> EntityID binding;
- Character State autosave baseline and persistence ownership.

No Character State save is emitted merely because Session ownership changes. No durable restore is performed.

This avoids replacing newer in-memory authoritative state with an older durable snapshot during an active handoff.

## Session-scoped state reset

A replacement connection must bootstrap its own delivery truth. On successful transfer S3-F.19:

- removes the old Session from the Session registry;
- adds the replacement Session using the same EntityID;
- removes old Session replication ownership;
- removes old Session Vitals delivery state;
- clears old Session dynamic-state revision progress;
- registers fresh replication state for the replacement Session;
- mints and activates a newer ownership epoch;
- removes the old by-session ownership mapping without clearing the newer by-character mapping.

Input/action sequence trackers are Session-local, so the replacement Session begins with fresh sequence state.

## Movement boundary

The old Session's last held movement input must not continue moving the character after control transfers. S3-F.19 therefore writes zero movement input at the handoff boundary before the simulation phase.

FIFO semantics remain explicit: an old-owner Move command ahead of the transfer is processed normally, but the transfer clears its held movement intent before that tick's simulation. An old-owner Move behind the transfer is rejected by the S3-F.18 ownership fence.

## Old transport behavior

S3-F.19 intentionally does **not** close or evict the old network peer from inside the world owner.

After transfer:

- the old Session no longer receives world replication;
- old fenced Move / Action / Leave commands fail as stale;
- old Leave cannot save or delete the replacement owner's authoritative state;
- the old transport connection may remain physically open until a later transport integration stage closes it.

Keeping transport eviction out of this stage lets the world-owner correctness primitive be tested independently from tcpudp peer lifecycle races.

## Durability relationship

S3-F.15 journal and S3-F.16 autosave semantics are unchanged. Active handoff neither strengthens nor weakens their durability boundary.

The authoritative in-memory Entity stays active, so no leave snapshot is required for handoff correctness. A later real disconnect of the current owner still uses the existing fenced leave -> Character State capture path.

## Scaling / performance

The transfer command performs bounded map lookups, one Session add/remove, replication registry reset for two Session IDs, and one movement-input reset. It performs no filesystem I/O and no durable-store access in the world owner.

No scaling budget is raised:

- lifecycle churn ceiling remains 6,000/snapshot;
- Dirty gameplay Vitals remains 4,000/tick;
- Initial Vitals budgets are unchanged;
- Network LOD is unchanged;
- transform batch max64 is unchanged;
- UDP MTU 1200 is unchanged;
- WorldSnapshot chunking is unchanged.

## Tests

Focused coverage verifies:

- active trusted ownership lookup returns the exact current fence;
- transfer preserves Entity and character state while advancing ownership epoch;
- old transport is not closed by the world-owner transfer primitive;
- old held movement intent is cleared before simulation;
- two transfers racing with the same expected fence produce exactly one winner;
- a stale old Leave after transfer cannot remove the replacement Session or Entity;
- mismatched replacement EntityID fails without mutating ownership.

The existing S3-F.18 stale Move / Action / Leave tests continue to cover the command-side fence behavior that S3-F.19 now advances through a real transfer operation.

## Explicit non-goals

S3-F.19 does not implement:

- tcpudp active takeover routing;
- automatic old-peer close/eviction;
- SessionWelcome behavior for a replacement peer;
- reconnect retry/backoff policy;
- takeover authorization policy, cooldown, rate limiting, or operator controls;
- an active handoff reservation/lease beyond optimistic expected-fence CAS;
- Client-visible ownership epoch;
- Protocol v6 changes;
- distributed or multi-worldd ownership fencing;
- persistence schema changes;
- synchronous fsync or zero-window durability;
- broader gameplay-state persistence;
- world actor split, quantization, or delta compression.

No Go <-> Godot E2E claim is made. The Client is unchanged.

## Next bounded stage

With S3-F.19 in place, a later stage can wire trusted tcpudp reconnect to:

1. resolve an already-active trusted CharacterID;
2. obtain its current ownership fence and EntityID;
3. construct the replacement Session against that same EntityID;
4. atomically transfer ownership;
5. bind the returned new fence to the replacement peer;
6. close/evict the old peer outside the world owner;
7. send SessionWelcome only after the transfer is committed.

That transport stage must preserve the F.18 publication ordering rule: the replacement peer's immutable ownership and ingress must be initialized before it is published as joined/ready.

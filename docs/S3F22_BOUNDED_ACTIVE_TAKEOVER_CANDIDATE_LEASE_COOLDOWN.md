# S3-F.22 — Bounded Active Takeover Candidate Lease & Cooldown Seam

## Scope

S3-F.22 adds a process-local transport policy gate above the S3-F.21 takeover authorizer and below the S3-F.20 trusted connection-plan seam.

The stage prevents one trusted `CharacterID` from running multiple active takeover candidates through authorization at the same time, and adds a short exact-owner cooldown after a successful ownership transfer.

This is an anti-thrash policy seam. It is not an ownership authority and does not replace S3-F.19 compare-and-swap fencing.

## Why this stage exists

S3-F.21 made active takeover fail closed unless an upstream `CharacterTakeoverAuthorizer` explicitly approves the candidate. However, two TCP candidates can still observe the same F.20 active ownership fence and both enter an expensive authorization callback concurrently.

Even though S3-F.19 guarantees that only a matching expected ownership fence can commit a transfer, repeated approved candidates could also replace successive owners rapidly.

S3-F.22 bounds those transport-level behaviors without adding Client credentials or changing Protocol v6.

## Candidate lease

For an active trusted takeover attempt, the transport now acquires one process-local candidate lease keyed by `CharacterID` before calling the S3-F.21 authorizer.

The lease contains:

- trusted CharacterID;
- candidate SessionID;
- the exact expected S3-F.18/S3-F.19 ownership fence returned by the F.20 connection plan;
- a strictly increasing process-local generation;
- an expiry timestamp.

Default candidate TTL is 10 seconds through `DefaultConfig`. A non-positive configured TTL is normalized back to that bounded default.

Only one unexpired candidate lease may exist for one CharacterID. Different CharacterIDs remain independent.

## Ordering

For the active trusted path:

1. Resolve trusted CharacterID.
2. F.20 `AwaitCharacterConnectionPlan` returns the exact current ownership fence.
3. S3-F.22 attempts to acquire the per-CharacterID candidate lease.
4. If another candidate lease is still active, reject the new candidate before calling the S3-F.21 authorizer.
5. If a matching exact-owner cooldown is still active, reject before authorization.
6. Call the S3-F.21 authorizer.
7. Build the replacement transport Session only after authorization succeeds.
8. Revalidate the exact candidate lease immediately before F.19 `AwaitOwnershipTransfer`.
9. F.19 exact-fence CAS remains the final authority for transfer.
10. After a successful and validated transfer, commit the candidate lease into a cooldown bound to the exact newly returned ownership fence.
11. Publish the new peer ownership and retain the existing F.20 old-peer eviction ordering.

Denied authorization, invalid PlayerFactory/session setup, failed candidate validation, and failed ownership transfer do not create cooldown.

## Generation fencing and lazy expiry

Candidate expiry is lazy: the gate removes an expired lease when another acquire/validate/commit touches the CharacterID.

When an expired lease is replaced, the new candidate gets a newer generation. Release and commit operations must match the exact current generation, candidate SessionID, CharacterID, and expected ownership fence.

Therefore a late release or commit from an old candidate cannot clear, overwrite, or convert a newer candidate lease.

If a slow authorizer outlives its TTL and a replacement candidate is admitted, the slow candidate fails candidate validation before reaching F.19 transfer.

If a transfer call itself were to remain blocked long enough for the lease to expire after the pre-transfer validation, S3-F.19 still protects ownership correctness. A stale candidate-gate commit may fail, but the already-committed authoritative ownership is never rolled back.

## Cooldown

A successful transfer converts the exact current candidate lease into a process-local cooldown.

`DefaultConfig` uses a 2-second cooldown. Setting `CharacterTakeoverCooldown` to zero disables cooldown while keeping candidate serialization; negative values normalize to zero.

Cooldown is bound to the exact new `SessionOwnershipFence`, not only CharacterID or wall-clock time.

A new active takeover request is blocked only when its F.20 expected ownership fence exactly matches the cooldown owner. If the character has moved to a different ownership fence through another valid lifecycle transition, the older cooldown is treated as stale and is cleared rather than blocking the newer owner.

This prevents stale timers from fencing a future legitimate ownership generation.

## Failure behavior

- Existing active candidate lease: candidate closes before authorization, old owner unchanged.
- Active cooldown for exact current owner: candidate closes before authorization, old/current owner unchanged.
- Authorizer denial: candidate lease is released, no cooldown.
- Candidate TTL expiry after authorization: candidate closes before transfer.
- F.19 transfer rejection: candidate lease is released, no cooldown, old owner remains authoritative.
- Successful transfer: cooldown is installed for the exact returned new owner.
- Candidate-gate bookkeeping failure after successful F.19 transfer is telemetry only; it cannot roll back authoritative ownership.

## Concurrency semantics

Two candidates may still receive the same active fence from F.20 before either acquires the S3-F.22 lease. Only one can own the current candidate generation and enter authorization.

If the first candidate expires, a later candidate can replace the lease with a newer generation. The old candidate cannot release or commit the newer lease.

S3-F.22 is intentionally a transport-level policy gate. S3-F.19 remains the final correctness primitive for all ownership races.

## Scaling boundary

The candidate gate is a small mutex-protected process-local map keyed by trusted CharacterID and is touched only for active takeover attempts.

It adds no world-owner command, no per-tick work, no per-snapshot work, no filesystem I/O, and no network I/O to simulation.

It is not a general DDoS or connection-rate limiter.

## Preserved boundaries

S3-F.22 does not change:

- Protocol v6;
- Client code;
- GameV1 codec;
- SessionWelcome schema;
- UDP MTU 1200;
- WorldSnapshot chunking;
- Network LOD / transform batch max64;
- lifecycle desired-vs-known truth;
- Spawn/Despawn Confirm-after-`TrySend` semantics;
- lifecycle churn max 6,000/snapshot;
- Initial Vitals budgets;
- dirty gameplay Vitals max 4,000/tick;
- F.17 admission lease semantics;
- F.18 ownership fence semantics;
- F.19 ownership transfer transaction;
- F.20 old-peer eviction ordering;
- F.21 explicit takeover authorization semantics;
- Character State persistence formats or autosave behavior;
- process-local ownership epoch semantics.

## Explicit non-goals

- no Client reconnect credential or proof;
- no account/session implementation;
- no distributed candidate lease;
- no cross-worldd cooldown;
- no persistent cooldown;
- no IP/device authentication;
- no general connection rate limiter;
- no automatic transfer retry after stale CAS;
- no Protocol v7 change;
- no Client change;
- no Go↔Godot end-to-end claim.

## Focused acceptance tests

Tests cover:

- only one in-flight candidate per CharacterID;
- different CharacterIDs remain independent;
- TTL expiry mints a newer generation;
- stale release cannot clear a replacement lease;
- stale commit cannot overwrite a replacement lease or install cooldown;
- successful commit installs cooldown for the exact new owner;
- cooldown expires lazily;
- cooldown tied to an older ownership fence cannot block a different newer ownership fence;
- a second concurrent TCP candidate is rejected before the authorizer while the first candidate is blocked in authorization;
- immediate re-takeover after success is rejected before authorization and leaves the current owner routing Actions;
- failed F.19 transfer does not create cooldown and releases the candidate path for retry;
- existing S3-F.20/S3-F.21 takeover, authorization, fencing, and old-peer eviction regressions remain covered.

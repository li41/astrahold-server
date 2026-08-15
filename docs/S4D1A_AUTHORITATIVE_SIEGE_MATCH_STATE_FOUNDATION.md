# S4-D.1A — Authoritative Siege Match State Foundation

## Scope

This bounded Server stage introduces the authoritative siege match state machine that later throne capture and siege resolution stages will extend.

It deliberately keeps Protocol v6 unchanged and does not modify the Godot Client.

## Authority model

`internal/siege.Service` remains the single siege gameplay authority. S4-D.1A adds:

- explicit attacker and defender side identities;
- explicit per-entity attacker/defender team assignment owned by Server state;
- a revisioned match state;
- `Gate` and `Throne` phases, with `Completed` reserved for the later resolution stage;
- breach-gate and throne-objective identities;
- an idempotent authoritative gate-breach transition.

Client input never advances siege phase. `worldruntime` first applies Gate damage through the existing authoritative combat + siege Gate transaction. Only the resulting `GateState{Destroyed:true}` is observed by the match state machine. Destroying the configured breach gate advances `Gate -> Throne` once.

## WorldRuntime seam

`WithSiegeMatch` configures a match after `WithSiegeGates` during Runtime construction. Invalid static match definitions fail fast. `Runtime.SiegeMatchState()` exposes a read-only snapshot for future replication and observability without exposing mutable match internals.

The existing Gate dynamic revision remains responsible only for blocker/Gate replication. S4-D.1A does not overload it with siege-match semantics.

## Protocol boundary

Protocol v6 has no Server -> Client siege match/objective message. Existing Client framing requires an exact protocol version and the Reliable JSON decoder accepts only known typed message contracts. Therefore S4-D.1A does not add a hidden or partially compatible wire field.

A later S4-D.1B Reliable Siege State Replication stage should define an explicit message contract and bump the protocol only if that new wire contract is introduced. This is independent of remote Attack animation and is not a reason to add ActionStarted/ActionImpact/DamageEvent in this stage.

## Preserved invariants

S4-D.1A does not change:

- Protocol v6 or GameV1 wire layout;
- UDP MTU 1200;
- WorldSnapshot chunking;
- Network LOD / transform batch max64;
- lifecycle desired-vs-known truth;
- Spawn/Despawn Confirm-after-TrySend semantics;
- lifecycle churn max 6,000/snapshot;
- Initial Vitals budgets;
- dirty gameplay Vitals max 4,000/tick;
- gameplay proxy movement/blocker/LOS authority;
- Client gameplay authority boundaries.

## Explicit non-goals

- no throne capture zone;
- no capture progress;
- no contest or interrupt/reset policy;
- no winner or siege-completed transition;
- no castle ownership persistence/transition;
- no Client HUD/presentation;
- no remote Attack/Cast presentation events;
- no Go Server <-> Godot Client E2E claim.

## Acceptance coverage

Focused tests cover:

- valid initial authoritative match state;
- invalid match definitions and unknown breach gates;
- explicit attacker/defender participant assignment;
- live or unrelated Gates not advancing phase;
- authoritative breach advancing `Gate -> Throne` exactly once;
- worldruntime generic Gate action integration advancing match state only after the existing authoritative Gate/blocker transaction succeeds.

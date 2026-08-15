# S4-D.4A — Siege Result & Castle Ownership Reliable Contract

## Scope

S4-D.4A exposes the Siege result and current durable castle owner to the Godot Client through the existing Reliable `SiegeMatchState` message.

This is an intentional wire-incompatible contract change, so the protocol version advances from **v7 to v8**. Message type 106 remains `SiegeMatchState`; realtime binary layouts remain unchanged.

## New v8 fields

`SiegeMatchState` now includes:

- `round`: Server-authoritative gameplay round identity;
- `winner_team`: `unknown`, `attacker`, or `defender`;
- `winner_id`: stable configured side ID of the winner, empty before completion;
- `castle_owner_id`: current durable castle owner side ID.

The existing `revision` remains the per-match Reliable resend revision. It is not the same thing as `round` and must not be used as a round ID.

## Authoritative sources

The WorldRuntime adapter copies these fields directly from existing Server-owned domain state:

- `round` from `siege.MatchState.Round`;
- `winner_team` and `winner_id` from `siege.MatchState`;
- `castle_owner_id` from `siege.CastleOwnershipState.OwnerID`.

No Client message, visual state, Gate mesh, local timer, or inferred capture progress can write these values.

## Phase semantics

For the current MVP capture lifecycle:

- Gate / Throne: winner is unknown/empty and the durable castle owner is the current defender;
- Completed: winner is the authoritative completed Match winner and the durable castle owner is the winner after D.3B's durability barrier succeeds;
- next-round reset: D.3C rotates the durable owner into defender, clears winner state, advances `round`, and existing Reliable replication publishes the fresh Gate round.

This wire contract can represent a future defender victory because `winner_team` supports both roles; D.4A does not add a defender-timeout victory policy.

## Reliable delivery

D.4A reuses the existing per-session Siege delivery stamp:

- Match revision + recipient team remains the resend key;
- the stamp advances only after `TrySend` succeeds;
- D.3D still consumes the same delivery result for Completed hold scheduling;
- no second session scan is added.

Ownership changes occur inside the same authoritative completion transition that advances the Match revision, so the v8 full snapshot carries the matching winner + owner truth together.

## Preserved boundaries

D.4A does not change:

- Message type 106;
- Reliable delivery ordering;
- GameV1 realtime binary payloads;
- UDP MTU 1200;
- WorldSnapshot chunking;
- Network LOD transform max64;
- D.3B durable ownership store schema or fsync/CAS barrier;
- D.3C round reset and role rotation;
- D.3D 2s / 10s Completed scheduling policy;
- Siege config schema 3 / `s4d2b-001`;
- Gameplay World schema 2 / `s3d-001`;
- lifecycle / Vitals scaling budgets.

## Explicitly deferred

- capture progress/timer on the wire;
- defender timeout victory;
- multi-process/distributed Siege authority;
- mid-round Gate/phase/capture persistence;
- true Go Server ↔ Godot multi-client S4-E E2E.

## Acceptance coverage

Server tests cover:

- strict JSON round-trip of v8 Gate/Throne result fields;
- strict JSON round-trip of Completed winner + owner fields;
- rejection of unknown JSON fields;
- WorldRuntime Reliable view publishing round 1 / defender ownership before completion;
- WorldRuntime Reliable view publishing attacker winner + transferred ownership after authoritative throne resolution.

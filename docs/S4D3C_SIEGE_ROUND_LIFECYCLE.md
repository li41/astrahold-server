# S4-D.3C — Authoritative Siege Round Lifecycle & Ownership-Derived Role Rotation

## Scope

S4-D.3C turns the one-shot completed Siege from D.3A/D.3B into a repeatable Server-authoritative round lifecycle without changing the Client or Protocol v7.

This stage deliberately separates **round reset mechanics** from **round scheduling policy**. A completed round remains Completed until a Server scheduler/management layer explicitly enqueues the next-round command. The Client has no reset command and cannot decide when a new round starts.

## Durable round epoch

`MatchState` gains an internal `Round` epoch. It remains off-wire in Protocol v7.

The durable castle-ownership revision is the round epoch source:

- a first/default ownership record at revision 1 starts round 1;
- a successful attacker capture commits ownership revision `N+1` while the just-completed Match remains round `N`;
- starting the next round requires ownership revision to equal exactly `Round+1`;
- the reset then advances `Round` to that durable revision.

On process startup, `ConfigureCastleOwnershipPersistence` restores `Round` directly from the durable ownership revision. This prevents a restarted process from pretending a durable revision-9 castle is back in round 1.

## Ownership-derived roles

The checked-in Siege model has exactly two configured side IDs. D.3C treats those IDs as stable sides while attacker/defender are per-round roles.

For every fresh round:

- durable `OwnerID` is the DefenderID;
- the other configured side is the AttackerID.

If durable startup ownership is the side originally named as `attacker_id` in config, the fresh runtime roles are rotated immediately before gameplay starts.

The D.2B participant roster schema is unchanged. When roles rotate, both:

- trusted CharacterID roster assignments; and
- already-connected entity team assignments

are flipped attacker ↔ defender together. A trusted reconnect therefore receives the role appropriate to its stable side in the current round instead of the startup role from the JSON file.

No Client-provided field, join order, EntityID, IP address, or transport property influences this rotation.

## Current-round capture semantics

Throne capture now resolves against the **current MatchState attacker**, not the static startup `MatchDefinition.AttackerID`.

Therefore a second round can be won by the side that was the original defender after ownership/roles rotated. Durable ownership follows the current attacker exactly as it did for round 1.

## Explicit next-round reset

`siege.Service.StartNextRound` is valid only from `MatchPhaseCompleted` and only when:

1. current durable ownership is valid and belongs to one configured side;
2. durable owner equals the just-completed winner;
3. durable ownership revision is exactly the completed round epoch + 1;
4. the configured breach gate and blocker are available.

All owner/role/epoch validation occurs before the first mutable world-side reset.

A successful reset performs one world-owner transition:

- re-enable the configured breach-gate blocker when needed;
- restore the configured breach gate to full HP;
- clear legacy per-gate attack cooldown entries for that gate;
- reset Throne presence to inactive/empty;
- reset Throne capture progress to zero and clear `ReadyForResolution`;
- set phase back to Gate;
- clear `GateBreached`, winner team and winner ID;
- rotate attacker/defender roles from durable ownership;
- advance internal Round to the durable ownership revision;
- advance Match revision exactly once.

A repeated reset request after the Match is already back in Gate is a no-op.

D.3C intentionally resets only the configured authoritative breach gate. It does not invent a multi-gate round transaction for future optional gates.

## WorldRuntime queue boundary

`WorldRuntime.EnqueueStartNextSiegeRound` is a Server-only bounded command-queue seam. The caller only enqueues intent; Gate, blocker, Match, roster and objective mutation still happen on the single world-owner `Step` path.

When a reset succeeds, WorldRuntime bumps the existing Dynamic revision once so Reliable `WorldDynamicState` can resend the restored blocker + Gate HP. The Match revision/team changes independently make existing Reliable `SiegeMatchState` replication resend the new Gate phase and current roles.

No network handler exposes this command to the Client.

## Why D.3C does not auto-start the next round

Protocol v7 has `phase=completed`, but no explicit round/result contract. `SiegeMatchState` delivery stamps advance only after successful `TrySend`.

If the Server reset immediately after completion, a backpressured session that had not yet accepted the Completed state could receive the later Gate state and never observe completion. Conversely, waiting forever for every active client would let one unhealthy connection stall global gameplay.

D.3C therefore keeps Completed stable and establishes the authoritative reset seam only. A later scheduling/policy stage can define delay, delivery acknowledgement/disconnect rules, maintenance windows, or administrative control without coupling those policy choices to the Siege domain reset transaction.

## Preserved boundaries

D.3C does not change:

- Protocol v7 or Message 106 shape;
- GameV1 codec;
- Client repository;
- Gameplay World schema 2 / `castle-sandbox` revision `s3d-001`;
- Siege config schema 3 / revision `s4d2b-001`;
- 10-second configured throne capture duration;
- durable ownership store schema v1;
- ownership CAS/fsync barrier from D.3B;
- UDP MTU 1200;
- WorldSnapshot chunking;
- Network LOD transform max64;
- lifecycle desired-vs-known / confirm-after-`TrySend` semantics;
- lifecycle churn max 6000/snapshot;
- Initial Vitals budgets;
- dirty Vitals max 4000/tick.

## Explicitly deferred

- automatic next-round timer/scheduler policy;
- distributed/multi-process round coordination;
- defender timeout victory;
- Gate/phase/capture-progress persistence across a mid-round restart;
- winner/owner/round/capture wire fields;
- Godot Siege result HUD;
- Protocol v8.

## Acceptance coverage

Focused tests cover:

- durable startup ownership becoming the fresh-round defender;
- trusted roster role alignment after startup rotation;
- first-round capture and durable owner change;
- explicit Completed → Gate reset;
- exact durable epoch fencing before reset;
- main Gate HP and blocker restoration;
- Throne presence/capture reset;
- existing entity team rotation;
- trusted reconnect role rotation;
- no-op repeated reset;
- a second-round win by the newly rotated attacker;
- queue-based WorldRuntime reset and Dynamic revision advance.

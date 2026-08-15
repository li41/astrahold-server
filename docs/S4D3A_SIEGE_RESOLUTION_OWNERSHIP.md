# S4-D.3A — Authoritative Siege Resolution & Castle Ownership Foundation

## Scope

S4-D.3A consumes the Server-authoritative D.2B throne capture readiness latch and completes the first bounded siege resolution transaction.

The stage is intentionally Server-only. It does not add winner/ownership fields to Protocol v7 and does not change the Godot Client.

## Resolution rule

The current MVP ruleset has exactly one configured completion path:

1. the authoritative breach gate is destroyed;
2. the match advances Gate -> Throne;
3. D.2A derives attacker/defender throne presence from Server world state;
4. D.2B accumulates continuous attacker capture time while capture is eligible;
5. when `ThroneCaptureState.ReadyForResolution` becomes true, D.3A resolves the match in the same world-owner step.

D.3A does not invent a siege timeout, defender timer, score comparison, client vote, or arbitrary defender-win path. Those rules do not exist in the repository today.

## Match completion

`siege.Service.ResolveThroneCapture` is the only D.3A completion seam.

It succeeds only when:

- a match is configured;
- a throne capture runtime exists;
- the match is currently in `MatchPhaseThrone`;
- no winner has already been recorded; and
- `ThroneCaptureState.ReadyForResolution` is true.

A successful resolution performs one world-owner transaction:

- Match phase: `Throne -> Completed`
- Winner team: `TeamAttacker`
- Winner ID: configured `AttackerID`
- Match revision increments exactly once
- Castle ownership transfers from configured `DefenderID` to configured `AttackerID`
- Ownership revision increments exactly once
- `PreviousOwnerID` records the defender
- `LastTransferMatchID` records the resolving match

Repeated resolution attempts are no-ops and do not churn any revisions.

## Castle ownership state

D.3A introduces process-local `CastleOwnershipState`:

- `Revision`
- `OwnerID`
- `PreviousOwnerID`
- `LastTransferMatchID`

At match configuration, the defender is the initial authoritative castle owner with ownership revision 1. This follows the existing attacker/defender match model: the defender is the side defending the castle at match start.

When the attacker completes the throne capture, ownership revision advances to 2 and the attacker becomes owner.

This is a foundation only. Ownership is not yet persisted across process restart or distributed across servers.

## Objective settlement

Resolution settles throne objective activity in the same call:

- throne presence becomes inactive and clears counts/contest/eligibility;
- throne capture becomes inactive;
- capture progress remains capped at its required duration;
- `ReadyForResolution` remains latched as historical completion evidence.

A later tick therefore does not repeatedly complete the match or mutate ownership.

## WorldRuntime ordering

`worldruntime.Runtime.Step` keeps the D.2B post-simulation order:

1. simulation completes;
2. one stable Siege session list is built;
3. authoritative throne presence is observed;
4. Server-clock capture delta is applied;
5. D.3A resolution is attempted immediately;
6. normal dynamic and Siege reliable replication runs.

Because resolution happens before Siege replication, an existing Protocol v7 client may receive the already-defined `SiegeMatchState.phase = completed` in that same tick. Winner and owner details remain off-wire.

## Protocol boundary

Protocol v7 remains unchanged.

D.3A does not modify:

- protocol version 7;
- Message 106 `SiegeMatchState` shape;
- GameV1 wire codec;
- realtime Move / Snapshot / Correction layouts;
- UDP MTU 1200;
- WorldSnapshot chunking.

`MatchState.WinnerTeam`, `MatchState.WinnerID`, and `CastleOwnershipState` are Server domain state only.

A future Client winner/ownership HUD contract must be designed explicitly. It must not silently add fields to the strict v7 JSON contract.

## Gameplay World boundary

`worlds/castle-sandbox/gameplay.json` remains authoritative for movement surfaces, blockers, gate geometry, portals, and LOS.

D.3A does not change Gameplay World schema/revision or turn visual assets into gameplay truth.

## Persistence boundary

Castle ownership is process-local in D.3A.

This stage deliberately does not add:

- durable ownership storage;
- restart recovery;
- distributed ownership consensus;
- multi-world/castle ownership IDs;
- next-match scheduling;
- defender victory timeout policy.

Those need separate authority and persistence decisions instead of being hidden inside the match completion method.

## Scaling boundary

Resolution is O(1) and happens at most once per match.

D.3A adds no extra session enumeration, AOI query, snapshot candidate scan, lifecycle work, or network message type.

Existing ceilings remain unchanged:

- lifecycle churn max 6,000/snapshot;
- Initial Vitals budgets unchanged;
- Dirty Vitals max 4,000/tick;
- Network LOD transform batch max64;
- UDP MTU 1200.

## Focused validation

Tests cover:

- defender is the initial castle owner;
- Gate phase cannot resolve;
- Throne phase below capture threshold cannot resolve;
- ready attacker capture completes the match;
- winner is the configured attacker;
- castle ownership transfers defender -> attacker;
- match and ownership revisions advance exactly once;
- presence/capture become inactive at completion while ready remains latched;
- repeated resolution is idempotent;
- WorldRuntime resolves on the same post-simulation tick that reaches the capture threshold;
- a later completed tick does not churn match/ownership/capture revisions.

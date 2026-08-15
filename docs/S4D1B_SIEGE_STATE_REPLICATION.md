# S4-D.1B — Siege Match Configuration & Reliable Replication Contract

## Scope

S4-D.1B turns the S4-D.1A in-process siege match state into a configured worldd feature and an explicit Server -> Client Reliable contract.

This is a coordinated Server/Client protocol stage. It intentionally does not add throne capture progress, winner resolution, castle ownership mutation, Client HUD, or remote Attack presentation events.

## Why Protocol v7

Protocol v6 has no message that can truthfully carry attacker/defender identity, recipient team, siege phase, breach gate identity, or throne objective identity. Both frame decoders require an exact protocol version and both Reliable JSON codecs accept only known typed messages.

Adding `SiegeMatchState` is therefore wire-incompatible. S4-D.1B increments the protocol to v7 specifically for this new authoritative Siege State contract. The compact GameV1 realtime layouts for Move / WorldSnapshot / PositionCorrection are unchanged.

## Siege configuration

`config/siege-match.json` is a separate strict config, not part of Gameplay World schema v2.

Default castle-sandbox values:

- config schema: 1
- revision: `s4d1b-001`
- match: `castle-sandbox-siege`
- attacker identity: `attackers`
- defender identity: `defenders`
- breach gate: `main-gate`
- throne objective: `throne`

worldd loads it through `-siege-match` and validates the configured breach gate against the authoritative Gameplay World gate definitions before Runtime construction. Gameplay geometry, blocker, Layer, movement and LOS truth remain in `gameplay.json` / Gameplay Navigator.

## Reliable message

Message type 106 is `SiegeMatchState` and is `ReliableOrdered`.

Fields:

- `revision`: authoritative global match-state revision
- `match_id`
- `attacker_id`
- `defender_id`
- `your_team`: `unknown`, `attacker`, or `defender`
- `phase`: `gate`, `throne`, or future `completed`
- `breach_gate_id`
- `throne_objective_id`
- `gate_breached`

No Client-provided capture percentage or phase transition exists.

## Delivery semantics

WorldRuntime keeps a per-session delivery stamp consisting of global match revision plus that session's Server-owned team value.

A session is sent a new snapshot when either changes. The delivery stamp advances only after `Connection.TrySend` succeeds. Backpressure therefore leaves the session pending and the state is retried on a later world step.

This is separate from `WorldDynamicState` revisioning. A destroyed visual Gate is not itself a Client-side rule for deciding siege phase; both states originate from Server authority.

## Team boundary

S4-D.1A already owns per-entity team assignment in `siege.Service`. D.1B exposes the recipient's current assignment as `your_team`. The default worldd does not invent a matchmaking/team policy: a character remains `unknown` until a trusted Server-side integration assigns it. Throne capture eligibility policy belongs to the next gameplay stage.

## Preserved invariants

S4-D.1B does not change:

- Gameplay World schema v2 or `castle-sandbox` gameplay revision `s3d-001`;
- UDP MTU 1200;
- compact realtime Move / Snapshot / Correction layouts;
- WorldSnapshot chunking;
- Network LOD / transform batch max64;
- lifecycle desired-vs-known truth;
- Spawn / Despawn Confirm-after-TrySend semantics;
- lifecycle churn max 6,000/snapshot;
- Initial Vitals budgets;
- dirty gameplay Vitals max 4,000/tick;
- Gate damage authority;
- remote Attack/Cast presentation semantics.

## Acceptance coverage

Focused Server tests cover strict Siege config parsing, breach-gate cross-validation, JSON v7 Siege State round-trip/unknown-field rejection, per-session team views, retry-after-backpressure, same-match-revision team resend, and Gate -> Throne revision replication.

The coordinated Client stage decodes the same contract and retains only Server-observed state. Its runtime probe remains a C# loopback fixture and is not S4-E true Go Server <-> Godot Client E2E.

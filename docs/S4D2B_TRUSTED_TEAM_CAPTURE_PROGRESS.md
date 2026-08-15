# S4-D.2B — Trusted Team Assignment & Authoritative Throne Capture Progress

## Scope

S4-D.2B connects the existing trusted CharacterID boundary to the Siege match roster and lets the D.2A throne presence/contest state drive bounded Server-clock capture progress.

This remains a Server-only gameplay stage. It intentionally stops before winner resolution, castle ownership mutation, Client Siege HUD, or any new network message.

## Protocol boundary

Astrahold Protocol remains **v7**.

`SiegeMatchState` (message 106) is unchanged. Capture progress and contest state are internal Server truth in D.2B, so there is no wire-incompatible reason to extend the strict v7 JSON contract.

## Siege config schema v3

`config/siege-match.json` advances to schema 3 / revision `s4d2b-001` and adds:

- `throne_capture_seconds`
- `participants[]`
  - `character_id`
  - `team`: `attacker` or `defender`

The default checked-in roster is empty. Development connections still receive ephemeral CharacterIDs and therefore remain `unknown`; D.2B does not invent team membership from EntityID parity, connection order, remote address, or Client claims.

Production deployments may provide a different `-siege-match` file containing trusted CharacterIDs resolved by the existing S3-F authentication/admission path.

Config loading is strict. Capture duration must be positive and no more than one hour. Participant CharacterIDs must be valid trusted identity keys, team names must be known, and duplicate CharacterIDs are rejected.

## Trusted team assignment

`MatchDefinition` carries a Server-owned CharacterID -> Team roster. `siege.Service.ConfigureMatch` copies that roster into per-world match runtime state.

When a Session enters the world, WorldRuntime asks `siege.Service.AssignResolvedParticipant` to bind its EntityID:

- `AssuranceTrusted` + roster hit -> attacker/defender assignment;
- trusted but unlisted -> `unknown`;
- ephemeral -> `unknown`;
- invalid team data cannot reach runtime because startup config validation rejects it.

Normal unregister/leave removes the EntityID assignment. S3-F ownership takeover preserves the same EntityID and entity-scoped gameplay state, so team assignment survives the transfer without being recalculated from transport data.

## Throne capture state

A configured throne now owns internal `ThroneCaptureState`:

- `Revision`
- `ObjectiveID`
- `Active`
- `Progress`
- `Required`
- `ReadyForResolution`

Capture state has its own revision and does not mutate `MatchState.Revision` while progress changes.

## Capture semantics

After each simulation step, WorldRuntime samples authoritative throne presence and advances capture using that same Server tick `delta`.

While match phase is `Throne`:

- attacker present + no eligible defender -> progress accumulates;
- defender contest, no eligible attacker, or any other loss of D.2A `CaptureEligible` -> partial progress resets to zero;
- progress is capped at the configured required duration;
- reaching the duration latches `ReadyForResolution=true`;
- after latching, later contest does not undo readiness.

This is a continuous-hold policy. It is deterministic from the world-owner Server clock; no wall-clock timer and no Client-provided percentage exists.

`ReadyForResolution` is deliberately not victory. D.2B leaves the global match in `Throne` phase and does not choose a winner or mutate castle ownership. That transaction belongs to the next bounded Siege resolution stage.

## WorldRuntime ordering and scaling

D.2A already used the Siege replication session list to sample throne occupancy. D.2B makes the ordering explicit:

1. queued commands / due respawns;
2. authoritative simulation step;
3. one stable Siege session list;
4. throne presence update;
5. throne capture update from the same tick delta;
6. existing autosave / dynamic state / SiegeMatchState replication.

The same Siege session slice is passed to reliable Siege state replication. D.2B therefore does not add another full session sort/enumeration per tick.

Capture advancement itself is O(1) after the existing D.2A presence scan.

## Preserved invariants

S4-D.2B does not change:

- Protocol v7 or message 106 fields;
- Gameplay World schema v2 / `castle-sandbox` revision `s3d-001`;
- UDP MTU 1200;
- realtime Move / Snapshot / Correction layouts;
- WorldSnapshot chunking;
- Network LOD / transform max64;
- lifecycle desired-vs-known / Confirm-after-TrySend semantics;
- lifecycle churn max 6,000/snapshot;
- Initial Vitals budgets;
- dirty Vitals max 4,000/tick;
- Gate damage / blocker transaction authority;
- remote Attack presentation semantics;
- S3-F trusted identity / takeover fencing semantics.

## Acceptance coverage

Focused tests cover:

- strict schema v3 parsing;
- positive/bounded capture duration;
- trusted roster lookup;
- invalid/duplicate CharacterID and team rejection;
- ephemeral and unlisted trusted identities remaining unknown;
- automatic EntityID team assignment on world registration;
- assignment cleanup on unregister;
- Server-delta capture accumulation;
- contest interruption/reset;
- capture restart after contest clears;
- required-duration cap and readiness latch;
- no global match phase/revision change when capture becomes ready.

## Next stage

S4-D.3 can consume `ReadyForResolution` in one authoritative transaction to set the Siege winner and castle ownership state. Client-facing progress/contest/winner presentation should be designed only when that UI needs a wire contract; because v7 strict JSON is fixed, any incompatible wire extension must be evaluated explicitly rather than silently appended.

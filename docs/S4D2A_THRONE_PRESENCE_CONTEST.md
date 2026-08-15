# S4-D.2A — Authoritative Throne Presence & Contest Foundation

## Scope

S4-D.2A adds the Server-owned spatial/presence foundation required before throne capture progress can exist.

It deliberately stops before capture timers, progress percentages, reset policy, winner resolution, ownership transfer, Client HUD, or any new wire message.

## Protocol boundary

Astrahold Protocol remains **v7**.

`SiegeMatchState` (message 106) is unchanged. Throne occupancy is an internal authoritative gameplay state in this stage, so there is no wire-incompatible reason to bump the protocol.

## Siege config schema v2

`config/siege-match.json` advances from schema 1 to schema 2 and revision `s4d2a-001`.

It adds the Server-only objective rule volume:

- `throne_zone.layer`
- `throne_zone.bounds.min_x/max_x/min_z/max_z`

The castle-sandbox zone is on Layer 0 at X `[-4,4]`, Z `[27,33]`, inside the existing castle ground surface. This zone does not replace Gameplay World movement surfaces, blockers, portals, Gate geometry, or LOS truth; it is only the authoritative siege objective occupancy rule.

Strict JSON loading remains enabled, and invalid/degenerate zone bounds are rejected at startup.

## Throne presence state

`siege.Service` owns `ThronePresenceState` with an independent revision:

- `ObjectiveID`
- `Active`
- `AttackerCount`
- `DefenderCount`
- `Contested`
- `CaptureEligible`

The state is active only while the authoritative match phase is `Throne`.

`CaptureEligible` means only that at least one eligible attacker is in the zone and no eligible defender is contesting. S4-D.2A does not accumulate progress from this value.

## Eligibility

A presence observation counts only when all of the following are Server true:

1. entity is an active world session;
2. entity has a Server-owned attacker or defender assignment in `siege.Service`;
3. entity is not `Defeated` in authoritative character state;
4. entity position is on the configured throne Layer;
5. entity X/Z position is inside the configured throne bounds;
6. match phase is `Throne`.

Unknown/unassigned entities, defeated characters, wrong-layer entities and out-of-zone entities do not affect capture eligibility or contest state.

Duplicate observations for the same entity are counted once.

## WorldRuntime sampling

WorldRuntime samples throne presence from post-simulation state on every world step through the existing Siege replication stage:

- position comes from `simulation.World.Entity`;
- defeat truth comes from `character.Service`;
- team truth comes from `siege.Service`.

This keeps the observation on the single world-owner execution boundary. No Client-provided capture position, team, phase, alive flag, progress or contest bit exists.

## Revision semantics

Throne presence has a separate internal revision from `MatchState.Revision`.

Its revision changes only when the semantic occupancy state changes. Repeating the same observations is idempotent. This prevents temporary occupancy churn from pretending the global match phase changed and avoids unnecessary Protocol v7 SiegeMatchState retransmission.

## Preserved invariants

S4-D.2A does not change:

- Protocol v7 wire contract;
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
- remote Attack presentation semantics.

## Acceptance coverage

Focused tests cover:

- strict siege config schema v2 and throne-zone validation;
- Gate phase locking the throne objective;
- Gate -> Throne activation;
- attacker-only capture eligibility;
- defender contest;
- defeated defender exclusion;
- wrong-layer / out-of-zone / unassigned exclusion;
- duplicate participant de-duplication;
- idempotent presence revisions;
- WorldRuntime post-simulation position + authoritative defeat integration.

## Next stage

S4-D.2B can consume `CaptureEligible` to add bounded authoritative capture progress / interruption semantics. A wire change should be considered only when the Client actually needs progress/contest data; S4-D.2A does not create that requirement.

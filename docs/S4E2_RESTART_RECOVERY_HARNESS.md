# S4-E.2 — Durable Server Restart & Reconnect E2E Harness

## Goal

Extend the S4-E real-process harness so the Client repository can prove process-death recovery across a durable castle ownership transfer.

This stage does not change production `cmd/worldd`, Protocol v8, Siege gameplay rules, Gameplay World data, or the default D.3D completion scheduling policy.

## Harness-only Completed hold

`cmd/e2eserver` adds `-hold-completed`.

When enabled, the harness sets both `SiegeCompletedMinHold` and `SiegeCompletedMaxHold` to zero before constructing `WorldRuntime`. The existing runtime contract treats `0/0` as automatic next-round scheduling disabled.

The default remains `false`, so the S4-E.1 workflow continues to exercise the normal 2s / 10s D.3D scheduling policy.

S4-E.2 uses the hold only for the first process so CI can:

1. drive Round 1 through authoritative PvP, Gate breach, throne capture, and durable ownership transfer;
2. observe authoritative `Completed` while ownership revision 2 is already durable;
3. terminate the Go process before any in-process Round 2 reset;
4. restart a fresh Go process with the same ownership directory.

## Durable markers

The harness emits:

- `ASTRAHOLD_E2E_SERVER_COMPLETED_DURABLE` only when Round 1 is authoritative `Completed`, winner/owner are `attackers`, ownership revision is 2, and a fresh read from `siegeownership.Store` exactly matches the in-memory ownership state.
- `ASTRAHOLD_E2E_SERVER_RECOVERED` when a fresh process starts with durable ownership revision greater than 1. The marker reports the startup round, attacker, defender, owner, and ownership revision.
- `ASTRAHOLD_E2E_SERVER_READY` now also includes the startup round/roles/owner/revision and whether `hold-completed` is active.

The existing `ASTRAHOLD_E2E_SERVER_OK` marker remains unchanged and still proves Round 2 / Gate plus durable ownership agreement.

## Recovery contract under test

The actual recovery behavior is existing production Siege logic:

`ConfigureCastleOwnershipPersistence` restores durable ownership before gameplay, sets the fresh Gate round epoch from `ownership.Revision`, and derives defender from the durable owner with the other configured side as attacker.

For the S4-E fixture, ownership revision 2 with owner `attackers` therefore boots as:

- round 2
- phase Gate
- attacker side `defenders`
- defender side `attackers`
- castle owner `attackers`

The fresh Gameplay World construction restores `main-gate` to max HP with its movement blocker enabled. No mid-round Gate HP, capture progress, or old process memory is restored.

## Preserved boundaries

- production `cmd/worldd`: unchanged
- Protocol v8 / Message 106: unchanged
- GameV1 codecs: unchanged
- Gameplay World schema 2 / `s3d-001`: unchanged
- production Siege and Respawn configs: unchanged
- default D.3D scheduling: unchanged
- no Client gameplay authority
- no distributed/multi-process ownership CAS claim

## Validation

Server CI must pass `go test ./...`, `go vet ./...`, and the existing race-detector boundary suite. The paired Client S4-E.2 workflow is responsible for the full kill/restart/two-new-Godot-client proof.

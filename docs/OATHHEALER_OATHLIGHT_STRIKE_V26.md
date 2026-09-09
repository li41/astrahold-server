# Astrahold Oathhealer Oathlight Strike — Protocol v26 Additive Gameplay Contract

## Status

This slice adds the first Server-authoritative Oathhealer combat action and the visible Oath Seal resource without changing the Protocol v26 wire shape.

The existing v26 `CharacterClassResourceState` message remains generic and now carries Oath Seals as another stable class-resource vocabulary value.

## Canonical action

ActionID:

```text
oathhealer-oathlight-strike
```

Class-balance values used by this slice:

- required ClassID: `class_oathhealer`
- target: entity
- basic ranged attack
- D85
- 1.2 second attack cadence
- each successful Server-resolved hit: +20 hidden Oath Seal progress

Current Server V1 vertical-slice mapping:

- range: 12 meters
- damage taxonomy: `magic`

The formal class design fixes the ranged role, D85, 1.2 second cadence and +20 progress. It does not currently fix one production meter value or a separate holy/light damage taxonomy. The 12 meter range and mapping into the existing `magic` damage type are therefore explicit Server V1 implementation values, not claimed as final balance canon.

A request rejected for wrong class, cooldown, invalid target, out-of-range legality, defeated actor, or another pre-hit failure does not add Oath Seal progress. Only a successful authoritative hit advances progress.

## Oath Seal

Stable visible class-resource ID:

```text
oath_seal
```

Visible range:

```text
0..3
```

The design uses a hidden progress meter:

```text
0..99 progress
100 progress -> +1 oath_seal
```

`oathhealer-oathlight-strike` contributes `+20` progress on each successful Server-resolved hit, so five effective hits from zero progress produce one visible Oath Seal.

The hidden progress value is Server-only combat state. It is intentionally not a second Protocol resource and is not authoritative Client state. When visible Oath Seals are already at the cap of 3, extra progress is not banked behind that cap.

## Resource ownership

Oath Seal follows the same single-owner policy as other v26 class resources:

- mutable truth lives in `character.Service` on the world-owner path;
- hidden seal progress lives in that same `character.State`, not a second service;
- Oath Seal and progress are combat-incarnation runtime state;
- neither is stored in the durable character snapshot yet;
- a trusted durable Oathhealer join/reconnect reconstructs visible Oath Seals as `0/3` and hidden progress as `0` from authoritative `ClassID`.

The Server sends a new visible class-resource state only when conversion changes the visible Oath Seal count. Intermediate hidden progress such as 20, 40, 60 or 80 does not create a fake `CharacterClassResourceState` update.

## Protocol compatibility

Protocol version remains:

```text
26
```

No new message type or field is added.

The existing Server -> Client reliable message remains:

```text
117 CharacterClassResourceState
```

Oathhealer initial example:

```json
{
  "entity_id": 42,
  "resource_id": "oath_seal",
  "current": 0,
  "max": 3
}
```

After five successful Oathlight Strike hits from zero progress:

```json
{
  "entity_id": 42,
  "resource_id": "oath_seal",
  "current": 1,
  "max": 3
}
```

The Client must replace presentation state with the received complete value. It must not count hits, estimate hidden progress, generate seals locally, or persist a local seal balance.

`ActionRejected.reason = wrong_class` remains the authoritative rejection when a non-Oathhealer character requests `oathhealer-oathlight-strike`.

## Initial class lifecycle

For a trusted durable initial Oathhealer selection, the authoritative publish order remains:

1. `CharacterClassState(class_oathhealer)`
2. `CharacterClassResourceState(oath_seal 0/3)`
3. correlated `InitialClassSelectionResult(committed)`

No live ClassID, Oath Seal or hidden seal progress exists before the durability gate completes.

Reliable backpressure may delay these messages, but retrying presentation must not rerun persistence or gameplay mutation.

## Authority flow

```text
ClientUseAction intent
-> world-owner ClassID / target / range / cooldown validation
-> Server hit / damage resolution and HP mutation
-> hidden Oath Seal progress +20
-> if progress crosses 100: visible oath_seal +1, clamp 3
-> CharacterClassResourceState only when visible seal count changes
-> authoritative action / combat events
```

The slice does not define a new externally observable ordering guarantee between resource-state delivery and combat-event presentation beyond the existing message-channel semantics. Clients consume each Server message for its own authoritative purpose.

## Not included in this slice

This slice intentionally does not implement Oathhealer healing, shielding, cleansing, Oath Seal spending, or support-scaling formulas. Those require their own Server-owned gameplay contracts; Oathlight Strike and seal generation remain correct without Client participation.

## Client boundary

The formal Client may expose `oathhealer-oathlight-strike` and present `oath_seal` from type117. It must not infer action permission, range legality, hit, damage, cooldown, hidden seal progress, seal generation or future seal spending from animation, raycast, projectile visuals, equipment, UI state or cached ClassID.

# Astrahold Breaker Heavy Slash — Protocol v26 Additive Gameplay Contract

## Status

This slice adds the first Server-authoritative Breaker gameplay action without changing the Protocol v26 wire shape.

The existing v26 `CharacterClassResourceState` message is intentionally generic, so Breaker can use the same reliable complete-state contract with a new stable resource vocabulary value.

## Canonical action

ActionID:

```text
breaker-heavy-slash
```

Server-authored bounded V1 values:

- required ClassID: `class_breaker`
- target: entity
- current melee range: 4.5 meters
- physical base damage: 135 (`D135` reference slice)
- blockable: yes
- cooldown / attack interval: 1.35 seconds
- successful Server-resolved hit: `momentum +10`, clamped to 100

A miss, rejected action, invalid target, out-of-range request or cooldown rejection does not grant Momentum.

The canonical balance baseline currently locks the first implementation to `+10` Momentum per successful Heavy Slash hit. Older class-fantasy text mentioning extra Momentum against staggered or armor-broken targets is not implemented by this slice; those conditions require their own authoritative status mechanics.

## Momentum

Stable class-resource ID:

```text
momentum
```

Breaker runtime range:

```text
0..100
```

Momentum follows the same v26 runtime ownership policy as Oathguard Resolve:

- mutable truth lives in `character.Service` on the world-owner path;
- it is combat-incarnation runtime state;
- it is intentionally not stored in the durable character snapshot yet;
- a trusted durable Breaker join/reconnect reconstructs it from authoritative `ClassID` as `0/100`.

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

Breaker example:

```json
{
  "entity_id": 42,
  "resource_id": "momentum",
  "current": 10,
  "max": 100
}
```

The Client must replace presentation state with the received complete value. It must not increment Momentum locally.

`ActionRejected.reason = wrong_class` remains the authoritative rejection when a non-Breaker requests `breaker-heavy-slash`.

## Authority flow

```text
ClientUseAction intent
-> world-owner ClassID / target / range / cooldown validation
-> Server hit + damage resolution
-> authoritative Momentum +10
-> CharacterClassResourceState(updated full state)
-> CombatEvent(hit)
```

ClassID, hit, damage, cooldown and Momentum are all Server decisions.

## Client boundary

The formal Client may expose `breaker-heavy-slash` as an intent and present `momentum`, but must not derive permission, hit, damage, cooldown or resource changes from animation, equipment, UI selection or cached local class state.

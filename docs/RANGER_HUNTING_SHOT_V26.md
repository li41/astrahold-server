# Astrahold Ranger Hunting Shot — Protocol v26 Additive Gameplay Contract

## Status

This slice adds the first Server-authoritative Ranger combat action without changing the Protocol v26 wire shape.

The existing v26 `CharacterClassResourceState` message remains generic and now carries the Ranger primary resource as another stable vocabulary value.

## Canonical action

ActionID:

```text
ranger-hunting-shot
```

Server-authored V1 vertical-slice values:

- required ClassID: `class_ranger`
- target: entity
- current Server combat range: 12 meters
- physical base damage: 100 (`D100` reference slice)
- blockable: yes
- cooldown / attack interval: 1.1 seconds
- successful Server-resolved hit: `hunt_momentum +8`, clamped to 100

The class balance baseline fixes `D100 / 1.1s / +8` for Hunting Shot. It describes Ranger as a mid-to-long-range role but does not yet fix a single canonical meter value. The 12 meter value is therefore an explicit vertical-slice Server range aligned with the existing ranged combat envelope; it is not claimed as final balance canon.

A miss, rejected action, invalid target, out-of-range request or cooldown rejection does not grant Hunt Momentum.

## Hunt Momentum

Stable class-resource ID:

```text
hunt_momentum
```

Ranger runtime range:

```text
0..100
```

Hunt Momentum follows the same v26 ownership policy as Resolve and Momentum:

- mutable truth lives in `character.Service` on the world-owner path;
- it is combat-incarnation runtime state;
- it is intentionally not stored in the durable character snapshot yet;
- a trusted durable Ranger join/reconnect reconstructs it from authoritative `ClassID` as `0/100`.

## Locked Prey boundary

The Ranger design also defines the passive `Locked Prey` loop from repeated effective hits against the same target. Current design text gives an example threshold of roughly two to three hits but does not lock one exact production threshold.

This slice therefore does **not** invent a threshold or emit a half-authoritative lock state. Hunting Shot and Hunt Momentum ship first; Locked Prey requires its own Server-owned target-progress state and explicit numeric contract before gameplay effects depend on it.

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

Ranger example:

```json
{
  "entity_id": 42,
  "resource_id": "hunt_momentum",
  "current": 8,
  "max": 100
}
```

The Client must replace presentation state with the received complete value. It must not increment Hunt Momentum locally.

`ActionRejected.reason = wrong_class` remains the authoritative rejection when a non-Ranger requests `ranger-hunting-shot`.

## Authority flow

```text
ClientUseAction intent
-> world-owner ClassID / target / range / cooldown validation
-> Server hit + damage resolution
-> authoritative Hunt Momentum +8
-> CharacterClassResourceState(updated full state)
-> CombatEvent(hit)
```

ClassID, range legality, hit, damage, cooldown and Hunt Momentum are Server decisions.

## Client boundary

The formal Client may expose `ranger-hunting-shot` as an intent and present `hunt_momentum`, but must not derive permission, range legality, hit, damage, cooldown or resource changes from animation, raycast, local target selection, equipment, UI state or cached ClassID.

# Astrahold Oathguard Sword Strike / Resolve Protocol v26

## Status

Protocol v26 introduces the first Server-authoritative Oathguard gameplay action and its runtime class resource presentation contract.

The Server remains the sole gameplay authority:

```text
ClientUseAction intent
    -> world-owner ClassID / target / cooldown validation
    -> Server hit + damage resolution
    -> authoritative HP / Resolve mutation
    -> ActionStarted / CombatEvent / CharacterClassResourceState
```

The Client never decides action eligibility, hit, damage, cooldown or Resolve.

## Canonical action

ActionID:

```text
oathguard-sword-strike
```

Server-authored v1 values:

- required ClassID: `class_oathguard`
- target: entity
- range: 4.5 meters
- physical base damage: 100
- blockable: yes
- cooldown: 1.1 seconds
- successful hit resource gain: `resolve +8`

A miss, rejected action, invalid target, out-of-range request or cooldown rejection does not grant Resolve.

## Resolve

Stable resource ID:

```text
resolve
```

Oathguard runtime range:

```text
0..100
```

Resolve is currently combat-incarnation runtime state owned by `character.Service` on the world-owner path. It is intentionally not stored in the durable character snapshot in v26.

A trusted durable Oathguard join/reconnect reconstructs Resolve from the authoritative ClassID as `0/100`. This contract does not yet define cross-login Resolve persistence.

## Protocol changes

Protocol version:

```text
26
```

### ActionRejected

New rejection reason:

```text
wrong_class
```

The Server returns this when a valid class-scoped ActionID is requested by a character whose authoritative ClassID does not satisfy that action policy.

### CharacterClassResourceState

Server -> Client, `ReliableOrdered`.

MessageType:

```text
117
```

JSON v1 example:

```json
{
  "entity_id": 42,
  "resource_id": "resolve",
  "current": 8,
  "max": 100
}
```

Fields:

- `entity_id`: authoritative character EntityID for this world incarnation.
- `resource_id`: stable Server resource vocabulary; v26 ships `resolve` for Oathguard.
- `current`: authoritative current amount.
- `max`: authoritative cap.

This message is complete resendable resource truth, not a delta. The Client should replace its presented value with the received state rather than incrementing locally.

## Lifecycle ordering

For a trusted durable Oathguard character join/reconnect, the class feedback path sends:

```text
CharacterClassState(class_oathguard)
CharacterClassResourceState(resolve 0/100)
```

For a newly committed initial Oathguard assignment:

```text
durable checkpoint
-> world-owner ClassID commit
-> CharacterClassState(class_oathguard)
-> CharacterClassResourceState(resolve 0/100)
-> InitialClassSelectionResult(committed)
```

For a successful Sword Strike hit:

```text
Server hit/damage mutation
-> Resolve +8 (clamped to 100)
-> CharacterClassResourceState(updated complete state)
-> CombatEvent(hit)
```

Reliable transport backpressure may delay class-resource presentation but must not rerun gameplay mutation. The bounded class feedback FIFO is process-local; if its contract cannot be maintained the connection is closed fail-closed.

## Compatibility boundary

Existing compatibility/development actions retain their current class eligibility in v26. This slice does not retroactively class-lock `basic-attack`, `shatter-strike`, `fireball`, `meteor-strike`, `resurrect` or monster actions.

The Client must not infer permission to use `oathguard-sword-strike` from UI state, model, animation, equipment or cached ClassID. Only the Server decision is authoritative.

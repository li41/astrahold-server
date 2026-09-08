# Starfire Cold Star Channel — Protocol v27

This slice gives Starfire its first authoritative heat-management action while reusing the existing Server-owned self-mitigation effect added for Oathguard Fortify. Protocol remains v27.

## Player-visible contract

Action ID: `starfire-cold-star-channel`

Required class: `class_starfire_mage`

Canonical balance values implemented here:

- cooldown: 20 s
- Star Heat reduction: 45
- self damage reduction: 10%
- self-mitigation duration: 2 s
- D0

The action is self-targeted through the existing `ClientUseAction` entity-target shape. `target_id` must equal the actor's authoritative EntityID.

The intended Starfire rhythm now has both directions of resource movement:

`fire bolt -> +8 Star Heat`

`cold star channel -> reduce Star Heat by up to 45 -> 2 s defensive reset window`

## Heat reduction is not a minimum cost

Star Heat represents accumulated burden rather than currency that must be available before cooling can occur.

For this action the canonical `-45` is therefore implemented as an accepted reduction clamped at zero, not as `CostResource` legality:

- 80 Heat -> 35
- 45 Heat -> 0
- 32 Heat -> 0
- 0 Heat -> 0

Heat below 45 does not cause `insufficient_resource`.

The action can still be accepted at 0 Heat and create its short self-mitigation window. This follows the class design that describes Cold Star Channel as both resource management and a defensive rhythm skill, rather than only as a resource spender.

On acceptance the Server publishes complete type117 `CharacterClassResourceState`, including a complete `star_heat 0/100` replacement when cooling reaches zero.

## Server-owned mitigation

Cold Star Channel reuses `combat.EffectSelfMitigation`.

On authoritative commit the existing `combat.Service` creates a 10% self-damage-reduction window lasting 2 seconds. Incoming damage reads this state through the centralized Server mitigation path.

The reduction:

- applies to both physical and magic incoming damage;
- composes after ordinary damage-type-specific defense/block mitigation;
- preserves the existing single final-round rule;
- does not modify equipment, block chance, block reduction or attacker state;
- lazily expires on the Server;
- is cleared on authoritative defeat or formal entity leave;
- is not durable character state.

A Client timer may drive presentation, but it never decides whether the reduction is active for gameplay.

## Target and rejection semantics

The action uses:

```json
{"action_id":"starfire-cold-star-channel","target_kind":"entity","target_id":"<own EntityID>"}
```

Only the explicit `self_mitigation` effect uses this self-target path. Existing damage and resurrection actions keep their existing self-target restrictions.

Rejected intents do not reduce Heat, start the 20 s cooldown or create/extend mitigation. This includes:

- `wrong_class`
- `cooldown`
- `invalid_target` when the target is not the actor

A failed other-target attempt does not consume the cooldown; a corrected self-target attempt can still succeed immediately if otherwise legal.

## Protocol v27

No wire delta is required.

The action reuses:

- `ClientUseAction`
- `ActionStarted`
- `ActionRejected`
- type117 `CharacterClassResourceState`
- existing authoritative CombatEvent / vitals presentation for later incoming damage

The Client must replace its displayed Star Heat with each received type117 state. It must not subtract 45 locally or decide mitigation from local animation timing.

## Deliberately not shipped

The canonical balance baseline also describes about 2 seconds of minor casting stability. Astrahold does not yet have a reusable authoritative casting-stability / interruption-resistance subsystem, so this slice does not invent one inside Starfire.

This slice also does not invent precise overheat legality. The design baseline identifies 70–89 as high heat and 90–100 as an overheat zone, but does not yet lock which individual high-heat skills are rejected or altered. Existing Fire Bolt remains legal under its current v26 contract until that generic Server policy is specified.

## Authority flow

`ClientUseAction(starfire-cold-star-channel, self)`
-> Server validates session / ClassID / action cooldown / exact self target
-> world-owner clamps Star Heat reduction to `min(current,45)`
-> `character.Service` commits the authoritative Star Heat mutation
-> Server publishes complete type117 Star Heat state
-> Server emits `ActionStarted`
-> `combat.Service.Commit` starts the 20 s cooldown and 2 s / 10% mitigation window
-> later incoming damage reads the Server-owned mitigation state
-> Server mutates HP and emits normal authoritative combat presentation

No Client raycast, local Heat bar, local buff timer or animation state participates in gameplay legality or final damage.

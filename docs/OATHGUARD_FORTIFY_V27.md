# Oathguard Fortify — Protocol v27

This slice closes the first authoritative Oathguard Resolve build -> defensive spend loop without changing Protocol v27.

## Player-visible contract

- Action ID: `oathguard-fortify`
- Required class: `class_oathguard`
- Cooldown: 20 s
- Cost: 30 `resolve`
- Damage: D0
- Self damage reduction: 45%
- Duration: 3 s
- Target: the acting Oathguard's own EntityID

The 20 s cooldown, 30 Resolve cost, approximately 45% self damage reduction and 3 s duration come from the canonical class balance baseline.

The existing `oathguard-sword-strike` builds Resolve by +8 on a successful Server-resolved hit, so the first playable defensive loop is:

`Sword Strike hits -> build Resolve -> Fortify (-30) -> survive a short danger window`

## Self-only target authority

Fortify reuses the existing reliable `ClientUseAction` message. The Client sends:

- `action_id = oathguard-fortify`
- `target_kind = entity`
- `target_id = the player's current authoritative EntityID`

Only the `self_mitigation` action effect is allowed to consume this self-target shape. Existing damage and resurrection entity actions continue through the normal target validator and retain the existing self-target rejection rule.

A Fortify request targeting another entity is rejected as `invalid_target` before Resolve is spent and before cooldown or mitigation begins.

## Authoritative Resolve spend

Fortify uses the existing class-action resource-cost path:

- Server validates `class_oathguard`.
- Server validates Resolve >= 30.
- If insufficient, `ActionRejected.reason = insufficient_resource` is returned.
- Rejected use does not spend Resolve, start cooldown or start mitigation.
- Accepted use spends 30 Resolve in `character.Service`.
- The owning Client receives complete replacement type117 `CharacterClassResourceState`.

The Client never subtracts Resolve locally as gameplay truth.

## Server-owned mitigation window

The combat action definition contains:

- `effect = self_mitigation`
- `self_damage_reduction_percent = 45`
- `duration_seconds = 3`

The existing `combat.Service`, which already owns action preparation/cooldown commit, also owns this transient committed mitigation window. No second gameplay authority or general-purpose status framework is introduced.

At a 50 ms simulation delta, a Fortify committed at tick 2 is active for the half-open interval `[2, 62)` and is inactive at tick 62.

The transient state:

- is runtime-only, not durable character state;
- lazily expires on Server read;
- is cleared on authoritative defeat;
- is cleared when the entity formally leaves the world;
- must not survive an EntityID incarnation change.

## Incoming-damage pipeline

All normal damage still resolves through the central Server incoming-damage path.

For an active Fortify window, after ordinary damage-type-specific defense/block reduction has been applied, the same damage instance receives the self mitigation multiplier:

`damage *= 1 - 0.45`

Intermediate math remains float64 and positive damage is rounded once at the end, preserving the existing mitigation contract.

Examples without other mitigation:

- physical D100 -> 55 final damage while Fortify is active;
- magic D100 -> 55 final damage while Fortify is active.

Fortify does not change shield block chance, shield block reduction, equipment defense, attacker state, or later attacks after expiry.

## Protocol v27

Protocol remains **v27**. No new message type or wire field is introduced.

This slice reuses:

- `ClientUseAction` for intent;
- `ActionStarted` for accepted-action presentation;
- `ActionRejected` for wrong class / insufficient resource / invalid target / cooldown feedback;
- type117 `CharacterClassResourceState` for complete authoritative Resolve state;
- normal `CombatEvent.damage` and `EntityVitalsState` for authoritative post-mitigation outcomes.

The mitigation duration is not replicated as a new gameplay timer in this slice. Client presentation may start a visual from authoritative `ActionStarted`, but a local timer must never decide whether incoming damage is reduced.

## Deliberately not shipped

The canonical Fortify description also calls for high displacement resistance and a slight movement-speed reduction during the defensive window. Those mechanics require reusable authoritative movement/control-resistance semantics and are **not shipped in this slice**.

This implementation therefore claims only:

- Resolve -30,
- 20 s cooldown,
- 45% self damage reduction,
- 3 s Server-owned runtime window.

It does not claim the movement slow or displacement resistance yet.

## Authority flow

`ClientUseAction(oathguard-fortify, self EntityID)`
-> Server validates session / ClassID / action cooldown / exact self target
-> Server validates Resolve >= 30
-> rejection: no spend, no cooldown, no mitigation
-> world-owner spends 30 Resolve
-> Server publishes complete type117 Resolve state
-> Server emits `ActionStarted`
-> `combat.Service.Commit` starts cooldown and mitigation window
-> future incoming damage reads current Server mitigation state
-> central mitigation computes final damage
-> world-owner mutates HP / death state
-> Server emits ordinary authoritative combat/vitals presentation

Client animation, local Resolve, local cooldown, local buff timer and local damage calculations never decide Fortify legality or mitigation.

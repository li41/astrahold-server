# Ranger Armor Piercing Arrow — Protocol v27

This slice closes the first authoritative Ranger Hunt Momentum build -> spend loop without adding a target debuff or changing Protocol v27.

## Player-visible contract

- Action ID: `ranger-armor-piercing-arrow`
- Required class: `class_ranger`
- Cooldown: 7 s
- Cost: 30 `hunt_momentum`
- Base damage: D210
- This attack ignores 25% of the target's Server-owned physical defense for this damage instance.
- Current Server V1 envelope: 12 m, physical, blockable.

The 7 s cooldown, 30 Hunt Momentum cost, D210 base damage and approximately 25% armor ignore come from the canonical balance baseline. The current 12 m / physical / blockable envelope follows the existing Ranger Server V1 ranged-action mapping and is not presented as a separately locked final production value.

Four successful `ranger-hunting-shot` hits currently produce 32 Hunt Momentum, so the basic playable loop is:

`hunting shot (+8) x4 -> armor piercing arrow (-30, D210)`

The remaining 2 Hunt Momentum stays authoritative on the character.

## Attack-local armor ignore

`physical_defense_ignore_percent` is combat action data. It is copied into the Server-owned prepared damage and then into the centralized incoming-damage resolver.

For physical damage, the mitigation formula uses:

`effective defense = physical defense * (1 - ignore percent / 100)`

The normal defense formula, block chance and block damage reduction then continue unchanged. Ignore does not:

- mutate the target's equipment,
- create an armor-break debuff,
- help another attacker,
- alter block chance,
- alter block damage reduction,
- persist after this damage instance.

Example using the current Guard Shield physical defense 4:

- D210 with normal defense: effective defense 4 -> final unblocked damage 175.
- D210 with 25% ignore: effective defense 3 -> final unblocked damage 183.
- If the Guard Shield also blocks, its existing 30% block reduction still applies after defense mitigation; the same piercing hit becomes 128.

All intermediate mitigation math remains float64 and positive damage is rounded once at the end, matching the existing authoritative mitigation contract.

## Resource legality

Hunt Momentum remains character-global authoritative state in `character.Service` and replicates through reliable type 117 `CharacterClassResourceState`.

A request with less than 30 Hunt Momentum is rejected as `insufficient_resource` before acceptance. It does not deal damage, spend Hunt Momentum or start the 7 s cooldown.

Wrong-class, invalid-target, out-of-range and cooldown rejections likewise do not spend Hunt Momentum.

On successful cost commit, the Server sends the complete replacement type117 state. The Client does not subtract 30 locally or infer future legality from animation state.

## Protocol v27

Protocol stays **v27**. No new wire message or field is needed.

The slice reuses:

- type 117 `CharacterClassResourceState`, ReliableOrdered,
- `ActionRejected.reason=insufficient_resource`,
- existing `wrong_class`, range and cooldown rejection semantics,
- existing `CombatEvent.damage` / `blocked` authoritative result presentation.

The Client does not need the physical-defense-ignore percentage to calculate gameplay damage. It may present the action by ActionID, while final damage and block outcome remain Server truth.

## Deliberately not shipped

The canonical baseline says Armor Piercing Arrow reaches about D260 when it consumes the Weakpoint Arrow window. Weakpoint target state is not yet implemented as a reusable authoritative status system, so this slice does not invent that conditional tier.

It also does not add a team armor debuff. The balance baseline explicitly keeps the 25% ignore on this attack rather than granting a global team reduction.

Locked for a later reusable target-status slice:

- Weakpoint Arrow,
- the Weakpoint -> D260 interaction,
- Locked Prey interactions.

## Authority flow

`ClientUseAction(ranger-armor-piercing-arrow)`
-> Server validates session / ClassID / action cooldown / target / range
-> Server validates Hunt Momentum >= 30
-> insufficient: `ActionRejected(insufficient_resource)`, no cooldown commit
-> world-owner spends 30 Hunt Momentum in `character.Service`
-> Server publishes complete type117 Hunt Momentum state
-> prepared damage carries 25% physical-defense ignore
-> centralized Server mitigation resolves defense and block
-> world-owner mutates HP / death state
-> normal authoritative combat presentation follows

Client raycasts, local resource bars, local armor values, local cooldowns and animation timing never authorize the spender or calculate the final damage.

# Breaker Stagger Strike — Protocol v27

This slice closes the first authoritative Breaker Momentum build -> spend loop while keeping Protocol v27 unchanged.

## Player-visible contract

- Action ID: `breaker-stagger-strike`
- Required class: `class_breaker`
- Cooldown: 10 s
- Cost: 20 `momentum`
- Base damage: D190
- Current Server V1 envelope: 4.5 m, physical, blockable

The 10 s cooldown, 20 Momentum cost and D190 base tier come from the canonical balance baseline. The current 4.5 m / physical / blockable envelope follows the existing Server V1 Breaker melee implementation and is not presented as a separately locked final production value.

Two successful `breaker-heavy-slash` hits currently build 20 Momentum, so the basic playable loop is:

`heavy slash (+10) -> heavy slash (+10) -> stagger strike (-20, D190)`

## Resource legality

Momentum remains character-global authoritative state in `character.Service` and continues to replicate through reliable type 117 `CharacterClassResourceState`.

A Stagger Strike request with less than 20 Momentum is rejected as `insufficient_resource` before the action is accepted. It does not:

- spend Momentum,
- damage the target,
- start the 10 s action cooldown.

Wrong-class, invalid target, out-of-range and normal action-cooldown rejections likewise do not spend Momentum.

After target/range legality succeeds, the Server validates all action resource costs before mutating any of them. The class-resource cost is then committed immediately before the action becomes accepted, matching the existing MP-cost boundary. Accepted action costs are Server truth and are not reconstructed or refunded by Client hit presentation.

A successful cost commit publishes the complete replacement type117 state, for example:

```json
{"entity_id":10,"resource_id":"momentum","current":0,"max":100}
```

The Client must not subtract Momentum locally to authorize later actions.

## Protocol v27

No new message type, field or rejection reason is added.

This slice reuses:

- type 117 `CharacterClassResourceState`, ReliableOrdered,
- `ActionRejected.reason=insufficient_resource`,
- existing `wrong_class` and cooldown rejection semantics.

## Deliberately not shipped

The canonical design says Stagger Strike rises to about D230 against staggered / armor-broken targets and also contributes additional stagger pressure. Those effects depend on reusable authoritative armor-break / stagger state that is not yet present in the formal Server.

This slice therefore does **not** invent:

- an armor-break status,
- a stagger meter or threshold,
- a hidden Client-side condition,
- the conditional D230 tier.

The shipped action is the complete base D190 / 20 Momentum spender. Conditional amplification can be added later through a reusable Server-owned status / stagger system without changing this resource-cost authority boundary.

## Authority flow

`ClientUseAction(breaker-stagger-strike)`
→ Server validates session / ClassID / action cooldown / target / range
→ Server validates Momentum >= 20
→ insufficient: `ActionRejected(insufficient_resource)`, no cooldown commit
→ world-owner spends 20 Momentum in `character.Service`
→ Server publishes complete type117 Momentum state
→ Server resolves damage / mitigation / HP / death
→ normal authoritative combat presentation follows

Client animation, local resource bars, local cooldowns, raycasts or cached target state never decide whether the spender is legal or how much Momentum remains.

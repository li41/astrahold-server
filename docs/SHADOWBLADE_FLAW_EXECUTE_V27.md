# Shadowblade Flaw Execute — Protocol v27

This slice closes the first authoritative Shadowblade build -> spend combat loop without adding a second gameplay authority or a new wire shape.

## Player-visible contract

- Action ID: `shadowblade-flaw-execute`
- Required class: `class_shadowblade`
- Cooldown: 14 s
- Requires at least 1 `flaw` on the exact source × target pair.
- 1 Flaw -> D140 and consumes 1.
- 2 Flaw -> D220 and consumes 2.
- 3 Flaw -> D300 and consumes 3.
- The action consumes the source Shadowblade's complete current Flaw stack on that target after a successful Server-resolved hit.

The 14 s cooldown and D140 / D220 / D300 tiers are the locked baseline values. The current Server V1 action envelope uses 4.5 m, physical damage and blockable damage. Those envelope details are implementation policy for this vertical slice unless and until the higher-priority design contract fixes different production values.

The design also describes a small low-HP bonus, but does not lock its production number. This slice deliberately does not invent that bonus.

## Resource legality and cooldown

Flaw remains runtime target-scoped state keyed by:

`(source_entity_id, target_entity_id, resource_id)`

A request with no Flaw on the selected target is rejected as `insufficient_resource` before action acceptance. It does not deal damage, consume Flaw or start the 14 s cooldown.

Wrong-class, cooldown, range and other normal action rejections do not consume Flaw. A future miss-capable target-resource spender must likewise only commit its spend after a Server-resolved hit.

Spending to zero retains the Server-only `ReadyTick` for that source-target resource state. This preserves an already-running Dual Blade 2.5 s build ICD: Execute cannot reset Flaw to a fresh state and immediately bypass the build interval.

## Protocol v27

Protocol remains v27. No new message type or field is required.

The existing reliable type 118 `CharacterTargetResourceState` remains the complete presentation contract. A successful Execute publishes the resulting full state:

```json
{"source_entity_id":10,"target_entity_id":9703,"resource_id":"flaw","current":0,"max":3}
```

`current=0` clears the Client presentation for that source-target Flaw key. The Client does not know or reproduce the Server-only build-ready tick.

If Execute is lethal, the spend already publishes `current=0`; subsequent incarnation cleanup removes the internal zero-current state without sending a duplicate clear.

## Authority flow

`ClientUseAction(shadowblade-flaw-execute)`
→ Server validates session / ClassID / target / range / action cooldown
→ Server reads authoritative Flaw for the exact source × target
→ no Flaw: `ActionRejected(insufficient_resource)`
→ Server resolves hit and incoming mitigation
→ Server selects D140 / D220 / D300 from authoritative current Flaw
→ world-owner spends Flaw in `character.Service`
→ Server publishes type118 complete state
→ world-owner mutates target HP / death state
→ Server publishes normal action / combat / vitals presentation

Client animation, local hit counting, cached Flaw, local target selection, local HP, raycasts or local cooldown timers must not choose the damage tier, authorize the action or spend Flaw.

## Deliberately not shipped

- No low-HP damage bonus until an explicit numeric contract is approved.
- No new Shadowblade execute side/back requirement; the locked baseline ties side/back to Flaw construction, not to spending.
- No durable Flaw persistence. Flaw remains combat-incarnation runtime state.

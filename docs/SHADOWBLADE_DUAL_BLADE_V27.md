# Shadowblade Dual Blade / Flaw — Protocol v27

This slice adds the first authoritative Shadowblade combat loop without creating a second gameplay authority.

## Player-visible contract

- Action ID: `shadowblade-dual-blade-strike`
- Required class: `class_shadowblade`
- Current Server V1 range: 4.5 m
- Base damage: D90 physical
- Blockable: yes
- Cooldown: 0.95 s
- A successful Server-resolved side/back hit may build `flaw +1` on that target.
- Flaw maximum: 3.
- Flaw build internal cooldown: 2.5 s per source Shadowblade and target.
- A valid front hit still deals damage but does not build Flaw.
- Misses and class/range/cooldown rejections do not build Flaw.

The 4.5 m range and the current positional classifier are Server V1 gameplay policy for this vertical slice. The design source requires side/back play but does not lock a final production angle. V1 therefore classifies a 120-degree target-forward cone as front (`dot >= 0.5`) from authoritative target yaw; everything outside that cone is side/back. Client presentation must not treat that angle as independent gameplay truth.

## Target-scoped resource authority

Flaw is not a character-global primary resource and is intentionally not represented by Protocol type 117. It is runtime state keyed by:

`(source_entity_id, target_entity_id, resource_id)`

This means two Shadowblades attacking the same target do not share Flaw. The Server-owned target-resource store also keeps the build-ready tick used by the 2.5 s deterministic internal cooldown.

Flaw is incarnation-scoped, not durable character persistence. Defeat/removal/despawn clears affected target-resource state. When a surviving source still has a session, the Server sends an authoritative clear before the removed target incarnation can be reused.

## Protocol v27

Protocol version is 27. New reliable message type 118 is `CharacterTargetResourceState`.

Example full-state replacement after a successful build:

```json
{"source_entity_id":10,"target_entity_id":9403,"resource_id":"flaw","current":1,"max":3}
```

A clear uses the same key with `current: 0` and the resource max. Clients replace presentation from the complete state; they never increment/decrement Flaw locally.

Reliable backpressure coalesces pending type 118 updates for the same source/target/resource key. A newer complete state replaces an older pending state. Retrying delivery therefore retries presentation only; it never replays gameplay mutation or the Flaw build ICD.

## Authority flow

`ClientUseAction`
→ world-owner validates session/class/target/range/cooldown
→ Server resolves hit and damage
→ Server reads authoritative actor/target transforms and target yaw
→ if target survives and side/back build policy is ready, Server mutates Flaw
→ Server emits type 118 complete state
→ Server emits normal combat presentation events

The Client must not decide side/back legality, range, hit, damage, cooldown, Flaw count, Flaw build timing, target sharing, or lifecycle clear from local raycasts, animation, cached transforms, UI state, or hit counting.

## Deliberately not shipped

This slice does not add a Flaw-consuming execute/finisher or invent unapproved consumption numbers. Such an action must be a later Server-owned contract with explicit legality, consumption, damage/effect and failure semantics.

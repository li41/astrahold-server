# Astrahold Current Product Direction

This file records the current product-development direction. Historical milestone/security documentation remains useful evidence, but it must not override this document when choosing the next gameplay work.

## Production architecture

```text
Unreal Engine 5.8 Client
        |
        | Client intent only
        v
Astrahold Go Server
        |
        +-- single-owner world loop
        +-- authoritative movement / combat / resources / inventory
        +-- AOI / replication
        +-- PostgreSQL target persistence
```

Three.js is tooling only through `li41/astrahold-tools`; it is not a production game client or Server runtime dependency.

## Verified player-facing slices

Verified with the production-compatible Unreal Client / `worldd` path:

- HTTPS login and issued session credential
- TLS 1.3 + ASTRAH1 + SessionWelcome
- authenticated realtime UDP
- authoritative movement
- production-worldd playtest Monster Spawn/Vitals/AOI lifecycle
- Basic Attack
- `shatter-strike` entity-target skill
- Server-authoritative cooldown rejection
- Server-authoritative MP / MaxMP and `insufficient_resource`
- point-target `fireball` action/resource contract

Do not rebuild these as separate systems.

## Current gameplay slice: Inventory

Inventory is the current vertical slice.

The first bounded contract is:

```text
Server-owned inventory
  -> bounded unique stacks
  -> ItemArchetypeID + quantity
  -> monotonic revision
  -> reliable full InventorySnapshot
  -> Unreal read-only cache/presentation
```

The Client must not decide inventory transactions. Unreal asset paths, display names, icons and meshes are presentation data and must not enter the Server inventory model.

The initial inventory authority/codec core is merged. Runtime bootstrap delivery and the Unreal inventory presentation path are the next work.

Expected product order after this slice:

```text
Inventory
-> Equipment
-> Item / Drop
-> NPC
-> Shop
-> Death / Respawn polish
-> Party / Chat / Guild / Trade / Warehouse / Quest
```

## Protocol version policy

Current shipping/playtest fence is Protocol v13.

`InventorySnapshot` message shape is being introduced, but the protocol must not advance merely because a dormant type exists. Advance the protocol fence only when production runtime begins emitting the new wire shape and the Unreal Client decoder is enabled in the same compatibility slice.

The expected next fence is v14 when InventorySnapshot becomes live.

Avoid independent handwritten Go/C++/TypeScript protocol drift. Codegen/schema work may be introduced incrementally, but it must not block gameplay vertical slices.

## Authority invariants

Client never decides:

- final position
- HP / MP
- damage / hit / death
- cooldown legality
- inventory/equipment transactions
- drops / EXP
- PvP results
- siege winner / castle ownership

Network goroutines do not mutate world state directly. Gameplay mutations remain on the single world-owner path.

## Rendering boundary

DLSS and other GPU features are Client presentation concerns only. If adopted, Astrahold should use Unreal-native rendering and official NVIDIA Unreal/Streamline integration. Third-party DLL swapping, ReShade feeder injection and synthetic motion-vector shims are not part of Server or production gameplay architecture.

## CI note

The repository currently contains historical workflow debt unrelated to the Inventory slice. Recent Server CI has exposed pre-existing failures such as duplicate combat helper declarations and character-state compatibility/test drift.

Do not hide these failures, but do not mix unrelated infrastructure repairs into gameplay commits merely to make a feature PR appear green. Fix shared CI debt in focused maintenance work, then keep gameplay changes bounded.

## Priority rule

When choosing between deeper infrastructure and a player-visible vertical-slice result, prefer the player-visible result unless authority, data integrity or security would be compromised.
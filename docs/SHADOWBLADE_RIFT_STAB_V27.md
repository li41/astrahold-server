# Shadowblade Rift Stab v27

## Player-visible slice

`shadowblade-rift-stab` adds a second deliberate Flaw-building attack for `class_shadowblade`.

- Base damage: D135.
- Cooldown: 7 seconds.
- Damage type: physical.
- Blockable: yes.
- Target: hostile entity.
- V1 Server melee range: 4.5m, reusing the existing Shadowblade melee envelope. The formal balance baseline does not currently lock a distinct Rift Stab range, so 4.5m is an implementation choice rather than a new canonical design claim.
- A successful Server-resolved side/back hit builds +1 target-scoped Flaw, capped at 3.
- A front hit still deals D135 but builds no Flaw.
- A lethal hit builds no new Flaw; existing defeat cleanup remains authoritative.

The formal design's optional extra benefit while the target is casting/charging is not shipped here because the Server does not yet own a reusable cast/interrupt state contract.

## Flaw / Dual Blade interaction

Flaw remains scoped by `(source Shadowblade, target, flaw)` and is never shared between Shadowblades.

Dual Blade Strike retains its existing 2.5s per-source/per-target build ICD. Rift Stab does not consume, reset, extend, or bypass that timestamp:

1. Dual Blade may establish `ReadyTick` while building Flaw.
2. Rift Stab may independently add +1 Flaw during that window when its own action is legal and the hit is side/back.
3. The stored `ReadyTick` is preserved exactly.
4. A subsequent Dual Blade hit before that original `ReadyTick` still deals damage but does not build Flaw.
5. Dual Blade can build again once the original `ReadyTick` is reached.

This is implemented with the reusable `targetresource.Store.GainPreservingReadyTick` mutation instead of overloading a zero ICD or resetting the existing store state.

## Authority and rejection behavior

All legality and resource mutation remains in the authoritative world-owner path.

- Client position/animation does not decide side/back eligibility.
- Wrong class rejects before damage or Flaw mutation.
- Cooldown rejection does not damage or mutate Flaw.
- Out-of-range rejection does not damage or mutate Flaw.
- Full Flaw remains capped at 3 and emits no duplicate state when no visible value changes.

## Protocol

Protocol remains v27. No wire type or field changes are introduced.

Rift Stab reuses:

- `ClientUseAction` intent.
- `ActionStarted` / `ActionRejected`.
- existing combat events and vitals.
- type 118 `CharacterTargetResourceState` for complete Flaw replacement state.

`ReadyTick` remains internal Server state and is not replicated. Client must not add Flaw locally or classify front/side/back for gameplay.

## Validation scope

The slice includes unit/integration coverage for direct target-resource gain preserving `ReadyTick`, class policy, side/back gain, front hit, out-of-range, wrong class, cooldown, lethal behavior, and Dual Blade ICD interaction.

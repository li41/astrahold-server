# S3-F.13 — Durable Defeated Character Respawn State Seam

## Goal

S3-F.13 extends the existing trusted character-state durability boundary so an already-defeated trusted character can be restored without inventing gameplay truth. The slice remains server-only and keeps Protocol v6 unchanged.

The source state already exists before persistence: S3-F.4 binds death context and respawn destination at defeat time; S3-F.7 applies the configured death penalty after that binding; S3-F.11 captures trusted character state on leave; S3-F.12 restores trusted alive state before SessionWelcome.

## Schema evolution

Character State schema is now v2.

- readers accept v1 and v2;
- writers emit v2;
- v1 alive records remain valid restore inputs;
- v1 defeated records remain readable but fail closed at restore because they have no durable respawn metadata;
- v2 alive records must not contain defeated-respawn metadata;
- v2 defeated records must contain complete defeated-respawn metadata.

The durable defeated payload contains:

- `DeathContext` (`pve`, `pvp`, `siege`);
- the immutable death-time `SpawnPointID`, `SpawnClass`, and `Position`;
- `RemainingTicks` captured in the world tick domain;
- the settled post-penalty acquired `CheckpointID`, if one remains.

`DefeatRevision`, `PenaltyTransactionApplied`, and `CheckpointForfeited` are intentionally not duplicated into character-state. Their existing durable audit/history boundary is the Death Outcome Journal. Restore consumes the already-settled gameplay state and never re-runs the death penalty.

## Death-time binding is immutable

Persistence reads `respawnpolicy.Pending(entity)` after the death outcome has already bound its destination. A later checkpoint clear (including the current PvE checkpoint-forfeiture penalty) does not rewrite that pending destination.

On restore, the persisted binding is not passed back through normal spawn selection. The current respawn policy must recognize the same death context and still contain the same spawn point with the same allowed class and exact configured position. A mismatch fails closed instead of silently choosing a new destination.

The persisted `CheckpointID` is separate from the already-bound respawn destination. This preserves post-penalty checkpoint truth for later deaths while keeping the current death destination immutable.

## Restart timing semantics

`DueTick` is process/world-loop state and is not durable. `worldruntime.Loop` starts its tick counter again for a new process, so persisting a raw old-process `DueTick` would give it a false meaning after restart.

S3-F.13 stores:

`RemainingTicks = max(DueTick - captureTick, 0)`

When a defeated record joins at tick `T`, Runtime reconstructs:

`DueTick = T + RemainingTicks`

with overflow validation.

This deliberately preserves existing world-tick semantics: offline wall-clock time does not consume the countdown. No wall-clock deadline or new gameplay timing policy is introduced.

`RemainingTicks == 0` restores with `DueTick == joinTick`. Because normal command processing still precedes `applyDueRespawns`, the character is eligible for automatic respawn in that same tick's existing due phase. This preserves the S3-F.5 ordering boundary rather than creating a second timer path.

## Restore transaction

The transport still completes durable lookup and common validation before SessionWelcome. Runtime validates again on the world owner and additionally validates the respawn policy binding.

For a v2 defeated restore Runtime:

1. validates trusted CharacterID, durable revision, exact Gameplay World provenance, HP/transform, and defeated metadata;
2. converts remaining ticks into the current tick domain;
3. validates the bound respawn and checkpoint against the loaded respawn policy without mutating state;
4. spawns the new EntityID using the durable transform and registers restored HP/MaxHP/Defeated state;
5. installs the settled checkpoint and pending respawn for the new EntityID;
6. registers session/identity/replication state;
7. lets the existing due-respawn phase own the eventual revive transition.

Partial join failures remove any installed respawn/checkpoint state together with the new entity state.

## CharacterID ownership

Durability remains keyed by trusted CharacterID. Respawn policy is still EntityID-local live runtime state, but restore rebinds the durable death truth to the newly allocated EntityID. EntityID reuse by another CharacterID therefore does not select another character's durable record.

S3-F.10's active CharacterID invariant is unchanged. This stage does not add session takeover or admission locking; a reconnect that arrives before the old leave command has produced a save intent is still rejected by duplicate active ownership.

## Durability boundary

No file I/O enters the world tick. The world owner captures immutable state into the existing bounded process-local character-state outbox. The worldd worker performs the existing optimistic CAS save and durability sequence:

`temp write -> file fsync -> atomic rename -> directory fsync`

The S3-F.12 read-after-leave coordinator still flushes only save intents that already exist before a restore read.

## Non-goals

S3-F.13 does not add SQL, PostgreSQL, MySQL, Redis, Kafka, distributed CAS, account authentication, client-selected CharacterID, inventory, currency, equipment durability, progression, XP, loot, guild state, seamless session takeover, autosave, Death Outcome Journal compaction, world actor split, quantization, or delta compression.

It does not change lifecycle, Initial Vitals, dirty Vitals, Siege Load Lab, or S3-E scaling thresholds and filters.

## Test coverage

The stage covers:

- v1 alive compatibility;
- v1 defeated readability without invented respawn truth and fail-closed restore;
- v2 defeated roundtrip/reopen across PvE/PvP/Siege contexts;
- incomplete v2 defeated metadata rejection;
- immutable bound destination versus post-binding checkpoint clear;
- remaining-tick capture and restart-domain reconstruction;
- exact due-boundary behavior including zero remaining ticks;
- checkpoint restoration;
- policy/config binding drift fail-closed before partial spawn;
- no duplicate pending respawn after successful automatic respawn.

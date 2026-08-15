# S3-F.16 — Bounded Trusted Character Autosave Seam

## Goal

S3-F.16 adds a bounded periodic Character State capture seam for active trusted characters.

S3-F.15 made a save command restart-durable after the persistence worker appended and fsync'ed it to the Character State save journal. Its remaining crash window was explicit: an authoritative state change could live only in the active world for the whole session, and even a leave snapshot could still be lost if the process died before the process-local outbox reached journal fsync.

This stage reduces that exposure without putting filesystem I/O into the world owner. It does **not** provide zero-window durability.

## Production policy

`worldd` adds:

- `-character-state-autosave-seconds`, default `30.0`; `0` disables periodic autosave;
- `-character-state-autosaves-per-tick`, default `32`; when autosave is enabled it must be greater than zero.

At the default 20 Hz tick rate, 30 seconds is 600 ticks.

`worldruntime.DefaultConfig()` deliberately keeps `CharacterStateAutosaveEveryTicks == 0`. The production `worldd` opt-in is explicit so load tools, focused unit fixtures, and other Runtime embeddings do not silently acquire periodic persistence work.

## World-owner capture boundary

Autosave runs after the authoritative simulation tick and before replication. It performs only:

1. select a bounded set of due active trusted sessions;
2. read the authoritative Character State and world transform;
3. reconstruct already-bound defeated respawn truth when the character is defeated;
4. copy the resulting immutable v2 snapshot into the existing bounded process-local Character State outbox.

No journal append, fsync, Store load/save, checkpoint write, or network I/O is added to the world owner.

The existing S3-F.15 worker remains the durability path:

`autosave capture -> process-local outbox -> Store revision read -> journal append + fsync -> outbox Confirm -> Store CAS -> atomic checkpoint`

A snapshot is restart-durable only after journal append + fsync succeeds.

## Trusted-only policy

Only active sessions carrying a server-trusted CharacterID participate.

Ephemeral development identities:

- do not receive an autosave baseline;
- are skipped by the periodic sweep;
- never enter the durable Character State outbox through autosave.

This preserves the S3-F.10 durable identity boundary.

## Interval baseline

A successful register/join establishes the character's autosave baseline at that world tick.

Therefore a character admitted when the server tick is already large does not autosave immediately. The first periodic capture becomes eligible only after a full configured interval.

Leave/unregister cleanup removes the baseline.

The existing trusted leave path still captures a final authoritative save before entity/character cleanup. A recent periodic autosave does not suppress the final leave save.

## Bounded scheduling and fairness

The sweep has an independent `MaxCharacterStateAutosavesPerTick` ceiling. It does not borrow from or raise lifecycle, Initial Vitals, Dirty Vitals, snapshot, or command budgets.

The Runtime tracks the earliest tick at which any trusted character can next become due. Before that tick the autosave phase returns immediately without listing or sorting sessions. Session enumeration therefore occurs only when a sweep may contain due work, or on following ticks while a budget-limited backlog is being drained.

Active sessions are visited with a persistent round-robin cursor. When more trusted characters are due than the per-tick budget permits:

- only the configured maximum are attempted in that tick;
- `CharacterStateAutosaveBudgetExhausted` is reported;
- the cursor resumes at the first due session that did not receive an attempt;
- the next possible sweep is the following world tick;
- later ticks continue the sweep instead of restarting from the lowest SessionID.

When a sweep completes without exhausting the budget, the Runtime recomputes the earliest next-due tick from the trusted sessions it just examined.

The budget-exhaustion metric is intentionally a boolean rather than a fabricated deferred-count: the scheduler does not scan the rest of the due set after it reaches its work ceiling.

## Failure and retry

An autosave baseline advances only after the immutable snapshot is successfully enqueued into the process-local outbox.

If the bounded outbox is full or snapshot capture fails:

- the failure is reported through the existing Character State save failure path;
- the character remains due;
- the next possible sweep is no earlier than the next world tick;
- a later sweep retries it;
- active gameplay is not rolled back.

Persistence worker/journal/Store/checkpoint failures continue to use the S3-F.15 fail-closed production behavior.

## Defeated characters

Periodic capture reuses the existing S3-F.13 Character State v2 snapshot rules.

For a defeated character it persists only already-established server truth:

- death Context;
- death-time bound SpawnPointID;
- bound SpawnClass;
- exact bound respawn Position;
- remaining world ticks at capture time;
- current post-penalty CheckpointID.

It never re-runs respawn destination policy during autosave.

If the scheduled respawn becomes due before the autosave phase of that tick, the existing due-respawn phase applies first, so autosave captures the resulting alive state instead of stale defeated metadata.

## Crash/RPO semantics

S3-F.16 reduces the amount of active-session progress that can be absent from durable Character State truth, but the bound is intentionally not described as an exact wall-clock guarantee.

With a healthy worker, durable freshness is approximately constrained by:

`configured autosave interval + bounded sweep delay + outbox/journal worker delay`

plus filesystem latency until journal fsync succeeds.

A process crash can still lose:

- gameplay changes after the latest journal-fsync'ed autosave;
- a newly captured autosave that is still only in the process-local outbox;
- a final leave capture that has not yet reached journal fsync.

Therefore this stage does not claim synchronous durability, zero data-loss window, or a transactional world-tick/fsync boundary.

## Preserved reconnect behavior

S3-F.14 and S3-F.15 reconnect ordering remains unchanged:

- an already-active trusted CharacterID fails admission closed;
- after an older leave is ordered first, restore flushes existing pending Character State persistence before Store read;
- world-owner join must commit before SessionWelcome.

Autosave does not add takeover, old-session eviction, reservation, lease, or fencing semantics.

## Observability

`StepMetrics` adds:

- `CharacterStateAutosaveBudget`;
- `CharacterStateAutosaveAttempts`;
- `CharacterStateAutosaveEnqueued`;
- `CharacterStateAutosaveBudgetExhausted`.

Existing `CharacterStateSaveIntentsEnqueued` and `CharacterStateSaveIntentFailures` remain the shared capture outcome counters for both periodic and leave saves.

`worldd` startup logging reports the configured autosave tick interval and per-tick capture ceiling.

## Preserved scaling boundaries

S3-F.16 does not change:

- Protocol v6;
- GameV1 codec or UDP MTU;
- WorldSnapshot chunking;
- Network LOD or transform batch maximum;
- lifecycle desired-vs-known truth or TrySend-confirm semantics;
- lifecycle churn ceiling of 6,000 per snapshot;
- Initial Vitals budgets;
- Dirty Vitals ceiling of 4,000 per tick;
- snapshot cadence or replication budgets;
- workflow path filters or acceptance thresholds.

The new autosave capture budget is separate and bounded, and non-due ticks avoid autosave session enumeration entirely.

## Explicit non-goals

S3-F.16 does not add synchronous journal/fsync acknowledgement to the world tick, zero-window durable capture, seamless session takeover, old-session eviction, admission reservation/lease/fencing token, distributed storage, multi-process writers, journal compaction/rotation, inventory/currency/equipment/progression/XP/loot/guild persistence, Client CharacterID selection, account authentication protocol, Client changes, protocol revision, world actor split, quantization, or delta compression.

No Go↔Godot E2E claim is made.

## Test coverage

The stage covers:

- first autosave only after a full join/register interval;
- post-simulation authoritative HP/transform capture;
- trusted-only participation and no ephemeral bookkeeping;
- per-tick budget enforcement and round-robin eventual service;
- outbox-full failure without baseline advancement followed by retry;
- defeated autosave preserving exact bound respawn and post-penalty checkpoint truth;
- seconds-to-ticks conversion, including ceil behavior and disabled mode;
- invalid interval/budget configuration rejection.

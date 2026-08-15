# S3-F.15 — Durable Character State Save-Intent Journal

## Goal

S3-F.15 closes the next bounded Character State persistence gap left explicitly deferred by S3-F.11: process-local save intents that have reached the persistence worker can now cross a `worldd` restart before the per-character Store update completes.

The stage does not add autosave, session takeover, distributed storage, or filesystem I/O to the world tick.

Before this stage the chain was:

`world-owner leave capture -> process-local outbox -> Store.Save`

A crash after outbox capture but before `Store.Save` could lose the pending process-local intent.

The new chain is:

`world-owner leave capture -> process-local outbox -> append + fsync save journal -> outbox Confirm -> Store.Save -> atomic consumer checkpoint`

## Durability boundary

WorldRuntime is unchanged. Trusted leave still captures an immutable Character State snapshot into the bounded process-local outbox without file I/O.

The worldd persistence worker now drains that outbox oldest-first:

1. append one framed save-intent record to the Character State save journal;
2. `fsync` the journal file;
3. only after append + fsync succeeds, Confirm the process-local outbox intent.

Therefore a successfully journaled intent survives loss of the process-local outbox and can be recovered after restart.

This does **not** claim that `Outbox.Enqueue` itself is durable. A sudden process crash before the worker has appended and fsync'ed a newly captured intent can still lose that process-local intent. Eliminating that remaining window would require a different synchronous/durable capture architecture or an autosave policy, both outside this bounded stage.

## Journal identity and record identity

Each save journal has a random 128-bit Journal ID in its durable header.

Every durable record receives a journal-local monotonic `RecordID = 1, 2, 3, ...` across process restart.

The original `IntentID` is retained only for process-local diagnostics. It is not the durable identity because a new `worldd` process creates a new in-memory outbox sequence.

Each record contains:

- durable RecordID;
- original process-local IntentID;
- trusted CharacterID;
- complete current Character State v2 snapshot fields;
- complete defeated-respawn metadata when defeated.

Only trusted identities and valid v2 snapshots can be appended.

## Framing and corruption policy

Journal schema v1 uses the same bounded local durability pattern already proven by the S3-F.9 Death Outcome Journal:

- durable header magic + random Journal ID;
- 4-byte big-endian payload length;
- strict JSON payload;
- 4-byte CRC32C Castagnoli checksum;
- 1 MiB maximum payload.

Startup validates:

- journal header and non-zero Journal ID;
- frame length;
- CRC;
- strict JSON and schema version;
- contiguous RecordID sequence;
- trusted CharacterID;
- current Character State v2 snapshot invariants.

Only an incomplete final frame is automatically repaired as a crash-torn append. CRC mismatch, malformed JSON, invalid identity/state, RecordID gaps, and checkpoint mismatch fail closed.

## Durable Store consumer checkpoint

A separate checkpoint file records:

- Journal ID;
- last successfully applied RecordID;
- exact end offset of that record.

Checkpoint update is durable and atomic:

`temp write -> temp fsync -> rename -> directory fsync`

Store side effect occurs before checkpoint advancement. Recovery is therefore at-least-once.

If the Store write succeeds and the process crashes before checkpoint advancement, the same record may be replayed. The consumer compares the current Store snapshot with the journal snapshot; an exact match is treated as already applied, so replay can advance the checkpoint without unnecessarily incrementing the optimistic Character State revision.

## Startup recovery

Before the network server is opened, worldd:

1. opens and validates the Character State Store;
2. opens and validates/repairs-only-torn-tail the save journal;
3. loads and validates the save checkpoint;
4. replays every durable journal record after the checkpoint into the Store;
5. atomically checkpoints each successfully applied record;
6. only then proceeds with normal world/network startup.

Thus a save intent that was already journal-fsync durable before a crash becomes current Store truth before a returning trusted character can be admitted and restored.

## Runtime reconnect ordering

S3-F.14 ordering is preserved and strengthened without adding takeover.

`LoadRestore` still runs outside the world tick under the process-local Character State persistence coordinator. Before reading the Store it now:

1. drains all currently pending process-local save intents into the durable journal;
2. catches the Store/checkpoint up to the journal tail;
3. reads and validates the resulting Store record.

For a successful older leave already ahead of S3-F.14 admission, the bounded chain is now:

`leave capture -> journal fsync -> Store apply/checkpoint -> durable restore read -> world-owner join commit -> SessionWelcome`

If the old session is still active because no leave was enqueued, S3-F.14 still rejects the new trusted connection. No ownership stealing or eviction is introduced.

## Failure policy

Journal append/sync, journal corruption, Store application, or checkpoint persistence errors stop the Character State persistence worker and cancel the production worldd context. The server does not silently continue while claiming durable Character State progress.

Gameplay leave semantics are not rolled back after the world owner has already released a character. This stage adds a durable handoff layer; it does not make filesystem failure part of the world-tick gameplay transaction.

## Shutdown ordering

After the world loop stops producing new save intents, the Character State worker:

1. journals + fsyncs all remaining process-local outbox intents;
2. consumes all journal records through the durable Store;
3. advances the checkpoint to the journal tail;
4. exits before the journal file is closed.

## Configuration

worldd adds:

- `-character-state-save-journal`, default `data/character-state-saves.journal`;
- `-character-state-save-checkpoint`, default `data/character-state-saves.checkpoint.json`.

Existing `-character-state-dir` and bounded `-character-state-outbox-capacity` remain unchanged.

## Preserved semantics

- Protocol remains v6.
- Client repo is unchanged.
- Character State Store schema remains v2; this journal has its own schema v1.
- S3-F.13 defeated respawn restore semantics are unchanged.
- S3-F.14 trusted reconnect admission and join-completion barriers are unchanged.
- No file/network I/O is added to the world tick.
- Lifecycle, Initial Vitals, Dirty Vitals, snapshot cadence, input/action sequencing, combat cooldown, and scaling budgets are unchanged.
- No workflow filter or threshold is relaxed.

## Explicit non-goals

S3-F.15 does not add:

- periodic or gameplay-triggered autosave;
- zero-window synchronous durable capture at `Outbox.Enqueue`;
- seamless session takeover or old-session eviction;
- admission leases/fencing tokens;
- SQL/PostgreSQL/MySQL, Redis, Kafka, or a distributed backend;
- multi-process writers to the same Character State directory/journal;
- journal compaction, retention, or rotation;
- inventory, currency, equipment, progression, XP, loot, or guild persistence;
- Client CharacterID selection or account authentication protocol;
- world actor split, quantization, or delta compression.

## Test coverage

The stage covers:

- alive and defeated save-intent journal roundtrip across reopen;
- durable checkpoint reopen and resume;
- incomplete trailing frame repair only;
- CRC corruption fail-closed;
- outbox intent journal-fsync before process-local Confirm;
- Store remains unchanged until durable journal consumption;
- sequential journal intents preserve final Store truth and revision progression;
- startup replay of durable uncheckpointed intents;
- idempotent replay after Store write but before checkpoint;
- trusted reconnect flush through journal + Store before read;
- shutdown drains both process-local outbox and durable journal;
- existing Character State restore, defeated respawn, reconnect admission, Server CI, race, and Siege Load Lab remain regression coverage.

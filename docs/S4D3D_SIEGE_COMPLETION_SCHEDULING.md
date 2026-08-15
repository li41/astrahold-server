# S4-D.3D — Siege Completion Delivery & Next-Round Scheduling Policy

## Scope

S4-D.3D turns the explicit D.3C `Completed -> Gate` reset seam into a bounded Server-side scheduling policy while preserving Protocol v7 and the existing Godot Client.

The policy is intentionally about **Server scheduling around the existing reliable send boundary**. It does not add or pretend to have a Client acknowledgement protocol.

## Reliable completion evidence

`SiegeMatchState` already advances its per-session delivery stamp only after `TrySend` succeeds. D.3D reuses that exact invariant.

During the existing Siege replication loop the Server now records, for the current authoritative Match revision:

- number of active sessions in the stable Siege session list;
- number of sessions whose current `SiegeMatchState` still could not be accepted by `TrySend`.

A session is considered delivery-complete for scheduling purposes only when its delivery stamp matches both the current Match revision and its current Server-owned team role.

This means "delivered" in D.3D means **accepted into the reliable outbound path**. Protocol v7 has no Client ACK, application receipt receipt, or render acknowledgement, and D.3D does not infer one.

No second session enumeration is added. Presence sampling and Siege replication still reuse the same stable session list established in D.2A/D.2B, and scheduling consumes the summary produced by that replication pass.

## Minimum and maximum Completed hold

`worldruntime.DefaultConfig()` enables two duration bounds:

- `SiegeCompletedMinHold = 2s`
- `SiegeCompletedMaxHold = 10s`

Durations accumulate from `Step(delta)` rather than assuming a fixed 20 Hz tick rate.

When a new authoritative `Completed` Match revision is first observed, elapsed hold begins at zero. The simulation time that produced the completion is not retroactively counted as display/hold time.

The next-round reset is scheduled when either:

1. minimum hold has elapsed **and** every session active in the current replication pass has accepted the current Completed revision; or
2. maximum hold has elapsed, even if one or more active sessions remain backpressured.

With zero active sessions, the all-accepted condition is vacuously true, so the world waits the minimum hold and then continues.

`SiegeCompletedMaxHold == 0` with `SiegeCompletedMinHold == 0` disables automatic scheduling for tests or specialized runtimes. Invalid negative durations, minimum-without-maximum, or minimum greater than maximum fail fast at Runtime construction.

## Why both bounds exist

The minimum hold prevents a healthy low-latency session set from causing `Completed` to collapse into the next Gate round immediately after the same replication cycle.

The maximum hold prevents one unhealthy or permanently backpressured connection from blocking the global Siege forever.

A session that becomes active only after the scheduling decision boundary is not retroactively part of the completed-round delivery set. That connection joins whatever authoritative Match state exists when its registration is processed.

## Queue ordering

Scheduling never calls `StartNextRound` inline from replication.

When the policy becomes eligible it calls the existing Server-only `EnqueueStartNextSiegeRound`. The command therefore executes through the normal bounded world command queue on the next `Step`.

This preserves the D.3C single-world-owner mutation boundary:

1. current Step replicates `Completed`;
2. policy evaluates existing delivery stamps;
3. policy enqueues next-round intent;
4. next Step drains the command;
5. D.3C atomically restores Gate/blocker/objectives/roles and returns Match to Gate;
6. existing Dynamic and Siege reliable replication publishes the new round.

If queue insertion fails, the policy does not latch the reset as scheduled and retries on a later Completed tick. If execution of a scheduled reset fails, the queued latch is cleared so the policy can retry instead of becoming permanently stuck.

## Observability

`StepMetrics` adds:

- `SiegeCompletedActiveSessions`
- `SiegeCompletedPendingDeliveries`
- `SiegeCompletedElapsed`
- `SiegeRoundResetsScheduled`
- `SiegeRoundResetsForcedByMaxHold`
- `SiegeRoundResetScheduleFailures`

These metrics do not change gameplay truth or wire format.

## Preserved boundaries

S4-D.3D does not change:

- Protocol v7 or Message 106 fields;
- GameV1 codec;
- Client repository;
- Gameplay World schema 2 / `castle-sandbox` revision `s3d-001`;
- Siege config schema 3 / revision `s4d2b-001`;
- 10-second throne capture duration;
- durable ownership store schema v1;
- D.3B ownership CAS/fsync completion barrier;
- D.3C ownership-derived attacker/defender rotation;
- UDP MTU 1200;
- WorldSnapshot chunking;
- Network LOD transform max64;
- lifecycle desired-vs-known and Confirm-after-`TrySend` behavior;
- lifecycle churn max 6000/snapshot;
- Initial Vitals budgets;
- Dirty Vitals max 4000/tick.

## Explicitly deferred

- Client ACK or application-level Siege receipt protocol;
- winner/owner/round/capture fields on the wire;
- Godot result / ownership HUD;
- Protocol v8;
- defender timeout victory;
- distributed/multi-process round coordination;
- mid-round Gate/phase/capture persistence;
- administrator-facing live tuning of the 2s/10s defaults.

## Acceptance coverage

Focused WorldRuntime tests cover:

- a healthy session accepting Completed immediately but still respecting the minimum hold;
- scheduling only after the minimum hold expires;
- reset executing on the following world-owner Step rather than inline in replication;
- a permanently backpressured Completed delivery remaining pending through the minimum hold;
- the maximum hold forcing reset despite that pending session;
- the forced path preserving D.3C round rotation and returning to Gate.

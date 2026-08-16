# S4-F.27 - Rollout Observation Evidence / Convergence Timing

## Scope

F.27 does not add remote process execution, service discovery, quorum membership, or another authorization fence. F.25 remains the `worldd` security authority and F.26 remains the explicit all-required acknowledgement gate.

F.26 already makes missed or delayed activation observable as `incomplete`: a member does not count until its durable F.25 acknowledgement matches the target epoch, revocation revision, semantic digest, and lease. The remaining decision gap is operational measurement: there was no controller-owned evidence showing when each required acknowledgement first became visible or how long a rollout spent waiting for convergence.

F.27 adds that evidence to `revocationctl wait` and `revocationctl rollout` without changing the schema-v1 rollout plan.

## Controller-owned timing evidence

Successful or incomplete wait results now include an `observation` object:

```json
{
  "timing_source": "controller",
  "started_at": "2026-08-17T00:00:00Z",
  "completed_at": "2026-08-17T00:00:00.3Z",
  "elapsed_ms": 300,
  "acks": [
    {
      "instance_id": "instance-a",
      "first_observed_at": "2026-08-17T00:00:00.1Z",
      "observed_elapsed_ms": 100
    },
    {
      "instance_id": "instance-b",
      "first_observed_at": "2026-08-17T00:00:00.3Z",
      "observed_elapsed_ms": 300
    }
  ]
}
```

`first_observed_at` is deliberately not the F.25 ack file's `acknowledged_at`. The latter is written by each `worldd` and can involve a different host clock. F.27 measures only when this controller invocation first observes an already-valid exact acknowledgement, so all elapsed values share one controller timing domain.

Existing acknowledgements present when `wait` begins are observed at elapsed zero. A later `wait` invocation starts a new observation window; F.27 is not a durable telemetry database.

## Timeout and lease clocks

The acknowledgement timeout is measured from the controller's own elapsed clock. With Go's production `time.Now`, elapsed subtraction uses the process monotonic clock when available, so wall-clock corrections do not manufacture a longer or shorter `ack_timeout`.

The F.25 `valid_until` lease remains an absolute UTC security boundary. The wait loop therefore stops at whichever boundary comes first:

```text
controller elapsed >= ack_timeout
OR
controller UTC >= valid_until
```

A backwards controller clock in a clock source without monotonic information is rejected rather than producing misleading negative evidence.

## Security and privacy boundary

Observation evidence contains instance IDs and controller timestamps only. It does not add revoked SPKI identifiers, certificate material, private keys, account secrets, recovery proofs, forwarding values, target file contents, or process-control credentials.

F.27 does not infer why a member was delayed. A pending member can represent delayed SIGHUP/restart activation, deployment transport delay, process failure, filesystem propagation delay, or another operator issue. The evidence is intended to decide whether those delays are material before adding a larger control plane.

## Production acceptance

The F.27 production gate reuses the real two-`worldd` F.26 rollout scenario and adds controller timing assertions:

- epoch 1 already-acknowledged members appear in one controller observation window;
- epoch 2 reloads instance A first, deliberately pauses, then reloads B; the controller reports A's exact ack before B's and converges only after B;
- epoch 3 reloads only A and times out; the result keeps A's observation evidence, names B pending, and records approximately the configured two-second wait;
- later `wait` after B reloads converges without changing the F.25/F.26 authority contract;
- revoked Leaf A, healthy Leaf B, direct Client behavior, and rollback fencing remain unchanged.

## Decision gate after F.27

F.27 exists to produce evidence for the next decision, not to predetermine it. If deployment measurements show that publish-success followed by missing or excessively delayed activation is a material operational risk, a later bounded stage may define an activation/supervisor handoff contract. Without that evidence, remote execution, service discovery, consensus, and a larger PKI control plane remain out of scope.

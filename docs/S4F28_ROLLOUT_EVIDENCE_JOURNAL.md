# S4-F.28 - Rollout Evidence Journal / Decision Metrics

## Scope

F.28 is the evidence-retention stage after F.27. It does **not** add remote process execution, service discovery, dynamic membership, quorum/consensus, or another trusted-proxy authorization fence. F.25 remains the `worldd` security authority, F.26 remains the explicit all-required acknowledgement gate, and F.27 remains the controller timing model.

F.27 deliberately made each `wait` invocation an in-memory observation window rather than a durable telemetry database. That is sufficient to prove correctness, but it is not sufficient to accumulate real deployment samples over multiple rollouts. F.28 closes only that measurement gap.

## CLI

Evidence capture is opt-in and only applies to commands that produce a final F.27 observation:

```text
revocationctl wait    -plan rollout.json -evidence-dir /var/lib/astrahold/revocation-evidence
revocationctl rollout -plan rollout.json -evidence-dir /var/lib/astrahold/revocation-evidence
revocationctl report  -evidence-dir /var/lib/astrahold/revocation-evidence
```

`publish` does not produce acknowledgement timing and therefore rejects `-evidence-dir`.

Without `-evidence-dir`, `wait` and `rollout` preserve the F.27 behavior and output contract. With it, the command still emits the same final rollout result to stdout and then durably records that final `converged` or `incomplete` observation. If the requested evidence write fails, the rollout/security state is not rolled back, but the command returns exit 1 so an operator cannot mistake missing measurement for a successfully recorded sample.

The existing rollout exit meanings remain:

- converged + evidence recorded: exit 0;
- incomplete timeout/lease expiry + evidence recorded: exit 2;
- invalid config/ack/target, publication failure, malformed evidence, or requested evidence persistence failure: exit 1.

## Owner-only immutable record directory

The evidence directory is controller-local state, not a new authority source.

- If absent during `wait`/`rollout`, it is created as 0700.
- If already present, it must be a real directory rather than a symlink and must have no group/other permission bits.
- Each record is written as 0600.
- The record body is written and `fsync`ed to a same-directory temporary file first.
- A random 128-bit lowercase-hex record ID becomes both the record's `record_id` and immutable filename identity.
- The fully written inode is committed with a same-directory no-overwrite hard link, the temporary name is removed, and the directory is `fsync`ed.
- A matching existing filename is never replaced; a random-ID collision is retried with a new ID.

Record names are:

```text
rollout-evidence-v1-<32 lowercase hex>.json
```

The schema-v1 record contains only:

- record ID, command (`wait` or `rollout`), and controller `recorded_at`;
- the existing final F.27 rollout result, including target epoch/revision/semantic digest/lease, required/acknowledged/pending instance IDs, status/reason, and controller observation timing.

It does not persist the rollout plan path, revocation/distribution/ack target paths, raw revoked SPKI identifiers, certificate/key material, passwords, recovery proofs, forwarding values, or process-control credentials.

## Strict bounded report

`revocationctl report` is read-only. It scans only filenames matching the F.28 immutable record pattern and ignores unrelated operator files. Matching records are bounded to 1024 files and 64 KiB each.

Every matching record is strict-decoded and revalidated before it contributes to a report:

- schema and random ID must match the immutable filename;
- file must be a regular non-symlink owner-only file;
- command must be `wait` or `rollout`;
- result must be a final `converged` or `incomplete` wait result rather than a publication/partial result;
- required, acknowledged, pending, and observation ack instance lists must remain sorted, unique, and internally consistent;
- incomplete results must partition the required set exactly into acknowledged and pending members;
- observation timing source must remain `controller`, with non-negative elapsed values bounded by the recorded observation window;
- `recorded_at` must equal that observation's `completed_at`.

A malformed, broadened-permission, symlinked, or internally inconsistent matching record makes the report fail hard instead of silently skewing deployment evidence.

The report is intentionally descriptive:

```json
{
  "schema_version": 1,
  "timing_source": "controller",
  "records": 4,
  "converged_records": 3,
  "incomplete_records": 1,
  "timeout_records": 1,
  "lease_expired_records": 0,
  "max_elapsed_ms": 2000,
  "instances": [
    {
      "instance_id": "instance-a",
      "required_records": 4,
      "observed_records": 4,
      "pending_records": 0,
      "max_observed_elapsed_ms": 100
    }
  ]
}
```

F.28 does not assign an activation cause, calculate a policy threshold, or automatically trigger a supervisor. A pending record can still mean delayed SIGHUP/restart activation, deployment transport delay, process failure, filesystem propagation delay, or another operator issue. Correlation with deployment logs remains an operator responsibility.

## Authority boundary

Evidence files and reports are **not** trusted-proxy authorization inputs. Deleting, altering, or failing to write F.28 evidence cannot make a revoked proxy credential valid, extend an F.25 lease, satisfy an F.26 required acknowledgement, or change `worldd` traffic decisions.

Conversely, evidence persistence failure is surfaced as an operational command failure because the operator explicitly requested a durable sample. Retrying an identical F.26 target remains safe under the existing idempotent publication and exact-ack rules.

The Godot Client is unchanged. It does not receive, submit, store, or acknowledge evidence directory paths, record IDs, report metrics, observation timestamps, pending counts, supervisor state, or any deployment authority.

## Production acceptance

The F.28 production gate reuses the two-real-`worldd` TLS 1.3 F.26/F.27 rollout harness and enables one owner-only evidence directory for every `wait`/`rollout` observation. It proves that:

- epoch 1 convergence, staggered epoch 2 convergence, epoch 3 timeout, and the resumed epoch 3 convergence each create one immutable evidence record;
- the report sees four records: three converged and one timeout-incomplete;
- instance B has one pending sample while instance A does not;
- the timeout remains approximately the configured two-second controller window;
- the evidence directory/records stay 0700/0600 and broadening the directory mode makes report fail closed;
- raw revoked SPKI identifiers and account/private-key secrets do not appear in evidence records;
- rollback fencing, revoked Leaf A, healthy Leaf B, and unchanged direct Client behavior remain intact.

## Decision gate after F.28

F.28 makes real rollout samples durable enough to support the decision that F.27 could only frame. The next stage is still **not automatic remote execution**.

Collect real production/deployment evidence first. If those records, correlated with deployment logs, show that publish-success followed by missed or excessive activation delay is a material recurring risk, a later bounded stage may define an activation/supervisor handoff contract. Because F.28 now occupies the evidence-retention milestone, that candidate would be S4-F.29 rather than retroactively expanding F.28.

Without such evidence, service discovery, remote command execution, consensus/quorum membership, and a larger PKI control plane remain out of scope.

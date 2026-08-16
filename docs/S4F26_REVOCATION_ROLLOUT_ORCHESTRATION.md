# S4-F.26 - Revocation Rollout Orchestration / Required-Ack Gate

## Scope

F.26 keeps the F.25 `worldd` authorization fence unchanged. Protocol v9, schema-v4 account state, F.19 edge-policy schema v1, F.20 retirement semantics, F.23 matched identity, F.24 SPKI credential identity, and the F.25 epoch/digest/lease/ack rules remain the runtime authority.

The F.25 remaining gap was operational: each member could produce durable convergence evidence, but there was no bounded Server-side tool that defined the required member set for one rollout, published the same paired F.24/F.25 target to those members, and refused to call the rollout complete until every required member acknowledged that exact target.

F.26 adds `cmd/revocationctl`. It is a deployment/controller tool, not a new network service and not a consensus system.

## Rollout plan

`revocationctl` accepts a strict schema-v1 plan:

```json
{
  "schema_version": 1,
  "epoch": 43,
  "valid_until": "2026-08-17T00:10:00Z",
  "ack_timeout": "30s",
  "poll_interval": "250ms",
  "revocation_source_file": "candidate-revocations.json",
  "required_instances": [
    {
      "instance_id": "worldd-a",
      "revocation_file": "a/revocations.json",
      "distribution_file": "a/distribution.json",
      "ack_file": "a/ack.json"
    },
    {
      "instance_id": "worldd-b",
      "revocation_file": "b/revocations.json",
      "distribution_file": "b/distribution.json",
      "ack_file": "b/ack.json"
    }
  ]
}
```

Relative paths resolve from the plan directory. Bounds are deliberately small: at most 64 required instances, 64 KiB plan/source files, the existing F.24 limit of 256 revoked SPKI identifiers, `ack_timeout` from 100 ms through 30 minutes, and `poll_interval` from 10 ms through 5 seconds. Instance IDs use the same F.25 character and length contract.

`valid_until` remains the F.25 lease and therefore must be canonical UTC RFC3339 seconds, still in the future at publish time, and no more than 24 hours ahead. F.26 additionally requires the configured acknowledgement timeout to end no later than that lease.

## Exact F.24 semantic authority

The source file is the existing strict F.24 schema v1. `revocationctl` validates lowercase 64-hex SPKI identifiers, deduplicates and sorts them, and computes the same semantic digest as `worldd`:

```text
SHA-256(
  "astrahold/session-leaf-revocation-authority/v1\x00"
  || sorted raw 32-byte SPKI digests
)
```

It writes a canonical F.24 target and an F.25 manifest whose `revocation_authority_sha256` is that digest. The production gate consumes those generated files with unmodified F.25 `worldd`, so controller/runtime digest compatibility is continuously tested.

## Bounded publish protocol

One plan defines one target epoch for an explicit required instance set. Before any write, the controller preflights every current distribution target and rejects:

- a target already at a higher epoch; or
- conflicting reuse of the same epoch with a different semantic digest or lease.

Publication is then two-phase across the local target paths:

```text
validate all required targets
→ atomically stage canonical F.24 revocation candidate to every unique target pair
→ only after every revocation stage succeeds, atomically publish each F.25 distribution manifest
```

The distribution manifest is the per-target commit marker. If a `worldd` reload races after its revocation file changes but before its manifest changes, F.25 sees the digest mismatch and retains its complete LKG authority.

If manifest publication fails after some members committed, F.26 deliberately does not roll those members back. A rollback could re-authorize a compromised key. The command reports a partial result; rerunning the identical epoch/digest/lease is idempotent and completes the remaining targets.

F.26 does not claim cross-filesystem atomicity or cross-host locking.

## Required-ack gate

`revocationctl wait` and `revocationctl rollout` collect the F.25 durable ack for every member in `required_instances`.

A member counts as acknowledged only when all of these exactly match the target:

```text
instance_id
AND epoch
AND revocation_revision
AND revocation_authority_sha256
AND valid_until
```

A missing or older ack is pending. A malformed ack, same-epoch metadata conflict, or ack that has already advanced beyond the target epoch is a hard error rather than ambiguous success.

The controller returns:

- `status=converged`, exit 0: every explicitly required member acknowledged the exact target;
- `status=incomplete`, exit 2: timeout or lease expiry occurred with required members still pending;
- exit 1: invalid plan/source/target/ack state or publication failure.

Results list instance IDs only. They do not emit revoked SPKI identifiers, certificate material, account secrets, forwarding values, or recovery proofs.

## Commands

```text
revocationctl publish -plan rollout.json
revocationctl wait    -plan rollout.json
revocationctl rollout -plan rollout.json
```

`publish` only publishes the paired files. `wait` only evaluates convergence. `rollout` performs publish then waits for convergence.

F.26 intentionally does not invent a remote process-control protocol. A deployment supervisor remains responsible for delivering the existing `SIGHUP` (or equivalent restart/reload action) after files are published. This keeps host/process control outside the security meaning of the F.25 acknowledgement.

## Production acceptance

The F.26 production gate uses two real TLS 1.3 `worldd` instances and the same F.24/F.25 proxy-key scenario. It proves:

- `revocationctl publish` creates an epoch-1 pair that both instances consume and acknowledge;
- one epoch-2 `rollout` publishes the same revocation/digest/lease target to both required instances;
- after only instance A reloads, A acknowledges epoch 2 but the controller remains blocked because B is explicitly required;
- after B reloads, the controller returns `converged` for exactly A+B;
- the revoked Leaf A fails on both members while healthy different-key Leaf B survives and direct Client TLS remains unchanged;
- an epoch-3 lease/revision rollout with only A reloaded returns `incomplete` / exit 2 naming B as pending;
- the same epoch-3 plan can later `wait` to convergence after B reloads; and
- an attempted publish of older epoch 2 is rejected before target writes, retaining epoch 3.

## Non-goals / next gap

F.26 is not Raft/Paxos, quorum membership, service discovery, a central online revocation service, remote process execution, CRL/OCSP ingestion, ACME/PKI automation, HSM attestation, automatic compromise detection, distributed rate limiting, WAF/CDN policy, or PROXY protocol.

After F.26, the bounded revocation path has local credential revocation, stale-member lease fencing, explicit rollout membership, and an all-required ack decision. Any next stage should be selected from measured deployment evidence rather than automatically adding consensus or a larger PKI control plane.

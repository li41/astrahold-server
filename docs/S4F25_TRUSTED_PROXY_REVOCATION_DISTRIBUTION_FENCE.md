# S4-F.25 - Multi-Instance Trusted Proxy Revocation Distribution Fence

## Scope

F.25 keeps Protocol v9, schema-v4 account state, F.17 forwarding parsing, F.19 edge-policy schema v1, F.20 opt-in connection retirement, F.21 semantic no-op detection, F.22/F.23 selective identity retirement, and the F.24 SHA-256 SPKI credential identifier unchanged.

F.24 deliberately ended at one `worldd` process: an operator-owned revocation file could revoke one compromised proxy key while preserving another healthy key for the same exact DNS identity. The remaining operational gap was a stale multi-instance member that missed the file update or SIGHUP and could continue accepting the compromised key indefinitely.

F.25 adds a bounded distribution lease and per-instance durable acknowledgement. It is not consensus and it does not make `worldd` a distributed PKI service.

## Distribution manifest

F.25 is opt-in and requires all three flags together in F.24 edge-policy mode:

```text
-session-login-trusted-proxy-leaf-revocation-distribution-file
-session-login-trusted-proxy-leaf-revocation-instance-id
-session-login-trusted-proxy-leaf-revocation-ack-file
```

The distribution file is strict schema v1:

```json
{
  "schema_version": 1,
  "epoch": 42,
  "revocation_authority_sha256": "64-lowercase-hex",
  "valid_until": "2026-08-17T00:00:00Z"
}
```

`revocation_authority_sha256` is the F.24 semantic digest over the sorted revoked SPKI identifiers. The manifest and revocation file are therefore a paired candidate: a revocation change cannot publish unless the manifest digest matches it.

`epoch` is a monotonic unsigned distribution sequence. A higher epoch may change the revoked set or only renew the lease. Reusing one epoch with a different digest or `valid_until`, or moving to a lower epoch, is rejected and preserves the complete current F.24/F.25 last-known-good authority.

`valid_until` must be canonical UTC RFC3339 seconds, must still be in the future when loaded, and may be at most 24 hours ahead. A member that misses later distribution stops granting trusted-proxy authority when its current lease expires. Direct/untrusted TLS peers are outside this fence.

## Per-instance acknowledgement and restart floor

Each instance has a stable operator-provided `instance-id` and a distinct local ack file. After a valid startup or reload, `worldd` atomically writes a 0600 acknowledgement:

```json
{
  "schema_version": 1,
  "instance_id": "edge-worldd-a",
  "epoch": 42,
  "revocation_revision": "leaf-017",
  "revocation_authority_sha256": "64-lowercase-hex",
  "valid_until": "2026-08-17T00:00:00Z",
  "acknowledged_at": "2026-08-16T23:45:00Z"
}
```

The ack provides two bounded properties:

1. operators can collect `(instance_id, epoch, digest, valid_until)` and determine whether the intended set of instances converged; and
2. on restart the local ack is a durable epoch floor, so a member cannot silently restart on a distribution epoch older than the last one it acknowledged.

Deleting the ack file is an explicit operator action that removes that local restart floor. F.25 does not attempt to protect the host from an administrator who can rewrite both configuration and state.

## Fail-closed publication order

For F.25 reload, `worldd`:

```text
load F.24 revocation candidate
→ load F.25 distribution candidate
→ require manifest digest == revocation semantic digest
→ reject epoch rollback / conflicting epoch reuse
→ publish F.24 authority (generation++ only if revoked set changed)
→ publish F.25 epoch/lease
→ atomically write local acknowledgement
```

The in-memory transition is intentionally fail-closed. If the F.24 and F.25 snapshots are momentarily mismatched, credential authorization denies proxy authority. If the final ack write fails after a valid candidate has published, the new authority remains active but the instance marks acknowledgement unhealthy and denies all trusted-proxy credential authority until the same epoch can be acknowledged successfully. Rolling back to an older authority just because the ack output failed would be less safe when the new authority revokes a compromised key.

A same-epoch SIGHUP may retry a failed acknowledgement only when digest and lease are exactly unchanged.

## Runtime fence

Every fresh trusted-proxy handshake and every request on an established trusted-proxy connection now requires all of:

```text
F.19/F.23 network + exact identity authority
AND verified F.24 SPKI credential
AND SPKI not revoked
AND F.25 manifest digest matches current F.24 authority
AND local ack is healthy
AND now < valid_until
```

Therefore a missed distribution is bounded by the previously acknowledged lease instead of remaining trusted forever. An expired member fails closed for the entire trusted-proxy forwarding authority, not only for one credential, because it can no longer prove that its local revocation set is current.

F.20 still controls active socket retirement on successful policy/revocation publications. Lease expiry itself is an authorization fence: fresh handshakes fail and established keep-alive requests lose forwarding authority even if an idle socket is not proactively closed at the exact expiry instant.

## Production acceptance

The F.25 production gate runs two real `worldd` instances with the same edge CA, exact DNS identity, and two different proxy keys. It proves:

- both instances acknowledge epoch 1;
- instance A receives epoch 2 revoking Leaf A and rejects A while preserving healthy Leaf B;
- instance B deliberately misses the update and still behaves under epoch 1 only until its short lease expires;
- after expiry, B rejects trusted-proxy handshakes for both leaves while direct server-auth-only Client TLS remains unchanged;
- after B receives epoch 2 it writes an ack matching A on epoch/digest/lease, rejects Leaf A, and accepts Leaf B;
- an attempted epoch rollback is rejected with the epoch-2 LKG and ack retained; and
- the local durable ack floor rejects the same rollback across restart in focused tests.

## Non-goals / remaining gap

F.25 does not add a central publisher, quorum service, consensus, cross-host locking, CRL/OCSP ingestion, ACME automation, HSM attestation, automatic compromise detection, distributed rate limiting, WAF/CDN policy, or PROXY protocol.

The next bounded operational gap is orchestration: an operator tool or deployment controller can publish one epoch, collect the required instance ack set, and make an explicit rollout decision. That is separate from the `worldd` authorization fence introduced here.

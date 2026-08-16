# S4-F.24 - Trusted Proxy Leaf Credential Revocation / Certificate Instance Fence

## Scope

F.24 adds a bounded Server-owned revocation authority for trusted reverse-proxy mTLS credentials. It does not change Protocol v9, durable account schema v4, the F.19 edge-policy schema, gameplay authority, or the direct Godot Client TLS contract.

## Threat model and identifier

The remaining F.23 gap is private-key compromise when two proxy credentials share the same trusted CA, socket binding, and exact DNS SAN identity. F.24 therefore uses exactly one canonical identifier:

`lowercase hex SHA-256(leaf certificate RawSubjectPublicKeyInfo)`

This is an SPKI key identifier rather than a certificate-DER fingerprint. Reissuing a certificate with the compromised key remains revoked, while a healthy certificate for the same exact DNS identity using another key remains usable. Certificate serial numbers are not an authority primitive.

## Independent revocation source

The F.19 schema stays at v1. F.24 adds:

`-session-login-trusted-proxy-leaf-revocation-file`

The file is valid only with `-session-login-trusted-proxy-edge-policy-file` and uses strict schema v1:

```json
{
  "schema_version": 1,
  "revision": "leaf-revocations-2026-08-16",
  "revoked_spki_sha256": [
    "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
  ]
}
```

Bounds and validation:

- file size: at most 64 KiB;
- entries: at most 256;
- identifiers: exactly 64 lowercase hexadecimal characters;
- duplicate identifiers: deterministically deduplicated;
- unknown fields and trailing JSON: rejected;
- replacement validation is atomic;
- invalid replacement retains the previous LKG generation;
- revision labels, array order, and duplicates are representation-only and do not advance generation when effective authority is unchanged.

## Handshake fence

For an F.19 trusted proxy TLS handshake, existing CA, ClientAuth EKU, exact normalized DNS SAN, no-CN-fallback, no-wildcard, no-IP-substitution, and non-CA leaf requirements remain unchanged.

After those checks, worldd computes SHA-256 over the leaf certificate SPKI, checks the current F.24 revocation generation, and pins that credential identifier together with the F.23 matched DNS identity set. The check is repeated at bind publication so a revocation racing the handshake cannot be bypassed. A late handshake that captured an old TLS config still consults the current revocation generation and fails closed.

## Established connection fence

When F.20 immediate retirement is enabled, a successful revocation publication closes only established trusted-proxy connections whose pinned SPKI identifier is currently revoked. Another credential with the same exact DNS identity and a different SPKI survives.

The request-time attribution path also checks the current revocation generation. This prevents a revoked connection from retaining F.19 forwarding authority in the interval between revocation publication and socket close.

Direct/untrusted Client TLS sockets are never members of the trusted-proxy credential map and remain TLS server-auth-only.

## Reload ordering and independent LKG domains

A worldd SIGHUP uses deterministic ordering:

1. Server TLS certificate reload;
2. F.19 edge-policy reload and F.20-F.23 retirement;
3. F.24 leaf-revocation reload and credential retirement.

The F.19 and F.24 candidates are not a distributed transaction. Each validates and publishes independently. An invalid edge-policy candidate does not prevent a valid F.24 revocation from publishing, and an invalid revocation candidate does not roll back a valid edge-policy cutover.

After a successful F.24 cutover log is emitted, immediate-retirement mode has already applied the revoked-credential close fence. Request-time attribution remains fail-closed against the current revocation generation throughout the race window.

## Observability

Logs expose only safe metadata:

- generation;
- revision label;
- revoked credential count;
- applied / no-op / rejected state;
- retirement count;
- identifier algorithm name (`spki-sha256`).

Logs do not emit SPKI bytes, credential hashes, DNS identities, forwarding header values, attributed source IPs, account secrets, or recovery proofs.

## Preserved contracts

- Protocol v9 unchanged.
- Durable account schema v4 unchanged.
- F.17 1024-byte / 16-hop forwarding bounds and right-to-left stripping unchanged.
- F.19 edge-policy schema v1 unchanged.
- F.20 immediate retirement remains opt-in.
- F.21 semantic no-op behavior remains intact.
- F.22 global-vs-binding retirement semantics remain intact.
- F.23 handshake-authorized matched DNS identity preservation remains intact.
- F.16 Server TLS certificate LKG remains independent.
- Gameplay authority is untouched.
- Direct Godot Client TLS remains server-auth-only and never requires a proxy client certificate.

## Remaining bounded gap after F.24

F.24 is a local operator-owned revocation generation. It does not add distributed PKI revocation ingestion, OCSP/CRL lifecycle automation, multi-host revocation consensus, HSM key attestation, or automatic compromise detection. The next bounded stage should be selected from the actual post-merge operational gap rather than expanding F.24 into those systems.

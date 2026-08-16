# S4-F.16 — TLS Certificate Rotation / Runtime Reload

## Scope

S4-F.16 removes the remaining process-start-only TLS certificate/key boundary for the two production Server listeners that terminate TLS:

- the session-login / public-recovery HTTPS control plane configured by `-session-login-tls-cert` and `-session-login-tls-key`; and
- the trusted ASTRAH1 game ingress configured by `-trusted-tls-cert` and `-trusted-tls-key`.

The public login/recovery HTTP contract, Client F.11 recovery UX, durable account schema v4, recovery provider/outbox contracts through F.15, trusted game bootstrap contract, and Protocol v9 remain unchanged.

S4-F.16 does **not** add Client certificate-rotation authority. Clients continue to authenticate the Server through their deployment trust configuration and TLS hostname validation.

## Runtime certificate generation

Each TLS listener owns one independent `reloadableTLSCertificate` runtime. A successful startup load publishes generation 1. Each successful SIGHUP reload publishes the next monotonic generation, even if an operator intentionally reloads the same certificate material.

A generation contains an immutable loaded `tls.Certificate` plus non-secret operational metadata used for validation/testing. New TLS handshakes resolve the current generation through `tls.Config.GetCertificate`.

```text
TLS ClientHello
→ GetCertificate
→ snapshot current immutable certificate generation
→ TLS 1.3 handshake
```

The certificate selected for a handshake is not changed underneath that handshake. An already established TLS connection keeps the TLS state negotiated when it connected. Certificate rotation therefore does not disconnect an existing login HTTP keep-alive connection or trusted game TLS connection solely because the certificate generation changed.

New handshakes after publication use the new generation.

## Validation-before-publication

Reload first loads and validates the complete candidate certificate/key pair without changing the live generation.

The reference implementation requires:

1. certificate and private-key files can be parsed by `tls.LoadX509KeyPair`;
2. the leaf certificate can be parsed as X.509;
3. the private key matches the certificate public key;
4. the leaf is currently within `NotBefore <= now < NotAfter`;
5. when Extended Key Usage is present, it permits `serverAuth` or `any`.

Only after all checks succeed is the generation pointer replaced under the runtime publication lock.

A malformed PEM file, mismatched key, not-yet-valid/expired leaf, or explicitly client-auth-only certificate rejects the reload and leaves the previous generation live.

## Last-known-good behavior

Invalid replacement is fail-closed with respect to publication, but does not take down an already valid listener:

```text
read candidate cert + key
→ validate complete pair
→ failure
→ do not mutate generation
→ keep last-known-good certificate serving new handshakes
```

Operational logs report listener role, success/rejection, and generation metadata. They do not log certificate PEM, private-key bytes, or key material.

The Go runtime owns memory lifetime for retired certificate/key objects. S4-F.16 does not claim deterministic in-place private-key RAM zeroization after a generation becomes unreachable for new handshakes.

## SIGHUP domains remain independent

The same process SIGHUP can trigger several existing runtime reload domains, but S4-F.16 does not create a cross-file transaction.

Issued-session mode treats the following independently:

- session-login TLS certificate/key;
- durable account snapshot, when the account schema supports runtime reload;
- schema-v2 recovery provider / relay credential / private CA from F.14;
- trusted game-ingress TLS certificate/key.

For example, an invalid login certificate replacement can remain on its previous generation while a valid account or recovery-provider replacement succeeds. The trusted ingress certificate has its own independent last-known-good generation as well.

Operators that intend both TLS listeners to rotate together should finish publishing coherent certificate/key files for both listeners and then send SIGHUP. If one candidate pair is invalid, only that listener rejects its replacement.

## TLS policy

The minimum negotiated version remains TLS 1.3 for both listeners. S4-F.16 changes certificate lifecycle only; it does not relax cipher/authentication policy or change realtime UDP.

Realtime UDP remains the existing Protocol v9 authenticated plaintext datagram contract.

## Production acceptance

`Production TLS Certificate Reload E2E` uses the real `worldd` binary and one unchanged CA trust root to prove the runtime behavior:

1. start login and trusted game TLS listeners with certificate A;
2. establish TLS 1.3 connections on A and keep them open;
3. publish coherent certificate/key pair B for both listeners;
4. send SIGHUP;
5. observe login generation 1 → 2 and trusted-ingress generation 1 → 2;
6. prove the already-established A connections remain usable;
7. prove new handshakes receive certificate B;
8. perform the unchanged `/v1/session/login` API successfully after rotation;
9. publish certificate C while deliberately retaining key B, creating mismatched pairs;
10. send SIGHUP and prove both reloads reject while generation 2 remains last-known-good;
11. prove new handshakes still receive certificate B;
12. retain TLS 1.3, schema-v4/account authority, and Protocol v9 contracts.

The exact F.16 product head is also required to pass the existing Server CI and F.9/F.10/F.12/F.13/F.14/F.15 production recovery/delivery gates. Those gates continue to exercise the unchanged Client F.11 product flow where applicable.

## Non-goals

S4-F.16 deliberately does not add:

- Client CA bundle or trust-policy hot reload;
- ACME enrollment/renewal or other PKI automation;
- OCSP stapling lifecycle automation;
- distributed certificate coordination or multi-host atomic cutover;
- mTLS client-certificate identity;
- public registration, MFA/WebAuthn/passkeys/OIDC;
- refresh-token / remember-session storage;
- distributed account storage;
- Protocol v10, DTLS, QUIC, or gameplay changes.

## Resulting boundary

After F.16, recovery relay credential/private-CA rotation and both Server-facing TLS certificate/key pairs have runtime generation semantics. The remaining deployment-edge concerns such as trusted reverse-proxy source attribution, distributed rate limits, automated PKI issuance, and multi-host coordination remain separate decision gates rather than being hidden inside certificate reload.

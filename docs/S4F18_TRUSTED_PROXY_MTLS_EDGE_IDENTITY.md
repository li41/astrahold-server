# S4-F.18 — Trusted Proxy Upstream Authentication / mTLS Edge Identity

## Scope

S4-F.18 strengthens the optional S4-F.17 reverse-proxy source-attribution boundary without changing the public Client contract.

Unchanged contracts:

- Godot Client runtime remains S4-F.11.
- Public login remains `login_id + login_secret` over HTTPS/TLS 1.3.
- Public recovery request/reset shapes remain the F.10/F.11 contract.
- Durable account schema remains v4.
- Wire Protocol remains v9.
- F.13/F.14/F.15 recovery delivery/outbox semantics are unchanged.
- F.16 Server certificate/key generation semantics are unchanged.
- F.17 source-selection, forwarding parser, hop bounds, and throttle bucket semantics are unchanged.
- Gameplay authority is unchanged.

F.18 is only about the identity of the **direct reverse proxy that is allowed to submit forwarding metadata** to the session-login/public-recovery TLS listener.

## Compatibility mode and F.18 mode

F.17 remains backwards-compatible and opt-in:

```text
-session-login-trusted-proxy-cidrs
-session-login-forwarded-header
```

Without an F.18 policy file, those two flags retain the F.17 IP/CIDR-only trust model.

F.18 is enabled by adding:

```text
-session-login-trusted-proxy-mtls-file=/secure/proxy-mtls.json
```

The F.18 flag is invalid unless the F.17 trusted-proxy allowlist/header pair is also enabled.

When F.18 is enabled, forwarding authority requires both:

```text
actual TLS socket peer is in the F.17 allowlist
AND
TLS client certificate chains to the current F.18 proxy CA set
AND
leaf certificate has one exact allowlisted DNS SAN identity
```

An IP allowlist match alone is no longer sufficient in F.18 mode.

## Direct Client behavior remains server-auth TLS

F.18 does **not** turn the public Godot Client into an mTLS Client.

TLS configuration is chosen per new handshake from the real socket peer:

```text
ClientHello
→ inspect Conn.RemoteAddr
├── peer NOT in F.17 proxy allowlist
│     → existing TLS 1.3 server-auth configuration
│     → no client certificate required
│
└── peer IS in F.17 proxy allowlist
      → if F.18 disabled: existing F.17 behavior
      → if F.18 enabled: RequireAndVerifyClientCert
```

Therefore a normal direct Client keeps the same server certificate hostname/CA verification and sends no client certificate, proxy identity, source IP, or forwarding metadata authority.

## Proxy mTLS policy schema v1

Example:

```json
{
  "schema_version": 1,
  "revision": "edge-trust-2026-08-16-a",
  "client_ca_file": "proxy-client-ca.pem",
  "dns_names": [
    "edge-a.astrahold.internal",
    "edge-b.astrahold.internal"
  ]
}
```

The policy file is strict JSON. Unknown fields and trailing JSON values are rejected.

Bounds:

```text
policy file size        <= 64 KiB
client CA bundle size   <= 256 KiB
CA roots                1..16
DNS identities          1..32
revision                1..128 trimmed bytes
DNS identity            <= 253 bytes
```

A relative `client_ca_file` is resolved relative to the policy file directory.

The CA bundle must contain only PEM `CERTIFICATE` blocks. Every trust anchor must:

- parse as X.509,
- have valid CA basic constraints,
- permit certificate signing when KeyUsage is present,
- be currently valid at candidate-load time.

No private key belongs in this policy or CA bundle.

## Exact proxy identity policy

F.18 uses exact DNS SAN identity, not Common Name fallback and not wildcard matching.

Configured identities are normalized to lowercase and must be strict DNS names. The policy rejects:

- wildcard identities,
- IP literals,
- empty labels,
- labels longer than 63 bytes,
- leading/trailing `-`,
- underscores and other non hostname characters,
- trailing root dots.

After the normal TLS client-certificate chain validation succeeds with `clientAuth` usage, the leaf must contain at least one DNS SAN that exactly matches one configured identity after lowercase normalization.

A certificate such as:

```text
DNS:*.astrahold.internal
```

never matches configured identity:

```text
edge-a.astrahold.internal
```

This stage intentionally uses one allowlisted identity set for all direct trusted proxy prefixes. Prefix-to-specific-identity mapping is a separate future policy problem.

## Relationship to F.17 source attribution

F.18 does not change how F.17 selects the originating source after the direct proxy has been authenticated.

The order is:

```text
TLS socket peer
→ F.17 IP/CIDR allowlist
→ F.18 mTLS proxy identity, when enabled
→ exactly one selected forwarding field
→ 1024-byte bound
→ at most 16 hops
→ IPv4/IPv6 canonicalization
→ right-to-left trusted-hop stripping
→ first untrusted boundary becomes the throttle source
```

Direct/untrusted peers still ignore `X-Forwarded-For` and `Forwarded` completely.

A trusted IP peer in F.18 mode that does not complete proxy client authentication never reaches the HTTP forwarding parser. The TLS handshake fails before login KDF or recovery-provider work.

The HTTP attribution layer also requires verified TLS client-chain state for trusted peers while F.18 is enabled. This is a defense-in-depth assertion for handler/tests; the production listener establishes that state through TLS `RequireAndVerifyClientCert`.

## Runtime generations and SIGHUP

The proxy mTLS policy is an independent last-known-good runtime domain.

Startup loads and validates policy generation 1 before the session-login listener is published.

Each `SIGHUP` attempts a complete candidate reload:

```text
read strict proxy policy
→ load bounded CA bundle
→ validate all trust anchors
→ normalize exact DNS identity set
→ candidate fully valid?
   ├── no  → keep current generation unchanged
   └── yes → atomically publish generation N+1
```

The textual `revision` does not need to change for a successful generation reload. This permits an in-place CA bundle rotation while preserving a stable operator revision if desired.

Invalid policy JSON, missing files, malformed PEM, expired/not-yet-valid CA, non-CA roots, or invalid identity entries leave the current generation untouched.

Operational logs include only metadata such as generation, revision, root count, and identity count. They do not log:

- client certificate bytes,
- private keys,
- forwarding values,
- attributed client IPs,
- login passwords,
- recovery proofs,
- recovery destinations.

## Handshake generation semantics

A new trusted-proxy handshake snapshots the current proxy-trust generation through `GetConfigForClient`.

That snapshot supplies:

- `RequireAndVerifyClientCert`,
- the generation's Client CA pool,
- the generation's exact DNS SAN verification callback.

After the handshake has authenticated a proxy, later SIGHUP reload does not rewrite that established TLS connection's verified peer state.

Therefore:

```text
proxy connection established under generation A
→ trust reload publishes generation B
→ established A connection remains usable
→ new handshake with A is evaluated against B and fails if A is no longer trusted
→ new handshake with B succeeds
```

This is intentional graceful cutover behavior. Operators that require immediate retirement of all existing upstream proxy connections must terminate/recycle those connections at the proxy or Server transport layer; F.18 does not add an active connection registry solely for CA rotation.

## Proxy client-certificate rotation

A leaf certificate can rotate without a Server reload when the replacement:

- chains to a currently trusted F.18 CA,
- permits `clientAuth`,
- preserves one exact allowlisted DNS SAN.

A CA rotation uses the F.18 SIGHUP generation path. A CA bundle may contain both old and new roots during an overlap window, subject to the 16-root bound.

## Independence from F.16 server certificate generations

F.16 and F.18 share the session-login TLS listener but they are independent trust directions:

```text
F.16: Server proves its identity to Client/proxy
F.18: trusted reverse proxy proves its identity to Server
```

The same process SIGHUP triggers both candidate reloads, but there is no cross-file transaction.

Examples:

- invalid replacement Server cert/key + valid proxy CA policy → F.16 keeps Server-cert LKG while F.18 can publish the new proxy-trust generation;
- valid replacement Server cert/key + invalid proxy CA policy → F.16 can publish while F.18 keeps proxy-trust LKG.

This independent publication is deliberate. A single invalid file must not block an unrelated valid rotation domain.

## Recovery and login authority remain unchanged

Once the source IP has been attributed, the existing guards are unchanged:

```text
login guard    → existing fixed-window bucket
recovery guard → separate existing fixed-window bucket
```

F.18 client-certificate success does not authenticate an Astrahold account and does not grant gameplay authority.

A proxy cannot choose:

- CharacterID,
- account_id,
- credential_version,
- takeover authority,
- recovery eligibility,
- recovery destination,
- gameplay state.

It only earns the right to submit the bounded F.17 forwarding field for source-IP throttle attribution.

## Production acceptance

`Production Trusted Proxy mTLS E2E` uses real production `worldd` and a local TLS reverse-proxy harness.

The gate proves:

1. direct Client traffic still succeeds through ordinary server-auth TLS without a client certificate;
2. an allowlisted proxy IP with no client certificate cannot complete the trusted upstream TLS handshake;
3. a certificate signed by the configured CA but carrying the wrong DNS SAN is rejected;
4. a future CA/client certificate is rejected before its generation is published;
5. real proxy A with CA A + exact identity can forward login and recovery requests;
6. one generation-A upstream TLS connection is held open across the trust rotation;
7. CA A → B publishes proxy-trust generation 2 while an intentionally mismatched F.16 Server cert/key candidate is independently rejected and kept at Server-cert LKG;
8. the established A connection remains usable after cutover;
9. new A handshakes fail after cutover;
10. proxy B with the same exact identity under CA B succeeds on new handshakes;
11. malformed replacement CA is rejected and generation-2 B remains last-known-good;
12. direct Client behavior remains unchanged after the proxy trust rotation;
13. logs do not contain private key material, login/recovery secrets, or proxy DNS identities.

Expected marker:

```text
ASTRAHOLD_F18_E2E_OK direct_client_unchanged=true proxy_mtls_required=true exact_dns_identity=true ca_rotation=true old_connection_survives=true invalid_lkg=true tls_reload_independent=true recovery_unchanged=true protocol=9 tls=1.3
```

## Non-goals

F.18 does not add:

- Godot Client mTLS,
- Client certificate enrollment or storage,
- public registration,
- MFA/TOTP/WebAuthn/passkeys/OIDC,
- PROXY protocol,
- dynamic F.17 trusted IP/CIDR allowlist reload,
- distributed rate limiting,
- IP reputation/CAPTCHA/credential-stuffing intelligence,
- WAF/CDN vendor integration,
- multi-host edge trust consensus,
- ACME/PKI automation,
- OCSP lifecycle automation,
- distributed account storage,
- refresh tokens / remember-session,
- Protocol v10, DTLS, QUIC, or gameplay changes.

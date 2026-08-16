# S4-F.19 — Trusted Proxy Edge Policy Runtime Reload / Network+Identity Binding

## Scope

S4-F.19 closes the remaining split-authority gap between the S4-F.17 network/header policy and the S4-F.18 reverse-proxy mTLS identity policy.

Unchanged contracts:

- Godot Client runtime remains S4-F.11.
- Public login remains `login_id + login_secret` over HTTPS/TLS 1.3.
- Public recovery request/reset shapes remain the F.10/F.11 contract.
- Durable account schema remains v4.
- Wire Protocol remains v9.
- F.13/F.14/F.15 recovery delivery/outbox semantics are unchanged.
- F.16 Server certificate/key generation semantics are unchanged.
- F.17 forwarding parsing, 1024-byte header bound, 16-hop bound, IP normalization, right-to-left source selection, and fixed-window throttle semantics are unchanged.
- F.18 TLS client-certificate verification and exact DNS SAN identity semantics are unchanged.
- Gameplay authority is unchanged.

F.19 changes only how the Server deployment defines and reloads the trusted reverse-proxy edge boundary.

## Compatibility and authority modes

The existing F.17/F.18 flags remain supported as the legacy compatibility path:

```text
-session-login-trusted-proxy-cidrs
-session-login-forwarded-header
-session-login-trusted-proxy-mtls-file
```

Their existing semantics remain unchanged. In particular, the F.17 CIDR/header pair remains process-start configuration while the optional F.18 mTLS file can reload independently.

F.19 adds a separate authoritative mode:

```text
-session-login-trusted-proxy-edge-policy-file=/secure/edge-policy.json
```

The F.19 flag is mutually exclusive with all three legacy F.17/F.18 trusted-proxy flags. A deployment therefore has exactly one source of edge authority:

```text
legacy F.17/F.18 flags
OR
F.19 edge-policy file
```

There is no merge, precedence rule, or partial fallback between the two modes.

## Edge-policy schema v1

Example:

```json
{
  "schema_version": 1,
  "revision": "edge-2026-08-16-a",
  "forwarded_header": "x-forwarded-for",
  "client_ca_file": "proxy-client-ca.pem",
  "bindings": [
    {
      "prefixes": ["10.20.0.10/32"],
      "dns_names": ["edge-a.astrahold.internal"]
    },
    {
      "prefixes": ["10.20.0.11/32"],
      "dns_names": ["edge-b.astrahold.internal"]
    },
    {
      "prefixes": ["10.30.0.0/16"],
      "dns_names": ["internal-hop.astrahold.internal"]
    }
  ]
}
```

The file is strict JSON. Unknown fields and trailing JSON values are rejected.

The generation contains, in one immutable snapshot:

```text
revision
forwarded_header mode
client CA roots
network prefix bindings
exact DNS SAN identities per binding
trusted-prefix union used for F.17 right-to-left hop stripping
```

A relative `client_ca_file` is resolved relative to the edge-policy file.

## Bounds and normalization

F.19 keeps the existing F.17/F.18 hard bounds and adds a binding bound:

```text
edge-policy file size       <= 64 KiB
bindings                    1..32
total trusted prefixes      1..64
total DNS identities        1..64
client CA bundle size       <= 256 KiB
CA roots                    1..16
revision                    1..128 trimmed bytes
forwarding header           x-forwarded-for | forwarded
forwarding field size       <= 1024 bytes
forwarding hops             <= 16
```

Prefix parsing uses the F.17 normalization rules:

- exact IPv4/IPv6 addresses are accepted and converted to host prefixes;
- CIDRs are masked;
- scoped IPv6 addresses are rejected;
- IPv4-mapped IPv6 is normalized to canonical IPv4 where valid.

Bindings must not contain overlapping network prefixes. This deliberately prevents longest-prefix or configuration-order ambiguity. One real socket peer can match at most one identity binding in a generation.

DNS identities reuse the F.18 exact normalization contract:

- lowercase exact DNS SAN matching;
- no wildcard identities;
- no IP identity entries;
- no Common Name fallback;
- no underscores, empty labels, trailing dots, or malformed labels.

The CA bundle has the same F.18 validation rules: PEM certificates only, valid X.509 CA basic constraints, certificate-signing usage when KeyUsage is present, current validity, and no private keys.

## Network + identity binding

F.18 used one identity set for every trusted F.17 network prefix. F.19 makes the direct socket peer select a specific identity binding.

For a new TLS handshake:

```text
real Conn.RemoteAddr
→ current edge-policy generation
→ exactly one matching prefix binding?
   ├── no
   │    → ordinary TLS 1.3 server-auth Client path
   │    → no client certificate required
   │    → forwarding metadata has no authority
   │
   └── yes
        → RequireAndVerifyClientCert
        → chain must verify against this generation's client CA pool
        → leaf must contain an exact DNS SAN allowed by that specific binding
        → on success, bind this TLS connection to the generation snapshot
```

Therefore a valid certificate from one configured proxy cannot be replayed from another trusted proxy network unless the second binding explicitly allows the same DNS identity.

A prefix match alone never grants forwarding authority in F.19 mode.

## Per-connection generation pinning

F.19 extends the F.18 established-connection guarantee to the complete edge policy, not only CA/identity state.

A successful trusted-proxy handshake registers the immutable snapshot that authenticated that exact upstream connection. HTTP source attribution then uses that connection snapshot for:

- forwarding header mode;
- exact proxy identity re-check;
- trusted prefix union;
- right-to-left hop stripping.

It does **not** reread the current edge policy on every request.

This prevents a mixed-generation condition such as:

```text
TLS proxy identity authenticated under generation A
+
Forwarded parsing / prefixes suddenly read from generation B
```

After cutover:

```text
established trusted connection A
→ keeps generation-A identity + header + prefix semantics until close

new connection
→ snapshots current generation B
→ must satisfy B network binding + CA + exact identity
→ uses B forwarding mode + prefix set
```

The HTTP server removes the connection binding on `StateClosed` and `StateHijacked`.

As a defense-in-depth rule, if a request arrives from an address that the **current** policy marks trusted but no authenticated connection binding exists, the request fails closed instead of silently falling back to socket-only attribution.

A pre-existing direct connection whose peer becomes newly trusted after reload can therefore be rejected until reconnect. This is intentional: F.19 does not silently upgrade an already established unauthenticated upstream connection into forwarding authority.

## Source attribution remains F.17

Once a trusted connection snapshot is established, the existing F.17 algorithm remains unchanged:

```text
exactly one selected forwarding field
→ 1024-byte bound
→ parse at most 16 hops
→ normalize IPv4/IPv6
→ inspect advertised chain right-to-left
→ strip consecutive trusted hops from that connection generation
→ first untrusted boundary is the attributed source
→ if all advertised hops are trusted, use the left-most advertised hop
```

Login and recovery still use separate fixed-window guards after resolving the same attributed source abstraction.

Malformed/missing selected metadata on a trusted connection remains generic `400 invalid_request` before password KDF or recovery-provider work.

Direct/untrusted connections continue to ignore forwarding headers completely.

## Atomic SIGHUP generation reload

In F.19 mode, one SIGHUP candidate contains the entire edge authority:

```text
read strict edge-policy JSON
→ validate forwarding mode
→ load and validate CA bundle
→ normalize every prefix
→ reject overlapping bindings
→ normalize every exact DNS identity
→ validate all bounds
→ candidate fully valid?
   ├── no  → retain current generation unchanged
   └── yes → atomically publish generation N+1
```

No prefix, header mode, CA root, or identity mapping becomes visible before the entire candidate is valid.

The textual `revision` does not need to change for publication; the runtime generation remains monotonic and records every successful reload.

Invalid JSON, missing CA file, malformed PEM, expired CA, invalid DNS name, invalid prefix, overlap, bound overflow, or unsupported header mode retains the last-known-good generation.

Existing authenticated connections keep references to their handshake generations until close. New handshakes immediately resolve the newly published generation.

## Relationship to F.16 and other reload domains

F.19 uses the same process SIGHUP as the existing reload domains but remains an independent last-known-good domain.

```text
F.16  Server certificate/key generation
F.19  reverse-proxy edge-policy generation
account durable generation
F.14  recovery provider/relay generation
```

They validate and publish independently; there is no cross-file transaction.

For example:

```text
invalid replacement Server cert/key
+
valid F.19 edge-policy candidate
→ F.16 keeps Server certificate LKG
→ F.19 still publishes its new edge generation
```

The reverse is also allowed. This preserves the established Astrahold rule that unrelated operational trust domains do not block one another's valid rotation.

## Observability

Startup/reload logs expose metadata only:

- source-attribution mode;
- trusted prefix count;
- proxy authentication mode;
- edge generation;
- edge revision;
- binding/root/identity counts;
- previous/new forwarding mode on successful reload.

Logs do not intentionally emit:

- proxy DNS identities;
- client certificate bytes;
- private keys;
- forwarding values;
- attributed client IPs;
- account passwords;
- recovery proofs or destinations.

## Production acceptance

`Production Trusted Proxy Edge Policy Reload E2E` uses real `worldd`, real TLS 1.3, generated Server and proxy PKI, real reverse-proxy processes, and the existing recovery endpoint.

Generation 1 uses:

```text
header      X-Forwarded-For
client CA   A
127.0.0.2   → edge-a identity
127.0.0.3   → edge-b identity
10.0.0.0/8  → trusted intermediary binding
```

Generation 2 atomically changes all dimensions:

```text
header         Forwarded
client CA      B
127.0.0.2      → edge-a-v2 identity
127.0.0.4      → edge-c identity
172.16.0.0/12  → trusted intermediary binding
```

The gate proves:

1. direct Clients still use ordinary server-auth TLS and spoofed forwarding metadata has no authority;
2. a certificate valid for the `.3` binding cannot authenticate from the `.2` network;
3. generation-1 proxy A can attribute XFF and share the expected login throttle bucket;
4. one generation-1 authenticated keep-alive connection remains open across SIGHUP;
5. a deliberately invalid F.16 replacement Server keypair is independently rejected while the valid F.19 candidate publishes generation 2;
6. the established generation-1 connection remains usable with generation-1 XFF semantics after publication;
7. new generation-1 proxy A handshakes fail against generation 2;
8. new `.2` proxy A2 succeeds only with CA B + the new exact identity and `Forwarded` mode;
9. the newly added `.4` proxy C binding succeeds;
10. the removed `.3` network falls back to direct/socket attribution and its forwarding metadata is ignored;
11. the new `172.16/12` trusted-hop set is stripped while the removed `10/8` range becomes an untrusted source boundary;
12. recovery consumes the same generation-2 attribution contract;
13. an overlapping-prefix replacement is rejected and generation 2 remains LKG;
14. secret/identity/forwarding values do not appear in ordinary Server logs.

Expected marker:

```text
ASTRAHOLD_F19_E2E_OK atomic_edge_generation=true network_identity_binding=true header_rotation=true prefix_rotation=true old_connection_pinned=true new_handshake_cutover=true invalid_lkg=true tls_reload_independent=true direct_client_unchanged=true recovery_unchanged=true protocol=9 tls=1.3
```

## Non-goals

F.19 does not add:

- Godot Client mTLS or Client certificate enrollment;
- dynamic policy distribution across multiple Server processes;
- distributed rate limiting;
- IP reputation, CAPTCHA, or credential-stuffing intelligence;
- WAF/CDN vendor integration;
- PROXY protocol;
- automatic proxy connection draining during policy rotation;
- ACME/PKI or OCSP automation;
- public registration, MFA/TOTP/WebAuthn/passkeys/OIDC;
- distributed account storage or multi-writer recovery CAS;
- refresh tokens / remember-session;
- Protocol v10, DTLS, QUIC, or gameplay changes.

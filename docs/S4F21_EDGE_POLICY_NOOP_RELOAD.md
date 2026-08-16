# S4-F.21 — Edge Policy No-op Reload Detection / Change-Aware Connection Retirement

## Scope

S4-F.21 closes one bounded operational gap left by F.19/F.20: a process-wide `SIGHUP` can be intended only for account, recovery-provider, or Server TLS lifecycle, while the F.19 trusted-proxy edge policy remains effectively unchanged. Before F.21, every valid edge-policy candidate published a new edge generation. With F.20 retirement enabled, that meant an unchanged edge authority could still cause healthy reverse-proxy keep-alive connections to reconnect.

F.21 makes F.19 publication **change-aware**. A candidate is still fully read and validated, but a new edge generation is published only when the effective network/header/CA/identity authority actually changes.

This stage does **not** change:

- paired Godot Client S4-F.11 login/recovery UX or payloads;
- durable account schema v4;
- Protocol v9;
- F.17 forwarding parser, bounds, normalization, or right-to-left trusted-hop selection;
- legacy F.18 proxy-mTLS mode;
- F.19 schema-v1 edge-policy fields, overlap rules, TLS binding, or per-connection generation pinning;
- F.20 opt-in connection-retirement semantics after a real edge generation publication.

## Effective edge authority

F.21 compares the validated policy by effective authority rather than source-file bytes or operator labels.

The authority digest includes:

```text
selected forwarding mode
actual client CA certificate DER set
normalized trusted prefix -> exact allowed DNS identity set mapping
```

The following are deliberately **not** authority by themselves:

```text
revision label
JSON whitespace
binding array order
binding grouping when every normalized prefix retains the same DNS identity set
prefix spelling that normalizes to the same prefix
DNS identity case
repeated DNS identities
client_ca_file path text
PEM whitespace / ordering
repeated copies of the same CA certificate
```

For example, these two binding representations are authority-equivalent when all other authority is unchanged:

```json
{
  "bindings": [
    {
      "prefixes": ["127.0.0.2/32", "127.0.0.3/32"],
      "dns_names": ["edge-a.example", "edge-b.example"]
    }
  ]
}
```

and:

```json
{
  "bindings": [
    {
      "prefixes": ["127.0.0.3"],
      "dns_names": ["EDGE-B.EXAMPLE", "edge-a.example"]
    },
    {
      "prefixes": ["127.0.0.2"],
      "dns_names": ["edge-a.example", "edge-b.example", "edge-a.example"]
    }
  ]
}
```

F.19 non-overlap validation still applies before this comparison. F.21 does not permit ambiguous overlapping network authority merely because a canonical representation could be imagined.

## CA comparison

F.21 does not compare CA filenames, PEM bytes, subjects, serial numbers, or root count alone. During the normal bounded F.19 load it validates every trust anchor with the same requirements already used by F.18/F.19, then hashes the parsed certificate's raw DER.

The authority comparison uses a sorted, de-duplicated set of SHA-256 DER digests. Therefore:

- reordering the same roots is a no-op;
- duplicating the same root PEM is a no-op;
- moving the same root material to an equivalent policy path does not create authority;
- a different certificate at the same path, even with the same subject/serial/count, is an authority change.

The digest is an internal comparison primitive only. It is not emitted to ordinary logs and is not a public protocol identifier.

## Reload ordering

Candidate validation remains fail-closed and happens before the publish decision:

```text
read strict F.19 policy + CA bundle
→ validate schema / bounds / forwarding mode
→ validate CA certificates and current validity
→ normalize prefixes / reject overlap
→ normalize exact DNS identities
→ compute effective authority digest
→ lock current edge generation
   ├── digest equal
   │     → keep current snapshot
   │     → keep current revision metadata
   │     → keep current generation N
   │     → F.20 retirement sees N and retires 0
   │
   └── digest different
         → publish candidate snapshot
         → generation N+1
         → F.20, when enabled, retires authenticated generations < N+1
```

An invalid candidate never reaches semantic comparison and retains the complete edge-policy last-known-good state exactly as F.19 already required.

The equality check and actual publication run under the same existing edge-policy mutex. This prevents concurrent reloads from comparing against one generation and publishing relative to another.

## Revision semantics

`revision` remains required, bounded operator metadata in the schema, but F.21 explicitly treats a revision-only edit as a semantic no-op.

When a no-op candidate has a different revision label, the running Server keeps the current snapshot and therefore keeps the currently published revision in observability metadata. The candidate revision becomes visible only when it accompanies a real authority change that is published.

This prevents an operator label from becoming an indirect connection-revocation command.

## F.20 interaction

F.20 remains opt-in through:

```text
-session-login-trusted-proxy-edge-retire-old-connections
```

Without that flag, F.19 established trusted proxy connections continue using their pinned generation until close.

With that flag, F.21 changes only **when a new generation exists**:

```text
semantic no-op
→ generation unchanged
→ existing proxy connection remains current
→ retired_connections=0

real authority change
→ generation advances
→ old authenticated proxy connections become stale
→ F.20 closes them before the reload is reported applied
```

Late-handshake stale-generation handling from F.20 remains unchanged.

## Independent SIGHUP domains

A single process signal can still trigger multiple independent reload domains:

- durable account generation;
- recovery provider / relay credential / private CA;
- session-login Server TLS certificate;
- trusted game-ingress Server TLS certificate;
- legacy F.18 proxy trust when configured;
- F.19/F.21 edge policy when configured.

F.21 does not make those domains transactional. It only prevents the F.19 edge domain from creating a new generation when its own effective authority did not change.

A valid F.16 Server certificate rotation may therefore succeed while the edge-policy reload is a semantic no-op, with an established proxy connection remaining alive. Conversely, a real edge-policy change still cuts over independently even if another reload domain rejects its candidate.

## Validation

Unit/race coverage locks down:

- revision-only and representation-only policy rewrites are no-op;
- binding reordering/grouping does not matter when each normalized prefix retains the same identity set;
- bare IP and equivalent host-prefix notation compare the same;
- DNS case/order/duplicates do not create authority;
- duplicate copies of the same CA PEM do not create authority;
- a different actual CA certificate at the same path does create authority;
- existing F.19 real-change generation and LKG tests remain valid;
- concurrent Snapshot/Reload remains race-safe.

`Production Trusted Proxy Edge No-op Reload E2E` uses real `worldd`, real account storage, TLS 1.3, F.19 edge mTLS, and F.20 retirement to prove:

1. generation 1 proxy keep-alive is established;
2. an independent Server certificate A→B reload succeeds;
3. an edge candidate with only representation/revision changes keeps edge generation 1;
4. that established proxy connection continues serving trusted XFF requests;
5. no-op retirement count is zero;
6. a real CA/identity/header change publishes edge generation 2;
7. F.20 then closes the old generation-1 proxy connection;
8. a fresh generation-2 proxy certificate works with the new `Forwarded` authority;
9. the old proxy certificate cannot create a new trusted handshake;
10. a direct Godot-compatible server-auth-only Client path remains unchanged;
11. ordinary logs remain free of passwords, exact proxy identities, and forwarding metadata.

## Non-goals

F.21 does not add:

- no-op detection to account, recovery-provider, or F.16 Server-certificate generations;
- distributed edge generation coordination;
- multi-host proxy draining;
- distributed rate limiting or IP reputation;
- WAF/CDN vendor integration;
- PROXY protocol;
- ACME/PKI automation;
- MFA/OIDC/passkeys;
- Protocol v10 or gameplay changes.

Those remain separate decision gates driven by observed operational or product requirements.

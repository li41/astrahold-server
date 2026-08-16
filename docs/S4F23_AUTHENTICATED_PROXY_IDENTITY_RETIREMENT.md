# S4-F.23 — Authenticated Proxy Identity-Aware Retirement / Partial Binding Rotation

## Scope

S4-F.23 refines the optional F.20/F.22 trusted-proxy connection retirement fence without changing the public Client contract or the F.19 edge-policy schema.

Unchanged contracts:

- Godot Client runtime/public login/recovery/game contract remains S4-F.11.
- Durable account schema remains v4.
- Wire Protocol remains v9.
- F.17 forwarding parser, header bounds, hop bounds, normalization, and source-selection rules are unchanged.
- F.19 strict schema-v1 edge-policy format, non-overlapping prefix rule, and immutable per-connection snapshot pinning are unchanged.
- F.20 retirement remains explicit opt-in through `-session-login-trusted-proxy-edge-retire-old-connections`.
- F.21 semantic no-op detection remains the publication gate.
- F.22 still treats forwarding-mode changes, actual CA-root-set changes, and normalized trusted-prefix-topology changes as global fail-closed retirement boundaries.

F.23 only changes the finest retirement decision inside one otherwise-compatible network binding.

## Problem left by F.22

F.22 compares the complete exact-DNS identity set attached to the socket peer's binding. This is intentionally safe, but it is coarser than the authenticated connection itself.

Example:

```text
generation 1 binding 127.0.0.2/32
  allowed DNS = {edge-a, edge-canary}

established connection A
  certificate DNS = {edge-a}

established connection B
  certificate DNS = {edge-canary}

candidate generation 2
  allowed DNS = {edge-a, edge-next}
```

The global forwarding mode, CA roots and trusted-prefix topology are unchanged. Only `edge-canary` is removed and `edge-next` is added.

F.22 sees the binding set change and retires both old connections. F.23 preserves connection A because the identity that actually authenticated it remains authorized, while connection B is retired because its authenticated identity was removed.

## Handshake-time authenticated identity capture

The F.19 TLS handshake already verifies all of the following before a trusted proxy connection is bound:

1. the real socket peer matches exactly one current trusted network binding;
2. normal TLS client-certificate chain verification succeeds against the current edge CA roots;
3. the leaf is not a CA certificate;
4. at least one normalized exact DNS SAN appears in that binding's allowlist.

F.23 extends the immutable connection binding with the bounded set of exact DNS identities that satisfy both sides at that handshake:

```text
pinned matched identities
    = normalized leaf DNS SANs
      INTERSECT
      generation-N binding allowed DNS identities
```

The set is normalized, de-duplicated, sorted, bounded by the existing F.19 identity limit, and retained only in process memory with the connection binding. Identity values are not added to ordinary logs.

## Why F.23 pins a matched set instead of one arbitrary SAN

A certificate can legitimately contain multiple DNS SANs. Choosing the first matching SAN would make retention depend on certificate SAN ordering even when more than one SAN was already authorized at the original handshake.

F.23 therefore pins every DNS SAN that was simultaneously:

- present in the leaf certificate; and
- allowed by the original binding.

A later identity-only rotation may preserve the connection if at least one of those originally authorized identities remains allowed.

This is not retroactive authorization.

## Multi-SAN no-retroactive-promotion rule

Consider a leaf certificate with:

```text
DNS SANs = {edge-a, edge-future}
```

and generation 1 allows only:

```text
{edge-a, edge-canary}
```

The pinned matched set is only:

```text
{edge-a}
```

If generation 2 later allows:

```text
{edge-future, edge-next}
```

the established connection must retire even though the certificate also contains `edge-future`. `edge-future` was not part of the authority accepted at the original handshake and cannot be promoted after the fact.

A fresh handshake under generation 2 may succeed through `edge-future`, because that is a new authentication decision against the new policy.

## Retirement decision

F.23 applies only when the F.20 retirement flag is enabled and the connection is older than the current effective edge generation.

The decision is:

```text
old authenticated connection
  |
  +-- forwarding mode changed? -------------------- yes --> retire
  |
  +-- actual CA root set changed? ----------------- yes --> retire
  |
  +-- normalized trusted-prefix topology changed?  yes --> retire
  |
  +-- peer no longer maps to a valid binding? ----- yes --> retire
  |
  +-- pinned matched identity set empty/invalid? -- yes --> retire
  |
  +-- any pinned matched identity still allowed
      by the current binding for this peer?
          |
          +-- yes --> preserve old pinned connection
          +-- no  --> retire
```

A preserved connection is not promoted to the new generation. It continues using its original immutable F.19 TLS/forwarding snapshot until it naturally closes or a later cutover makes its pinned authority incompatible.

## Late-handshake semantics

A trusted-proxy TLS handshake can begin before an edge-policy reload and complete after publication.

F.23 applies the same pinned-identity compatibility rule when `http.ConnState` first observes that completed connection:

- if its original matched identity still remains authorized for the same peer under otherwise-compatible global authority, it may remain;
- if the matched identity was removed, it is closed immediately;
- if a global header, CA-root, or trusted-prefix-topology change occurred, it is closed immediately.

No separate generation-change history is required. The old connection carries its pinned snapshot and matched identities; the runtime compares them directly with current authority.

## Fail-closed behavior

F.23 intentionally retires an old connection when any required comparison state is unavailable or inconsistent, including:

- missing pinned snapshot;
- missing authenticated generation;
- invalid socket peer address;
- peer no longer resolves to a valid binding;
- connection binding index does not match the old snapshot's binding for that peer;
- empty pinned matched identity set.

Invalid replacement policies never enter the retirement fence because F.19/F.21 validation/publication fails first and the last-known-good snapshot remains current.

## Graceful mode remains unchanged

When `-session-login-trusted-proxy-edge-retire-old-connections` is not enabled, F.19 graceful semantics remain unchanged:

- established trusted proxy connections stay pinned to the generation that authenticated them;
- a later edge-policy publication does not force reconnect solely because of F.23;
- request source attribution continues using the old pinned snapshot on those established connections.

F.23 is an immediate-cutover refinement, not a new default.

## Source attribution remains F.17/F.19

F.23 does not change request parsing or bucket selection.

After TLS authentication and connection binding, requests continue to use the pinned snapshot's:

- selected `X-Forwarded-For` or `Forwarded` mode;
- 1024-byte forwarding-field bound;
- 16-hop bound;
- IPv4/IPv6 normalization;
- right-to-left trusted intermediary stripping;
- fail-closed malformed/missing trusted metadata behavior.

Direct/untrusted Clients still use the actual TLS socket peer and do not present proxy client certificates.

## Observability and secret handling

Existing reload logs continue to report only bounded metadata such as:

- previous/current edge generation;
- revision;
- header mode;
- root/binding/prefix/identity counts;
- connection cutover mode;
- retired connection count.

F.23 does not log:

- exact proxy DNS identities;
- certificate contents;
- source IPs;
- forwarding metadata;
- passwords or recovery proofs;
- issued session credentials.

## Unit and race coverage

F.23 unit coverage proves:

- handshake capture includes only DNS SANs that were allowed by the original binding;
- normalization/de-duplication does not create duplicate authority;
- a still-allowed authenticated identity survives a partial binding rotation;
- an authenticated identity removed from the same binding retires;
- if multiple identities were genuinely matched at the original handshake, retaining any one of them preserves the connection;
- a DNS SAN that existed in the certificate but was not authorized at the original handshake cannot retroactively preserve the connection after a reload;
- late handshakes use the same identity-aware compatibility rule;
- missing pinned identity state fails closed.

Existing F.19/F.20/F.21/F.22 tests continue to prove schema validation, no-op detection, global cutovers, invalid-LKG behavior, and binding-aware retirement outside the partial-identity case.

## Production acceptance

`Production Trusted Proxy Edge Identity Retirement E2E` uses a real `worldd`, real schema-v4 account store, TLS 1.3, and one F.19 network binding at `127.0.0.2/32`.

Generation 1 allows:

```text
edge-a.astrahold.test
edge-canary.astrahold.test
```

Three persistent mTLS upstream connections are established:

```text
A       certificate SANs = {edge-a}
Canary  certificate SANs = {edge-canary}
Multi   certificate SANs = {edge-a, edge-future}
```

Generation 2 keeps the same header, CA and prefix topology but changes the binding allowlist to:

```text
{edge-a, edge-next}
```

Acceptance requires:

- exactly one old connection retired (`Canary`);
- `A` remains on the same TCP/TLS keep-alive socket;
- `Multi` also remains on the same socket because its originally accepted `edge-a` identity is still authorized;
- fresh `edge-canary` handshake fails;
- fresh `edge-next` handshake succeeds.

Generation 3 changes only the allowlist to:

```text
{edge-future, edge-next}
```

Acceptance then requires:

- the old `A` connection retires;
- the old `Multi` connection also retires because `edge-future` was not in its generation-1 matched identity set;
- a fresh handshake using the same multi-SAN certificate succeeds through the now-explicitly-authorized `edge-future` identity;
- an invalid overlapping replacement preserves generation-3 LKG;
- direct Client TLS remains server-auth-only;
- ordinary logs remain secret/source-safe.

## Non-goals

F.23 does not add:

- certificate serial/SPKI pinning or per-leaf credential revocation;
- CRL/OCSP lifecycle management;
- new F.19 policy schema fields;
- proxy drain acknowledgements or distributed edge coordination;
- distributed rate limiting, IP reputation, WAF/CDN integration, or PROXY protocol;
- Client mTLS enrollment;
- Client edge-policy authority;
- MFA/OIDC/passkeys;
- Protocol v10;
- gameplay changes.

A compromised proxy leaf certificate that still chains to an unchanged trusted CA and still presents an allowed DNS identity cannot be selectively revoked by F.23 without changing broader CA or identity authority. That remains a separate decision gate.

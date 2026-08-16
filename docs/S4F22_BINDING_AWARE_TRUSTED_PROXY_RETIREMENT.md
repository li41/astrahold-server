# S4-F.22 — Binding-Aware Trusted Proxy Retirement / Selective Edge Cutover

## Purpose

S4-F.22 refines the optional F.20 immediate trusted-proxy connection retirement fence after F.21 made edge-policy publication change-aware.

F.20 originally used edge generation age as the retirement boundary: after any real F.19 authority publication, every authenticated proxy connection pinned to an older generation was closed. That is safe, but it is broader than necessary when the only authority change is the exact DNS identity set of one existing network binding.

F.22 keeps the same fail-closed deployment model while making retirement peer-specific:

- global forwarding-header mode change => retire every older authenticated proxy connection;
- client CA root-set change => retire every older authenticated proxy connection;
- normalized trusted-prefix topology change => retire every older authenticated proxy connection;
- if all three global authorities are unchanged, compare the old and current exact DNS identity set for the connection's actual socket-peer prefix;
- retire only connections whose own prefix->identity authority changed;
- preserve older connections from unaffected bindings.

This is a Server deployment-edge lifecycle change. It does not change the Astrahold Client public login/recovery/game contract.

## Invariants preserved

F.22 intentionally keeps these contracts unchanged:

- Godot Client runtime remains S4-F.11;
- durable account schema remains v4;
- wire Protocol remains v9;
- F.17 forwarding parser, 1024-byte bound, 16-hop bound, address normalization, and right-to-left trusted-hop stripping remain unchanged;
- F.18 exact DNS SAN verification and TLS client-auth chain verification remain unchanged;
- F.19 schema-v1 edge-policy format and non-overlapping binding rule remain unchanged;
- F.19 TLS connections still pin the complete immutable snapshot selected by their handshake;
- F.20 remains opt-in through `-session-login-trusted-proxy-edge-retire-old-connections`;
- F.21 semantic no-op detection remains unchanged;
- direct/untrusted Clients remain server-auth-only TLS and never present a reverse-proxy client certificate;
- account/recovery/TLS reload domains remain independent last-known-good domains;
- gameplay authority is unchanged.

No new operator flag is introduced.

## Why generation age alone is too broad

Consider generation 1:

```text
header = X-Forwarded-For
CA     = edge-root-A

127.0.0.2/32 -> edge-a.astrahold.test
127.0.0.3/32 -> edge-b.astrahold.test
```

Both proxies hold long-lived authenticated TLS keep-alive connections.

The operator rotates only the first binding:

```text
127.0.0.2/32 -> edge-a2.astrahold.test
127.0.0.3/32 -> edge-b.astrahold.test   # unchanged
```

The F.19 effective authority changed, so F.21 correctly publishes generation 2. Under the original F.20 rule, both generation-1 connections were older than generation 2 and therefore both were closed.

The `.3` connection, however, still has exactly the same global forwarding contract, the same CA trust anchors, the same trusted-prefix topology, and the same exact DNS identity authority for its own peer network. Closing it does not increase authority revocation correctness.

F.22 removes that unnecessary blast radius.

## Snapshot compatibility model

F.22 does not maintain a separate history describing what changed between every pair of generations. Instead, each established trusted-proxy connection already carries the complete immutable snapshot that authenticated its handshake.

At retirement time the Server compares:

```text
connection.pinned_snapshot
        versus
current_edge_snapshot
        for
actual socket peer
```

This is important because a connection may survive multiple identity-only publications affecting other bindings. A later global cutover must still be able to revoke that older connection without relying on an accumulated change log.

The peer-specific compatibility predicate is:

```text
same forwarding mode
AND same actual CA certificate DER set
AND same normalized trusted-prefix set
AND old snapshot still maps socket peer to a binding
AND current snapshot still maps socket peer to a binding
AND old peer binding exact-DNS set == current peer binding exact-DNS set
```

If any condition is false, the old connection is stale under F.22 and is closed when immediate retirement is enabled.

## Global authority components

### Forwarding mode

The forwarding mode is global in F.19 schema v1:

```text
x-forwarded-for
or
forwarded
```

An established connection pinned to XFF must not survive a transition to `Forwarded` while retaining the old parser/header authority. Therefore any forwarding-mode change remains a full old-generation retirement boundary.

### Client CA root set

The edge CA bundle determines which reverse-proxy client certificates may authenticate at the TLS layer.

F.21 already normalizes this authority as the parsed certificate DER set rather than PEM bytes, path, order, or duplicate blocks. F.22 reuses the same semantic root-set representation for global retirement compatibility.

If the actual root-certificate set changes, every established connection authenticated under an older root set is retired when F.20/F.22 immediate cutover is enabled. F.22 does not selectively preserve an established TLS client session across a CA trust-anchor rotation.

### Trusted-prefix topology

The normalized set of trusted socket prefixes is also global source-attribution authority because F.17 right-to-left stripping treats every trusted prefix as a possible intermediary hop.

A prefix addition, removal, or replacement may therefore change the meaning of an advertised forwarding chain even for a connection whose direct socket prefix itself appears unchanged.

For that reason any normalized trusted-prefix-set change remains a full old-generation retirement boundary.

## Binding-local identity authority

When forwarding mode, CA roots, and trusted-prefix topology are all unchanged, the remaining authority difference can only be the mapping from an existing prefix to its accepted exact DNS SAN identities.

For an old connection from `127.0.0.3`, F.22 compares the exact normalized identity set attached to the binding containing `127.0.0.3` in both snapshots.

Example:

```text
generation 1
127.0.0.2/32 -> { edge-a }
127.0.0.3/32 -> { edge-b }

generation 2
127.0.0.2/32 -> { edge-a2 }
127.0.0.3/32 -> { edge-b }
```

With immediate retirement enabled:

```text
old .2 connection -> close
old .3 connection -> keep
```

The preserved `.3` connection continues using its pinned generation-1 snapshot. This is safe because, for that peer, the relevant global and binding-local authority is semantically identical to generation 2.

The connection is not silently promoted or rewritten to generation 2.

## Late handshake semantics

F.20 already handled the race where a TLS handshake selected an old snapshot before publication but finished afterward.

F.22 applies the same peer-specific compatibility predicate to that race.

After an identity-only `.2` rotation:

```text
late generation-1 handshake from .2 -> close
late generation-1 handshake from unchanged .3 -> may remain open
```

After a global header/CA/prefix change:

```text
late handshake from any old trusted binding -> close
```

This avoids both silent promotion and unnecessary disconnection.

## Multiple-generation behavior

A connection may remain pinned to generation 1 while generation 2 changes another binding.

If generation 3 later changes a global authority, F.22 directly compares the generation-1 pinned snapshot to generation 3 current state. The global mismatch causes immediate retirement.

Therefore F.22 does not need a retained per-generation change journal or a transitive compatibility cache.

## Relationship to F.21 no-op detection

F.21 remains the first publication decision:

```text
load candidate
-> fully validate
-> effective authority unchanged
   -> keep current snapshot + generation
   -> retire 0
-> effective authority changed
   -> publish next generation
   -> F.22 evaluates old connections peer-by-peer
```

Representation-only edits still do not create an F.22 cutover at all.

The revision label remains metadata and does not independently revoke connections.

## Invalid replacement / LKG

Invalid candidates still fail before publication.

Examples include:

- malformed JSON;
- unknown fields;
- invalid CA material;
- expired/not-yet-valid CA;
- invalid exact DNS identity;
- binding bounds violation;
- overlapping trusted prefixes.

Failure behavior remains:

```text
candidate invalid
-> current edge snapshot unchanged
-> generation unchanged
-> no connection retirement
-> last-known-good authority remains active
```

F.22 never treats a rejected candidate as a retirement instruction.

## Graceful compatibility mode

If `-session-login-trusted-proxy-edge-retire-old-connections` is not enabled, F.22 does not change F.19 graceful behavior.

Established authenticated proxy connections retain their pinned snapshot until normal connection close regardless of later valid edge publications.

New handshakes always use the current generation.

This historical compatibility gate remains part of exact-head CI.

## Immediate retirement mode

With the existing F.20 flag enabled:

```text
-session-login-trusted-proxy-edge-policy-file=/secure/edge-policy.json
-session-login-trusted-proxy-edge-retire-old-connections
```

successful real publication now follows:

```text
publish generation N+1
-> inspect tracked authenticated older proxy connections
-> compare pinned snapshot to current snapshot for actual socket peer
-> close incompatible connections
-> preserve peer-compatible old connections
-> log aggregate retired_connections count
```

No client IP, proxy DNS SAN, certificate content, or forwarding metadata is added to ordinary logs.

## TLS security boundary

F.22 compatibility is a retirement decision, not a replacement TLS verification.

An existing connection was already authenticated during its original handshake against the pinned snapshot. If F.22 finds that the old authority remains semantically compatible for that peer, the connection continues under that original verified TLS state and pinned forwarding snapshot.

If authority is incompatible, the Server closes the connection and requires a fresh handshake under the current snapshot.

F.22 never mutates an existing TLS connection into a newly verified generation.

## Source attribution remains pinned

A preserved old connection continues using its pinned snapshot for F.17 source parsing.

This is intentional. Preservation is allowed only when:

- forwarding mode is unchanged;
- trusted-prefix topology is unchanged;
- CA roots are unchanged;
- that peer's exact identity set is unchanged.

Therefore the pinned source-attribution semantics are equivalent for the preserved connection.

## Unit and race coverage

F.22 adds coverage for:

- two established proxy bindings where only binding A changes identity;
- changed binding A connection is retired;
- unchanged binding B connection remains open;
- late old-generation handshake for changed binding is closed;
- late old-generation handshake for unaffected binding survives;
- forwarding-mode change still retires every old trusted proxy connection;
- trusted-prefix topology change still retires every old trusted proxy connection;
- actual CA certificate change still retires every old trusted proxy connection.

Existing F.20 retirement tests continue to use a real global authority cutover and must remain green.

Server CI race detection continues to cover the edge reload / connection-tracker concurrency boundary.

## Production acceptance

`Production Trusted Proxy Edge Binding Retirement E2E` uses a real production `worldd`, real durable account store, TLS 1.3, F.19 edge mTLS, and the F.20 retirement flag.

Initial generation 1:

```text
header = X-Forwarded-For
CA = one edge CA
127.0.0.2/32 -> edge-a.astrahold.test
127.0.0.3/32 -> edge-b.astrahold.test
```

Two persistent proxy TLS connections are established.

### Selective cutover

Generation 2 changes only:

```text
127.0.0.2/32
edge-a.astrahold.test -> edge-a2.astrahold.test
```

The gate requires:

- generation advances 1 -> 2;
- `retired_connections=1`;
- old `.2` connection loses authority;
- old `.3` keep-alive remains usable with XFF;
- fresh old `edge-a` certificate no longer handshakes from `.2`;
- fresh `edge-a2` certificate works from `.2`;
- fresh `edge-b` certificate still works from `.3`.

### Global cutover

Generation 3 then changes only global forwarding mode:

```text
X-Forwarded-For -> Forwarded
```

The gate requires:

- generation advances 2 -> 3;
- the remaining old `.3` keep-alive loses its old XFF authority;
- fresh `.3` handshake with `Forwarded` succeeds.

### LKG and direct Client

The gate then supplies an invalid overlapping-prefix replacement and requires generation-3 LKG to remain usable.

A direct/untrusted TLS Client without a client certificate remains on the ordinary server-auth-only path; forged forwarding metadata is still ignored for trust purposes.

The gate also checks that secret test credentials and attributed source addresses do not appear in ordinary `worldd` logs.

## Observability

F.22 retains the existing edge reload log shape including:

- previous/current generation;
- previous/current revision;
- previous/current forwarding mode;
- root/binding/prefix/identity counts;
- connection cutover mode;
- aggregate `retired_connections`.

It intentionally does not log which proxy binding or DNS identity was retired.

The aggregate count is enough for the bounded acceptance contract without creating a new source/identity telemetry surface.

## Non-goals

F.22 does not add:

- a new edge-policy schema version;
- per-binding CA roots;
- per-binding forwarding-header mode;
- per-certificate/session retirement accounting;
- identity-level preservation within a changed binding;
- distributed edge generation coordination;
- distributed rate limiting or IP reputation;
- WAF/CDN integration;
- PROXY protocol;
- ACME/PKI automation or OCSP lifecycle;
- Client certificate enrollment for Godot Clients;
- public registration, MFA, WebAuthn/passkeys, or OIDC;
- Protocol v10;
- gameplay changes.

### Identity-set granularity note

F.22 compares the complete exact-DNS identity set attached to the peer's binding.

If a binding changes from:

```text
{edge-a, edge-a-canary}
```

to:

```text
{edge-a, edge-a-next}
```

all older connections from that binding are considered stale, even if a particular established leaf certificate is still `edge-a` and remains present in the new set.

That more granular per-established-certificate preservation is intentionally outside F.22. Any future stage should justify the additional certificate-state and observability complexity separately.

## Stage boundary

S4-F.22 completes the bounded selective-retirement layer on top of:

```text
F.17 source attribution
-> F.18 upstream proxy mTLS identity
-> F.19 atomic network/header/CA/identity edge generation
-> F.20 immediate old-generation connection retirement
-> F.21 semantic no-op publication detection
-> F.22 peer-binding-aware retirement
```

The Astrahold Client remains S4-F.11, account schema remains v4, and Protocol remains v9.

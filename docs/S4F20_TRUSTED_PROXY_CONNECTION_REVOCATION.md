# S4-F.20 — Trusted Proxy Connection Revocation / Immediate Edge Cutover Fence

## Scope

S4-F.20 closes the remaining graceful-cutover gap in the optional S4-F.19 trusted reverse-proxy edge-policy mode without changing the public Client contract.

Unchanged contracts:

- Godot Client runtime remains S4-F.11.
- Public login remains `login_id + login_secret` over HTTPS/TLS 1.3.
- Public recovery request/reset shapes remain the F.10/F.11 contract.
- Durable account schema remains v4.
- Wire Protocol remains v9.
- F.13/F.14/F.15 recovery delivery/outbox semantics are unchanged.
- F.16 Server certificate/key lifecycle is unchanged.
- F.17 forwarding parser, bounds, source-selection, and fixed-window throttle semantics are unchanged.
- F.18 legacy proxy-mTLS compatibility mode is unchanged.
- F.19 schema-v1 edge-policy format, network/identity binding, overlap rejection, and validate-before-publish generation semantics are unchanged.
- Gameplay authority is unchanged.

F.20 only changes what may happen to **already-open reverse-proxy TLS connections after a successful F.19 edge-policy generation cutover**.

## Compatibility and F.20 mode

F.19 keeps its original graceful established-connection behavior unless the new F.20 deployment flag is enabled:

```text
-session-login-trusted-proxy-edge-policy-file=/secure/edge-policy.json
-session-login-trusted-proxy-edge-retire-old-connections
```

The retirement flag is only valid with the F.19 edge-policy file. It does not apply to:

- direct Godot Clients,
- socket-only operation,
- F.17 IP-only compatibility mode,
- F.18 legacy global proxy-mTLS policy mode.

When the F.20 flag is absent, F.19 remains fully backwards-compatible: an established proxy connection stays pinned to the generation that authenticated its TLS handshake.

When the F.20 flag is present, a successful F.19 reload becomes an immediate cutover fence for established trusted-proxy connections.

## Why this stage exists

F.19 intentionally pinned each authenticated proxy connection to an immutable edge generation. That prevented mixed state such as:

```text
old TLS client identity
+ new forwarding mode
+ new network allowlist
```

on one keep-alive connection.

The availability tradeoff was that a proxy authenticated before cutover could continue to submit forwarding metadata using its old complete generation until that connection naturally ended.

That is useful for graceful rotations, but it is not sufficient when an operator is removing a compromised proxy identity, CA, or network boundary and needs old upstream authority to stop promptly.

F.20 adds an explicit stricter cutover mode rather than silently changing the historical F.19 behavior.

## Cutover ordering

A valid F.20 SIGHUP follows this order:

```text
load strict F.19 edge-policy candidate
→ validate JSON / CA / forwarding mode / prefixes / overlaps / identities
→ candidate invalid?
   ├── yes → keep F.19 last-known-good generation
   │         do not retire any proxy connection
   │
   └── no  → publish F.19 generation N+1
             → identify tracked trusted-proxy TLS connections authenticated by generation < N+1
             → close those old-generation connections
             → only then report the edge-policy reload as applied
```

The retirement fence is therefore downstream of successful validation/publication and upstream of the operational `reload applied` log line.

An invalid replacement must never turn a configuration error into a connection outage. Existing LKG proxy connections remain open when the candidate is rejected.

## Which connections are retired

The session-login HTTP server observes accepted connection lifecycle through `http.Server.ConnState`.

In F.20 mode, the Server tracks live TLS connections for the F.19 listener and associates them with the generation already recorded by the F.19 authenticated edge binding.

A successful generation cutover closes a connection only when:

```text
connection has an authenticated F.19 edge generation
AND
connection generation < newly published generation
```

Therefore:

- direct Client connections are not retired by the F.20 edge fence;
- untrusted/socket-only connections are not retired by the F.20 edge fence;
- current-generation proxy connections are not retired;
- all authenticated older F.19 proxy generations are retired, even if the new policy still contains the same socket prefix;
- certificate-only, identity-only, forwarding-mode-only, CA-only, or prefix-only F.19 rotations can all trigger retirement because generation, not a partial field comparison, is the fence.

The generation comparison deliberately avoids trying to decide whether two complete policies are semantically equivalent. Every successful F.19 publication is a new operator authority generation.

## Handshake races

A TLS handshake can select an F.19 generation shortly before SIGHUP and finish shortly after the new generation publishes.

F.20 treats that connection as stale when it becomes active:

```text
handshake selected generation N
→ generation N+1 publishes
→ handshake N finishes later
→ ConnState observes authenticated generation N < current N+1
→ connection is closed
```

The connection is not silently promoted to generation N+1. A proxy must establish a new TLS handshake that satisfies the new generation's network binding, CA, and exact DNS SAN identity.

## In-flight HTTP semantics

F.20 is a transport-authority fence, not a transaction rollback mechanism.

An HTTP request that had already passed source attribution and entered the login/recovery handler before the cutover may finish its existing Server-side work. Closing the underlying connection can also cause the client to lose the response even when Server-side work already completed.

F.20 does **not** attempt to roll back:

- a password KDF already running,
- an account mutation already committed,
- a recovery provider operation already entered,
- an issued bearer already committed by the existing account/session fence.

After the F.20 reload is reported applied, the old keep-alive connection is no longer available for another trusted forwarding request. The proxy must reconnect and satisfy the new edge generation.

This preserves the existing account-generation and recovery mutation boundaries instead of inventing a cross-layer rollback protocol.

## Direct Client behavior remains unchanged

F.20 does not turn public Clients into mTLS Clients and does not close ordinary direct Client TLS connections merely because the edge policy rotated.

Direct Client behavior remains:

```text
TLS socket peer does not match current F.19 proxy binding
→ ordinary TLS 1.3 server-auth connection
→ no proxy client certificate required
→ forwarding metadata has no authority
```

The new flag creates no Client payload field, header, certificate enrollment, environment variable, or UI state.

## Relationship to F.16 Server certificate reload

F.16 and F.20 remain independent operational domains on the same listener:

```text
F.16: Server certificate generation
F.19/F.20: reverse-proxy edge authority generation + optional old-connection retirement
```

A SIGHUP may therefore produce:

```text
invalid replacement Server cert/key
→ F.16 keeps certificate LKG

valid replacement F.19 edge policy
→ F.19 publishes generation N+1
→ F.20 retires old proxy connections
```

or the reverse combination.

The edge connection fence does not require a Server certificate generation change.

## Relationship to F.18 legacy mTLS mode

F.20 intentionally applies only to F.19 edge-policy mode.

The older F.18 deployment surface:

```text
-session-login-trusted-proxy-cidrs
-session-login-forwarded-header
-session-login-trusted-proxy-mtls-file
```

keeps its established-connection semantics. F.20 does not add a hidden active-connection retirement policy to legacy F.18 deployments.

Operators that need network+identity atomicity plus immediate old-connection retirement should use F.19 with the F.20 retirement flag.

## Observability

Startup session-login metadata reports the edge connection cutover mode:

```text
edge_policy_connection_cutover=none|graceful|retire-old
```

A successful F.19 reload reports:

```text
connection_cutover=graceful|retire-old
retired_connections=<count>
```

The count is operational metadata only. Logs do not include:

- proxy certificate bytes,
- proxy DNS identities,
- client source IPs,
- forwarding header values,
- login passwords,
- recovery proofs,
- recovery destinations,
- private keys.

Rejected edge-policy reload logs retain the existing LKG metadata and do not report a retirement count because no retirement fence runs.

## Production acceptance

`Production Trusted Proxy Edge Connection Revocation E2E` uses real production `worldd` and proves:

1. F.20 starts only as an explicit extension of F.19 edge-policy mode;
2. direct Client TLS remains server-auth-only and unchanged;
3. a generation-1 proxy establishes a real mTLS keep-alive connection and successfully receives the normal login authentication result;
4. CA/identity/header/trusted-hop policy rotates from generation 1 to generation 2;
5. an intentionally invalid F.16 Server cert/key replacement remains independent and keeps certificate LKG;
6. the F.19 reload reports `connection_cutover=retire-old` and retires the old trusted-proxy connection before reporting success;
7. the old connection cannot issue another successful trusted forwarding request after cutover;
8. a fresh generation-2 handshake with the new CA/exact identity succeeds;
9. login and recovery continue to use their unchanged public APIs through generation 2;
10. an invalid later F.19 replacement is rejected without retiring the current generation-2 LKG connection;
11. direct Client behavior remains unchanged after rotation;
12. Server logs remain secret-safe and do not disclose proxy identity or forwarding metadata.

Expected marker:

```text
ASTRAHOLD_F20_E2E_OK immediate_edge_revocation=true old_connection_retired=true new_handshake_cutover=true invalid_lkg_connection_survives=true tls_reload_independent=true direct_client_unchanged=true recovery_unchanged=true f19_schema_unchanged=true protocol=9 tls=1.3
```

## Non-goals

F.20 does not add:

- Godot Client mTLS,
- a new F.19 policy schema,
- active rollback of already-entered login/recovery mutations,
- graceful drain coordination with a proxy fleet,
- distributed edge-policy consensus,
- cross-host connection registries,
- distributed rate limiting,
- IP reputation/CAPTCHA/credential-stuffing intelligence,
- WAF/CDN vendor integration,
- PROXY protocol,
- ACME/PKI automation,
- OCSP lifecycle automation,
- public registration,
- MFA/TOTP/WebAuthn/passkeys/OIDC,
- refresh tokens / remember-session,
- Protocol v10, DTLS, QUIC, or gameplay changes.

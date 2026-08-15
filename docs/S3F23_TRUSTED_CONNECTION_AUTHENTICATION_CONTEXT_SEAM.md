# S3-F.23 — Trusted Connection Authentication Context Seam

## Scope

S3-F.23 adds an opt-in, server-side authenticated connection bootstrap seam before the existing GameV1 session bootstrap. The goal is to let a production embedding validate a real transport-layer credential or proof and bind the resulting trusted CharacterID to the same connection-scoped takeover authority.

This stage deliberately does not invent a standard reconnect token format, account/session database, or new Protocol message. The repository currently has no credential issuer or account/session service to make such a wire format authoritative.

## Why this stage exists

S3-F.21 introduced an explicit takeover authorizer, but `CharacterIdentityFactory` and `CharacterTakeoverAuthorizer` are independent configuration hooks. That is sufficient for correctness tests and trusted upstream integrations, but a production authentication system should not resolve CharacterID through one global path and then accidentally authorize takeover through another unrelated global policy hook.

S3-F.23 therefore introduces one optional per-connection authentication result that carries both:

- the trusted `characteridentity.Binding` selected by the authenticated proof; and
- an optional connection-scoped `CharacterTakeoverAuthorizer` bound to the same proof/claims.

If the authenticated connection later hits an active owner, only the authorizer returned by that authentication result may approve takeover. The config-global F.21 authorizer is not used as fallback on this path.

## API

`Config.TrustedCharacterConnectionAuthenticator` is optional. When nil, the existing `CharacterIdentityFactory` + config-global `CharacterTakeoverAuthorizer` behavior remains unchanged.

When configured, it receives `TrustedCharacterConnectionAuthenticationRequest` containing:

- candidate SessionID;
- provisional allocated EntityID;
- TCP remote address;
- the accepted `net.Conn`.

The authenticator returns `TrustedCharacterConnectionAuthentication` containing:

- `Identity`, which must be valid and `AssuranceTrusted`;
- optional `TakeoverAuthorizer` scoped to this authenticated connection.

A nil scoped authorizer is valid for an inactive trusted join, but active takeover fails closed.

## Transport authentication I/O boundary

The authenticator runs on the accepted TCP connection goroutine before world admission/connection-plan processing and before any GameV1 SessionWelcome is written.

It may inspect transport state or consume a bounded authentication preface from the raw connection. Examples for future integrations include:

- identity established by a TLS-wrapped connection;
- a short signed session/reconnect preface owned by a gateway integration;
- an external account/session verifier that consumes an authentication frame before GameV1 begins.

The authenticator must consume only its own authentication bytes and leave the connection positioned for normal GameV1 traffic.

S3-F.23 does not define that preface format.

## Bounded authentication deadline

Authentication I/O is bounded by `Config.TrustedCharacterAuthenticationTimeout`.

- `DefaultConfig` uses 5 seconds.
- non-positive values normalize to the same bounded default;
- if the parent server context has an earlier deadline, the earlier deadline wins;
- the TCP read/write deadline is set for authentication and reset to no deadline before GameV1 bootstrap continues.

This prevents a configured authenticator that is blocked on normal socket I/O from holding a connection goroutine indefinitely. An authenticator doing non-I/O CPU work must still honor the supplied context.

## Ordering

Authenticated path:

1. Accept TCP connection.
2. Allocate candidate SessionID and provisional EntityID.
3. Run `TrustedCharacterConnectionAuthenticator` under the bounded authentication deadline.
4. Require a valid `AssuranceTrusted` identity.
5. Reset the transport deadline.
6. Enter the existing F.20 trusted connection-plan seam.
7. If inactive, proceed through F.17 admission/join. The scoped takeover authorizer is unused.
8. If active, enter F.22 candidate serialization/cooldown.
9. Run F.21 authorization using only the connection-scoped authorizer returned by step 3.
10. Revalidate the F.22 lease and call F.19 exact-fence ownership transfer.
11. Preserve F.20 old-peer eviction and Welcome ordering.

Legacy path when the new authenticator is nil is unchanged.

## Failure semantics

- authenticator error: candidate closes before connection-plan/admission/transfer;
- invalid authentication result or non-trusted identity: fail closed before world ownership work;
- authentication socket timeout: fail candidate only;
- missing scoped authorizer on inactive join: allowed because no takeover is occurring;
- missing scoped authorizer on active takeover: `ErrCharacterTakeoverUnauthorized`;
- config-global authorizer is never fallback for authenticated connections;
- authorization denial, F.22 lease rejection, or F.19 stale CAS leaves the old owner authoritative exactly as before.

## Security boundary

This stage is an integration seam, not a complete authentication product.

It does not define:

- credential issuance;
- credential persistence;
- credential rotation/revocation;
- account/session storage;
- reconnect token schema;
- proof replay protection format;
- TLS deployment policy;
- device/IP identity;
- Client credential UX.

A production provider is responsible for validating its credential/proof and returning `AssuranceTrusted` only after successful authentication.

## Protocol and Client boundary

Protocol v6 is unchanged.

The default development server still uses the existing ephemeral `CharacterIdentityFactory`, so the existing Client continues to connect and expects `SessionWelcome` as the first GameV1 message.

When a production deployment configures `TrustedCharacterConnectionAuthenticator` to consume a custom preface, that deployment must pair it with a client/gateway that sends or establishes the corresponding transport-layer proof. S3-F.23 itself does not add that client behavior and does not claim generic interoperability for an arbitrary custom authenticator.

## Concurrency and ownership correctness

S3-F.23 does not create a new ownership primitive.

- F.20 connection-plan still returns the exact active fence.
- F.22 still serializes in-flight takeover candidates and enforces exact-owner cooldown.
- F.21 still expresses allow/deny semantics.
- F.19 exact-fence CAS remains the final ownership authority.
- F.20 still retires the old peer only after transfer commit.

The connection-scoped authorizer receives the same `CharacterTakeoverRequest` including the exact expected fence and candidate SessionID.

## Scaling boundary

Authentication runs only on new TCP connections and outside the world-owner tick.

S3-F.23 adds:

- no world-owner command;
- no per-tick work;
- no per-snapshot work;
- no replication or simulation work;
- no filesystem or network I/O on the world-owner thread.

## Preserved boundaries

S3-F.23 does not change:

- Protocol v6;
- GameV1 codec;
- UDP MTU 1200;
- WorldSnapshot chunking;
- Network LOD / transform batch max64;
- lifecycle desired-vs-known truth;
- Spawn/Despawn Confirm-after-TrySend semantics;
- lifecycle churn max 6,000/snapshot;
- Initial Vitals budgets;
- dirty gameplay Vitals max 4,000/tick;
- F.17 admission semantics;
- F.18 ownership fencing;
- F.19 ownership transfer;
- F.20 old-peer eviction;
- F.21 fail-closed authorization;
- F.22 candidate lease/cooldown semantics;
- persistence formats or autosave behavior;
- process-local ownership epoch semantics.

## Focused acceptance tests

Tests cover:

- an authenticator consuming a pre-GameV1 proof and producing an inactive trusted join;
- authenticated path bypassing the legacy identity factory;
- invalid/non-trusted authentication result failing before Welcome;
- authentication socket I/O timing out under the configured bound;
- successful authentication resetting the transport deadline before normal GameV1 bootstrap;
- active takeover using the connection-scoped authorizer;
- authenticated takeover not invoking a conflicting config-global authorizer;
- nil scoped authorizer failing active takeover even when the global authorizer would allow it;
- denied authenticated takeover leaving the old owner able to route Actions;
- all existing F.20/F.21/F.22 regressions remaining in the suite.

## Explicit non-goals

- no standard reconnect token;
- no Protocol v7;
- no Client change;
- no account/session implementation;
- no worldd production authenticator provider;
- no distributed ownership/candidate fencing;
- no persistent takeover cooldown;
- no Go↔Godot end-to-end authentication claim.

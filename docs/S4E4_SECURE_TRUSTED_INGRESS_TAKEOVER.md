# S4-E.4 Secure Trusted Ingress & Duplicate-Session Policy

## Scope

S4-E.4 moves the S4-E.3 trusted-character credential path behind an optional production `worldd` TLS 1.3 ingress and exposes active-session replacement only as an explicit server-side credential policy.

The stage does not change GameV1, Protocol v8, realtime packet layout, Gameplay World data, Siege rules, or world-owner ownership semantics.

## Secure trusted TCP ingress

`worldd` keeps its existing reliable TCP listener as the authoritative backend. When all of the following flags are supplied, it also opens a TLS 1.3 listener in the same process:

- `-trusted-tls-listen`
- `-trusted-tls-cert`
- `-trusted-tls-key`

The TLS listener performs the TLS handshake before opening a backend connection and forwards bytes only to the existing `-tcp` address. The backend address must still be a literal loopback IP, so enabling TLS does not weaken the S4-E.3 rule that bearer credentials are never accepted by a directly exposed plaintext `worldd` listener.

TLS ingress requires `-trusted-character-auth-file`; partial TLS configuration or a non-loopback backend fails startup.

Only reliable TCP/auth bootstrap is protected by this ingress. Realtime UDP remains the existing GameV1 token-authenticated channel and is not encrypted by this stage.

## Credential-scoped active takeover

A credential entry may now opt in to active-session replacement:

```json
{
  "token_sha256": "<64 lowercase hex characters>",
  "character_id": "character-123",
  "allow_active_takeover": true
}
```

The field defaults to false. A credential without it authenticates the returning character but remains unable to replace an already-active session.

When enabled, the authenticator returns the existing connection-scoped `CharacterTakeoverAuthorizer`. It is bound to the server-selected CharacterID from that same credential and authorizes only a takeover request whose exact current ownership fence belongs to that CharacterID. The client does not send an `allow takeover` bit or choose a CharacterID.

## Existing ownership guarantees reused

No second ownership system is introduced. S4-E.4 continues to rely on the established production path:

1. world owner returns the exact active `SessionOwnershipFence`.
2. transport reserves one takeover candidate for that CharacterID.
3. the connection-scoped authorizer approves or rejects the exact candidate.
4. world owner performs the existing ownership-transfer CAS.
5. successful transfer increments the ownership epoch while retaining the live EntityID.
6. the old peer is retired and its normal Leave is suppressed; any already-enqueued fenced command/Leave is stale against the new epoch.
7. the existing candidate TTL and exact-owner cooldown continue to bound repeated replacement attempts.

This stage therefore changes policy and transport protection, not gameplay authority.

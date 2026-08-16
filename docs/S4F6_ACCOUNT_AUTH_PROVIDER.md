# S4-F.6 — Account Authentication Provider / Human Credential Hardening

## Status

S4-F.6 adds a bounded account-authentication seam in front of the existing S4-F.4 / S4-F.5 issued-session flow. It does not replace the session credential provider, trusted TLS game bootstrap, ownership fencing, realtime generation, or Protocol v9.

The production sequence remains:

```text
human login proof
  -> TLS 1.3 /v1/session/login
  -> Server account authenticator
  -> Server-owned CharacterID / takeover grant
  -> 32-byte CSPRNG opaque issued session credential
  -> existing trusted TLS + ASTRAH1 game bootstrap
  -> existing ownership / realtime / logout / expiry fences
```

The Client still submits only `login_id` and `login_secret`. CharacterID, active-takeover authority, team, HP, position, winner, ownership and revocation scope remain Server-owned claims.

## Provider seam

The login control plane now depends on a `sessionAccountAuthenticator` contract rather than directly on one static SHA-256 map implementation.

Each provider returns only a validated `sessioncredential.Grant` and exposes revision / method metadata. The issuance runtime remains responsible for generating the opaque bearer, assigning the per-session revocation scope, publishing the transport allow-set, logout revocation and exact Server-clock expiry.

This keeps identity verification separate from game-session proof lifecycle and allows a later account DB, external identity service or OIDC adapter without rewriting game admission.

## Schema v1 compatibility provider

Existing schema v1 account maps remain supported for compatibility with S4-F.4 / S4-F.5 deployment and E2E fixtures:

```json
{
  "schema_version": 1,
  "revision": "legacy-001",
  "accounts": [
    {
      "login_id": "service-account",
      "login_secret_sha256": "<64 lowercase hex characters>",
      "character_id": "character-1",
      "allow_active_takeover": false
    }
  ]
}
```

This mode is intentionally labeled `sha256-high-entropy`. It is suitable only for already-high-entropy machine-style login secrets. It is not the human-password storage contract.

## Schema v2 Argon2id password provider

Schema v2 uses an encoded Argon2id password verifier:

```json
{
  "schema_version": 2,
  "revision": "password-accounts-001",
  "accounts": [
    {
      "login_id": "alice",
      "password_argon2id": "$argon2id$v=19$m=65536,t=3,p=4$<salt>$<digest>",
      "character_id": "alice-character",
      "allow_active_takeover": false
    }
  ]
}
```

The reference provider fails closed unless all of the following are true:

- Argon2id version is `v=19`.
- Memory cost is between 64 MiB and 128 MiB.
- Time cost is between 3 and 10 passes.
- Parallelism is between 1 and 8.
- Salt is 16..64 bytes, encoded as unpadded standard Base64.
- Digest is exactly 32 bytes, encoded as unpadded standard Base64.
- Every account in one file uses the same memory / time / parallelism policy.
- `login_id` values are unique and strictly bounded.
- Character identity is validated by the existing trusted character identity rules.

The upper bounds are deliberate: a malformed or hostile configuration must not turn login verification into unbounded process memory or CPU consumption.

## Account-enumeration hardening

A missing `login_id` does not use a fast failure path. The provider executes one Argon2id derivation using a dummy verifier with the same KDF cost policy, then returns the same `invalid_credentials` result used for a wrong password.

This does not claim perfect network-level timing indistinguishability, but it removes the large KDF-vs-no-KDF timing split from the application path.

At most four password KDF operations may run concurrently in one `worldd` process. Additional login requests wait for a KDF slot or fail via request-context cancellation. With the configured cost ceiling, this bounds password-verification memory pressure independently of connection count.

## TLS and response behavior

The formal login endpoint remains TLS 1.3-only. Request and response bounds, strict JSON field handling, `Cache-Control: no-store`, opaque bearer handling and generic credential failure response remain unchanged from S4-F.4 / S4-F.5.

A successful account check never returns CharacterID or authority claims to the Client. The login HTTP response remains only:

```json
{
  "session_credential": "<opaque bearer>",
  "expires_at": "<Server-clock RFC3339 timestamp>"
}
```

## What does not change

S4-F.6 does not change:

- Protocol v9.
- GameV1 messages.
- ASTU / RoutingID / realtime HMAC domains.
- UDP MTU 1200.
- Gameplay World / Siege authority.
- Issued bearer format.
- Logout, exact expiry or revocation ordering.
- Duplicate-session rejection or Server-authorized takeover semantics.
- Client authority boundaries.

Schema v1 remains accepted so the existing S4-F.5 production E2E remains a compatibility gate while schema v2 gains its own validation.

## Remaining production gaps

The schema v2 file provider is a reference/bootstrap human-password verifier, not a complete account platform. Still outside S4-F.6:

- account registration / recovery / email verification;
- durable account database and administrative lifecycle;
- password-change / reset workflows and verifier migration policy;
- MFA / WebAuthn;
- OIDC / external identity provider adapter;
- refresh / reauthentication flow;
- distributed login issuance / revocation across multiple `worldd` processes;
- abuse throttling / IP reputation / CAPTCHA policy;
- secure Client credential storage and full visual login state machine.

Those concerns can now attach to the account-authentication seam without changing the Server-authoritative game-session contract.

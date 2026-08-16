# S4-F.10 — Verified Recovery Provider / Public Reset Exchange

## Status

S4-F.10 exposes a bounded public password-recovery exchange on the existing TLS 1.3 login control plane while keeping recovery identity, account generation and gameplay authority entirely Server-owned.

It builds on, rather than replaces, the S4-F.9 durable recovery primitive.

Protocol remains v9. There is no GameV1, ASTU, UDP MTU, realtime HMAC, CharacterID or gameplay-authority change.

## Public surface

When `-session-recovery-provider-file` is configured, `worldd` adds:

```text
POST /v1/account/recovery/request
POST /v1/account/recovery/reset
```

Both routes share the same TLS 1.3 listener already used for `/v1/session/login` and `/v1/session/logout`.

### Recovery request

Request:

```json
{
  "login_id": "alice"
}
```

For a syntactically valid login identifier, the public success shape is always:

```text
HTTP 202
Cache-Control: no-store
Pragma: no-cache
```

```json
{
  "request_id": "opaque-random-request-id",
  "expires_at": "2026-08-16T05:00:00Z"
}
```

A known eligible account and an unknown, disabled, or provider-unconfigured account use the same HTTP status and the same JSON field set. The response never contains:

- account existence;
- `account_id`;
- `credential_version`;
- CharacterID;
- takeover authority;
- recovery-code digest;
- provider eligibility.

S4-F.10 claims **no explicit account-existence signal in the response contract**. It does not claim formally constant network timing across all provider implementations, host load or external delivery systems. Provider adapters must not add an existence-specific public response.

### Recovery reset

Request:

```json
{
  "request_id": "opaque-random-request-id",
  "recovery_proof": "provider-proof",
  "new_password": "replacement-password"
}
```

Successful reset returns:

```text
HTTP 204
Cache-Control: no-store
Pragma: no-cache
```

Wrong proof, unknown/non-authorizing challenge, expired challenge, exhausted challenge, consumed challenge, stale credential generation and a valid proof for an account that became disabled all fail closed. Public proof failures use the same `401 invalid_recovery` class.

Malformed bounded JSON remains `400 invalid_request`; service/storage/provider failure remains `503 recovery_unavailable`; source-IP throttling returns `429 recovery_throttled` with bounded `Retry-After`.

## Provider seam

`internal/accountrecovery.Provider` separates public HTTP exchange logic from the mechanism that proves account recovery eligibility.

The interface owns:

```text
Begin(Server-owned Subject) -> opaque Challenge
Verify(request_id, proof) -> Server-owned Grant
Consume(request_id)
Method / Revision metadata
```

The `Subject` passed to a provider includes Server-owned `login_id`, stable `account_id`, current `credential_version`, and an internal eligibility bit. Only the opaque challenge is exposed publicly.

A verified `Grant` contains only the stable account ID and the credential generation that the provider verified. It does not mutate account state. The durable account writer re-validates that generation again immediately before committing a password replacement.

This double check closes the race:

```text
challenge issued at credential generation N
-> provider proof verifies
-> password / disable / KDF mutation moves account to N+1
-> recovery reset reaches durable writer
-> generation mismatch
-> fail closed; no password mutation
```

## Reference provider

The built-in reference provider is deliberately narrow:

```text
method = sha256-high-entropy-recovery-code
schema_version = 1
```

Example configuration:

```json
{
  "schema_version": 1,
  "revision": "recovery-provider-001",
  "subjects": [
    {
      "login_id": "alice",
      "recovery_code_sha256": "<64 lowercase hex>"
    }
  ]
}
```

This provider is suitable for bounded testing, bootstrap/operations and integration of the provider seam. It is **not** an email or SMS delivery provider.

The raw recovery code is not stored by `worldd`; configuration contains only its SHA-256 digest. Codes are required to be high entropy. A human-memorable low-entropy phrase must not be used with this provider because a stolen digest would permit offline guessing.

The provider creates CSPRNG request IDs and stores only process-local challenge state. Unknown or ineligible subjects receive non-authorizing challenge state that preserves the public response shape.

## Challenge bounds

Default policy:

```text
challenge TTL                 10 minutes
proof attempts / challenge    5
maximum active challenges     4096
recovery POSTs / source IP    10 / minute
```

Configurable flags:

```text
-session-recovery-challenge-ttl
-session-recovery-challenge-max-attempts
-session-recovery-ip-attempt-window
-session-recovery-ip-max-attempts
```

Limits:

```text
challenge TTL                 1 minute .. 1 hour
proof attempts / challenge    1 .. 20
source-IP attempts            1 .. 10000 / window
source-IP window              1 second .. 1 hour
```

Challenge expiry is exact: a challenge is invalid when Server time reaches `expires_at`.

The source identity is the actual TLS socket `RemoteAddr`; arbitrary `X-Forwarded-For` remains untrusted. Trusted reverse-proxy attribution is still a separate deployment feature.

The 4096 active-challenge cap is a process-local memory bound, not a distributed abuse-control system. Large distributed recovery-request floods still require an upstream trusted edge / distributed control plane.

## Durable reset commit

Public reset requires durable account schema v4. Recovery cannot be enabled on restart-only schema v1/v2 or legacy durable schema v3.

After a provider verifies the proof, `worldd` derives the replacement Argon2id verifier with the **currently published durable account KDF policy**. This preserves the existing one-uniform-online-policy rule used for equivalent unknown-login KDF work.

The password mutation then runs under the same `sessionLoginRuntime.mu` used by issued-session mutation and SIGHUP account reload:

```text
acquire issuance/account mutation lock
-> confirm live authenticator is durable schema v4
-> read durable store
-> require disk revision == live authenticator revision
-> require stable account_id match
-> require credential_version match provider grant
-> require account is enabled
-> replace password verifier
-> increment credential_version
-> update password_changed_at
-> remove all legacy F.9 recovery grants for that account
-> increment durable store revision
-> validate/build next account authenticator
-> SaveIfRevision
-> clone current issued-session provider
-> remove bearers whose account generation is no longer active
-> publish reduced transport scope allow-set
-> retire affected live peers
-> publish reduced bearer provider
-> publish next account authenticator
-> release mutation lock
-> consume provider challenge
```

No SIGHUP is required for this successful public-reset path.

### Why the live-session fence is immediate

The durable password commit changes the account generation. Existing issued bearers carry the old `AuthenticationSubject + AuthenticationGeneration`; they are removed from the issued credential provider and their realtime scopes are retired before the new authenticator is published to subsequent login issuance.

The existing `Issue` path takes the same mutation lock and re-validates the login grant generation. Therefore a password verification that completed just before reset cannot mint a bearer after the reset generation commits.

## Durable revision fence

S4-F.9 defined the JSON store as a single-writer operational contract. F.10 makes a recovery-enabled `worldd` an online account writer.

Therefore, while public recovery is enabled, production operation must treat that `worldd` instance as the active writer. `accountctl` mutations should be done in a controlled maintenance/reload procedure rather than concurrently racing the online recovery writer.

As an additional fail-closed fence, public reset requires the on-disk revision to equal the currently published live account revision before attempting its CAS write. If an external mutation has advanced disk state without being published into `worldd`, public reset returns unavailable and does not overwrite that newer state.

This is still not a distributed database transaction or multi-writer consensus protocol.

## SIGHUP interaction

Existing schema-v3/v4 durable SIGHUP lifecycle remains available.

When public recovery is enabled, a SIGHUP candidate that downgrades the durable store below schema v4 is rejected and last-known-good remains active. This prevents a running recovery route from silently losing the schema guarantees it depends on.

External password rotation/disable through the existing operator lifecycle still invalidates old recovery challenges by credential-generation mismatch even if the process-local challenge has not reached its TTL.

## Production acceptance

The F.10 production gate uses the unchanged normal F.8 `Main.tscn` product session state machine.

It proves:

```text
schema-v4 account + live product session
-> known recovery request = 202 request_id + expires_at
-> unknown recovery request = same 202 field set
-> unknown challenge + otherwise valid proof = 401 invalid_recovery
-> known challenge + wrong proof = same 401 invalid_recovery body
-> known challenge + correct proof + new password = 204
-> NO SIGHUP
-> old issued bearer / live game peer retires
-> Main enters ReauthenticationRequired
-> old password = 401 invalid_credentials
-> consumed request replay = 401 invalid_recovery
-> durable revision / credential generation advanced
-> legacy F.9 recovery grants removed
-> Main reauthenticates with new password
-> explicit product lifecycle completes
```

Captured Server/Client logs are checked for raw old/new passwords, recovery proofs and issued-session bearer fields.

## Compatibility

S4-F.10 preserves:

- Protocol v9;
- TLS 1.3 login and trusted game ingress;
- ASTRAH1 opaque bearer bootstrap;
- authenticated realtime UDP framing;
- F.3 transport revocation ordering;
- F.7 durable password/disable SIGHUP lifecycle;
- F.8 product login/reauthentication state machine;
- F.9 operator recovery / one-time durable recovery grants;
- schema-v3 durable login compatibility when public recovery is not enabled.

No Client recovery authority or GameV1 field is introduced.

## Remaining gaps / non-goals

S4-F.10 intentionally does not implement:

- verified email ownership or email delivery;
- verified SMS/phone ownership or SMS delivery;
- MFA / TOTP / WebAuthn / passkeys;
- OIDC / social/external IdP recovery;
- public registration;
- user-facing Godot forgot-password/reset UI;
- breached-password corpus checks;
- trusted reverse-proxy client-IP attribution;
- distributed rate limiting / reputation / CAPTCHA;
- distributed account DB / multi-writer CAS;
- cross-server challenge/recovery coordination;
- refresh tokens / remembered-device recovery;
- provider hot reload;
- TLS certificate hot reload.

Those features can attach above the provider seam without changing the authoritative game protocol.

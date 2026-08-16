# S4-F.7 — Durable Account Backend / Credential Lifecycle & Abuse Controls

## Status

S4-F.7 moves the S4-F.6 human-password provider from a restart-only reference file to a bounded durable account lifecycle without changing the game protocol or the issued-session wire contract.

The production authority chain remains:

```text
human password
  -> TLS 1.3 /v1/session/login
  -> Server account authenticator
  -> Server-owned CharacterID / takeover grant
  -> 32-byte CSPRNG opaque issued session credential
  -> TLS 1.3 trusted game ingress + ASTRAH1
  -> existing ownership / realtime / logout / expiry fences
```

Protocol remains v9. The Client still sends no CharacterID, takeover bit, position truth, HP, team, winner or ownership authority.

## Schema v3 durable account store

Schema v3 is a single-writer durable JSON account store:

```json
{
  "schema_version": 3,
  "revision": 4,
  "accounts": [
    {
      "account_id": "acct-...",
      "login_id": "alice",
      "password_argon2id": "$argon2id$v=19$m=65536,t=3,p=4$...$...",
      "credential_version": 2,
      "created_at": "2026-08-16T00:00:00Z",
      "password_changed_at": "2026-08-16T01:00:00Z",
      "character_id": "alice-character",
      "allow_active_takeover": false
    }
  ]
}
```

Each record has a stable Server-only `account_id` and monotonically changing `credential_version`. Password rotation, disable and enable operations advance the credential generation. The store revision must also move forward before a running `worldd` accepts a SIGHUP reload.

The durable writer uses:

```text
validate
-> temp file in target directory
-> chmod 0600
-> write
-> file fsync
-> atomic rename
-> destination chmod 0600
-> directory fsync
```

The update helper uses an expected store revision and fails on stale writers. This is a single-writer operational contract; S4-F.7 does not claim multi-host database transactions or distributed consensus.

## Operator lifecycle tool

`cmd/accountctl` is the bounded local/operator mutation surface:

```text
accountctl init
accountctl create
accountctl set-password
accountctl disable
accountctl enable
```

Passwords are accepted only through stdin (`-password-stdin`); the tool does not require plaintext passwords on command-line arguments. New/rotated passwords use the current Argon2id policy: v=19, 64 MiB memory, 3 passes, p=4, 16-byte random salt and 32-byte digest.

`create` assigns a random stable account ID. `set-password`, `disable` and `enable` increment `credential_version` and the store revision.

`accountctl` deliberately does not provide network administration, registration, email recovery or an online privileged API in this stage.

## Account proof generations

A schema-v3 successful password check returns normal trusted Server claims plus Server-only provenance:

```text
AuthenticationSubject    = stable account_id
AuthenticationGeneration = hash(account proof / policy generation)
```

These fields are never sent to the Client and do not grant gameplay authority. The issued-session runtime copies them only into the Server-side bearer record so it can answer one question later:

> Is the account proof that created this bearer still the current active proof generation?

The generation changes when security-relevant account state changes, including password verifier, login ID, CharacterID, credential version or takeover policy. A disabled account has no active generation.

## SIGHUP reload and live-session invalidation

Schema v1 and schema v2 remain restart-only compatibility providers. Schema v3 supports SIGHUP reload.

A valid reload requires:

- strict schema-v3 parsing and validation;
- a store revision strictly greater than the currently published revision;
- valid Argon2id verifiers using one consistent KDF cost policy;
- valid Server-owned CharacterID claims.

The reload order is fail-closed:

```text
load + validate replacement account snapshot
-> serialize with issuance mutation lock
-> clone current issued-bearer provider
-> remove expired bearers
-> remove bearers whose account generation is no longer active
-> publish reduced transport revocation-scope allow-set
-> retire affected live peers through existing F.3 transport teardown
-> publish reduced bearer provider
-> publish replacement account authenticator
```

Invalid, stale-revision or wrong-schema reloads retain the last-known-good snapshot.

### In-flight password verification race

Password verification intentionally runs outside the issuance mutation lock because Argon2id is bounded but expensive. Therefore an old password verification can finish while an operator rotates the password.

S4-F.7 closes that race at issuance time: `Issue` serializes under the same mutex used by account reload and re-validates `AuthenticationSubject + AuthenticationGeneration` against the currently published account snapshot before generating a bearer.

So this sequence fails closed:

```text
old password verifies
-> operator rotates password
-> SIGHUP reload commits new generation
-> stale verification reaches Issue
-> generation no longer current
-> no bearer issued
```

## Login abuse controls

The login listener now applies a bounded fixed-window attempt guard before JSON decoding and before Argon2id work.

Default policy:

```text
30 login POST attempts / 1 minute / observed source IP
maximum tracked source entries = 4096
```

Flags:

```text
-session-login-ip-attempt-window
-session-login-ip-max-attempts
```

A throttled request receives HTTP 429, `Retry-After`, and a `login_throttled` error body.

The source identity is the actual TLS socket `RemoteAddr`. `X-Forwarded-For` and similar request headers are ignored. If a deployment later places a trusted reverse proxy in front of this listener, proxy-aware client-IP attribution must be designed explicitly rather than trusting arbitrary headers.

This guard bounds direct password-KDF abuse. It is not IP reputation, CAPTCHA, global account lockout, credential stuffing intelligence or a distributed rate limiter.

## Compatibility

S4-F.7 preserves:

- schema-v1 `sha256-high-entropy` account maps as restart-only compatibility;
- schema-v2 `argon2id-password` account maps as restart-only compatibility;
- schema-v2 Argon2id limits and unknown-login equivalent KDF work;
- at most four concurrent password KDF checks per `worldd` process;
- TLS 1.3 login transport;
- login response containing only opaque `session_credential` + `expires_at`;
- logout semantics and exact Server-clock bearer expiry;
- duplicate-session rejection and Server-authorized active takeover;
- existing ASTRAH1 game bootstrap and Protocol v9 realtime transport.

## Durability boundary

The account store is durable across `worldd` restart. Issued session credentials remain deliberately process-local and short-lived. A Server restart therefore still invalidates all old issued bearers and requires login again; the durable object is the account credential state, not a gameplay/session bearer.

## Remaining production gaps

Still outside S4-F.7:

- public account registration and verified recovery;
- MFA / WebAuthn;
- OIDC / external IdP adapter;
- password-quality breach corpus checks and user-facing reset UX;
- trusted reverse-proxy client-IP attribution;
- distributed account database / multi-writer CAS;
- distributed rate limiting / reputation / CAPTCHA;
- refresh-token or durable issued-session recovery;
- cross-server issued-bearer revocation propagation;
- secure Client credential storage / OS keychain;
- visual login/account-management state machine.

These can now attach above the account-authentication seam without changing Protocol v9 or the Server-authoritative game-session boundary.

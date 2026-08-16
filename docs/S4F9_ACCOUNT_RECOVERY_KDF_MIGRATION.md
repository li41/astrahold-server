# S4-F.9 — Account Recovery / Password Reset / KDF Migration Workflow

## Status

S4-F.9 adds a bounded recovery and password-policy migration workflow above the existing S4-F.7 durable account generation fence.

It does **not** add a gameplay message, a Client-owned recovery authority, a public email reset endpoint, or a second online writer for the durable account file.

Protocol remains v9.

The security boundary remains:

```text
operator / verified out-of-band recovery process
  -> one-time short-lived recovery proof
  -> local accountctl password reset
  -> durable account credential generation advances
  -> worldd SIGHUP reload
  -> old issued bearer removed
  -> affected live peer retired
  -> fresh password login required
```

The Client still cannot choose CharacterID, account generation, recovery scope, takeover authority, position, HP, team, winner or ownership state.

## Durable schema v4

Schema v4 extends the durable account store with Server/operator-only recovery grants.

Schema v3 remains readable and reloadable for compatibility, but recovery fields are not silently accepted in schema v3. Recovery operations require an explicit `accountctl migrate` to schema v4.

A recovery grant contains only metadata and a SHA-256 digest of the bearer proof:

```json
{
  "schema_version": 4,
  "revision": 8,
  "accounts": [
    {
      "account_id": "acct-...",
      "login_id": "alice",
      "password_argon2id": "$argon2id$v=19$m=65536,t=3,p=4$...$...",
      "credential_version": 3,
      "created_at": "2026-08-16T00:00:00Z",
      "password_changed_at": "2026-08-16T01:00:00Z",
      "character_id": "alice-character"
    }
  ],
  "recovery_grants": [
    {
      "recovery_id": "recovery-...",
      "account_id": "acct-...",
      "credential_version": 3,
      "token_sha256": "<64 lowercase hex chars>",
      "issued_at": "2026-08-16T02:00:00Z",
      "not_before": "2026-08-16T02:00:00Z",
      "expires_at": "2026-08-16T02:15:00Z"
    }
  ]
}
```

The plaintext recovery token is never stored in this JSON file.

Validation is strict:

- schema v3 and v4 are the only durable schemas accepted;
- schema v3 must not contain recovery grants;
- `recovery_id` values are unique and bounded;
- recovery token digests are unique 32-byte SHA-256 values encoded as lowercase hex;
- every recovery grant references an existing stable `account_id`;
- every grant binds to the account `credential_version` present when the proof was issued;
- timestamps are UTC RFC3339;
- `issued_at <= not_before < expires_at`;
- recovery lifetime is at most 24 hours.

## Explicit v3 -> v4 migration

```text
accountctl migrate -path /secure/accounts.json
```

Migration:

- preserves account IDs, login IDs, password verifiers, CharacterIDs and credential versions;
- changes only the durable schema version and store revision;
- creates no recovery proof;
- does not itself revoke an account generation or live session;
- is idempotent once the store is already schema v4.

The existing expected-revision writer fence and atomic temp-file/fsync/rename durability contract remain in force.

## Recovery proof issuance

Recovery proof creation is intentionally local/operator controlled in F.9:

```text
accountctl issue-recovery \
  -path /secure/accounts.json \
  -login alice \
  -ttl 15m \
  -token-out /secure/handoff/alice-recovery.token
```

The command generates 32 random bytes from the operating-system CSPRNG and writes the URL-safe bearer proof exactly once to a **new** file opened with mode `0600`.

The command refuses to overwrite an existing token output file.

Only SHA-256(token) is written to the durable account store. A raw recovery token must therefore be handled like a password-equivalent bearer secret: do not put it in shell arguments, logs, tickets, chat, telemetry or the account JSON file.

Issuing a new recovery proof for an account supersedes any older outstanding recovery grant for that account. Issuance advances the store revision but deliberately does **not** advance `credential_version`, so merely preparing a recovery proof does not disconnect an active player.

## Credential-generation binding

Each recovery proof captures the account's current `credential_version`.

A reset is accepted only if the proof is inside its Server/operator-clock validity window **and** the account is still at that same credential version.

Therefore any later password rotation, disable/enable cycle, password reset or KDF rehash makes the older recovery proof stale even if its `expires_at` time has not yet arrived.

This prevents an old recovery proof from becoming a back door after account security state has already changed.

## One-time password reset

```text
printf '%s\n' 'new long password' | accountctl reset-password \
  -path /secure/accounts.json \
  -recovery-token-file /secure/handoff/alice-recovery.token \
  -password-stdin
```

The reset transaction is:

```text
load + validate schema v4 store
-> SHA-256 supplied recovery proof
-> find matching digest
-> require not_before <= now < expires_at
-> require account credential_version == grant credential_version
-> hash new password with current Argon2id default policy
-> increment account credential_version
-> update password_changed_at
-> remove every outstanding recovery grant for that account
-> increment store revision
-> SaveIfRevision via atomic durable writer
```

Password change and proof consumption therefore commit in the same durable JSON update. A successful proof is one-time. Exact `expires_at` is rejected.

Wrong, unknown, expired, superseded, stale-generation and already-consumed proofs fail without changing the durable store.

Resetting a disabled account does not implicitly enable it; account enable/disable remains a separate Server-owned lifecycle operation.

## Live-session invalidation after reset

`accountctl` remains an offline/local writer. It does not reach into a running `worldd` process.

After a successful password reset, the operator publishes the new durable revision with the existing S4-F.7 SIGHUP path:

```text
password reset commits credential_version N+1
-> SIGHUP worldd
-> strict replacement snapshot validation
-> old AuthenticationGeneration no longer active
-> old issued bearer removed
-> transport revocation-scope allow-set shrinks
-> affected live peer retired
-> replacement account authenticator published
```

The existing issuance race fence also remains: an old-password verification that started before the reload cannot mint a new bearer after the new generation commits, because `Issue` re-validates the grant under the issuance/reload mutex.

No F.9-specific game-session teardown mechanism is introduced.

## KDF policy migration

F.9 adds an explicit password-verifier rehash command:

```text
printf '%s\n' 'current password' | accountctl rehash-password \
  -path /secure/accounts.json \
  -login alice \
  -password-stdin \
  -memory-kib 65536 \
  -time 3 \
  -threads 4
```

The command:

- verifies the current password against the existing Argon2id verifier;
- accepts only the same bounded Argon2id v19 envelope already allowed by `worldd`;
- creates a fresh random salt and verifier at the requested target policy;
- increments `credential_version` and the durable store revision;
- intentionally preserves `password_changed_at`, because the human password did not change;
- is a no-op when the existing verifier already uses the target cost policy.

### Uniform online policy remains required

S4-F.9 deliberately does **not** allow a running durable login provider to keep accounts at different Argon2 cost policies. Unknown-login hardening uses equivalent KDF work; permanently mixed costs would make response timing policy-shaped and could weaken account-enumeration resistance.

For a multi-account policy migration:

```text
enter maintenance / do not SIGHUP partial policy state
-> run rehash-password for every account that must move
-> ensure all accounts converge on one target m/t/p policy
-> then start worldd or send one SIGHUP
```

If a partial mixed-policy store is accidentally presented to `worldd`, reload fails closed and the last-known-good account snapshot remains active.

A successful published KDF migration changes the account generation, so previously issued bearers from the old verifier generation are retired through the same S4-F.7 fence.

## Current default password policy

New accounts, operator password rotations and recovery resets continue to use:

```text
Argon2id v=19
memory = 65536 KiB
iterations/time = 3
parallelism = 4
salt = 16 random bytes
output = 32 bytes
```

The bounded `rehash-password` target range matches the existing online parser:

```text
memory: 65536..131072 KiB
time:   3..10
threads: 1..8
salt:   16..64 bytes on stored verifiers
digest: 32 bytes
```

F.9 does not raise the default cost merely to create a milestone; future cost changes should be driven by deployment memory/latency measurements.

## Recovery non-goals

Still outside S4-F.9:

- public self-service password reset endpoint;
- email/SMS ownership verification and delivery;
- recovery codes managed by the Client;
- MFA / TOTP / WebAuthn / passkeys;
- OIDC / external identity provider recovery;
- breach-corpus password checks;
- durable refresh tokens / remembered-device sessions;
- distributed account database or multi-writer recovery CAS;
- cross-server recovery/revocation control plane;
- Client OS keychain integration.

Those require their own threat model and product/identity decisions. They must not be smuggled into Protocol v9 gameplay authority.

## Acceptance criteria

S4-F.9 is complete when automated tests prove at least:

- schema v3 remains readable and v4 migration preserves account state;
- schema v3 rejects recovery fields while schema v4 strictly validates them;
- raw recovery tokens are not persisted in the account store;
- token output is a new `0600` file;
- latest recovery grant supersedes an older grant for the same account;
- exact expiry, unknown, reused and stale-generation recovery proofs fail closed;
- a successful reset changes the password, advances credential generation and atomically consumes recovery grants;
- `worldd` accepts durable schemas v3 and v4 while recovery metadata grants no gameplay authority;
- running durable login continues rejecting mixed Argon2 cost policies;
- KDF rehash preserves `password_changed_at`, advances credential generation, and is idempotent at the target policy;
- existing S4-F.7 reload/generation tests continue proving bearer removal and live-session retirement after a published credential-generation change;
- Protocol remains v9 and the normal login response remains only opaque `session_credential + expires_at`.

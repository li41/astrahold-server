# S4-F.12 Verified Recovery Delivery Adapter / Provider Integration

S4-F.12 adds a provider-neutral verified recovery delivery layer on top of the S4-F.10 public recovery control plane and the S4-F.11 client recovery UX. It does not redesign the public recovery protocol, account authority, gameplay protocol, or realtime transport.

## Scope

The Server remains authoritative for recovery eligibility, destination ownership, account identity, credential generation, proof verification, durable password mutation, bearer revocation, and live-peer retirement.

The public API remains unchanged:

- `POST /v1/account/recovery/request` accepts only `login_id` and returns only opaque `request_id` plus `expires_at` on an accepted request.
- `POST /v1/account/recovery/reset` accepts only `request_id`, `recovery_proof`, and `new_password`.
- The Client cannot choose a delivery destination and does not receive `account_id`, `credential_version`, provider identity, destination ownership, CharacterID, takeover authority, or revocation scope.

S4-F.12 keeps durable account schema v4 and Protocol v9. It does not change GameV1 messages, realtime UDP, gameplay authority, or the single-writer durable JSON account backend.

## Authority split

`accountrecovery.Provider` remains the recovery authority boundary. It owns challenge state, proof verification, generation binding, and the creation of a recovery `Grant`.

S4-F.12 adds `accountrecovery.DeliveryAdapter` as a transport-only seam. A delivery adapter receives Server-owned internal challenge material:

- opaque request correlation,
- Server-owned destination,
- recovery proof bytes,
- expiry.

An adapter cannot decide eligibility, mint a recovery grant, mutate durable account state, choose account identity, or change credential generation. Adapters must not write raw proof or opaque request identifiers to ordinary logs.

## Provider configuration

The existing `-session-recovery-provider-file` flag remains the single provider entrypoint.

Schema v1 remains supported without behavior changes for the S4-F.10 digest-only high-entropy recovery-code provider.

Schema v2 enables the S4-F.12 delivered reference provider. A reference configuration has this shape:

```json
{
  "schema_version": 2,
  "revision": "recovery-delivery-001",
  "proof_key_base64url": "<32-byte raw URL-safe base64 key>",
  "delivery": {
    "adapter": "filesystem-reference-v1",
    "revision": "filesystem-reference-001",
    "inbox_dir": "/run/astrahold/recovery-inbox"
  },
  "subjects": [
    {
      "login_id": "holder",
      "destination": "holder-recovery-channel"
    }
  ]
}
```

`destination` is a Server/provider configuration property. The public recovery request never supplies or overrides it. F.12 does not return a masked destination hint, so the public response does not gain a destination/account-existence oracle.

The reference filesystem adapter is deterministic and CI-friendly. It is not a production email/SMS provider and does not embed credentials for AWS SES, Twilio, SendGrid, or another vendor. Its inbox root must be a real directory with no group/other permissions. Each delivery atomically publishes only `<destination>.proof` with mode `0600`; it does not persist the opaque `request_id`.

## Reference delivered proof

For the schema-v2 reference provider, the proof is an HMAC-SHA256 value under the provider's 32-byte proof key. The HMAC input is domain-separated and bound to Server-owned:

- `login_id`,
- `account_id`,
- `credential_version`.

The encoded proof is URL-safe base64 without padding. The provider stores only the SHA-256 verifier in challenge state after delivery.

The reference proof is generation-bound rather than request-unique: concurrent challenges for the same account generation may receive the same proof. Every challenge is still independently TTL-bound and attempt-bounded. A successful reset increments `credential_version`, and the durable account writer rechecks the grant generation before committing, so every unconsumed challenge from the previous generation becomes stale immediately. The successfully committed challenge is consumed after the durable mutation.

This reference behavior is intentional for a deterministic provider-neutral CI adapter. A future production delivery provider may use request-unique proof material while implementing the same `DeliveryAdapter` transport boundary.

## Enumeration-safe delivery failure mapping

Per-subject delivery state must not become an account-existence oracle.

| Condition | Public request/reset behavior | Authorizing state |
| --- | --- | --- |
| known + eligible + delivery success | request returns `202` with only `request_id`, `expires_at` | challenge becomes authorizing until TTL/attempt/generation/consume fences reject it |
| unknown/ineligible account | request returns the same `202` field set | challenge is non-authorizing; no delivery is attempted |
| delivery transient failure | request returns the same `202` field set | reserved challenge remains non-authorizing |
| delivery permanent/config failure reached for one subject | request returns the same `202` field set | reserved challenge remains non-authorizing |
| provider unavailable before an account-specific delivery decision | generic `503 recovery_unavailable` | none |
| wrong, expired, exhausted, inactive, consumed, or stale-generation proof | generic `401 invalid_recovery` | none |
| source-IP recovery limit | `429 recovery_throttled` plus `Retry-After` | no weakening of the existing guard |

Startup configuration errors remain fail-closed: malformed schema v2, invalid proof-key length, duplicate login/destination ownership, unsupported adapter, unsafe filesystem destination, or an insecure filesystem inbox prevents recovery provider startup.

## Client contract

S4-F.11 is reused unchanged. `SessionLoginClient`, `SessionLoginPanel`, and `Main.SessionAuth.cs` already meet the F.12 public contract:

- request sends only `login_id`,
- successful request parsing allows only `request_id` and `expires_at`,
- proof/new-password reset input stays provider-neutral,
- recovery UI is generic and does not expose account existence or provider-internal identity,
- successful reset still requires a fresh normal login and does not silently replay the replacement password.

No Client product-code change is required for S4-F.12.

## Acceptance

The F.12 production E2E pins the paired S4-F.11 Client main and proves on the exact Server head that:

- an unknown account gets the same accepted public response shape and no reference delivery,
- a forced delivery failure for a known account gets the same accepted shape but produces no authorizing challenge,
- a normal known request writes the reference proof to the Server-owned filesystem destination with `0600` permissions,
- the delivered proof drives the real normal `Main.tscn` S4-F.11 recovery UX through reset and fresh login,
- unknown, inactive, wrong, and stale-generation recovery attempts map to generic `invalid_recovery`,
- old password authentication fails after reset and durable `credential_version` advances,
- an older delivered challenge cannot reset after the successful generation change,
- source-IP throttling still returns `Retry-After`,
- captured Server/Client logs do not contain passwords, delivered proof, opaque request identifiers, or issued bearer fields,
- Protocol remains v9.

The existing exact-head S4-F.10 Production Public Recovery E2E continues to prove the unchanged durable reset writer immediately revokes old issued bearers and retires the old live peer without SIGHUP. Together, these workflows fence the F.12 delivery seam and the pre-existing authoritative session lifecycle behavior.

## Non-goals

S4-F.12 does not add public registration, MFA/TOTP/WebAuthn/passkeys, OAuth/OIDC, refresh tokens, remember-session/keychain storage, distributed account storage, distributed recovery rate limiting, CAPTCHA/reputation, Protocol v10, realtime changes, gameplay authority changes, or a production third-party delivery vendor.

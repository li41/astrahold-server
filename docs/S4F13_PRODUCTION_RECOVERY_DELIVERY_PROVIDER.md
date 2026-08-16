# S4-F.13 Production Recovery Delivery Provider / Operational Hardening

S4-F.13 makes the S4-F.12 `DeliveryAdapter` seam deployable without binding Astrahold core Server code to a specific email/SMS vendor SDK. It adds a vendor-isolated HTTPS JSON relay adapter and operational bounds for credentials, TLS, timeout, retry, idempotency, failure classification, and secret-safe observability.

S4-F.13 does not redesign the public recovery API, the S4-F.11 Client UX, durable account authority, gameplay protocol, or realtime transport.

## Authority and public contract stay unchanged

The Server remains authoritative for recovery eligibility, destination ownership, account identity, credential generation, proof verification, durable password mutation, issued-bearer revocation, and live-peer retirement.

The public recovery exchange remains exactly:

- `POST /v1/account/recovery/request` accepts only `login_id` and returns only opaque `request_id` plus `expires_at` on an accepted request.
- `POST /v1/account/recovery/reset` accepts only `request_id`, `recovery_proof`, and `new_password`.
- the Client cannot choose or discover the delivery destination, provider credential, provider identity, account ID, credential generation, CharacterID, takeover authority, or revocation scope.

S4-F.13 keeps durable account schema v4 and Protocol v9. `accountrecovery.Provider` remains the recovery authority boundary; `accountrecovery.DeliveryAdapter` remains transport-only.

## Vendor-isolated HTTPS relay adapter

Schema-v2 recovery provider configuration can now select either:

- `filesystem-reference-v1` — the existing deterministic F.12 reference adapter; or
- `https-json-v1` — the F.13 deployable relay adapter.

Example:

```json
{
  "schema_version": 2,
  "revision": "recovery-delivery-002",
  "proof_key_base64url": "<32-byte raw URL-safe base64 key>",
  "delivery": {
    "adapter": "https-json-v1",
    "revision": "recovery-relay-001",
    "endpoint": "https://recovery-relay.internal.example/v1/deliver",
    "credential_file": "/run/astrahold/secrets/recovery-relay.token",
    "ca_file": "/etc/astrahold/recovery-relay-ca.pem",
    "request_timeout": "1s",
    "max_attempts": 3,
    "retry_backoff": "200ms"
  },
  "subjects": [
    {
      "login_id": "holder",
      "destination": "holder@example.invalid"
    }
  ]
}
```

`endpoint` must be an absolute HTTPS URL without embedded userinfo or fragment. Redirects are not followed. The HTTP client requires TLS 1.3 or newer. `ca_file` is optional and adds private relay trust roots without disabling normal certificate validation.

The adapter uses only Go standard-library HTTP/TLS primitives. AWS SES, Twilio, SendGrid, or another vendor can live behind the relay without becoming an Astrahold Server compile-time dependency.

## Credential lifecycle boundary

The relay Bearer credential is not stored inline in the recovery provider JSON. `credential_file` is loaded once at process startup and must be:

- a real regular file, not a symlink;
- owner-only (`group/other` permission bits must be zero);
- 16..4096 visible ASCII bytes, with an optional final CR/LF.

The credential is sent only in the HTTPS `Authorization: Bearer ...` header. It is not included in the relay JSON payload or ordinary Server logs.

S4-F.13 intentionally keeps credential/provider reload restart-only. Runtime rotation/hot reload is a separate decision gate so a failed reload cannot silently replace the last-known-good recovery transport.

## Relay wire contract and idempotency

For an eligible Server-owned subject, `https-json-v1` sends one JSON object:

```json
{
  "schema_version": 1,
  "delivery_id": "<derived opaque id>",
  "destination": "<Server-owned destination>",
  "proof": "<recovery proof>",
  "expires_at": "<UTC RFC3339Nano>"
}
```

The raw public `request_id` is deliberately not sent to the relay. `delivery_id` is derived as the first 128 bits of:

```text
SHA-256("astrahold-recovery-delivery-id-v1" || 0x00 || request_id)
```

and encoded as unpadded URL-safe base64. The same value is sent as:

- JSON `delivery_id`;
- `Idempotency-Key`;
- `X-Astrahold-Delivery-ID`.

All retries for the same recovery request reuse the same JSON body and idempotency key. The relay therefore has a stable key for suppressing duplicate vendor sends without learning the public recovery request ID.

## Timeout, retry, and outcome classification

Operational bounds are intentionally small enough to fit inside the existing login control-plane HTTP write fence:

```text
request_timeout    default 1s, allowed 100ms..2s
max_attempts       default 3, allowed 1..3
retry_backoff      default 200ms, allowed 0..500ms
```

Retry backoff is bounded exponential backoff capped at 500ms.

Outcome mapping:

| Relay/transport outcome | Adapter result | Retry |
| --- | --- | --- |
| HTTP 2xx | success | no |
| transport error / attempt timeout | transient | yes, if attempts remain |
| HTTP 408 / 425 / 429 | transient | yes, if attempts remain |
| HTTP 5xx | transient | yes, if attempts remain |
| other HTTP 3xx / 4xx | permanent | no |

Redirect responses are therefore permanent failures rather than a credential-forwarding path.

The F.12 enumeration-resistant provider mapping remains authoritative: a per-subject transient or permanent delivery failure still returns the same public `202 + request_id + expires_at` shape as a normal accepted request, while the reserved challenge remains non-authorizing. Unknown/ineligible subjects still do not invoke the adapter at all.

## Secret-safe observability

A completed delivery emits only bounded metadata such as:

```text
recovery delivery: adapter=https-json-v1 revision=recovery-relay-001 outcome=success attempts=2 status_class=2xx
```

Ordinary adapter logs do not include:

- destination;
- recovery proof;
- public `request_id`;
- derived `delivery_id` / idempotency key;
- Bearer credential;
- response body.

This is operational outcome visibility, not per-user tracing. A production relay may maintain its own audited delivery records under its own secret-handling policy.

## Client contract

S4-F.11 is reused unchanged. The normal Godot `Main.tscn` still:

- sends only `login_id` to request recovery;
- treats `request_id` as opaque process-memory state;
- accepts provider-neutral recovery proof input;
- submits only `request_id + recovery_proof + new_password`;
- returns to normal login after a successful reset and requires a fresh password submit.

No Client product-code or Protocol v9 change is required for S4-F.13.

## Acceptance

The F.13 production E2E uses a local TLS 1.3 fake relay and pins the paired S4-F.11 Client. On the exact Server head it proves that:

- an unknown account receives the same accepted public response and causes no relay request;
- an eligible subject receiving HTTP 400 is attempted once, still receives the generic public accepted shape, and its challenge is non-authorizing;
- an eligible subject receiving HTTP 503 then HTTP 202 is retried with the same derived idempotency key and identical payload;
- the relay receives a Bearer credential from a separate owner-only file, while the provider config itself contains only the credential path;
- the relay connection negotiates TLS 1.3;
- the successfully relayed proof drives the real normal F.11 `Main.tscn` reset and fresh-login flow;
- the old password is rejected and durable `credential_version` advances;
- captured Server, Client, and relay logs do not contain passwords, recovery proof, destinations, public request IDs, relay Bearer credential, or issued bearer fields;
- Server observability reports only safe adapter/revision/outcome/attempt/status-class metadata;
- Protocol remains v9.

Existing F.10 and F.12 exact-head workflows continue to fence no-SIGHUP old-generation bearer/live-peer retirement and the deterministic filesystem adapter compatibility path.

## Non-goals

S4-F.13 does not select or embed a third-party delivery vendor SDK, add public registration, MFA/TOTP/WebAuthn/passkeys, OAuth/OIDC, refresh tokens, remember-session/keychain storage, distributed account storage, distributed recovery rate limiting, CAPTCHA/reputation, runtime credential hot reload, Protocol v10, realtime changes, or gameplay authority changes.

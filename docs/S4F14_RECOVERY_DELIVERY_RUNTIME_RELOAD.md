# S4-F.14 Recovery Delivery Credential Rotation / Runtime Reload

S4-F.14 adds fail-closed runtime generation reload for the S4-F.12/F.13 schema-v2 delivered-recovery provider. It allows an operator to rotate the recovery provider definition, HMAC proof key, HTTPS relay Bearer credential, relay endpoint, private CA roots, destination map, and bounded relay settings with `SIGHUP` while the login/recovery control plane remains online.

S4-F.14 does not change the public recovery API, the S4-F.11 Client UX, durable account schema v4, Protocol v9, realtime transport, or gameplay authority.

## Authority and public contract stay unchanged

The Server remains authoritative for:

- recovery eligibility;
- destination ownership;
- account identity and `credential_version`;
- recovery proof generation/verification;
- durable password mutation;
- issued-bearer revocation and live-peer retirement.

The public exchange remains:

```text
POST /v1/account/recovery/request
  request:  login_id only
  response: opaque request_id + expires_at only

POST /v1/account/recovery/reset
  request:  request_id + recovery_proof + new_password
  response: no account/provider/destination authority
```

`accountrecovery.Provider` remains the recovery-authority boundary and `DeliveryAdapter` remains transport-only.

## Reload eligibility

Runtime recovery reload is enabled only when startup loaded a schema-v2 delivered provider. Schema-v1 digest-only recovery-code compatibility remains restart-only.

A successful schema-v2 startup provider is wrapped by a runtime generation router:

```text
generation 1
  current schema-v2 provider
  challenge route map
  bounded retired-generation set
```

Every successful recovery-provider `SIGHUP` publishes one new monotonic runtime generation, even when the provider's textual `revision` is unchanged. This deliberately allows credential-only or private-CA-only rotation without requiring a fake config revision bump.

A reload that attempts to downgrade the live runtime to schema v1 is rejected and retains the last-known-good schema-v2 generation.

## Validation before publication

The replacement provider is loaded and fully validated before it can become current. Validation includes the existing F.12/F.13 rules for:

- strict provider JSON fields and schema;
- 32-byte proof key;
- destination uniqueness/ownership;
- adapter-specific configuration;
- absolute HTTPS relay URL and redirect policy;
- request timeout / retry bounds;
- owner-only regular Bearer credential file;
- credential syntax/length;
- private CA PEM parsing.

Invalid config, missing files, broad credential permissions, malformed credentials, invalid private CA data, unsupported adapter configuration, or a schema-v1 replacement all fail closed. No partial replacement is published.

## Generation cutover and in-flight delivery fence

The critical cutover rule is:

```text
Begin on generation N starts
  -> generation-N delivery may block/retry
  -> delivery completes
  -> opaque challenge route is registered
  -> only then may generation N+1 publish
```

The generation router holds a read-side publication fence for the complete `Begin` operation. Reload takes the write side of the same fence. Therefore:

- an already-started generation-N delivery is allowed to complete with the generation-N credential/CA/provider;
- reload waits for that `Begin` to finish and register its opaque challenge route;
- once generation N+1 is published, every new `Begin` uses only generation N+1;
- no post-cutover delivery can start with the retired generation.

This avoids both unsafe boundary states:

1. a proof was delivered but its opaque `request_id` became immediately unroutable; or
2. a request that began after the reload still used the old relay credential.

## Old challenge routing after rotation

A challenge created before cutover remains routed to its original provider verifier until it is:

- consumed by a successful durable reset;
- expired; or
- safely retired by the bounded retired-generation cap.

The old provider does not need its delivery secrets to verify an existing challenge. Each challenge already stores the proof verifier digest and Server-owned Subject generation.

At cutover the retired provider immediately clears:

- its in-memory schema-v2 HMAC proof key; and
- adapter-specific delivery credentials when the adapter supports retirement.

For `https-json-v1`, retirement clears the in-memory Bearer credential and closes idle HTTP connections. New calls against a retired HTTP adapter fail closed. Private CA certificates are public trust material and are not treated as secret-bearing verifier state.

Durable password reset still re-checks `account_id + credential_version`. A challenge that survives provider rotation cannot bypass a later account-generation change.

## Bounded retired generations

The router retains at most four previous provider generations that still have unexpired challenges.

```text
current generation                1
retired generations retained   <= 4
active challenges/provider     <= 4096
```

Expired/consumed routes are removed. If repeated reloads would exceed the four-generation retired bound, the oldest remaining generation is safely retired: its routed challenges are consumed and later verification returns the existing generic `invalid_recovery` result.

This keeps reload memory bounded without introducing a new public status or account-enumeration signal.

## SIGHUP interaction with durable account reload

A single `SIGHUP` may trigger both existing durable-account reload and the new recovery-provider reload. They are validated and reported independently.

This is intentional:

- recovery credential/CA rotation must still work when the durable account file revision is unchanged;
- a rejected recovery-provider reload must not roll back a valid account-generation reload;
- a rejected account reload must not prevent an independently valid recovery credential rotation.

The two files are not a distributed transaction. Existing durable reset checks remain the final authority for stale account generations.

## Observable reload outcomes

Startup reports the recovery reload mode and generation:

```text
session recovery: ... recovery_reload=sighup generation=1
```

Successful reload emits bounded metadata:

```text
session recovery reload applied: previous_generation=1 generation=2 previous_revision=... revision=... previous_provider=... provider=... retained_challenges=1 retired_challenges=0
```

Rejected reload reports the retained generation/revision:

```text
session recovery reload rejected; last-known-good retained: generation=2 revision=... err=...
```

Reload logs do not contain recovery proofs, destination values, public request IDs, derived delivery IDs, Bearer credentials, passwords, or issued session credentials.

Existing F.13 per-delivery outcome logs remain unchanged and secret-safe.

## Client contract

S4-F.11 Client product code is reused unchanged. The Client does not know that a provider generation rotated and continues to:

- submit only `login_id` for recovery requests;
- keep opaque `request_id` only in process memory;
- accept provider-neutral proof input;
- submit only `request_id + recovery_proof + new_password` for reset;
- require a fresh normal login after successful reset.

No provider generation, destination, relay identity, CA identity, or credential lifecycle authority is added to the Client.

## Acceptance

The F.14 production E2E pins the existing S4-F.11 Client and uses two local TLS 1.3 fake relays with distinct credentials and trust roots. On the exact Server head it proves that:

- generation 1 delivers through relay A with credential A and CA A;
- provider config can rotate to relay B with credential B and CA B on `SIGHUP` without restarting `worldd`;
- the successful reload advances the runtime generation;
- a generation-1 challenge delivered before cutover remains redeemable after generation 2 publishes;
- subsequent recovery requests use relay B only;
- a malformed/broad-permission replacement credential is rejected while generation 2 remains last-known-good and still delivers successfully;
- the normal unchanged F.11 `Main.tscn` can use a generation-2 delivered proof to reset and fresh-login;
- old passwords and stale account-generation proofs remain rejected;
- captured Server, Client, and relay logs do not expose passwords, proofs, destinations, public request IDs, derived delivery IDs, relay credentials, or issued bearer fields;
- existing F.10/F.12/F.13 exact-head workflows remain green;
- durable account schema stays v4 and Protocol stays v9.

## Non-goals

S4-F.14 does not add TLS game/login certificate hot reload, vendor SDKs, a distributed or durable asynchronous delivery queue, public registration, MFA/TOTP/WebAuthn/passkeys, OAuth/OIDC, refresh tokens, remember-session/keychain storage, distributed account storage, distributed rate limiting, trusted reverse-proxy attribution, CAPTCHA/reputation, Protocol v10, realtime changes, or gameplay authority changes.

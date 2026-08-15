# S4-F.2 — Credential Expiry / Rotation / Revocation Lifecycle

## Scope

S4-F.2 adds explicit admission-time lifecycle semantics behind the S4-F.1 `sessioncredential.Provider` boundary. It does not change the trusted TCP preface, Protocol v9, realtime authentication, ownership fencing, or Client wire behavior.

The current static SHA-256 provider gains a lifecycle-aware schema v2 while retaining schema v1 compatibility.

## Lifecycle model

Each schema v2 credential has a unique Server-side `credential_id` and may define three RFC3339 boundaries:

- `not_before`: credential is rejected before this instant and accepted at exactly this instant;
- `expires_at`: credential is rejected at exactly this instant and afterward;
- `revoked_at`: credential is rejected at exactly this instant and afterward.

All comparisons use the Server clock. Parsed timestamps are normalized to UTC before comparison. Missing boundaries are unbounded.

`expires_at` must be strictly later than `not_before` when both are present. A zero or inverted validity window is invalid configuration and fails startup.

Lifecycle validation runs every time the provider resolves a credential during a new trusted TCP authentication. The Client does not provide clock, expiry, revocation, CharacterID, or takeover truth.

## Schema v2

Example:

```json
{
  "schema_version": 2,
  "revision": "prod-2026-08-rotation-01",
  "credentials": [
    {
      "credential_id": "keeper-2026-08-old",
      "token_sha256": "<64 lowercase hex chars>",
      "character_id": "keeper",
      "allow_active_takeover": true,
      "not_before": "2026-08-01T00:00:00Z",
      "expires_at": "2026-08-20T00:00:00Z"
    },
    {
      "credential_id": "keeper-2026-08-new",
      "token_sha256": "<different 64 lowercase hex chars>",
      "character_id": "keeper",
      "allow_active_takeover": true,
      "not_before": "2026-08-15T00:00:00Z",
      "expires_at": "2026-09-15T00:00:00Z"
    }
  ]
}
```

Schema v2 rules:

- `credential_id` is required, trimmed, 1..128 bytes, and unique within the file;
- `token_sha256` remains a unique 64-character lowercase SHA-256 digest;
- `character_id` remains Server-owned and must pass trusted identity validation;
- `allow_active_takeover` remains optional and defaults to false;
- lifecycle timestamps are optional RFC3339 values;
- unknown JSON fields remain rejected;
- duplicate credential digests remain rejected;
- duplicate `credential_id` values are rejected.

## Schema v1 compatibility

Existing schema v1 files continue to load with their original timeless static-map semantics.

Schema v1 is intentionally not allowed to use `credential_id`, `not_before`, `expires_at`, or `revoked_at`. Operators must explicitly move a file to schema v2 before lifecycle policy is interpreted. This prevents a deployment from silently changing security semantics while still declaring the legacy schema.

## Rotation

Rotation is modeled by multiple different credential digests resolving to the same trusted CharacterID with overlapping validity windows.

For example:

```text
old credential:  valid -----------| expires
new credential:        |---------- valid ---------------->
                       ^ overlap ^
```

During overlap both credentials may authenticate to the same CharacterID. After the old `expires_at`, the old credential fails while the new credential continues to resolve normally.

Rotation does not change EntityID, ownership-fence semantics, or realtime generation rules. A reconnect using the replacement credential still creates a fresh reliable/realtime connection generation as before.

## Revocation

`revoked_at` is an explicit Server-side admission cutoff. At or after that instant, a new authentication using that credential fails closed even if its normal expiry is later.

This stage intentionally applies revocation at credential resolution time only. It does **not** continuously revalidate or forcibly disconnect a session that was already admitted before the cutoff.

The current provider configuration is still loaded at process startup. Therefore an emergency revocation that was not already present in the loaded file requires an operational config update plus process restart. Runtime config reload / active-session invalidation is a separate bounded deployment stage.

## Provider errors and failure semantics

The lifecycle package exposes distinct internal errors for:

- not yet valid;
- expired;
- revoked;
- invalid lifecycle.

The static provider joins those reasons with the existing generic trusted-credential rejection error. This preserves a fail-closed authentication result while still allowing Server-side tests/logging to distinguish the reason.

Unknown credentials continue to produce the generic credential rejection path.

## Security boundaries preserved

- Client still sends only the opaque credential in the existing bounded `ASTRAH1` preface.
- The credential provider owns lifecycle evaluation; Client time is never trusted.
- CharacterID and active-takeover claim remain Server-owned provider output.
- A lifecycle-valid grant must still contain `AssuranceTrusted` before normal GameV1 admission.
- Active takeover still uses the existing connection-scoped exact-CharacterID authorizer, candidate lease/cooldown, ownership fence, and CAS transfer.
- Trusted plaintext backend TCP remains literal-loopback-only behind the existing TLS 1.3 ingress.
- Credential resolution remains outside the world-owner tick under the existing bounded authentication context.

## Protocol / realtime boundary

No wire change:

- Protocol remains v9;
- `ASTRAH1` preface is unchanged;
- MessageType values are unchanged;
- SessionWelcome ordering is unchanged;
- realtime RoutingID / HMAC domains are unchanged;
- UDP MTU remains 1200;
- WorldSnapshot max chunk remains 43 entities;
- same-IP authenticated NAT-like endpoint migration remains unchanged.

## Focused acceptance coverage

Tests cover:

- inclusive `not_before` boundary;
- exclusive `expires_at` boundary;
- exclusive `revoked_at` boundary;
- invalid / zero-length lifecycle windows;
- schema v1 remaining compatible;
- schema v1 refusing lifecycle fields;
- schema v2 duplicate `credential_id` rejection;
- schema v2 invalid-window rejection;
- old and new credentials both resolving during rotation overlap;
- old credential expiring while the new credential remains valid;
- lifecycle errors remaining fail-closed through the normal credential rejection path;
- legacy active-takeover credential behavior remaining unchanged.

## Deliberate non-goals

S4-F.2 does not add:

- runtime credential-file reload;
- emergency hot revocation of a newly edited file without restart;
- forced disconnect of already-admitted sessions when a credential later expires or is revoked;
- credential issuance / login service;
- refresh tokens;
- account database;
- device/IP binding;
- distributed session ownership;
- Protocol v10;
- periodic realtime rekey;
- DTLS or QUIC.

The next bounded credential stage can add operational reload / active-session invalidation or formal login/session issuance without changing the provider or transport wire contracts established here.

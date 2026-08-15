# S4-F.1 — Session Credential Provider Seam

## Scope

S4-F.1 turns the production trusted-character credential lookup into a formal Server-side provider boundary without changing the existing Client preface, Protocol v9, trusted TLS ingress, realtime authentication, or gameplay authority.

The existing static SHA-256 credential map remains the first production provider implementation. This stage changes the dependency direction so the connection authenticator no longer owns credential storage semantics.

## Problem

Before S4-F.1, `cmd/worldd/trusted_character_auth.go` did two jobs in one object:

1. parse the bounded `ASTRAH1` pre-GameV1 credential preface from the accepted TCP connection;
2. hash the credential and resolve trusted CharacterID / takeover policy directly from the static in-memory map loaded from `-trusted-character-auth-file`.

The transport authentication seam itself was already clean, but this composition made a future account/session backend require editing the preface authenticator instead of supplying a credential provider.

## Provider contract

S4-F.1 adds `internal/sessioncredential`.

A provider receives only:

- the authentication `context.Context` supplied under the existing bounded transport deadline;
- the opaque credential bytes extracted from the trusted preface.

It returns a `sessioncredential.Grant` containing Server-owned claims:

- a valid `characteridentity.Binding` with `AssuranceTrusted`;
- `AllowActiveTakeover`, selected by the same credential proof.

The provider does not receive TCP connections, SessionID, provisional EntityID, world state, or transport takeover request types. It therefore cannot become a second world/session authority.

The credential byte slice remains caller-owned and must not be retained after `Resolve` returns.

## Connection authenticator responsibility

The `worldd` trusted connection authenticator now owns only the transport-facing work:

1. validate the existing bounded `ASTRAH1` header;
2. read the opaque credential with the existing 256-byte maximum;
3. delegate the credential to the configured `sessioncredential.Provider` under the existing authentication context/deadline;
4. require a valid trusted grant;
5. convert `AllowActiveTakeover` into the existing connection-scoped exact-CharacterID `CharacterTakeoverAuthorizer`;
6. leave following GameV1 bytes untouched.

Provider error or invalid/non-trusted grant fails the candidate before normal GameV1 admission.

## Static SHA-256 provider

`-trusted-character-auth-file` remains compatible and continues to use the existing schema:

```json
{
  "schema_version": 1,
  "revision": "example-001",
  "credentials": [
    {
      "token_sha256": "<64 lowercase hex chars>",
      "character_id": "character-id",
      "allow_active_takeover": false
    }
  ]
}
```

At startup `worldd` still:

- strict-decodes the file with unknown fields rejected;
- requires a non-empty revision and credential set;
- rejects duplicate SHA-256 digests;
- validates every trusted CharacterID;
- requires the backend TCP listener to be literal loopback when trusted credential authentication is enabled.

The static provider is immutable after construction. Concurrent connection authentication performs only SHA-256 plus read-only map lookup.

## Preserved security boundaries

- Client still sends only an opaque credential; it never sends CharacterID or takeover authority.
- CharacterID remains selected by Server-side credential claims.
- `allow_active_takeover` remains Server-side policy and is bound to the exact trusted CharacterID selected by the same credential.
- Active takeover still uses the existing connection-scoped authorizer, candidate serialization/cooldown, exact ownership fence, and CAS transfer.
- Authentication remains outside the world-owner tick and under the existing bounded TCP authentication deadline.
- The trusted plaintext backend still requires literal loopback; production credential transport remains the S4-E.4 TLS 1.3 ingress.
- Default development mode with no trusted credential config remains ephemeral.

## Protocol / realtime boundary

No wire change is introduced:

- Protocol remains v9;
- `ASTRAH1` preface format is unchanged;
- MessageType values are unchanged;
- `SessionWelcome` ordering is unchanged;
- Protocol v9 realtime RoutingID / HMAC behavior is unchanged;
- UDP MTU remains 1200;
- `WorldSnapshot` max chunk remains 43 entities;
- same-IP authenticated NAT-like source-port migration remains unchanged.

## Focused acceptance tests

Coverage includes:

- `sessioncredential.Grant` accepts only valid trusted identity;
- existing static credential config and SHA-256 lookup remain compatible;
- authenticator delegates the exact opaque credential value to a replaceable provider;
- provider-backed authentication consumes only the preface and leaves following GameV1 bytes untouched;
- provider takeover claim becomes the existing connection-scoped exact-character authorizer;
- provider error fails closed;
- invalid/non-trusted provider grant fails closed;
- unknown static-map credential still fails closed;
- strict config and duplicate digest rejection remain covered;
- loopback-only trusted backend enforcement remains covered.

## Deliberate non-goals

S4-F.1 does not add:

- account database or login UI;
- credential issuance endpoint;
- credential expiry;
- credential rotation;
- credential revocation;
- refresh tokens;
- device/IP binding;
- distributed session ownership;
- new Client message or Protocol v10;
- periodic realtime rekey;
- DTLS or QUIC.

The next bounded credential stage can implement expiry / rotation / revocation behind the provider boundary instead of changing transport authentication again.

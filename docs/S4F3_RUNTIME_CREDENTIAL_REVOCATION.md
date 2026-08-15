# S4-F.3 — Runtime Credential Reload / Emergency Revocation / Active Session Invalidation

## Scope

S4-F.3 operationalizes the schema-v2 credential lifecycle added in S4-F.2.

The production static SHA-256 credential file can now be reloaded at runtime without restarting `worldd`. A successful reload changes admission policy for new connections and can immediately retire already-admitted sessions whose credential proof is removed or no longer lifecycle-active.

This stage remains Server-only. It does not change Protocol v9, the `ASTRAH1` trusted preface, Client messages, realtime HMAC framing, Gameplay World authority, or ownership primitives.

## Goals

S4-F.3 provides four bounded guarantees:

1. schema-v2 credential policy can be reloaded on `SIGHUP`;
2. invalid reloads keep the last-known-good policy and do not partially publish;
3. removed, revoked, expired, or proof/identity/takeover-mutated credential generations can no longer publish new peers and existing peers using those generations are retired;
4. retirement reuses the existing transport teardown and fenced world leave rather than creating a second session authority.

## Runtime requirement

Hot reload and live-session invalidation require trusted auth schema v2.

Schema v1 remains accepted at process startup for backward compatibility, but it has no `credential_id`, lifecycle metadata, or runtime proof-generation identity. Therefore schema-v1 configurations deliberately remain restart-only.

Operators that need emergency revocation should migrate the trusted auth file to schema v2 before relying on S4-F.3.

## Server-owned revocation scope

Each valid schema-v2 credential record produces an opaque Server-owned `RevocationScope`.

The scope is a SHA-256 fingerprint over the credential proof and authority fields under an Astrahold-specific domain:

- `credential_id`;
- `token_sha256`;
- trusted `CharacterID`;
- `allow_active_takeover`.

Lifecycle timestamps deliberately do not participate in the fingerprint. `not_before`, `expires_at`, and `revoked_at` control whether that scope belongs to the current Server-clock active set.

The resulting scope is internal metadata. It is never sent to the Client and is not a bearer credential.

Important semantics:

- loading the same proof/identity/takeover record again produces the same scope;
- changing token, CharacterID, credential ID, or takeover authority produces a different scope;
- changing a future lifecycle boundary keeps the same proof scope and lets the boundary timer enforce the new cutoff at the requested time;
- making a lifecycle rule already-invalid at reload time removes the scope immediately;
- changing only the top-level config revision does not itself keep or revoke a session;
- multiple rotation credentials for the same CharacterID remain separate scopes.

This lets runtime policy invalidate one credential proof generation without treating CharacterID itself as a revocation key, while preserving scheduled expiry/revocation semantics.

## Admission race fence

Reload must handle a connection that started authentication immediately before the operator revoked its credential.

The critical ordering is:

```text
load + validate replacement config
        ↓
compute active schema-v2 scopes at Server time
        ↓
atomically replace transport allowed-scope set
+ publish ready=false for removed peers
+ remove their realtime token / RoutingID lookups
        ↓
publish replacement credential provider
        ↓
finish TCP close + fenced leave outside transport mutex
```

The transport scope set and peer token/route publication use the same `tcpudp.Server` mutex.

`registerPeer` rechecks the trusted connection's `RevocationScope` under that mutex before publishing its realtime token and public RoutingID. Therefore an authentication result created from the old provider cannot become authoritative after a later reload has removed that proof generation.

For already-published peers, the same mutex section sets `ready=false` and removes both realtime lookup handles before the scope replacement lock is released. There is therefore no post-reload UDP lookup window where a retired route remains eligible.

A short fail-closed interval is acceptable during a proof-generation change: the new scope fence is installed before the new provider becomes visible. This prefers revocation safety over admitting a connection during the handoff.

## Existing-session invalidation

A successful scope replacement scans the currently registered peers.

Only peers produced by `TrustedCharacterConnectionAuthenticator` participate in the trusted scope policy. Development/ephemeral peers are not accidentally retired by a trusted credential reload.

A trusted peer is retired when its scope is empty or absent from the replacement active set.

Retirement preserves the established teardown ordering:

```text
under transport scope mutex:
  ready = false
  revoke realtime secret-token lookup
  revoke public RoutingID lookup

outside transport scope mutex:
  close reliable TCP connection
  enqueue fenced leave for joined trusted ownership
```

The outside-mutex teardown still goes through `closePeer`, whose exact-peer deletion and `closeOnce` / `leaveOnce` semantics remain idempotent.

This preserves the S4-E.6 realtime generation guarantees and the S3-F/S4-E ownership fencing guarantees. Credential reload does not directly mutate world state.

A stale leave from a retired generation therefore cannot remove a newer owner after a legitimate reconnect/takeover.

## Lifecycle-boundary invalidation

S4-F.2 already evaluated `not_before`, `expires_at`, and `revoked_at` whenever a credential was resolved for a new connection.

S4-F.3 also tracks the next future lifecycle boundary across the current schema-v2 provider and schedules a process-local timer for that exact boundary.

At the boundary the active scope set is recomputed from the Server clock:

- reaching `not_before` adds that credential proof scope to the allowed set;
- reaching `expires_at` removes it and retires matching live peers;
- reaching `revoked_at` removes it and retires matching live peers.

The boundary semantics remain exactly those from S4-F.2:

- `not_before` is inclusive;
- `expires_at` rejects at the exact cutoff and afterward;
- `revoked_at` rejects at the exact cutoff and afterward.

For example, adding a future `revoked_at` by SIGHUP does not disconnect an otherwise unchanged live proof immediately. The same scope remains active until that Server-clock boundary, then the timer removes it and retires matching sessions.

No Client clock participates.

## SIGHUP reload workflow

For a production schema-v2 deployment:

1. construct and validate the replacement credential JSON out of band;
2. atomically replace the file referenced by `-trusted-character-auth-file`;
3. send `SIGHUP` to the running `worldd` process;
4. inspect the `worldd` log for the applied revision, active scope count, and retired peer count.

Example operator action:

```text
kill -HUP <worldd-pid>
```

`SIGHUP` is intentionally scoped only to trusted credential reload in this stage. It does not hot-reload Gameplay World, combat data, TLS certificates, or other process-start configuration.

## Last-known-good behavior

A reload candidate is strict-decoded and fully validated before it affects live policy.

If reading, JSON parsing, schema validation, credential validation, lifecycle validation, or schema-version checks fail:

- the current provider remains installed;
- the current transport scope set remains installed;
- existing sessions remain under the current policy;
- an error is logged;
- `worldd` keeps running.

There is no partial publication of a malformed file.

A schema-v2 runtime cannot be downgraded to schema v1 through `SIGHUP`; that request is rejected and the last-known-good v2 provider remains active.

## Rotation behavior

Planned rotation remains compatible with the S4-F.2 overlap model.

During an overlap window, both the old and new credential records are lifecycle-active, so both revocation scopes are allowed. Existing sessions using the old credential stay connected while new sessions may authenticate with either valid proof.

At the old credential's `expires_at` or `revoked_at` boundary, its scope disappears automatically and sessions authenticated with that old generation are retired. The new credential generation remains active.

An operator can also remove the old record and issue `SIGHUP` for immediate retirement before its scheduled expiry.

## Takeover and ownership authority

`allow_active_takeover` remains a Server-owned claim selected by the same credential record that selects CharacterID.

Because takeover policy is included in the revocation-scope fingerprint, changing takeover permission changes the proof generation. Existing sessions authenticated under the previous takeover policy are retired on reload rather than silently retaining authority that the replacement record removed.

Actual active takeover still uses the existing connection-scoped authorizer, candidate gate, ownership fence, and CAS transfer path. S4-F.3 does not bypass those checks.

## Realtime boundary

No realtime wire change is introduced.

Protocol remains v9:

- public 128-bit RoutingID;
- HMAC-SHA256 truncated to 128 bits;
- C2S/S2C domain separation;
- UDP authenticity/integrity without confidentiality;
- MTU 1200;
- maximum GameV1 snapshot payload 1132 bytes / 43 entities;
- same-IP authenticated NAT source-port migration remains unchanged.

When credential policy retires a live peer, its current realtime generation is removed through the same exact-token/exact-route revocation logic already used for disconnect and takeover.

## Focused acceptance coverage

S4-F.3 adds coverage for:

- deterministic scope identity for an unchanged schema-v2 proof/identity/takeover record;
- scope rotation when token/identity/takeover authority changes;
- scheduled lifecycle changes keeping the proof scope stable until their Server-clock boundary;
- authenticator propagation of the Server-owned scope into the transport authentication result;
- stale in-flight authentication failing at peer publication after its scope is removed;
- existing peer retirement atomically removing token/route lookup and then closing the connection;
- allowed trusted peers remaining connected;
- development/unauthenticated peers remaining unaffected;
- successful runtime config replacement publishing the transport fence before swapping providers;
- removed old credential failing after reload while the new credential succeeds;
- malformed reload retaining the last-known-good provider and scope set;
- lifecycle-active scope calculation;
- exact next-boundary calculation;
- schema-v1 startup compatibility without pretending that v1 supports hot revocation.

Full Server CI remains the merge gate: `go test ./...`, `go vet ./...`, and the concurrency race detector.

## Deliberate non-goals

S4-F.3 does not add:

- a public login endpoint;
- credential/session issuance;
- refresh tokens;
- account database integration;
- distributed credential/revocation control plane;
- cross-server session invalidation;
- TLS certificate hot reload;
- Client-visible logout/revocation reason message;
- Protocol v10;
- UDP encryption;
- cross-IP realtime migration;
- periodic in-session realtime rekey.

The next bounded productization stage can add formal login/session credential issuance behind the provider/lifecycle/runtime-revocation contracts established by S4-F.1 through S4-F.3.

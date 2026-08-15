# S3-F.21 — Explicit Trusted Active Takeover Authorization Gate

## Scope

S3-F.21 hardens the S3-F.20 active TCP/UDP takeover path with an explicit server-side authorization gate.

S3-F.20 proved that an already-active trusted `CharacterID` can be transferred safely between network Sessions, but its only authorization premise was that the configured `CharacterIdentityFactory` returned the same `AssuranceTrusted` identity. S3-F.21 separates those responsibilities:

- identity resolution answers **which trusted character this connection represents**;
- takeover authorization answers **whether this connection may replace the exact currently active owner**.

This is a bounded server integration seam. It does not invent a Client credential protocol or claim that the default development transport authenticates reconnects.

## Secure default

`Config.CharacterTakeoverAuthorizer` defaults to `nil`.

A `nil` authorizer means an active trusted takeover is rejected with `ErrCharacterTakeoverUnauthorized`.

This fail-closed default affects only the active takeover branch. It does not change:

- ephemeral joins;
- inactive trusted admission;
- durable restore for inactive trusted characters;
- the default `worldd` development transport, which still emits fresh ephemeral identities and therefore never reaches trusted active takeover.

A deployment that supplies a trusted `CharacterIdentityFactory` must now also supply a `CharacterTakeoverAuthorizer` if it intentionally wants active takeover behavior.

## Authorization request

The authorizer receives a server-internal `CharacterTakeoverRequest` containing:

- candidate `SessionID`;
- trusted `CharacterIdentity`;
- the exact current `SessionOwnershipFence` returned by the S3-F.20 connection-plan barrier;
- the candidate TCP remote address string.

The request is valid only when:

- candidate SessionID is nonzero;
- identity is valid and `AssuranceTrusted`;
- expected ownership fence is valid;
- expected ownership CharacterID exactly matches the trusted identity.

The remote address is context for policy/telemetry only. It is not treated as a security credential.

## Ordering

For an active trusted connection attempt:

1. `CharacterIdentityFactory` resolves a trusted CharacterID.
2. S3-F.20 `AwaitCharacterConnectionPlan` returns the exact current active ownership fence.
3. S3-F.21 calls `CharacterTakeoverAuthorizer` outside the world-owner tick.
4. Missing authorizer or any authorizer error rejects the candidate.
5. Only an explicit `nil` result permits PlayerFactory / replacement Session construction to continue.
6. The exact same ownership fence is then passed to S3-F.19 `AwaitOwnershipTransfer`.
7. F.19 compare-and-swap remains the final ownership boundary.

Authorization therefore does not reserve ownership and does not weaken fencing. If ownership changes while the authorizer is running, the already-authorized stale fence cannot replace the newer owner; F.19 rejects it as stale.

There is deliberately no automatic re-authorization against a newer fence after such a failure.

## Failure behavior

### Missing authorizer

Active takeover fails with `ErrCharacterTakeoverUnauthorized` before replacement peer creation or ownership transfer.

The old Session remains active and its TCP/UDP routes are unchanged.

### Authorizer rejection

Any non-nil authorizer error fails closed. The returned transport error wraps both:

- `ErrCharacterTakeoverUnauthorized` for stable classification;
- the upstream authorizer error for diagnostics/audit context.

The old owner is not evicted and no F.19 transfer occurs.

### Stale ownership after authorization

An authorization decision applies only to the exact `ExpectedOwnership` fence supplied in the request. If another ownership change occurs before transfer, F.19 rejects the stale expected fence.

## I/O and scaling boundary

The authorizer runs from the per-connection TCP goroutine, outside the world-owner tick. An integration may therefore perform bounded external authentication/policy I/O without adding filesystem or network latency to simulation.

S3-F.21 adds no command type, no new world-owner work, and no per-tick/per-snapshot hot-path cost.

## Authentication boundary

S3-F.21 is not a reconnect proof protocol.

The repository currently has no Client-visible reconnect credential, account session token, or protocol message that can independently prove possession of a prior Session. The production `worldd` wiring also continues to use the default ephemeral identity factory.

A future authenticated deployment can bind `CharacterIdentityFactory` and `CharacterTakeoverAuthorizer` to its account/session system. Until such a credential source exists, S3-F.21 intentionally avoids fabricating a wire-level security scheme.

## Preserved boundaries

S3-F.21 does not change:

- Protocol v6;
- Client code;
- GameV1 codec;
- SessionWelcome schema;
- UDP MTU 1200;
- WorldSnapshot chunking;
- Network LOD / transform batch max64;
- lifecycle desired-vs-known truth;
- Spawn/Despawn Confirm-after-`TrySend` semantics;
- lifecycle churn max 6,000/snapshot;
- Initial Vitals budgets;
- dirty gameplay Vitals max 4,000/tick;
- F.17 admission lease semantics;
- F.18 ownership fence semantics;
- F.19 ownership transfer transaction;
- F.20 old-peer eviction ordering;
- Character State schema/store/journal/autosave behavior;
- death/respawn/protection/penalty/outcome behavior;
- process-local ownership epoch semantics.

## Explicit non-goals

- no Client-visible reconnect credential or proof;
- no reconnect grace timer;
- no account/session implementation inside this repository;
- no device binding or IP-address authentication;
- no automatic retry/re-authorization after a stale transfer CAS;
- no takeover candidate reservation or rate limiting;
- no distributed/multi-worldd authorization or fencing;
- no durable ownership epoch;
- no Protocol v7 change;
- no Client change;
- no Go↔Godot end-to-end claim.

## Focused acceptance tests

Tests verify:

- missing authorizer rejects active takeover by default;
- inactive trusted first join succeeds without a takeover authorizer;
- authorizer receives candidate SessionID, exact trusted identity, exact expected ownership fence, and remote address;
- explicit authorizer approval preserves the existing F.20 successful takeover behavior;
- authorizer denial preserves both the stable unauthorized classification and upstream denial cause;
- denied/missing authorization never reaches F.19 transfer;
- denied/missing authorization does not evict the old peer;
- the old peer continues routing authoritative Actions after a denied candidate;
- existing F.20 transfer-failure and pre-publication eviction regressions remain covered.

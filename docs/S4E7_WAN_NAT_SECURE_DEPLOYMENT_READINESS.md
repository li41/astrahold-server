# S4-E.7 WAN / NAT & Secure Deployment Readiness

## Scope

S4-E.7 validates the existing Protocol v9 realtime endpoint policy against NAT-like source-port migration, spoof attempts, WAN-like impairment, and bounded long-session health. It deliberately does not add a new wire message or transport stack when the current authenticated behavior is sufficient.

## Endpoint migration policy

Production `tcpudp` already implements a fail-closed same-IP rebind rule.

For a Client→Server realtime datagram, the Server performs the security-sensitive work in this order:

1. parse the public Protocol v9 RoutingID and resolve the live peer generation;
2. authenticate the C2S HMAC-SHA256-128 tag using that peer's secret;
3. decode and validate the realtime `ClientMoveInput` frame;
4. require a strictly newer realtime sequence;
5. require the UDP source IP to match the connection's existing realtime IP;
6. only then publish the source port as the peer's current S2C endpoint;
7. enqueue the bounded movement intent for authoritative world processing.

The endpoint is therefore not a claim supplied by the Client. It is transport metadata observed on an already authenticated, fresh packet.

### Allowed

A NAT-like UDP source-port change is allowed when:

- the connection generation is still live;
- RoutingID resolves to that exact peer;
- C2S HMAC authenticates;
- the input sequence is strictly newer;
- the source IP is unchanged.

After acceptance, subsequent Server realtime output targets the new source port. EntityID and world ownership do not change.

### Rejected before endpoint mutation

- unknown / retired route;
- wrong HMAC;
- tampered datagram;
- malformed realtime payload;
- duplicate or stale sequence;
- different source IP.

Focused transport tests already cover fresh-sequence rebind, stale replay rejection without endpoint mutation, and rejection of a foreign-IP rebind without consuming the valid sequence that can subsequently be used by the same-IP endpoint.

## Takeover interaction

S4-E.6 remains the generation fence. Successful trusted takeover retires the old peer before the replacement `SessionWelcome` is emitted. Retirement removes the old secret and public route lookup entries for that exact peer generation.

Consequently, an old-generation authenticated packet cannot use S4-E.7 endpoint migration to regain a route after takeover: the old RoutingID no longer resolves to a live peer. Changing its UDP source port does not restore generation authority.

The paired production Godot S4-E.6 workflow replays a captured old-generation authenticated datagram from a separate UDP socket after takeover and requires the replacement generation to retain realtime control.

## WAN / long-session evidence

The paired Client S4-E.7 workflow runs production `cmd/worldd`, TLS 1.3 trusted bootstrap, and a real Godot Client through a transparent UDP relay. The relay changes only its Server-facing UDP mapping and injects deterministic:

- latency;
- jitter;
- packet loss;
- burst loss;
- reordering;
- duplication.

The scenario requires:

- an authenticated newer Client packet to migrate S2C traffic to the new NAT mapping;
- the old endpoint to become quiet after a bounded drain window;
- attacker traffic with tamper/stale inputs not to receive S2C traffic or reclaim the route;
- authoritative movement to continue making progress;
- PositionCorrection sequence lag to remain bounded and recover;
- WorldSnapshot / PositionCorrection age to remain bounded after impairment;
- a stop-input phase to converge instead of continuously drifting;
- no Reliable local EntityDespawn caused solely by snapshot loss.

## Secure deployment assessment

The current bounded production boundary remains:

- trusted reliable bootstrap: same-process TLS 1.3 ingress;
- trusted plaintext backend: literal loopback only;
- character credential map: static server-side SHA-256 entries;
- takeover permission: server-owned credential policy;
- realtime secret: fresh for each reliable connection generation;
- realtime retirement: immediate exact-peer token / route revocation;
- realtime payload: authenticated plaintext, not confidential.

### Periodic in-session realtime rekey

Not introduced in S4-E.7. Connection-generation rotation plus immediate retirement already handles reconnect/takeover revocation. Periodic rekey becomes justified if long-lived-secret policy, credential exposure assumptions, or operational session duration requires a shorter cryptographic lifetime than the connection lifetime.

### Explicit rebind challenge

Not introduced in S4-E.7. The existing rule requires possession of the current HMAC key and a strictly newer sequence before a same-IP port migration. A challenge would add round trips and state without currently demonstrated protection against an attacker who already possesses the live realtime secret.

### DTLS / QUIC

Not introduced in S4-E.7. Protocol v9 already supplies the required MVP authenticity/integrity and the production E2E tests the actual NAT/WAN behavior. DTLS or QUIC should be evaluated when confidentiality, connection migration across IP addresses, congestion-control requirements, or profiling data creates a concrete requirement that the current split TCP+UDP transport cannot meet cleanly.

## Protocol boundary

Protocol remains v9:

- public RoutingID remains 16 bytes;
- HMAC-SHA256 tag remains truncated to 128 bits;
- C2S / S2C HMAC domains remain separate;
- UDP MTU remains 1200;
- `WorldSnapshot` max chunk remains 43 entities;
- gameplay message layouts are unchanged;
- snapshot absence remains distinct from Reliable lifecycle.

# S4-E.6 Realtime Key Lifecycle

## Scope

S4-E.6 makes the existing connection-generation realtime credential lifecycle explicit and fail-closed without changing Protocol v9 or GameV1 payloads.

Each accepted TCP connection already receives a fresh 128-bit cryptographic realtime token from `crypto/rand`. Protocol v9 derives the public UDP routing ID one-way from that secret and authenticates every datagram with a direction-separated truncated HMAC-SHA256 tag. A reconnect or trusted active takeover therefore creates a new realtime key generation naturally.

## Registration fence

Realtime peer publication is now atomic across both lookup maps:

- the secret-keyed peer map used by existing transport/takeover bookkeeping;
- the public routing-ID map used by UDP lookup.

If either the freshly generated token or its derived route collides with an existing registration, the candidate connection fails closed with `ErrRealtimeCredentialCollision`. No existing entry is overwritten and no partial registration is published.

The collision probability is intentionally not used as a reason to accept fail-open map replacement behavior. A collision is treated as a connection-local transport fault before world ownership is acquired.

## Revocation fence

Peer shutdown first clears `ready`, then atomically removes the secret token and public route only when each map still points to that exact peer. This preserves takeover/close race safety: a stale generation cannot erase a newer generation's lookup.

Trusted active takeover already commits authoritative world ownership and retires the old transport before sending the new `SessionWelcome`. Therefore the old realtime route is revoked before the replacement client learns and activates its fresh generation.

## Protocol boundary

Protocol remains v9:

- ASTU header: 24 bytes;
- public one-way routing ID: 16 bytes in ASTU bytes 8..24;
- authentication trailer: 16-byte truncated HMAC-SHA256;
- C2S/S2C domain separation unchanged;
- UDP MTU remains 1200;
- max snapshot chunk remains 43 entities;
- GameV1 layouts and message type values are unchanged.

This stage does not add in-session periodic key rotation or dual-key grace/acknowledgement. The bounded MVP contract is connection-generation rotation: reconnect/takeover receives a fresh key, and retiring the prior peer immediately revokes its route.

## Validation

Focused transport tests cover:

- token collision fails closed without replacing the original peer;
- public route collision fails closed without partial publication;
- revocation removes both lookup handles;
- stale revocation cannot erase a replacement generation;
- `closePeer` clears readiness and removes both lookup handles before returning.

The paired Client S4-E.6 production E2E captures only public route IDs at a transparent UDP relay, verifies takeover rotates the route, replays a captured old-generation authenticated packet after takeover, and requires the new authoritative Godot owner to continue receiving correction/snapshot traffic and movement without endpoint theft.

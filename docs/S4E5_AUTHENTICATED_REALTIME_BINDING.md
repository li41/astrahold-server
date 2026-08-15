# S4-E.5 Authenticated Realtime Binding

S4-E.5 hardens the existing GameV1 UDP realtime path without changing gameplay authority, snapshot semantics, Network LOD, or the MTU ceiling.

## Root security issue

Before this stage, every ASTU datagram carried the 128-bit `RealtimeToken` directly in header bytes 8..24. The Server used that bearer value as the peer lookup key. A passive observer that saw one UDP datagram therefore learned the credential needed to manufacture later datagrams for that session.

The UDP loop also rebound the peer's same-IP UDP port before the world-runtime stale-input sequence check ran. A replayed packet could therefore redirect outbound realtime traffic even when its movement command was subsequently rejected as stale.

## Protocol v9 ASTU contract

Protocol v9 changes only the realtime ASTU wrapper; GameV1 message payloads and MessageType values are unchanged.

The 24-byte ASTU header remains the same size:

- bytes 0..4: `ASTU` magic
- bytes 4..6: Protocol Version 9
- bytes 6..8: header size 24
- bytes 8..24: public 128-bit routing ID = truncated SHA-256 over a domain separator plus the session realtime secret

The bearer realtime token is no longer transmitted in UDP packets.

A 16-byte authentication trailer is appended after the ASTR frame. It is HMAC-SHA256 truncated to 128 bits over the full ASTU header plus ASTR frame/payload. The HMAC key is the realtime token delivered in `SessionWelcome`.

C2S and S2C use different HMAC domains, so a captured Server snapshot/correction cannot be reflected back as a valid Client input.

The maximum 43-entity GameV1 snapshot remains valid: 24 ASTU + 28 ASTR + 1132 GameV1 snapshot + 16 auth tag = exactly 1200 bytes.

## Endpoint rebind / replay rule

The Server resolves a peer by the public routing ID, authenticates the C2S HMAC, validates realtime delivery, and then atomically applies a fresh-sequence gate with same-IP endpoint binding.

Only a strictly newer authenticated input sequence can establish or change the UDP port. Replayed/stale packets are rejected before endpoint mutation. Legitimate NAT port rebinding remains supported when the new datagram is authenticated, same-IP, and fresh.

The world-runtime sequence check remains in place as the authoritative gameplay guard; the transport gate is specifically an anti-replay prerequisite for endpoint mutation.

## Preserved boundaries

- Go Server remains authoritative.
- MessageType values are unchanged; SiegeMatchState remains 106.
- GameV1 compact payload layouts are unchanged.
- `MaxSnapshotEntitiesPerChunk` remains 43.
- `MaxDatagramSize` remains 1200.
- lifecycle / Spawn / Despawn / Vitals / Dynamic / Siege reliable semantics are unchanged.
- no delta compression, quantization, or actor splitting is introduced.
- UDP contents are authenticated/integrity-protected but not encrypted by this stage.
- the realtime token must still be delivered over a protected bootstrap such as the S4-E.4 TLS 1.3 trusted ingress for production secrecy.

## Validation

Focused tests cover bearer-token non-disclosure, routing-ID derivation, HMAC tamper rejection, wrong-token rejection, S2C-to-C2S reflection rejection, exact 1200-byte max snapshot encoding, zero-allocation reusable server encoding, mailbox ownership, and stale replay prevention before endpoint rebinding.

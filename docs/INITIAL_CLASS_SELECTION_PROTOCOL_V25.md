# Astrahold Initial Class Selection Protocol v25

## Status

This document is the current Server contract for first-time profession selection.

The Server is the only gameplay authority. The Client may request one canonical `ClassID`; it never assigns, persists, normalizes, or infers profession truth locally.

This contract implements only the one-way transition:

```text
unassigned ClassID (empty)
    -> one canonical ClassID
```

It does not define class change, respec, reset, or a second assignment path.

## Canonical ClassID source

The canonical vocabulary remains `internal/classid` and `docs/CLASS_ID_CONTRACT_V1.md`:

- `class_oathguard`
- `class_breaker`
- `class_ranger`
- `class_starfire_mage`
- `class_oathhealer`
- `class_shadowblade`

Empty `ClassID` means unassigned. It is not a seventh class.

## Message allocation

All three v25 class-selection messages use `ReliableOrdered` delivery.

| Direction | MessageType | Message | Purpose |
|---|---:|---|---|
| Client -> Server | `9` | `ClientInitialClassSelection` | request one canonical ClassID |
| Server -> Client | `115` | `CharacterClassState` | complete authoritative current class identity |
| Server -> Client | `116` | `InitialClassSelectionResult` | correlate one selection request with the Server decision |

### ClientInitialClassSelection

JSON v1 payload:

```json
{"class_id":"class_ranger"}
```

`class_id` must be non-empty at the transport/gateway boundary. Canonical vocabulary validation remains a world-owner gameplay decision; the gateway must not silently normalize case, whitespace, aliases, UI labels, or future-looking values into another ClassID.

The client action sequence comes only from the Envelope/Frame sequence, not from this payload.

### CharacterClassState

JSON v1 payload when assigned:

```json
{"class_id":"class_ranger"}
```

JSON v1 payload when unassigned:

```json
{"class_id":""}
```

This is complete resendable profession identity truth for a trusted durable character. A reconnect must use this state rather than reconstructing ClassID from previous UI state, equipment, model, animation, or cached selection results.

Ephemeral development identities do not receive a durable class bootstrap because they have no durable profession identity.

### InitialClassSelectionResult

Committed JSON v1 example:

```json
{
  "client_action_sequence": 42,
  "class_id": "class_ranger",
  "outcome": "committed"
}
```

Rejected JSON v1 example:

```json
{
  "client_action_sequence": 42,
  "class_id": "",
  "outcome": "rejected",
  "reason": "invalid_class"
}
```

Fields:

- `client_action_sequence`: the accepted ReliableOrdered Envelope sequence for this request.
- `class_id`: authoritative live ClassID after the decision. It may therefore be empty for an unassigned character or contain the already-assigned class on a rejected repeat request.
- `outcome`: `committed` or `rejected`.
- `reason`: omitted for committed results; required by Server semantics for rejected results.

Defined rejection reasons:

- `invalid_class`
- `already_assigned`
- `assignment_pending`
- `equipment_illegal`
- `persistence_unavailable`
- `trusted_identity_required`
- `server_rejected`

Clients must treat reason strings as presentation/diagnostic input only. They do not grant permission to mutate profession state locally.

## Authoritative commit boundary

A valid Client request does not become gameplay truth when it reaches the gateway or runtime queue.

The success path is:

```text
ClientInitialClassSelection intent
    -> trusted SessionOwnershipFence validation
    -> world-owner canonical ClassID / current state / equipment validation
    -> immutable character-state save target
    -> persistence journal fsync
    -> Store compare-and-swap/application
    -> durable checkpoint advancement
    -> persistence completion acknowledgement
    -> world-owner live character.State.ClassID commit
    -> CharacterClassState
    -> InitialClassSelectionResult(outcome=committed)
```

A `committed` result must never be emitted before both durable checkpoint advancement and the world-owner live ClassID commit have succeeded.

World tick performs no blocking persistence I/O. Persistence completion is returned asynchronously to the world owner.

## Failure and retry semantics

Rejected requests do not mutate ClassID and do not create a successful assignment save.

A temporary Reliable transport backpressure condition may delay `CharacterClassState` and `InitialClassSelectionResult`, but it must not:

- rerun persistence;
- reapply ClassID mutation;
- convert an uncommitted request into success;
- allow the Client to guess success from local UI state.

Class feedback uses a bounded process-local FIFO. If that bounded feedback contract cannot be maintained, the source connection is closed fail-closed. Durable ClassID truth remains recoverable on reconnect through `CharacterClassState`.

## Ownership takeover

Selection correlation is fenced to the exact trusted ownership epoch that submitted the request.

If a Server-authorized session takeover happens while a durable selection is pending:

- durable persistence may still complete for the same CharacterID;
- the world owner may still commit the durable ClassID;
- the replacement/current owner receives authoritative `CharacterClassState`;
- the replacement does not receive the previous owner's correlated `InitialClassSelectionResult`;
- the stale owner cannot use its old ownership fence to submit a new profession mutation.

This keeps request correlation session-scoped while profession truth remains CharacterID-scoped.

## BrowserWS / ephemeral development identity

BrowserWS development sessions use ephemeral CharacterIdentity and are not a durable character authority path.

A class-selection intent from such a session is rejected with:

```text
outcome = rejected
reason  = trusted_identity_required
```

No durable ClassID is written and no live profession mutation occurs.

## Client obligations

The Client may:

- show the six canonical choices;
- send `ClientInitialClassSelection`;
- display pending/rejected/committed feedback;
- update presentation when authoritative `CharacterClassState` arrives.

The Client must not:

- write gameplay ClassID locally as truth;
- treat button click or request enqueue as success;
- infer ClassID from `ArchetypeID`, mesh, equipment, animation, UI label, or cached result;
- implement class-change authority;
- bypass Server rejection or durability semantics.

# Inventory Vertical Slice — Protocol v14 Server Checkpoint

Protocol v14 is adopted only because this branch now production-emits Reliable `InventorySnapshot` (message 110) and the coordinated Unreal client branch decodes the same authoritative contract.

Current server scope:

- world-owner inventory bootstrap keyed by durable character identity
- bounded authoritative inventory from the existing `internal/inventory` package
- deterministic starter snapshot
- Reliable Ordered production delivery through the existing session connection
- pending retry on reliable backpressure
- focused join replication regression test

The network layer does not mutate inventory or world state. Inventory remains Server truth.

Persistence, equipment, item drops, trade, warehouse, auction, weight, grid packing, and bag expansion remain out of scope for this checkpoint.

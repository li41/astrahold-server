package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

const mapTransferTestSHA = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

var (
	map0TestWorld = characterstate.WorldRef{MapID: "map0", WorldID: "gm-room", Revision: "map0-test", GameplaySHA256: mapTransferTestSHA}
	map1TestWorld = characterstate.WorldRef{MapID: "map1", WorldID: "castle-sandbox", Revision: "map1-test", GameplaySHA256: mapTransferTestSHA}
)

func TestMap0ExitWritesMap1SnapshotThenRemovesSource(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(4)
	rt, identity := makeMap0TransferRuntime(t, outbox)
	destination := world.Transform{Position: world.Position{X: 12, Y: 0, Z: -4, Layer: 0}, Yaw: 1.25}
	if err := rt.EnqueueMapExit(1, MapExitRequest{DestinationWorld: map1TestWorld, DestinationTransform: destination}); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 { t.Fatalf("exit errors=%#v", report.CommandErrors) }
	if report.Metrics.CharacterStateSaveIntentsEnqueued != 1 { t.Fatalf("save intents=%d", report.Metrics.CharacterStateSaveIntentsEnqueued) }
	pending := outbox.Pending(1)
	if len(pending) != 1 { t.Fatalf("pending=%#v", pending) }
	got := pending[0]
	if got.Identity != identity || got.Snapshot.World != map1TestWorld || got.Snapshot.Position != destination.Position || got.Snapshot.Yaw != destination.Yaw { t.Fatalf("transfer snapshot=%#v", got.Snapshot) }
	stacks, err := got.Snapshot.Inventory.Stacks(); if err != nil { t.Fatal(err) }
	if quantityOfStack(stacks, "item_training_blade") != 1 { t.Fatalf("training blade lost during transfer stacks=%#v", stacks) }
	if _, ok := rt.sessions.Get(1); ok { t.Fatal("source session remained after successful map exit") }
	if _, ok := rt.world.Entity(1); ok { t.Fatal("source entity remained after successful map exit") }
}

func TestMap0ExitRestrictedItemFailsWithoutMutation(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(4)
	rt, identity := makeMap0TransferRuntime(t, outbox)
	before := rt.inventories[identity.ID].Snapshot()
	if err := rt.EnqueueMapExit(1, MapExitRequest{DestinationWorld: map1TestWorld, RestrictedItemArchetypeIDs: []string{"item_training_blade"}}); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrMapExitRestrictedItem) { t.Fatalf("errors=%#v", report.CommandErrors) }
	if outbox.Depth() != 0 { t.Fatalf("blocked exit enqueued save depth=%d", outbox.Depth()) }
	if _, ok := rt.sessions.Get(1); !ok { t.Fatal("blocked exit removed source session") }
	if _, ok := rt.world.Entity(1); !ok { t.Fatal("blocked exit removed source entity") }
	after := rt.inventories[identity.ID].Snapshot()
	if len(before) != len(after) { t.Fatalf("inventory changed before=%#v after=%#v", before, after) }
	for i := range before { if before[i] != after[i] { t.Fatalf("inventory changed before=%#v after=%#v", before, after) } }
}

func TestMap0ExitRestrictedEquippedItemCannotBypassPolicy(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(4)
	rt, identity := makeMap0TransferRuntime(t, outbox)
	inv := rt.inventories[identity.ID]
	if err := inv.EquipMainHand("item_training_blade"); err != nil { t.Fatal(err) }
	if err := rt.EnqueueMapExit(1, MapExitRequest{DestinationWorld: map1TestWorld, RestrictedItemArchetypeIDs: []string{"item_training_blade"}}); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrMapExitRestrictedItem) { t.Fatalf("errors=%#v", report.CommandErrors) }
	if inv.MainHand() != "item_training_blade" || outbox.Depth() != 0 { t.Fatalf("blocked equipped item mutated main_hand=%q depth=%d", inv.MainHand(), outbox.Depth()) }
}

func TestMap0ExitOutboxFailureRollsBackSessionRegistry(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(1)
	blocker, _ := characteridentity.NewTrusted("character:map-exit-blocker")
	blockerSnapshot := characterstate.Snapshot{World: map0TestWorld, HP: 1000, MaxHP: 1000, MP: 100, MaxMP: 100, Position: world.Position{Layer: 0}, Inventory: characterstate.InventoryState{Initialized: true}, PrimaryStats: characterstats.DefaultPrimary()}
	if _, err := outbox.Enqueue(blocker, blockerSnapshot); err != nil { t.Fatal(err) }
	rt, _ := makeMap0TransferRuntime(t, outbox)
	if err := rt.EnqueueMapExit(1, MapExitRequest{DestinationWorld: map1TestWorld}); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, characterstate.ErrSaveOutboxFull) { t.Fatalf("errors=%#v", report.CommandErrors) }
	if _, ok := rt.sessions.Get(1); !ok { t.Fatal("save failure did not restore source session") }
	if _, ok := rt.world.Entity(1); !ok { t.Fatal("save failure removed source entity") }
	if outbox.Depth() != 1 || outbox.Pending(1)[0].Identity != blocker { t.Fatalf("outbox=%#v", outbox.Pending(0)) }
}

func TestMapExitRequiresIsolatedSource(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(4)
	rt, _ := makeMapTransferRuntime(t, outbox, map1TestWorld)
	if err := rt.EnqueueMapExit(1, MapExitRequest{DestinationWorld: map0TestWorld}); err != nil { t.Fatal(err) }
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrMapExitSourceNotIsolated) { t.Fatalf("errors=%#v", report.CommandErrors) }
	if _, ok := rt.sessions.Get(1); !ok { t.Fatal("non-isolated source was mutated") }
}

func TestSuccessfulMap0ExitSnapshotRestoresIntoMap1Runtime(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(4)
	source, identity := makeMap0TransferRuntime(t, outbox)
	destination := world.Transform{Position: world.Position{X: -3, Y: 0, Z: 8, Layer: 0}, Yaw: 0.5}
	if err := source.EnqueueMapExit(1, MapExitRequest{DestinationWorld: map1TestWorld, DestinationTransform: destination}); err != nil { t.Fatal(err) }
	if report := source.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("source exit errors=%#v", report.CommandErrors) }
	pending := outbox.Pending(1)
	if len(pending) != 1 { t.Fatalf("pending=%#v", pending) }
	store, err := characterstate.Open(t.TempDir()); if err != nil { t.Fatal(err) }
	record, err := store.Save(identity, 0, pending[0].Snapshot); if err != nil { t.Fatal(err) }
	restore := CharacterRestoreFromRecord(record)
	if restore.MapID != "map1" { t.Fatalf("restore map=%q", restore.MapID) }

	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	destinationRuntime := New(sim, DefaultConfig(), WithCharacterStateOutbox(nil, map1TestWorld))
	sess, err := session.NewWithCharacterIdentity(2, 1, identity, 64, session.NewQueueConnection(32, 32)); if err != nil { t.Fatal(err) }
	if err := destinationRuntime.EnqueueJoin(JoinRequest{Session: sess, Entity: world.EntityState{ID: 1, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5, Restore: &restore}); err != nil { t.Fatal(err) }
	report := destinationRuntime.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 { t.Fatalf("restore errors=%#v", report.CommandErrors) }
	entity, ok := destinationRuntime.world.Entity(1)
	if !ok || entity.Transform != destination { t.Fatalf("restored entity=%#v ok=%v", entity, ok) }
}

func TestRestrictedItemScanIncludesInstancesAndEquippedInstances(t *testing.T) {
	inv := inventory.New(8)
	instance := iteminstance.Instance{ID: "instance_restricted", ItemArchetypeID: "item_restricted_instance"}
	if err := inv.AddInstance(instance); err != nil { t.Fatal(err) }
	got, err := firstRestrictedItemArchetype(inv, []string{"item_restricted_instance"})
	if err != nil || got != "item_restricted_instance" { t.Fatalf("unequipped instance got=%q err=%v", got, err) }
	if err := inv.EquipInstance(inventory.SlotMainHand, instance.ID); err != nil { t.Fatal(err) }
	got, err = firstRestrictedItemArchetype(inv, []string{"item_restricted_instance"})
	if err != nil || got != "item_restricted_instance" { t.Fatalf("equipped instance got=%q err=%v", got, err) }
}

func makeMap0TransferRuntime(t *testing.T, outbox *characterstate.Outbox) (*Runtime, characteridentity.Binding) {
	t.Helper()
	return makeMapTransferRuntime(t, outbox, map0TestWorld)
}

func makeMapTransferRuntime(t *testing.T, outbox *characterstate.Outbox, worldRef characterstate.WorldRef) (*Runtime, characteridentity.Binding) {
	t.Helper()
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	if err := sim.Spawn(world.EntityState{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 1, Z: 1, Layer: 0}}}, 6, 0.35, 0.5); err != nil { t.Fatal(err) }
	rt := New(sim, DefaultConfig(), WithCharacterStateOutbox(outbox, worldRef))
	identity, err := characteridentity.NewTrusted("character:map-transfer"); if err != nil { t.Fatal(err) }
	sess, err := session.NewWithCharacterIdentity(1, 1, identity, 64, session.NewQueueConnection(32, 32)); if err != nil { t.Fatal(err) }
	if err := rt.EnqueueRegister(sess); err != nil { t.Fatal(err) }
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("register errors=%#v", report.CommandErrors) }
	return rt, identity
}

func quantityOfStack(stacks []characterstate.InventoryStack, id string) uint32 {
	for _, stack := range stacks { if stack.ItemArchetypeID == id { return stack.Quantity } }
	return 0
}

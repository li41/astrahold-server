package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestAstraholdTeleportRuneAccountOneTransfersToGMRoomCenterFacingStockKeeper(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(4)
	rt, s, identity, connection := makeTeleportRuneRuntime(t, outbox, map1TestWorld, "1", true)

	if err := rt.EnqueueUseItem(s.ID, 1, protocol.ClientUseItem{ItemArchetypeID: AstraholdTeleportRuneItemArchetypeID}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("teleport errors=%#v", report.CommandErrors)
	}
	if report.Metrics.CharacterStateSaveIntentsEnqueued != 1 {
		t.Fatalf("save intents=%d", report.Metrics.CharacterStateSaveIntentsEnqueued)
	}
	result := requireSingleItemUseResult(t, connection)
	if result.Outcome != protocol.ItemUseOutcomeUsed || result.ItemArchetypeID != AstraholdTeleportRuneItemArchetypeID || result.ClientActionSequence != 1 {
		t.Fatalf("item-use result=%#v", result)
	}

	pending := outbox.Pending(1)
	if len(pending) != 1 || pending[0].Identity != identity {
		t.Fatalf("pending=%#v", pending)
	}
	target := gameplayworld.GMRoomTransferTarget()
	wantWorld := characterstate.WorldRef{MapID: string(target.MapID), WorldID: target.WorldID, Revision: target.Revision, GameplaySHA256: target.GameplaySHA256}
	got := pending[0].Snapshot
	if got.World != wantWorld || got.Position != target.Transform.Position || got.Yaw != target.Transform.Yaw {
		t.Fatalf("gm-room snapshot=%#v target=%#v", got, target)
	}
	stacks, err := got.Inventory.Stacks()
	if err != nil {
		t.Fatal(err)
	}
	if quantityOfStack(stacks, AstraholdTeleportRuneItemArchetypeID) != 0 {
		t.Fatalf("rune persisted after committed use stacks=%#v", stacks)
	}
	if got := rt.inventories[identity.ID].Quantity(AstraholdTeleportRuneItemArchetypeID); got != 0 {
		t.Fatalf("live rune quantity=%d", got)
	}
	if _, ok := rt.sessions.Get(s.ID); ok {
		t.Fatal("source session remained after rune transfer")
	}
	if _, ok := rt.world.Entity(s.EntityID); ok {
		t.Fatal("source entity remained after rune transfer")
	}
}

func TestAstraholdTeleportRuneRejectsNonAccountOneWithoutConsumption(t *testing.T) {
	for _, subject := range []string{"", "2"} {
		t.Run("subject_"+subject, func(t *testing.T) {
			outbox, _ := characterstate.NewOutbox(4)
			rt, s, identity, connection := makeTeleportRuneRuntime(t, outbox, map1TestWorld, subject, true)
			if err := rt.EnqueueUseItem(s.ID, 1, protocol.ClientUseItem{ItemArchetypeID: AstraholdTeleportRuneItemArchetypeID}); err != nil {
				t.Fatal(err)
			}
			report := rt.Step(2, 50*time.Millisecond)
			if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrAstraholdTeleportRuneAccountDenied) {
				t.Fatalf("errors=%#v", report.CommandErrors)
			}
			result := requireSingleItemUseResult(t, connection)
			if result.Outcome != protocol.ItemUseOutcomeRejected || result.Reason != protocol.ItemUseRejectionServerRejected {
				t.Fatalf("result=%#v", result)
			}
			if outbox.Depth() != 0 || rt.inventories[identity.ID].Quantity(AstraholdTeleportRuneItemArchetypeID) != 1 {
				t.Fatalf("rejected use mutated state depth=%d inventory=%#v", outbox.Depth(), rt.inventories[identity.ID].Snapshot())
			}
			if _, ok := rt.sessions.Get(s.ID); !ok {
				t.Fatal("rejected use removed session")
			}
		})
	}
}

func TestAstraholdTeleportRuneMissingItemRejectsAsMissingItem(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(4)
	rt, s, _, connection := makeTeleportRuneRuntime(t, outbox, map1TestWorld, "1", false)
	if err := rt.EnqueueUseItem(s.ID, 1, protocol.ClientUseItem{ItemArchetypeID: AstraholdTeleportRuneItemArchetypeID}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, inventory.ErrInsufficient) {
		t.Fatalf("errors=%#v", report.CommandErrors)
	}
	result := requireSingleItemUseResult(t, connection)
	if result.Reason != protocol.ItemUseRejectionMissingItem || outbox.Depth() != 0 {
		t.Fatalf("result=%#v depth=%d", result, outbox.Depth())
	}
}

func TestAstraholdTeleportRuneOutboxFailureKeepsRuneAndSource(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(1)
	blocker, _ := characteridentity.NewTrusted("character:rune-outbox-blocker")
	blockerSnapshot := characterstate.Snapshot{
		World: map1TestWorld,
		HP: 1000, MaxHP: 1000, MP: 100, MaxMP: 100,
		Position: world.Position{Layer: 0},
		Inventory: characterstate.InventoryState{Initialized: true},
		PrimaryStats: characterstats.DefaultPrimary(),
	}
	if _, err := outbox.Enqueue(blocker, blockerSnapshot); err != nil {
		t.Fatal(err)
	}
	rt, s, identity, connection := makeTeleportRuneRuntime(t, outbox, map1TestWorld, "1", true)
	if err := rt.EnqueueUseItem(s.ID, 1, protocol.ClientUseItem{ItemArchetypeID: AstraholdTeleportRuneItemArchetypeID}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, characterstate.ErrSaveOutboxFull) {
		t.Fatalf("errors=%#v", report.CommandErrors)
	}
	result := requireSingleItemUseResult(t, connection)
	if result.Outcome != protocol.ItemUseOutcomeRejected || result.Reason != protocol.ItemUseRejectionServerRejected {
		t.Fatalf("result=%#v", result)
	}
	if rt.inventories[identity.ID].Quantity(AstraholdTeleportRuneItemArchetypeID) != 1 {
		t.Fatalf("rune consumed after failed save inventory=%#v", rt.inventories[identity.ID].Snapshot())
	}
	if _, ok := rt.sessions.Get(s.ID); !ok {
		t.Fatal("save failure did not restore source session")
	}
	if _, ok := rt.world.Entity(s.EntityID); !ok {
		t.Fatal("save failure removed source entity")
	}
	if outbox.Depth() != 1 || outbox.Pending(1)[0].Identity != blocker {
		t.Fatalf("outbox=%#v", outbox.Pending(0))
	}
}

func TestAstraholdTeleportRuneRejectsUseInsideGMRoom(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(4)
	rt, s, identity, connection := makeTeleportRuneRuntime(t, outbox, map0TestWorld, "1", true)
	if err := rt.EnqueueUseItem(s.ID, 1, protocol.ClientUseItem{ItemArchetypeID: AstraholdTeleportRuneItemArchetypeID}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrAstraholdTeleportRuneAlreadyInGMRoom) {
		t.Fatalf("errors=%#v", report.CommandErrors)
	}
	requireSingleItemUseResult(t, connection)
	if outbox.Depth() != 0 || rt.inventories[identity.ID].Quantity(AstraholdTeleportRuneItemArchetypeID) != 1 {
		t.Fatalf("gm-room rejection mutated state depth=%d inventory=%#v", outbox.Depth(), rt.inventories[identity.ID].Snapshot())
	}
}

func TestAstraholdTeleportRuneSnapshotRestoresAtGMRoomTarget(t *testing.T) {
	outbox, _ := characterstate.NewOutbox(4)
	source, s, identity, _ := makeTeleportRuneRuntime(t, outbox, map1TestWorld, "1", true)
	if err := source.EnqueueUseItem(s.ID, 1, protocol.ClientUseItem{ItemArchetypeID: AstraholdTeleportRuneItemArchetypeID}); err != nil {
		t.Fatal(err)
	}
	if report := source.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("source errors=%#v", report.CommandErrors)
	}
	pending := outbox.Pending(1)
	if len(pending) != 1 {
		t.Fatalf("pending=%#v", pending)
	}
	store, err := characterstate.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	record, err := store.Save(identity, 0, pending[0].Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	restore := CharacterRestoreFromRecord(record)
	target := gameplayworld.GMRoomTransferTarget()
	if restore.MapID != gameplayworld.MapIDGMRoom || restore.Transform != target.Transform {
		t.Fatalf("restore=%#v target=%#v", restore, target)
	}

	destinationWorld := characterstate.WorldRef{MapID: string(target.MapID), WorldID: target.WorldID, Revision: target.Revision, GameplaySHA256: target.GameplaySHA256}
	nav := navigation.Plane{MinX: -8.5, MaxX: 8.5, MinZ: -7.5, MaxZ: 7.5, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	destination := New(sim, DefaultConfig(), WithCharacterStateOutbox(nil, destinationWorld))
	connection := session.NewQueueConnection(32, 32)
	destinationSession, err := session.NewWithCharacterIdentity(2, 1, identity, 64, connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := destination.EnqueueJoin(JoinRequest{
		Session: destinationSession,
		Entity: world.EntityState{ID: 1, Kind: world.EntityPlayer},
		Speed: 6, Radius: 0.35, MaxStepHeight: 0.5,
		Restore: &restore,
	}); err != nil {
		t.Fatal(err)
	}
	if report := destination.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("destination restore errors=%#v", report.CommandErrors)
	}
	entity, ok := destination.world.Entity(1)
	if !ok || entity.Transform != target.Transform {
		t.Fatalf("restored entity=%#v ok=%v target=%#v", entity, ok, target.Transform)
	}
	if got := destination.inventories[identity.ID].Quantity(AstraholdTeleportRuneItemArchetypeID); got != 0 {
		t.Fatalf("restored rune quantity=%d", got)
	}
}

func makeTeleportRuneRuntime(
	t *testing.T,
	outbox *characterstate.Outbox,
	worldRef characterstate.WorldRef,
	authenticationSubject string,
	includeRune bool,
) (*Runtime, *session.Session, characteridentity.Binding, *session.QueueConnection) {
	t.Helper()
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	if err := sim.Spawn(world.EntityState{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 1, Z: 1, Layer: 0}}}, 6, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	config := DefaultConfig()
	if includeRune {
		config.StarterInventory = append(config.StarterInventory, inventory.Stack{ArchetypeID: AstraholdTeleportRuneItemArchetypeID, Quantity: 1})
	}
	rt := New(sim, config, WithCharacterStateOutbox(outbox, worldRef))
	identity, err := characteridentity.NewTrusted("character:teleport-rune")
	if err != nil {
		t.Fatal(err)
	}
	connection := session.NewQueueConnection(64, 32)
	sess, err := session.NewWithCharacterIdentityAndAuthenticationSubject(1, 1, identity, authenticationSubject, 64, connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueRegister(sess); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("register errors=%#v", report.CommandErrors)
	}
	drainReliable(connection)
	return rt, sess, identity, connection
}

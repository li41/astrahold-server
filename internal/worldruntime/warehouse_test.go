package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

type warehouseTestDynamicWorld struct {
	blocker gameplayworld.Blocker
	enabled bool
}

func newWarehouseTestDynamicWorld() *warehouseTestDynamicWorld {
	return &warehouseTestDynamicWorld{
		blocker: gameplayworld.Blocker{
			ID: warehouseKeeperBlockerID, Layer: 0,
			Bounds: gameplayworld.BoundsXZ{MinX: 0.5, MaxX: 1.5, MinZ: 0.5, MaxZ: 1.5},
			MinY: 0, MaxY: 2, BlocksMovement: true, Enabled: true,
		},
		enabled: true,
	}
}

func (d *warehouseTestDynamicWorld) SetBlockerEnabled(id string, enabled bool) error {
	if id != d.blocker.ID { return errors.New("warehouse test: blocker not found") }
	d.enabled = enabled
	return nil
}
func (d *warehouseTestDynamicWorld) BlockerEnabled(id string) (bool, error) {
	if id != d.blocker.ID { return false, errors.New("warehouse test: blocker not found") }
	return d.enabled, nil
}
func (d *warehouseTestDynamicWorld) BlockerDefinition(id string) (gameplayworld.Blocker, error) {
	if id != d.blocker.ID { return gameplayworld.Blocker{}, errors.New("warehouse test: blocker not found") }
	return d.blocker, nil
}
func (d *warehouseTestDynamicWorld) BlockerStates() []gameplayworld.BlockerState {
	return []gameplayworld.BlockerState{{ID: d.blocker.ID, Enabled: d.enabled}}
}
func (*warehouseTestDynamicWorld) HasLineOfSight(world.Position, world.Position) bool { return true }
func (*warehouseTestDynamicWorld) HasLineOfSightIgnoringBlocker(world.Position, world.Position, string) bool { return true }

func TestGMWarehouseSubjectOneWithdrawsInfiniteEnhancementScrollsExactlyOncePerSequence(t *testing.T) {
	rt, s, identity, connection := makeTeleportRuneRuntime(t, nil, map0TestWorld, gmWarehouseSubject, false)
	rt.dynamic = newWarehouseTestDynamicWorld()

	withdraw := protocol.ClientWarehouseCommand{
		Operation: protocol.WarehouseOperationWithdrawGM,
		ItemArchetypeID: WeaponEnhancementScrollItemArchetypeID,
		Quantity: 2,
	}
	if err := rt.queue.tryPush(useActionCommand{sessionID: s.ID, sequence: 1, warehouse: &withdraw}); err != nil { t.Fatal(err) }
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("first withdraw errors=%#v", report.CommandErrors) }
	if got := rt.inventories[identity.ID].Quantity(WeaponEnhancementScrollItemArchetypeID); got != 2 { t.Fatalf("first withdraw quantity=%d want=2", got) }
	result := requireWarehouseResult(t, drainWarehouseMessages(connection))
	if result.Outcome != protocol.WarehouseOutcomeWithdrawn || result.Quantity != 2 { t.Fatalf("first result=%#v", result) }

	// Reliable replay must never mint a second copy from the infinite source.
	if err := rt.queue.tryPush(useActionCommand{sessionID: s.ID, sequence: 1, warehouse: &withdraw}); err != nil { t.Fatal(err) }
	replay := rt.Step(3, 50*time.Millisecond)
	if len(replay.CommandErrors) != 1 || !errors.Is(replay.CommandErrors[0].Err, session.ErrStaleAction) { t.Fatalf("replay errors=%#v", replay.CommandErrors) }
	if got := rt.inventories[identity.ID].Quantity(WeaponEnhancementScrollItemArchetypeID); got != 2 { t.Fatalf("replay minted quantity=%d", got) }
	drainWarehouseMessages(connection)

	// A new sequence may withdraw the same fixed GM source again; the source is never decremented.
	if err := rt.queue.tryPush(useActionCommand{sessionID: s.ID, sequence: 2, warehouse: &withdraw}); err != nil { t.Fatal(err) }
	if report := rt.Step(4, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("second withdraw errors=%#v", report.CommandErrors) }
	if got := rt.inventories[identity.ID].Quantity(WeaponEnhancementScrollItemArchetypeID); got != 4 { t.Fatalf("second withdraw quantity=%d want=4", got) }
	drainWarehouseMessages(connection)

	open := protocol.ClientWarehouseCommand{Operation: protocol.WarehouseOperationOpenGM}
	if err := rt.queue.tryPush(useActionCommand{sessionID: s.ID, sequence: 3, warehouse: &open}); err != nil { t.Fatal(err) }
	if report := rt.Step(5, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("open errors=%#v", report.CommandErrors) }
	messages := drainWarehouseMessages(connection)
	snapshot := requireWarehouseSnapshot(t, messages)
	if len(snapshot.Items) != 2 || snapshot.Items[0].Quantity != 0 || snapshot.Items[1].Quantity != 0 { t.Fatalf("GM snapshot=%#v", snapshot) }
	if snapshot.Items[0].ItemArchetypeID != WeaponEnhancementScrollItemArchetypeID || snapshot.Items[1].ItemArchetypeID != ArmorEnhancementScrollItemArchetypeID { t.Fatalf("GM catalog=%#v", snapshot.Items) }
}

func TestGMWarehouseRejectsNonSubjectOneWithoutInventoryMutation(t *testing.T) {
	rt, s, identity, connection := makeTeleportRuneRuntime(t, nil, map0TestWorld, "2", false)
	rt.dynamic = newWarehouseTestDynamicWorld()
	intent := protocol.ClientWarehouseCommand{Operation: protocol.WarehouseOperationWithdrawGM, ItemArchetypeID: ArmorEnhancementScrollItemArchetypeID, Quantity: 1}
	if err := rt.queue.tryPush(useActionCommand{sessionID: s.ID, sequence: 1, warehouse: &intent}); err != nil { t.Fatal(err) }
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("errors=%#v", report.CommandErrors) }
	if got := rt.inventories[identity.ID].Quantity(ArmorEnhancementScrollItemArchetypeID); got != 0 { t.Fatalf("unauthorized quantity=%d", got) }
	result := requireWarehouseResult(t, drainWarehouseMessages(connection))
	if result.Outcome != protocol.WarehouseOutcomeRejected || result.Reason != protocol.WarehouseRejectionNotAuthorized { t.Fatalf("result=%#v", result) }
}

func TestPersonalWarehouseDepositWithdrawPersistsInCharacterSnapshot(t *testing.T) {
	rt, s, identity, connection := makeTeleportRuneRuntime(t, nil, map0TestWorld, "2", false)
	rt.dynamic = newWarehouseTestDynamicWorld()
	const itemID = "item_minor_healing_potion"
	if err := rt.inventories[identity.ID].Add(itemID, 5); err != nil { t.Fatal(err) }

	deposit := protocol.ClientWarehouseCommand{Operation: protocol.WarehouseOperationDepositPersonal, ItemArchetypeID: itemID, Quantity: 3}
	if err := rt.queue.tryPush(useActionCommand{sessionID: s.ID, sequence: 1, warehouse: &deposit}); err != nil { t.Fatal(err) }
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("deposit errors=%#v", report.CommandErrors) }
	if got := rt.inventories[identity.ID].Quantity(itemID); got != 2 { t.Fatalf("inventory after deposit=%d", got) }
	if got := rt.warehouses[identity.ID].Quantity(itemID); got != 3 { t.Fatalf("warehouse after deposit=%d", got) }
	if result := requireWarehouseResult(t, drainWarehouseMessages(connection)); result.Outcome != protocol.WarehouseOutcomeDeposited { t.Fatalf("deposit result=%#v", result) }

	withdraw := protocol.ClientWarehouseCommand{Operation: protocol.WarehouseOperationWithdrawPersonal, ItemArchetypeID: itemID, Quantity: 2}
	if err := rt.queue.tryPush(useActionCommand{sessionID: s.ID, sequence: 2, warehouse: &withdraw}); err != nil { t.Fatal(err) }
	if report := rt.Step(3, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("withdraw errors=%#v", report.CommandErrors) }
	if got := rt.inventories[identity.ID].Quantity(itemID); got != 4 { t.Fatalf("inventory after withdraw=%d", got) }
	if got := rt.warehouses[identity.ID].Quantity(itemID); got != 1 { t.Fatalf("warehouse after withdraw=%d", got) }
	drainWarehouseMessages(connection)

	_, snapshot, ok := rt.captureCharacterStateSnapshot(s.ID, s.EntityID, &StepReport{})
	if !ok { t.Fatal("failed to capture durable character snapshot") }
	canonical, err := characterstate.CanonicalWarehouseState(snapshot.Warehouse)
	if err != nil { t.Fatal(err) }
	if len(canonical.Items) != 1 || canonical.Items[0].ItemArchetypeID != itemID || canonical.Items[0].Quantity != 1 { t.Fatalf("durable warehouse=%#v", canonical.Items) }
	restored, err := restoreCharacterWarehouse(snapshot.Warehouse)
	if err != nil { t.Fatal(err) }
	if got := restored.Quantity(itemID); got != 1 { t.Fatalf("restored warehouse quantity=%d", got) }
}

func drainWarehouseMessages(connection *session.QueueConnection) []protocol.Message {
	messages := make([]protocol.Message, 0)
	for {
		select {
		case envelope := <-connection.Reliable():
			messages = append(messages, envelope.Message)
		default:
			return messages
		}
	}
}

func requireWarehouseResult(t *testing.T, messages []protocol.Message) protocol.WarehouseResult {
	t.Helper()
	for _, message := range messages {
		if result, ok := message.(protocol.WarehouseResult); ok { return result }
	}
	t.Fatalf("warehouse result missing from %#v", messages)
	return protocol.WarehouseResult{}
}

func requireWarehouseSnapshot(t *testing.T, messages []protocol.Message) protocol.WarehouseSnapshot {
	t.Helper()
	for _, message := range messages {
		if snapshot, ok := message.(protocol.WarehouseSnapshot); ok { return snapshot }
	}
	t.Fatalf("warehouse snapshot missing from %#v", messages)
	return protocol.WarehouseSnapshot{}
}

package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestItemDropExpiresAtAuthoritativeDeadline(t *testing.T) {
	runtime, sim, _ := newItemDropTestRuntime(t, world.Position{})
	dropID, err := runtime.spawnExpiringItemDrop(testItemDropArchetypeID, world.Position{X: 4}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := runtime.itemDropExpireTick[dropID]; got != 1+defaultItemDropLifetimeTicks {
		t.Fatalf("default expire tick = %d, want %d", got, 1+defaultItemDropLifetimeTicks)
	}

	// Compress only this focused test deadline; the production creation path above already proved
	// the authored default deadline and the world-owner stage is independent from wall-clock time.
	runtime.itemDropExpireTick[dropID] = 4
	for _, tick := range []uint64{2, 3} {
		runtime.Step(tick, 50*time.Millisecond)
		if _, exists := sim.Entity(dropID); !exists {
			t.Fatalf("drop disappeared before deadline at tick %d", tick)
		}
	}

	runtime.Step(4, 50*time.Millisecond)
	if _, exists := sim.Entity(dropID); exists {
		t.Fatal("expired drop still exists in authoritative world")
	}
	if _, tracked := runtime.itemDropExpireTick[dropID]; tracked {
		t.Fatal("expired drop still has expiry bookkeeping")
	}
}

func TestPickedItemDropClearsExpiryBookkeeping(t *testing.T) {
	runtime, sim, s := newItemDropTestRuntime(t, world.Position{})
	dropID, err := runtime.spawnExpiringItemDrop(testItemDropArchetypeID, world.Position{X: 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueuePickupItem(s.ID, 1, protocol.ClientPickupItem{DropEntityID: dropID}); err != nil {
		t.Fatal(err)
	}
	report := runtime.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("pickup errors: %#v", report.CommandErrors)
	}
	if _, exists := sim.Entity(dropID); exists {
		t.Fatal("picked drop still exists")
	}
	if _, tracked := runtime.itemDropExpireTick[dropID]; tracked {
		t.Fatal("picked drop still has expiry bookkeeping")
	}
}

func TestAutoLootClearsExpiryBookkeeping(t *testing.T) {
	runtime, sim, s := newItemDropTestRuntime(t, world.Position{})
	dropID, err := runtime.spawnExpiringItemDrop(testItemDropArchetypeID, world.Position{X: 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	report := StepReport{Tick: 1}
	if ok := runtime.tryAutoGrantMonsterLoot(monsterLootCandidate{
		sessionID: s.ID,
		characterID: s.CharacterIdentity.ID,
		damage: 1,
	}, testItemDropArchetypeID, dropID, &report); !ok {
		t.Fatalf("auto-loot failed: %#v", report.CommandErrors)
	}
	if _, exists := sim.Entity(dropID); exists {
		t.Fatal("auto-looted drop still exists")
	}

	runtime.Step(2, 50*time.Millisecond)
	if _, tracked := runtime.itemDropExpireTick[dropID]; tracked {
		t.Fatal("auto-looted drop still has expiry bookkeeping")
	}
	if got := runtime.inventories[s.CharacterIdentity.ID].Quantity(testItemDropArchetypeID); got != 1 {
		t.Fatalf("auto-loot quantity = %d, want 1", got)
	}
}

func TestRemovedItemDropCleansExpiryBookkeepingWithoutSecondMutation(t *testing.T) {
	runtime, sim, _ := newItemDropTestRuntime(t, world.Position{})
	dropID, err := runtime.spawnExpiringItemDrop(testItemDropArchetypeID, world.Position{X: 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	// Models same-owner spawn rollback removal before the post-loot expiry stage.
	sim.Remove(dropID)
	runtime.Step(2, 50*time.Millisecond)
	if _, tracked := runtime.itemDropExpireTick[dropID]; tracked {
		t.Fatal("removed drop still has expiry bookkeeping")
	}
	if _, exists := sim.Entity(dropID); exists {
		t.Fatal("removed drop unexpectedly reappeared")
	}
}

func TestExpiredItemDropUsesExistingEntityDespawnReplication(t *testing.T) {
	runtime, _, s := newItemDropTestRuntime(t, world.Position{})
	runtime.config.SnapshotEveryTicks = 1
	connection, ok := s.Connection().(*session.QueueConnection)
	if !ok {
		t.Fatalf("connection = %T, want *session.QueueConnection", s.Connection())
	}
	drainExpiryReliable(connection)

	dropID, err := runtime.spawnExpiringItemDrop(testItemDropArchetypeID, world.Position{X: 1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	runtime.itemDropExpireTick[dropID] = 3

	report := runtime.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.DeliveryErrors) != 0 {
		t.Fatalf("spawn replication report: %#v", report)
	}
	if !expiryReliableHasSpawn(connection, dropID) {
		t.Fatalf("missing EntitySpawn for drop %d", dropID)
	}

	report = runtime.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.DeliveryErrors) != 0 {
		t.Fatalf("expiry replication report: %#v", report)
	}
	if !expiryReliableHasDespawn(connection, dropID) {
		t.Fatalf("missing EntityDespawn for expired drop %d", dropID)
	}
}

func TestItemDropExpiryTickSaturates(t *testing.T) {
	max := ^uint64(0)
	if got := itemDropExpiryTick(max-2, 3); got != max {
		t.Fatalf("overflow expiry tick = %d, want %d", got, max)
	}
	if got := itemDropExpiryTick(10, 20); got != 30 {
		t.Fatalf("normal expiry tick = %d, want 30", got)
	}
}

func drainExpiryReliable(connection *session.QueueConnection) {
	for {
		select {
		case <-connection.Reliable():
		default:
			return
		}
	}
}

func expiryReliableHasSpawn(connection *session.QueueConnection, entityID world.EntityID) bool {
	for {
		select {
		case envelope := <-connection.Reliable():
			if message, ok := envelope.Message.(protocol.EntitySpawn); ok && message.EntityID == entityID {
				return true
			}
		default:
			return false
		}
	}
}

func expiryReliableHasDespawn(connection *session.QueueConnection, entityID world.EntityID) bool {
	for {
		select {
		case envelope := <-connection.Reliable():
			if message, ok := envelope.Message.(protocol.EntityDespawn); ok && message.EntityID == entityID {
				return true
			}
		default:
			return false
		}
	}
}

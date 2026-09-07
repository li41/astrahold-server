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
	runtime.config.ItemDropLifetimeTicks = 3

	dropID, err := runtime.spawnExpiringItemDrop(testItemDropArchetypeID, world.Position{X: 4}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := runtime.itemDropExpireTick[dropID]; got != 4 {
		t.Fatalf("expire tick = %d, want 4", got)
	}

	for _, tick := range []uint64{2, 3} {
		report := runtime.Step(tick, 50*time.Millisecond)
		if report.Metrics.ItemDropsExpired != 0 {
			t.Fatalf("tick %d expired %d drops before deadline", tick, report.Metrics.ItemDropsExpired)
		}
		if _, exists := sim.Entity(dropID); !exists {
			t.Fatalf("drop disappeared before deadline at tick %d", tick)
		}
	}

	report := runtime.Step(4, 50*time.Millisecond)
	if report.Metrics.ItemDropsExpired != 1 {
		t.Fatalf("expired drops = %d, want 1", report.Metrics.ItemDropsExpired)
	}
	if _, exists := sim.Entity(dropID); exists {
		t.Fatal("expired drop still exists in authoritative world")
	}
	if _, tracked := runtime.itemDropExpireTick[dropID]; tracked {
		t.Fatal("expired drop still has lifecycle bookkeeping")
	}
}

func TestPickedItemDropClearsExpiryWithoutCountingAsExpired(t *testing.T) {
	runtime, sim, s := newItemDropTestRuntime(t, world.Position{})
	runtime.config.ItemDropLifetimeTicks = 10
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
	if report.Metrics.ItemDropsExpired != 0 {
		t.Fatalf("picked drop counted as expired: %d", report.Metrics.ItemDropsExpired)
	}
	if _, exists := sim.Entity(dropID); exists {
		t.Fatal("picked drop still exists")
	}
	if _, tracked := runtime.itemDropExpireTick[dropID]; tracked {
		t.Fatal("picked drop still has expiry bookkeeping")
	}
}

func TestExpiredItemDropUsesExistingEntityDespawnReplication(t *testing.T) {
	runtime, _, s := newItemDropTestRuntime(t, world.Position{})
	runtime.config.SnapshotEveryTicks = 1
	runtime.config.ItemDropLifetimeTicks = 2
	connection, ok := s.Connection().(*session.QueueConnection)
	if !ok {
		t.Fatalf("connection = %T, want *session.QueueConnection", s.Connection())
	}
	drainReliable(connection)

	dropID, err := runtime.spawnExpiringItemDrop(testItemDropArchetypeID, world.Position{X: 1}, 1)
	if err != nil {
		t.Fatal(err)
	}

	report := runtime.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 || len(report.DeliveryErrors) != 0 {
		t.Fatalf("spawn replication report: %#v", report)
	}
	if !reliableHasSpawn(connection, dropID) {
		t.Fatalf("missing EntitySpawn for drop %d", dropID)
	}

	report = runtime.Step(3, 50*time.Millisecond)
	if report.Metrics.ItemDropsExpired != 1 {
		t.Fatalf("expired drops = %d, want 1", report.Metrics.ItemDropsExpired)
	}
	if !reliableHasDespawn(connection, dropID) {
		t.Fatalf("missing EntityDespawn for expired drop %d", dropID)
	}
}

func TestItemDropExpiryTickSaturates(t *testing.T) {
	max := ^uint64(0)
	if got := itemDropExpiryTick(max-2, 3); got != max {
		t.Fatalf("overflow expiry tick = %d, want %d", got, max)
	}
}

func drainReliable(connection *session.QueueConnection) {
	for {
		select {
		case <-connection.Reliable():
		default:
			return
		}
	}
}

func reliableHasSpawn(connection *session.QueueConnection, entityID world.EntityID) bool {
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

func reliableHasDespawn(connection *session.QueueConnection, entityID world.EntityID) bool {
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

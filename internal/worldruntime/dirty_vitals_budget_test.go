package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

type dirtyVitalsCaptureConnection struct {
	sent []protocol.Envelope
}

func (c *dirtyVitalsCaptureConnection) TrySend(envelope protocol.Envelope) error {
	c.sent = append(c.sent, envelope)
	return nil
}
func (*dirtyVitalsCaptureConnection) Close() error { return nil }

func TestDirtyVitalsBudgetMakesRoundRobinProgressAcrossEntities(t *testing.T) {
	nav := navigation.Plane{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	entities := []world.EntityState{
		{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 0, Z: 0, Layer: 0}}},
		{ID: 2, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{X: 1, Z: 0, Layer: 0}}},
	}
	for _, entity := range entities {
		if err := sim.Spawn(entity, 6, 0.35, 0.5); err != nil {
			t.Fatal(err)
		}
	}

	config := DefaultConfig()
	config.MaxDirtyVitalsPerTick = 2
	rt := New(sim, config)
	for _, entity := range entities {
		if err := rt.characters.Register(entity.ID); err != nil {
			t.Fatal(err)
		}
		rt.ensureEntityVitalsRevision(entity.ID)
	}

	connections := make([]*dirtyVitalsCaptureConnection, 0, 4)
	for i := 1; i <= 4; i++ {
		connection := &dirtyVitalsCaptureConnection{}
		s, err := session.New(session.ID(i), 1, 20, connection)
		if err != nil {
			t.Fatal(err)
		}
		if err := rt.sessions.Add(s); err != nil {
			t.Fatal(err)
		}
		rt.replication.Register(s.ID)
		rt.replication.Build(s.ID, 1, 0, 1, entities)
		for _, entity := range entities {
			rt.replication.ConfirmSpawn(s.ID, entity.ID)
			rt.ensureSessionVitalsDelivered(s.ID)[entity.ID] = 1
		}
		connections = append(connections, connection)
	}

	rt.markEntityVitalsDirty(1)
	rt.markEntityVitalsDirty(2)

	var totalSelected int
	for tick := uint64(1); tick <= 4; tick++ {
		report := StepReport{}
		rt.replicateEntityVitals(tick, &report)
		if report.Metrics.DirtyVitalsSelected > config.MaxDirtyVitalsPerTick {
			t.Fatalf("tick=%d dirty selected=%d exceeds budget=%d", tick, report.Metrics.DirtyVitalsSelected, config.MaxDirtyVitalsPerTick)
		}
		if report.Metrics.DirtyVitalsSelected != 2 {
			t.Fatalf("tick=%d dirty selected=%d want=2", tick, report.Metrics.DirtyVitalsSelected)
		}
		if tick < 4 && !report.Metrics.DirtyVitalsGlobalBudgetExhausted {
			t.Fatalf("tick=%d expected exhausted budget", tick)
		}
		totalSelected += report.Metrics.DirtyVitalsSelected
	}
	if totalSelected != 8 {
		t.Fatalf("total dirty selected=%d want=8", totalSelected)
	}
	if len(rt.dirtyVitalsEntities) != 0 {
		t.Fatalf("dirty entities remain=%v", rt.dirtyVitalsEntities)
	}

	// 最後一個 tick 可能恰好把 budget 用到0；nextEntity 是 lazy cursor，下一個空 tick 清掉。
	cleanup := StepReport{}
	rt.replicateEntityVitals(5, &cleanup)
	if cleanup.Metrics.DirtyVitalsSelected != 0 {
		t.Fatalf("cleanup tick resent dirty vitals=%d", cleanup.Metrics.DirtyVitalsSelected)
	}
	if rt.dirtyVitalsNextEntity != 0 {
		t.Fatalf("dirty next entity=%d want=0 after cleanup tick", rt.dirtyVitalsNextEntity)
	}

	for i, connection := range connections {
		var dirtyCount int
		for _, envelope := range connection.sent {
			if _, ok := envelope.Message.(protocol.EntityVitalsState); ok {
				dirtyCount++
			}
		}
		if dirtyCount != 2 {
			t.Fatalf("connection %d dirty vitals=%d want=2", i+1, dirtyCount)
		}
	}
}

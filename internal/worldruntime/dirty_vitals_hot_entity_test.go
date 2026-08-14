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

func TestDirtyVitalsHotEntityMakesProgressAcrossSessionsWhileRevisionKeepsAdvancing(t *testing.T) {
	nav := navigation.Plane{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	entity := world.EntityState{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}}
	if err := sim.Spawn(entity, 6, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}

	config := DefaultConfig()
	config.MaxDirtyVitalsPerTick = 1
	rt := New(sim, config)
	if err := rt.characters.Register(entity.ID); err != nil {
		t.Fatal(err)
	}
	rt.ensureEntityVitalsRevision(entity.ID)

	connections := make([]*dirtyVitalsCaptureConnection, 0, 4)
	for i := 1; i <= 4; i++ {
		connection := &dirtyVitalsCaptureConnection{}
		s, err := session.New(session.ID(i), entity.ID, 20, connection)
		if err != nil {
			t.Fatal(err)
		}
		if err := rt.sessions.Add(s); err != nil {
			t.Fatal(err)
		}
		rt.replication.Register(s.ID)
		rt.replication.Build(s.ID, entity.ID, 0, 1, []world.EntityState{entity})
		rt.replication.ConfirmSpawn(s.ID, entity.ID)
		rt.ensureSessionVitalsDelivered(s.ID)[entity.ID] = 1
		connections = append(connections, connection)
	}

	// 每個 tick 都先讓同一個 hot Entity revision 再前進一次，模擬上一版尚未完成 fan-out
	// 就再次受到 gameplay 更新。budget=1 時仍必須跨 Session 持續前進，不能永遠從 Session 1 重來。
	for tick := uint64(1); tick <= 4; tick++ {
		rt.markEntityVitalsDirty(entity.ID)
		report := StepReport{}
		rt.replicateEntityVitals(tick, &report)
		if report.Metrics.DirtyVitalsSelected != 1 {
			t.Fatalf("tick=%d dirty selected=%d want=1", tick, report.Metrics.DirtyVitalsSelected)
		}
		if !report.Metrics.DirtyVitalsGlobalBudgetExhausted {
			t.Fatalf("tick=%d expected exhausted dirty budget", tick)
		}
	}

	for i, connection := range connections {
		if len(connection.sent) != 1 {
			t.Fatalf("session=%d dirty vitals=%d want=1 after four continuously advancing revisions", i+1, len(connection.sent))
		}
		if _, ok := connection.sent[0].Message.(protocol.EntityVitalsState); !ok {
			t.Fatalf("session=%d message=%T want EntityVitalsState", i+1, connection.sent[0].Message)
		}
	}
	if len(rt.dirtyVitalsEntities) != 1 {
		t.Fatalf("hot entity should remain dirty until every session reaches latest revision; dirty=%v", rt.dirtyVitalsEntities)
	}

	latestRevision := rt.entityVitalsRevision[entity.ID]
	for tick := uint64(5); tick <= 12 && len(rt.dirtyVitalsEntities) != 0; tick++ {
		report := StepReport{}
		rt.replicateEntityVitals(tick, &report)
		if report.Metrics.DirtyVitalsSelected > config.MaxDirtyVitalsPerTick {
			t.Fatalf("tick=%d dirty selected=%d exceeds budget=%d", tick, report.Metrics.DirtyVitalsSelected, config.MaxDirtyVitalsPerTick)
		}
	}
	if len(rt.dirtyVitalsEntities) != 0 {
		t.Fatalf("hot entity failed to converge to latest revision=%d", latestRevision)
	}
	for i := 1; i <= 4; i++ {
		delivered := rt.sessionVitalsRevision[session.ID(i)][entity.ID]
		if delivered != latestRevision {
			t.Fatalf("session=%d delivered revision=%d want latest=%d", i, delivered, latestRevision)
		}
	}
}

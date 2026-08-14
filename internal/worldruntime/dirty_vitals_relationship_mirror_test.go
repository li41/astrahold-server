package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestDirtyVitalsUsesPendingOrDeliveredAsKnownRelationshipMirror(t *testing.T) {
	nav := navigation.Plane{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	entity := world.EntityState{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}}
	if err := sim.Spawn(entity, 6, 0.35, 0.5); err != nil {
		t.Fatal(err)
	}
	config := DefaultConfig()
	config.MaxDirtyVitalsPerTick = 4
	rt := New(sim, config)
	if err := rt.characters.Register(entity.ID); err != nil {
		t.Fatal(err)
	}
	rt.ensureEntityVitalsRevision(entity.ID)

	connection := &dirtyVitalsCaptureConnection{}
	s, err := session.New(1, entity.ID, 20, connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.sessions.Add(s); err != nil {
		t.Fatal(err)
	}
	rt.replication.Register(s.ID)
	rt.replication.Build(s.ID, entity.ID, 0, 1, []world.EntityState{entity})
	rt.replication.ConfirmSpawn(s.ID, entity.ID)

	// ConfirmSpawn delivery path一定會 queue Initial Vitals；尚未有 delivered revision時，pending本身
	// 就必須讓 Dirty Vitals知道這個 relationship已經 known，並可直接用 latest full state完成初始值。
	rt.queueEntityVitalsForSession(s.ID, entity.ID)
	rt.markEntityVitalsDirty(entity.ID)
	report := StepReport{}
	rt.replicateDirtyEntityVitals(1, rt.sessions.List(), &report)
	if report.Metrics.DirtyVitalsSelected != 1 {
		t.Fatalf("dirty selected=%d want=1 for pending-known relationship", report.Metrics.DirtyVitalsSelected)
	}
	if got := rt.sessionVitalsRevision[s.ID][entity.ID]; got != rt.entityVitalsRevision[entity.ID] {
		t.Fatalf("delivered revision=%d want=%d", got, rt.entityVitalsRevision[entity.ID])
	}
	if pending := rt.sessionVitalsPending[s.ID]; pending != nil {
		if _, ok := pending[entity.ID]; ok {
			t.Fatal("dirty delivery did not clear initial pending relationship")
		}
	}

	// ConfirmDespawn會同步移除 delivered/pending mirror；後續 dirty不能再 fan-out給此 Session。
	rt.confirmEntityDespawnVitals(s.ID, entity.ID)
	before := len(connection.sent)
	rt.markEntityVitalsDirty(entity.ID)
	report = StepReport{}
	rt.replicateDirtyEntityVitals(2, rt.sessions.List(), &report)
	if report.Metrics.DirtyVitalsSelected != 0 || len(connection.sent) != before {
		t.Fatalf("despawned relationship received dirty vitals: selected=%d sent_before=%d sent_after=%d", report.Metrics.DirtyVitalsSelected, before, len(connection.sent))
	}
}

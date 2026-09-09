package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/targetresource"
	"github.com/li41/astrahold-server/internal/world"
)

func TestLeaveClearsTargetResourceForSurvivingSource(t *testing.T) {
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	rt := New(sim, cfg)

	sourceConn := session.NewQueueConnection(128, 16)
	source, err := session.New(1, 10, 32, sourceConn)
	if err != nil { t.Fatal(err) }
	targetConn := session.NewQueueConnection(128, 16)
	target, err := session.New(2, 20, 32, targetConn)
	if err != nil { t.Fatal(err) }
	for _, request := range []JoinRequest{
		{Session: source, Entity: world.EntityState{ID: 10, Kind: world.EntityPlayer}, Speed: 6, Radius: .35, MaxStepHeight: .5},
		{Session: target, Entity: world.EntityState{ID: 20, Kind: world.EntityPlayer}, Speed: 6, Radius: .35, MaxStepHeight: .5},
	} {
		if err := rt.EnqueueJoin(request); err != nil { t.Fatal(err) }
	}
	report := rt.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 { t.Fatalf("join report=%#v", report) }
	drainReliable(sourceConn)
	drainReliable(targetConn)

	state, changed, err := rt.characters.GainTargetResource(source.EntityID, target.EntityID, targetresource.Flaw, 1, 3, 1, 51)
	if err != nil || !changed || state.Current != 1 { t.Fatalf("gain state=%+v changed=%v err=%v", state, changed, err) }
	if err := rt.EnqueueLeave(target.ID); err != nil { t.Fatal(err) }
	report = rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 { t.Fatalf("leave report=%#v", report) }
	if _, ok := rt.characters.TargetResourceState(source.EntityID, target.EntityID, targetresource.Flaw); ok { t.Fatal("target resource survived target leave") }
	assertTargetResourceClearMessage(t, drainReliable(sourceConn), source.EntityID, target.EntityID)
}

func TestMonsterDespawnClearsTargetResourceForSurvivingSource(t *testing.T) {
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	spawn := testMonsterLifecycleSpawn()
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1
	rt := New(sim, cfg, WithMonsterLifecycle(MonsterLifecycleConfig{Spawn: spawn, CorpseHoldTicks: 1, RespawnDelayTicks: 2}))
	sourceConn := session.NewQueueConnection(128, 16)
	source, err := session.New(1, 10, 32, sourceConn)
	if err != nil { t.Fatal(err) }
	if err := rt.EnqueueJoin(JoinRequest{Session: source, Entity: world.EntityState{ID: 10, Kind: world.EntityPlayer}, Speed: 6, Radius: .35, MaxStepHeight: .5}); err != nil { t.Fatal(err) }
	if err := rt.EnqueueSpawnEntity(spawn); err != nil { t.Fatal(err) }
	assertCleanMonsterLifecycleStep(t, rt.Step(1, 50*time.Millisecond))
	drainReliable(sourceConn)

	state, changed, err := rt.characters.GainTargetResource(source.EntityID, spawn.Entity.ID, targetresource.Flaw, 1, 3, 1, 51)
	if err != nil || !changed || state.Current != 1 { t.Fatalf("gain state=%+v changed=%v err=%v", state, changed, err) }
	if _, err := rt.characters.ApplyDamage(spawn.Entity.ID, spawn.MaxHP); err != nil { t.Fatal(err) }
	rt.markEntityVitalsDirty(spawn.Entity.ID)
	assertCleanMonsterLifecycleStep(t, rt.Step(2, 50*time.Millisecond))
	drainReliable(sourceConn)
	assertCleanMonsterLifecycleStep(t, rt.Step(3, 50*time.Millisecond))
	if _, ok := rt.characters.TargetResourceState(source.EntityID, spawn.Entity.ID, targetresource.Flaw); ok { t.Fatal("target resource survived monster despawn") }
	assertTargetResourceClearMessage(t, drainReliable(sourceConn), source.EntityID, spawn.Entity.ID)
}

func assertTargetResourceClearMessage(t *testing.T, envelopes []protocol.Envelope, sourceID, targetID world.EntityID) {
	t.Helper()
	for _, envelope := range envelopes {
		state, ok := envelope.Message.(protocol.CharacterTargetResourceState)
		if !ok { continue }
		if state.SourceEntityID == sourceID && state.TargetEntityID == targetID && state.ResourceID == "flaw" && state.Current == 0 && state.Max == 3 { return }
	}
	t.Fatalf("missing authoritative target-resource clear source=%d target=%d envelopes=%#v", sourceID, targetID, envelopes)
}

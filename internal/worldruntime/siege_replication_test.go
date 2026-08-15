package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/siege"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

type siegeRecordingConnection struct {
	failSiege bool
	messages  []protocol.Envelope
}

func (c *siegeRecordingConnection) TrySend(envelope protocol.Envelope) error {
	if _, ok := envelope.Message.(protocol.SiegeMatchState); ok && c.failSiege {
		return session.ErrBackpressure
	}
	c.messages = append(c.messages, envelope)
	return nil
}
func (*siegeRecordingConnection) Close() error { return nil }

func (c *siegeRecordingConnection) siegeMessages() []protocol.SiegeMatchState {
	out := make([]protocol.SiegeMatchState, 0)
	for _, envelope := range c.messages {
		if message, ok := envelope.Message.(protocol.SiegeMatchState); ok {
			out = append(out, message)
		}
	}
	return out
}

func TestSiegeStateReliableRetryTeamStampAndPhaseRevision(t *testing.T) {
	definition := gameplayworld.Definition{
		SchemaVersion: gameplayworld.SchemaVersion,
		WorldID: "siege-replication-test", Revision: "r1", Units: "meters",
		Agent: gameplayworld.AgentDefaults{Radius: 0.35, Height: 1.8, MaxStepHeight: 0.5},
		Surfaces: []gameplayworld.Surface{{ID:"ground",Layer:0,Bounds:gameplayworld.BoundsXZ{MinX:-10,MaxX:10,MinZ:-10,MaxZ:10},Plane:gameplayworld.SurfacePlane{}}},
		Blockers: []gameplayworld.Blocker{{ID:"main-gate",Layer:0,Bounds:gameplayworld.BoundsXZ{MinX:-1,MaxX:1,MinZ:1,MaxZ:2},MinY:0,MaxY:3,BlocksMovement:true,BlocksLOS:true,Enabled:true}},
		Gates: []gameplayworld.Gate{{ID:"main-gate",BlockerID:"main-gate",MaxHP:1000,Attack:gameplayworld.GateAttackProfile{Range:4.5,Damage:100,CooldownSeconds:0.5}}},
	}
	nav, err := navigation.NewGameplayNavigator(definition)
	if err != nil { t.Fatal(err) }
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	if err := sim.Spawn(world.EntityState{ID:1,Kind:world.EntityPlayer,Transform:world.Transform{Position:world.Position{Layer:0}}}, 6, 0.35, 0.5); err != nil { t.Fatal(err) }

	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 100
	rt := New(sim, cfg,
		WithSiegeGates(definition.Gates),
		WithSiegeMatch(siege.MatchDefinition{ID:"castle-sandbox-siege",AttackerID:"attackers",DefenderID:"defenders",BreachGateID:"main-gate",ThroneObjectiveID:"throne"}),
	)
	if err := rt.siege.AssignParticipant(1, siege.TeamAttacker); err != nil { t.Fatal(err) }
	conn := &siegeRecordingConnection{failSiege:true}
	s, err := session.New(1, 1, 20, conn)
	if err != nil { t.Fatal(err) }
	if err := rt.EnqueueRegister(s); err != nil { t.Fatal(err) }

	first := rt.Step(1, 50*time.Millisecond)
	if len(first.DeliveryErrors) == 0 || !errors.Is(first.DeliveryErrors[0].Err, session.ErrBackpressure) || len(conn.siegeMessages()) != 0 {
		t.Fatalf("failed first delivery report=%#v messages=%#v", first.DeliveryErrors, conn.siegeMessages())
	}
	if _, delivered := rt.sessionSiegeState[s.ID]; delivered {
		t.Fatal("failed TrySend must not advance siege delivery stamp")
	}

	conn.failSiege = false
	second := rt.Step(2, 50*time.Millisecond)
	if len(second.DeliveryErrors) != 0 { t.Fatalf("retry errors=%#v", second.DeliveryErrors) }
	messages := conn.siegeMessages()
	if len(messages) != 1 || messages[0].Revision != 1 || messages[0].YourTeam != protocol.SiegeTeamAttacker || messages[0].Phase != protocol.SiegePhaseGate || messages[0].GateBreached {
		t.Fatalf("initial siege messages=%#v", messages)
	}

	rt.Step(3, 50*time.Millisecond)
	if got := len(conn.siegeMessages()); got != 1 { t.Fatalf("unchanged state resent: %d", got) }

	if err := rt.siege.AssignParticipant(1, siege.TeamDefender); err != nil { t.Fatal(err) }
	rt.Step(4, 50*time.Millisecond)
	messages = conn.siegeMessages()
	if len(messages) != 2 || messages[1].Revision != 1 || messages[1].YourTeam != protocol.SiegeTeamDefender {
		t.Fatalf("team restamp messages=%#v", messages)
	}

	if !rt.siege.ObserveGateState(siege.GateState{ID:"main-gate",HP:0,MaxHP:1000,Destroyed:true}) {
		t.Fatal("authoritative breach did not advance match")
	}
	rt.Step(5, 50*time.Millisecond)
	messages = conn.siegeMessages()
	if len(messages) != 3 || messages[2].Revision != 2 || messages[2].Phase != protocol.SiegePhaseThrone || !messages[2].GateBreached || messages[2].YourTeam != protocol.SiegeTeamDefender {
		t.Fatalf("advanced siege messages=%#v", messages)
	}
}

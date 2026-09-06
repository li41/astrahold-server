package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func newNPCTestRuntime(t *testing.T, playerPosition world.Position) (*Runtime, *simulation.World, *session.Session, *session.QueueConnection) {
	t.Helper()
	sim := simulation.New(
		spatial.NewGrid(16),
		movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1),
	)
	config := DefaultConfig()
	config.SnapshotEveryTicks = 1000
	runtime := New(sim, config)
	connection := session.NewQueueConnection(32, 8)
	s, err := session.New(1, 10, 32, connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueJoin(JoinRequest{
		Session: s,
		Entity: world.EntityState{
			ID:        10,
			Kind:      world.EntityPlayer,
			Transform: world.Transform{Position: playerPosition},
		},
		Speed:         6,
		Radius:        0.35,
		MaxStepHeight: 0.5,
	}); err != nil {
		t.Fatal(err)
	}
	report := runtime.Step(1, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("join errors: %#v", report.CommandErrors)
	}
	for {
		select {
		case <-connection.Reliable():
			continue
		default:
			return runtime, sim, s, connection
		}
	}
}

func spawnTestNPC(t *testing.T, sim *simulation.World, position world.Position) world.EntityID {
	t.Helper()
	const npcID world.EntityID = 7001
	if err := sim.Spawn(world.EntityState{
		ID:          npcID,
		Kind:        world.EntityNPC,
		ArchetypeID: playtestNPCArchetypeID,
		Transform:   world.Transform{Position: position},
	}, 0, 0.35, 0); err != nil {
		t.Fatal(err)
	}
	return npcID
}

func TestInteractNPCEmitsAuthoritativeDialogue(t *testing.T) {
	runtime, sim, s, connection := newNPCTestRuntime(t, world.Position{})
	npcID := spawnTestNPC(t, sim, world.Position{X: 2})

	if err := runtime.EnqueueInteractNPC(s.ID, 1, protocol.ClientInteractNPC{NPCEntityID: npcID}); err != nil {
		t.Fatal(err)
	}
	report := runtime.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("interaction errors: %#v", report.CommandErrors)
	}
	if len(report.DeliveryErrors) != 0 {
		t.Fatalf("interaction delivery errors: %#v", report.DeliveryErrors)
	}

	select {
	case envelope := <-connection.Reliable():
		interaction, ok := envelope.Message.(protocol.NPCInteraction)
		if !ok {
			t.Fatalf("message = %T, want protocol.NPCInteraction", envelope.Message)
		}
		if envelope.Delivery != protocol.DeliveryReliableOrdered || interaction.NPCEntityID != npcID {
			t.Fatalf("envelope = %#v", envelope)
		}
		if interaction.NPCArchetypeID != playtestNPCArchetypeID || interaction.DisplayName != playtestNPCDisplayName || interaction.Text != playtestNPCDialogue {
			t.Fatalf("interaction = %#v", interaction)
		}
	default:
		t.Fatal("expected reliable NPCInteraction response")
	}
}

func TestInteractNPCRejectsOutOfRangeWithoutResponse(t *testing.T) {
	runtime, sim, s, connection := newNPCTestRuntime(t, world.Position{})
	npcID := spawnTestNPC(t, sim, world.Position{X: 10})

	if err := runtime.EnqueueInteractNPC(s.ID, 1, protocol.ClientInteractNPC{NPCEntityID: npcID}); err != nil {
		t.Fatal(err)
	}
	report := runtime.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrNPCOutOfRange) {
		t.Fatalf("interaction errors = %#v, want out of range", report.CommandErrors)
	}
	select {
	case envelope := <-connection.Reliable():
		t.Fatalf("unexpected response after rejected interaction: %#v", envelope)
	default:
	}
}

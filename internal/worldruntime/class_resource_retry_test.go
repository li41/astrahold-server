package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/classresource"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

type classResourceRetryConnection struct {
	backpressureOnce bool
	sent             []protocol.Envelope
}

func (c *classResourceRetryConnection) TrySend(envelope protocol.Envelope) error {
	if envelope.Message.Type() == protocol.MessageCharacterClassResourceState && c.backpressureOnce {
		c.backpressureOnce = false
		return session.ErrBackpressure
	}
	c.sent = append(c.sent, envelope)
	return nil
}

func (*classResourceRetryConnection) Close() error { return nil }

func TestStepRetriesPendingLegacyClassResourceFeedback(t *testing.T) {
	nav := navigation.Plane{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	entity := world.EntityState{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}}
	if err := sim.Spawn(entity, 6, 0.35, 0.5); err != nil { t.Fatal(err) }

	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 100
	rt := New(sim, cfg)
	connection := &classResourceRetryConnection{backpressureOnce: true}
	s, err := session.New(1, 1, 20, connection)
	if err != nil { t.Fatal(err) }
	if err := rt.sessions.Add(s); err != nil { t.Fatal(err) }
	rt.replication.Register(s.ID)
	if err := rt.characters.RegisterState(character.State{EntityID: 1, ClassResourceID: classresource.Resolve, HP: 1000, MaxHP: 1000}); err != nil { t.Fatal(err) }
	if _, err := rt.characters.GainClassResource(1, classresource.Resolve, 8); err != nil { t.Fatal(err) }

	first := StepReport{Tick: 1}
	rt.sendCurrentClassResourceState(s, &first)
	if len(first.DeliveryErrors) != 0 { t.Fatalf("backpressure should defer, errors=%#v", first.DeliveryErrors) }
	if got := len(rt.pendingClassMessages[s.ID]); got != 1 { t.Fatalf("pending class resource messages=%d want=1", got) }
	if len(connection.sent) != 0 { t.Fatalf("backpressured resource state was sent: %d envelopes", len(connection.sent)) }

	report := rt.Step(2, 50*time.Millisecond)
	if len(report.DeliveryErrors) != 0 { t.Fatalf("retry delivery errors=%#v", report.DeliveryErrors) }
	if _, ok := rt.pendingClassMessages[s.ID]; ok { t.Fatalf("pending class resource queue not drained: %#v", rt.pendingClassMessages[s.ID]) }

	resourceMessages := 0
	for _, envelope := range connection.sent {
		message, ok := envelope.Message.(protocol.CharacterClassResourceState)
		if !ok { continue }
		resourceMessages++
		if envelope.Delivery != protocol.DeliveryReliableOrdered || envelope.ServerTick != 2 { t.Fatalf("resource envelope=%#v, want reliable tick 2", envelope) }
		if message.EntityID != 1 || message.ResourceID != string(classresource.Resolve) || message.Current != 8 || message.Max != 100 {
			t.Fatalf("resource state=%#v, want entity=1 resolve 8/100", message)
		}
	}
	if resourceMessages != 1 { t.Fatalf("resource messages=%d want=1; sent=%#v", resourceMessages, connection.sent) }
}

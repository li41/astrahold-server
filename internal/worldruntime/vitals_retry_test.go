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

type vitalsRetryConnection struct {
	backpressureOnce bool
	sent []protocol.Envelope
}

func (c *vitalsRetryConnection) TrySend(envelope protocol.Envelope) error {
	if envelope.Message.Type() == protocol.MessageEntityVitalsState && c.backpressureOnce {
		c.backpressureOnce = false
		return session.ErrBackpressure
	}
	c.sent = append(c.sent, envelope)
	return nil
}
func (*vitalsRetryConnection) Close() error { return nil }

func TestEntityVitalsBackpressureRetriesWithoutDeliveryError(t *testing.T) {
	nav := navigation.Plane{MinX:-10,MaxX:10,MinZ:-10,MaxZ:10,Layer:0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	entity := world.EntityState{ID:1,Kind:world.EntityPlayer,Transform:world.Transform{Position:world.Position{Layer:0}}}
	if err := sim.Spawn(entity, 6, 0.35, 0.5); err != nil { t.Fatal(err) }

	rt := New(sim, DefaultConfig())
	connection := &vitalsRetryConnection{backpressureOnce:true}
	s, err := session.New(1,1,20,connection)
	if err != nil { t.Fatal(err) }
	if err := rt.sessions.Add(s); err != nil { t.Fatal(err) }
	rt.replication.Register(s.ID)
	if err := rt.characters.Register(1); err != nil { t.Fatal(err) }
	rt.ensureEntityVitalsRevision(1)

	// Build 一次 view state，模擬 EntitySpawn 已建立 AOI knowledge。
	rt.replication.Build(s.ID, 1, 0, 1, []world.EntityState{entity})

	first := StepReport{}
	rt.replicateEntityVitals(1, &first)
	if len(first.DeliveryErrors) != 0 { t.Fatalf("backpressure should defer, errors=%#v", first.DeliveryErrors) }
	if rt.sessionVitalsRevision[s.ID][1] != 0 { t.Fatalf("revision advanced on backpressure: %d", rt.sessionVitalsRevision[s.ID][1]) }

	second := StepReport{}
	rt.replicateEntityVitals(2, &second)
	if len(second.DeliveryErrors) != 0 { t.Fatalf("retry errors=%#v", second.DeliveryErrors) }
	if rt.sessionVitalsRevision[s.ID][1] != 1 { t.Fatalf("revision=%d want=1", rt.sessionVitalsRevision[s.ID][1]) }
	if len(connection.sent) != 1 {
		t.Fatalf("sent=%d want=1 successful vitals", len(connection.sent))
	}
	if _, ok := connection.sent[0].Message.(protocol.EntityVitalsState); !ok {
		t.Fatalf("message=%T want EntityVitalsState", connection.sent[0].Message)
	}
}

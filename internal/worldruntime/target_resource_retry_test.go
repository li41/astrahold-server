package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
)

type targetResourceRetryConnection struct {
	backpressureOnce bool
	sent             []protocol.Envelope
}

func (c *targetResourceRetryConnection) TrySend(envelope protocol.Envelope) error {
	if envelope.Message.Type() == protocol.MessageCharacterTargetResourceState && c.backpressureOnce {
		c.backpressureOnce = false
		return session.ErrBackpressure
	}
	c.sent = append(c.sent, envelope)
	return nil
}

func (*targetResourceRetryConnection) Close() error { return nil }

type persistentTargetResourceBackpressureConnection struct {
	attempts int
}

func (c *persistentTargetResourceBackpressureConnection) TrySend(envelope protocol.Envelope) error {
	if envelope.Message.Type() == protocol.MessageCharacterTargetResourceState {
		c.attempts++
		return session.ErrBackpressure
	}
	return nil
}

func (*persistentTargetResourceBackpressureConnection) Close() error { return nil }

func TestRetryPendingTargetResourceFeedback(t *testing.T) {
	rt := newTargetResourceRetryRuntime(t)
	connection := &targetResourceRetryConnection{backpressureOnce: true}
	s, err := session.New(1, 1, 20, connection)
	if err != nil { t.Fatal(err) }
	if err := rt.sessions.Add(s); err != nil { t.Fatal(err) }

	message := protocol.CharacterTargetResourceState{
		SourceEntityID: 1,
		TargetEntityID: 2,
		ResourceID:     "flaw",
		Current:        2,
		Max:            3,
	}
	first := StepReport{Tick: 1}
	rt.sendTargetResourceState(s.ID, message, &first)
	if len(first.DeliveryErrors) != 0 { t.Fatalf("backpressure should defer, errors=%#v", first.DeliveryErrors) }
	if got := len(rt.pendingResourceMessages[s.ID]); got != 1 { t.Fatalf("pending target resource messages=%d want=1", got) }
	if len(connection.sent) != 0 { t.Fatalf("backpressured target resource state was sent: %d envelopes", len(connection.sent)) }

	second := StepReport{Tick: 2}
	rt.retryPendingResourceMessages(2, &second)
	if len(second.DeliveryErrors) != 0 { t.Fatalf("retry delivery errors=%#v", second.DeliveryErrors) }
	if _, ok := rt.pendingResourceMessages[s.ID]; ok { t.Fatalf("pending target resource queue not drained: %#v", rt.pendingResourceMessages[s.ID]) }
	if len(connection.sent) != 1 { t.Fatalf("sent=%d want=1", len(connection.sent)) }
	envelope := connection.sent[0]
	got, ok := envelope.Message.(protocol.CharacterTargetResourceState)
	if !ok { t.Fatalf("message=%T want CharacterTargetResourceState", envelope.Message) }
	if envelope.Delivery != protocol.DeliveryReliableOrdered || envelope.ServerTick != 2 || got != message {
		t.Fatalf("retried envelope=%#v want tick=2 message=%#v", envelope, message)
	}
}

func TestRetryAttemptsPendingTargetResourceOncePerPass(t *testing.T) {
	rt := newTargetResourceRetryRuntime(t)
	connection := &persistentTargetResourceBackpressureConnection{}
	s, err := session.New(1, 1, 20, connection)
	if err != nil { t.Fatal(err) }
	if err := rt.sessions.Add(s); err != nil { t.Fatal(err) }

	message := protocol.CharacterTargetResourceState{
		SourceEntityID: 1,
		TargetEntityID: 2,
		ResourceID:     "flaw",
		Current:        1,
		Max:            3,
	}
	first := StepReport{Tick: 1}
	rt.sendTargetResourceState(s.ID, message, &first)
	if connection.attempts != 1 { t.Fatalf("initial attempts=%d want=1", connection.attempts) }
	if got := len(rt.pendingResourceMessages[s.ID]); got != 1 { t.Fatalf("pending target resource messages=%d want=1", got) }

	connection.attempts = 0
	second := StepReport{Tick: 2}
	rt.retryPendingResourceMessages(2, &second)
	if connection.attempts != 1 { t.Fatalf("retry attempts=%d want=1", connection.attempts) }
	if got := len(rt.pendingResourceMessages[s.ID]); got != 1 { t.Fatalf("pending after retry=%d want=1", got) }

	connection.attempts = 0
	third := StepReport{Tick: 3}
	rt.retryPendingResourceMessages(3, &third)
	if connection.attempts != 1 { t.Fatalf("second retry attempts=%d want=1", connection.attempts) }
	if got := len(rt.pendingResourceMessages[s.ID]); got != 1 { t.Fatalf("pending after second retry=%d want=1", got) }
}

func newTargetResourceRetryRuntime(t *testing.T) *Runtime {
	t.Helper()
	nav := navigation.Plane{MinX: -10, MaxX: 10, MinZ: -10, MaxZ: 10, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 100
	return New(sim, cfg)
}

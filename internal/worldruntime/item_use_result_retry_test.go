package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestItemUseResultBackpressureRetriesWithoutReapplyingGameplay(t *testing.T) {
	runtime, _, s := newItemDropTestRuntime(t, world.Position{})
	connection := s.Connection().(*session.QueueConnection)
	drainReliable(connection)

	if _, err := runtime.characters.ApplyDamage(s.EntityID, 400); err != nil {
		t.Fatal(err)
	}
	inv := runtime.inventories[s.CharacterIdentity.ID]
	fillReliableQueue(t, s, connection)

	if err := runtime.EnqueueUseItem(s.ID, 1, protocol.ClientUseItem{ItemArchetypeID: "item_minor_healing_potion"}); err != nil {
		t.Fatal(err)
	}
	first := runtime.Step(2, 50*time.Millisecond)
	if len(first.CommandErrors) != 0 {
		t.Fatalf("first errors=%#v", first.CommandErrors)
	}
	if len(first.DeliveryErrors) != 0 {
		t.Fatalf("temporary backpressure must defer item-use feedback, errors=%#v", first.DeliveryErrors)
	}
	state, _ := runtime.characters.State(s.EntityID)
	if state.HP != 850 || inv.Quantity("item_minor_healing_potion") != 4 {
		t.Fatalf("first mutation hp=%d quantity=%d", state.HP, inv.Quantity("item_minor_healing_potion"))
	}
	if pending := runtime.pendingItemUseResults[s.ID]; len(pending) != 1 {
		t.Fatalf("pending results=%#v want exactly one", pending)
	}
	for _, envelope := range drainReliable(connection) {
		if _, ok := envelope.Message.(protocol.ItemUseResult); ok {
			t.Fatalf("backpressured ItemUseResult unexpectedly entered reliable queue: %#v", envelope)
		}
	}

	second := runtime.Step(3, 50*time.Millisecond)
	if len(second.CommandErrors) != 0 || len(second.DeliveryErrors) != 0 {
		t.Fatalf("retry command_errors=%#v delivery_errors=%#v", second.CommandErrors, second.DeliveryErrors)
	}
	state, _ = runtime.characters.State(s.EntityID)
	if state.HP != 850 || inv.Quantity("item_minor_healing_potion") != 4 {
		t.Fatalf("retry reapplied gameplay hp=%d quantity=%d", state.HP, inv.Quantity("item_minor_healing_potion"))
	}
	result := requireSingleItemUseResult(t, connection)
	if result.ClientActionSequence != 1 || result.ItemArchetypeID != "item_minor_healing_potion" || result.Outcome != protocol.ItemUseOutcomeUsed || result.AppliedAmount != 250 || result.CooldownReadyTick != 42 {
		t.Fatalf("retried result=%#v", result)
	}
	if _, pending := runtime.pendingItemUseResults[s.ID]; pending {
		t.Fatalf("delivered result remained pending=%#v", runtime.pendingItemUseResults[s.ID])
	}

	runtime.Step(4, 50*time.Millisecond)
	for _, envelope := range drainReliable(connection) {
		if result, ok := envelope.Message.(protocol.ItemUseResult); ok {
			t.Fatalf("delivered result duplicated on later tick: %#v", result)
		}
	}
}

func TestItemUseResultBacklogIsBoundedAndClosesSourceBeforeFurtherMutation(t *testing.T) {
	runtime, _, s := newItemDropTestRuntime(t, world.Position{})
	connection := s.Connection().(*session.QueueConnection)
	drainReliable(connection)
	if _, err := runtime.characters.ApplyDamage(s.EntityID, 400); err != nil {
		t.Fatal(err)
	}
	inv := runtime.inventories[s.CharacterIdentity.ID]
	fillReliableQueue(t, s, connection)

	for sequence := uint32(1); sequence <= maxPendingItemUseResultsPerSession+1; sequence++ {
		if err := runtime.EnqueueUseItem(s.ID, sequence, protocol.ClientUseItem{ItemArchetypeID: "item_minor_healing_potion"}); err != nil {
			t.Fatalf("enqueue sequence %d: %v", sequence, err)
		}
	}
	report := runtime.Step(2, 50*time.Millisecond)

	var backlogErrors int
	for _, commandErr := range report.CommandErrors {
		if errors.Is(commandErr.Err, ErrItemUseFeedbackBacklog) {
			backlogErrors++
		}
	}
	if backlogErrors != 1 {
		t.Fatalf("backlog errors=%d command_errors=%#v", backlogErrors, report.CommandErrors)
	}
	select {
	case <-connection.Done():
	default:
		t.Fatal("source connection remained open after item-use feedback backlog reached its cap")
	}
	if got := s.LastProcessedActionSequence(); got != maxPendingItemUseResultsPerSession {
		t.Fatalf("last processed action=%d want=%d", got, maxPendingItemUseResultsPerSession)
	}
	state, _ := runtime.characters.State(s.EntityID)
	if state.HP != 850 || inv.Quantity("item_minor_healing_potion") != 4 {
		t.Fatalf("overflow command mutated gameplay hp=%d quantity=%d", state.HP, inv.Quantity("item_minor_healing_potion"))
	}
	if pending := len(runtime.pendingItemUseResults[s.ID]); pending > maxPendingItemUseResultsPerSession {
		t.Fatalf("pending item-use results=%d exceeds cap=%d", pending, maxPendingItemUseResultsPerSession)
	}
}

func fillReliableQueue(t *testing.T, s *session.Session, connection *session.QueueConnection) {
	t.Helper()
	capacity := cap(connection.Reliable())
	if capacity <= 0 {
		t.Fatal("reliable queue has no capacity")
	}
	for i := 0; i < capacity; i++ {
		err := connection.TrySend(protocol.Envelope{
			Delivery:   protocol.DeliveryReliableOrdered,
			Sequence:   s.NextOutboundSequence(protocol.DeliveryReliableOrdered),
			ServerTick: 1,
			Message:    protocol.InventorySnapshot{},
		})
		if err != nil {
			t.Fatalf("fill reliable queue at %d/%d: %v", i, capacity, err)
		}
	}
}

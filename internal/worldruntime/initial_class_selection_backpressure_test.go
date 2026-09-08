package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

func TestInitialClassSelectionCompletionBackpressureRetriesWithoutReapplyingPersistence(t *testing.T) {
	rt, outbox, fence := joinInitialClassAssignmentCharacter(t)
	conn := reliableConnectionForFence(t, rt, fence)
	drainReliable(conn)

	if err := rt.EnqueueFencedInitialClassSelection(fence, 21, protocol.ClientInitialClassSelection{ClassID: string(classid.Ranger)}); err != nil {
		t.Fatal(err)
	}
	prepare := rt.Step(2, 50*time.Millisecond)
	if len(prepare.CommandErrors) != 0 {
		t.Fatalf("prepare errors=%#v", prepare.CommandErrors)
	}
	pending := outbox.Pending(1)
	if len(pending) != 1 {
		t.Fatalf("pending=%#v", pending)
	}

	fillReliableQueue(t, mustSessionForFence(t, rt, fence), conn)
	durable := pending[0]
	if err := outbox.Confirm(durable.IntentID); err != nil {
		t.Fatal(err)
	}
	outbox.Complete(durable)

	completion := rt.Step(3, 50*time.Millisecond)
	if len(completion.CommandErrors) != 0 || len(completion.DeliveryErrors) != 0 {
		t.Fatalf("completion command_errors=%#v delivery_errors=%#v", completion.CommandErrors, completion.DeliveryErrors)
	}
	state, _ := rt.characters.State(fence.EntityID)
	if state.ClassID != classid.Ranger {
		t.Fatalf("durable completion did not commit live class=%q", state.ClassID)
	}
	if got := outbox.Pending(0); len(got) != 0 {
		t.Fatalf("durable intent remained pending after completion=%#v", got)
	}
	queued := rt.pendingClassMessages[fence.SessionID]
	if len(queued) != 2 {
		t.Fatalf("pending class feedback=%#v want state+result", queued)
	}
	if _, ok := queued[0].(protocol.CharacterClassState); !ok {
		t.Fatalf("first pending feedback=%T want CharacterClassState", queued[0])
	}
	if result, ok := queued[1].(protocol.InitialClassSelectionResult); !ok || result.Outcome != protocol.InitialClassSelectionCommitted || result.ClientActionSequence != 21 {
		t.Fatalf("second pending feedback=%#v", queued[1])
	}

	// Drain only the synthetic filler. No class feedback was allowed into the full transport queue.
	for _, envelope := range drainReliable(conn) {
		switch envelope.Message.(type) {
		case protocol.CharacterClassState, protocol.InitialClassSelectionResult:
			t.Fatalf("backpressured class feedback unexpectedly entered reliable queue: %#v", envelope)
		}
	}

	retry := rt.Step(4, 50*time.Millisecond)
	if len(retry.CommandErrors) != 0 || len(retry.DeliveryErrors) != 0 {
		t.Fatalf("retry command_errors=%#v delivery_errors=%#v", retry.CommandErrors, retry.DeliveryErrors)
	}
	state, _ = rt.characters.State(fence.EntityID)
	if state.ClassID != classid.Ranger {
		t.Fatalf("retry changed authoritative class=%q", state.ClassID)
	}
	if got := outbox.Pending(0); len(got) != 0 {
		t.Fatalf("retry re-enqueued persistence=%#v", got)
	}
	messages := classProtocolMessages(drainReliable(conn))
	if len(messages) != 2 {
		t.Fatalf("retried class messages=%#v", messages)
	}
	if stateMessage, ok := messages[0].(protocol.CharacterClassState); !ok || stateMessage.ClassID != string(classid.Ranger) {
		t.Fatalf("retried state=%#v", messages[0])
	}
	if result, ok := messages[1].(protocol.InitialClassSelectionResult); !ok || result.ClassID != string(classid.Ranger) || result.Outcome != protocol.InitialClassSelectionCommitted || result.ClientActionSequence != 21 {
		t.Fatalf("retried result=%#v", messages[1])
	}
	if _, exists := rt.pendingClassMessages[fence.SessionID]; exists {
		t.Fatalf("delivered class feedback remained pending=%#v", rt.pendingClassMessages[fence.SessionID])
	}

	rt.Step(5, 50*time.Millisecond)
	for _, message := range classProtocolMessages(drainReliable(conn)) {
		t.Fatalf("class completion feedback duplicated on later tick: %#v", message)
	}
}

func mustSessionForFence(t *testing.T, rt *Runtime, fence SessionOwnershipFence) *session.Session {
	t.Helper()
	s, ok := rt.sessions.Get(fence.SessionID)
	if !ok {
		t.Fatalf("session %d missing", fence.SessionID)
	}
	return s
}

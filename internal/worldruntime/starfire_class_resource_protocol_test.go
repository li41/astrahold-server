package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/protocol"
)

func TestStarfireInitialClassCommitPublishesStarHeatBeforeResult(t *testing.T) {
	rt, outbox, fence := joinInitialClassAssignmentCharacter(t)
	conn := reliableConnectionForFence(t, rt, fence)
	drainReliable(conn)

	if err := rt.EnqueueFencedInitialClassSelection(fence, 34, protocol.ClientInitialClassSelection{ClassID: string(classid.StarfireMage)}); err != nil {
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
	if state, _ := rt.characters.State(fence.EntityID); state.ClassID != "" || state.ClassResourceID != "" {
		t.Fatalf("live class/resource mutated before durability: %+v", state)
	}

	durable := pending[0]
	if err := outbox.Confirm(durable.IntentID); err != nil {
		t.Fatal(err)
	}
	outbox.Complete(durable)
	completion := rt.Step(3, 50*time.Millisecond)
	if len(completion.CommandErrors) != 0 || len(completion.DeliveryErrors) != 0 {
		t.Fatalf("completion command_errors=%#v delivery_errors=%#v", completion.CommandErrors, completion.DeliveryErrors)
	}
	state, ok := rt.characters.State(fence.EntityID)
	if !ok || state.ClassID != classid.StarfireMage || state.ClassResourceID != "star_heat" || state.ClassResource != 0 || state.MaxClassResource != 100 {
		t.Fatalf("committed live class/resource=%+v ok=%v", state, ok)
	}

	messages := make([]protocol.Message, 0, 3)
	for _, envelope := range drainReliable(conn) {
		switch envelope.Message.(type) {
		case protocol.CharacterClassState, protocol.CharacterClassResourceState, protocol.InitialClassSelectionResult:
			messages = append(messages, envelope.Message)
		}
	}
	if len(messages) != 3 {
		t.Fatalf("messages=%#v, want class state + resource state + committed result", messages)
	}
	classState, ok := messages[0].(protocol.CharacterClassState)
	if !ok || classState.ClassID != string(classid.StarfireMage) {
		t.Fatalf("first message=%#v", messages[0])
	}
	resourceState, ok := messages[1].(protocol.CharacterClassResourceState)
	if !ok || resourceState.EntityID != fence.EntityID || resourceState.ResourceID != "star_heat" || resourceState.Current != 0 || resourceState.Max != 100 {
		t.Fatalf("second message=%#v", messages[1])
	}
	result, ok := messages[2].(protocol.InitialClassSelectionResult)
	if !ok || result.ClientActionSequence != 34 || result.ClassID != string(classid.StarfireMage) || result.Outcome != protocol.InitialClassSelectionCommitted {
		t.Fatalf("third message=%#v", messages[2])
	}
}

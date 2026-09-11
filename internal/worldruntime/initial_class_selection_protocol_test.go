package worldruntime

import (
	"errors"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/characterstate"
	"github.com/li41/astrahold-server/internal/classid"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func TestInitialClassSelectionProtocolCommitsOnlyAfterDurability(t *testing.T) {
	rt, outbox, fence := joinInitialClassAssignmentCharacter(t)
	conn := reliableConnectionForFence(t, rt, fence)
	drainReliable(conn)

	intent := protocol.ClientInitialClassSelection{ClassID: string(classid.Ranger)}
	if err := rt.EnqueueFencedInitialClassSelection(fence, 7, intent); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("prepare errors=%#v", report.CommandErrors)
	}
	if state, _ := rt.characters.State(fence.EntityID); state.ClassID != "" {
		t.Fatalf("class mutated before durable completion=%q", state.ClassID)
	}
	for _, envelope := range drainReliable(conn) {
		if _, ok := envelope.Message.(protocol.InitialClassSelectionResult); ok {
			t.Fatalf("selection result emitted before durability: %#v", envelope)
		}
	}
	pending := outbox.Pending(1)
	if len(pending) != 1 || !pending[0].CompletionRequested {
		t.Fatalf("pending=%#v", pending)
	}
	transaction, ok := rt.initialClassSelectionFeedback[pending[0].IntentID]
	if !ok || transaction.target != classid.Ranger || transaction.clientActionSequence != 7 || transaction.ownership != fence {
		t.Fatalf("transaction=%#v ok=%v", transaction, ok)
	}

	durable := pending[0]
	if err := outbox.Confirm(durable.IntentID); err != nil {
		t.Fatal(err)
	}
	outbox.Complete(durable)
	report = rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("completion errors=%#v", report.CommandErrors)
	}
	if state, _ := rt.characters.State(fence.EntityID); state.ClassID != classid.Ranger {
		t.Fatalf("completed class=%q", state.ClassID)
	}
	if _, ok := rt.initialClassSelectionFeedback[durable.IntentID]; ok {
		t.Fatal("completed selection retained process-local transaction")
	}

	messages := classProtocolMessages(drainReliable(conn))
	if len(messages) != 2 {
		t.Fatalf("class messages=%#v", messages)
	}
	stateMessage, ok := messages[0].(protocol.CharacterClassState)
	if !ok || stateMessage.ClassID != string(classid.Ranger) {
		t.Fatalf("first class message=%#v", messages[0])
	}
	result, ok := messages[1].(protocol.InitialClassSelectionResult)
	if !ok || result.ClientActionSequence != 7 || result.ClassID != string(classid.Ranger) || result.Outcome != protocol.InitialClassSelectionCommitted || result.Reason != "" {
		t.Fatalf("selection result=%#v", messages[1])
	}
}

func TestInitialClassSelectionProtocolRejectsUnknownClassWithoutSave(t *testing.T) {
	rt, outbox, fence := joinInitialClassAssignmentCharacter(t)
	conn := reliableConnectionForFence(t, rt, fence)
	drainReliable(conn)

	if err := rt.EnqueueFencedInitialClassSelection(fence, 9, protocol.ClientInitialClassSelection{ClassID: "class_typo"}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, character.ErrInvalidClassAssignment) {
		t.Fatalf("errors=%#v", report.CommandErrors)
	}
	if len(outbox.Pending(0)) != 0 {
		t.Fatalf("rejected selection wrote save=%#v", outbox.Pending(0))
	}
	if state, _ := rt.characters.State(fence.EntityID); state.ClassID != "" {
		t.Fatalf("rejected selection changed class=%q", state.ClassID)
	}
	messages := classProtocolMessages(drainReliable(conn))
	if len(messages) != 1 {
		t.Fatalf("class messages=%#v", messages)
	}
	result, ok := messages[0].(protocol.InitialClassSelectionResult)
	if !ok || result.ClientActionSequence != 9 || result.Outcome != protocol.InitialClassSelectionRejected || result.Reason != protocol.InitialClassSelectionInvalidClass || result.ClassID != "" {
		t.Fatalf("result=%#v", messages[0])
	}
}

func TestInitialClassSelectionProtocolRejectsEphemeralIdentity(t *testing.T) {
	rt, outbox, sess, conn := joinEphemeralClassSelectionCharacter(t)
	drainReliable(conn)

	if err := rt.EnqueueInitialClassSelection(sess.ID, 3, protocol.ClientInitialClassSelection{ClassID: string(classid.Oathguard)}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 1 || !errors.Is(report.CommandErrors[0].Err, ErrInitialClassAssignmentRequiresTrustedIdentity) {
		t.Fatalf("errors=%#v", report.CommandErrors)
	}
	if len(outbox.Pending(0)) != 0 {
		t.Fatalf("ephemeral selection wrote save=%#v", outbox.Pending(0))
	}
	messages := classProtocolMessages(drainReliable(conn))
	if len(messages) != 1 {
		t.Fatalf("class messages=%#v", messages)
	}
	result, ok := messages[0].(protocol.InitialClassSelectionResult)
	if !ok || result.Outcome != protocol.InitialClassSelectionRejected || result.Reason != protocol.InitialClassSelectionTrustedIdentityRequired {
		t.Fatalf("result=%#v", messages[0])
	}
}

func TestInitialClassSelectionCompletionAfterTakeoverSendsStateNotOldResult(t *testing.T) {
	rt, outbox, oldFence := joinInitialClassAssignmentCharacter(t)
	oldSession, ok := rt.sessions.Get(oldFence.SessionID)
	if !ok {
		t.Fatal("old session missing")
	}
	oldConn := reliableConnectionForFence(t, rt, oldFence)
	drainReliable(oldConn)

	if err := rt.EnqueueFencedInitialClassSelection(oldFence, 11, protocol.ClientInitialClassSelection{ClassID: string(classid.Shadowblade)}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(2, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("prepare errors=%#v", report.CommandErrors)
	}
	pending := outbox.Pending(1)
	if len(pending) != 1 {
		t.Fatalf("pending=%#v", pending)
	}

	newConn := session.NewQueueConnection(32, 32)
	replacement, err := session.NewWithCharacterIdentity(2, oldFence.EntityID, oldSession.CharacterIdentity, 64, newConn)
	if err != nil {
		t.Fatal(err)
	}
	newFence := SessionOwnershipFence{}
	if err := rt.applyOwnershipTransfer(OwnershipTransferRequest{Expected: oldFence, Replacement: replacement, Result: &newFence}); err != nil {
		t.Fatal(err)
	}
	if !newFence.Valid() || newFence == oldFence {
		t.Fatalf("new fence=%#v", newFence)
	}

	durable := pending[0]
	if err := outbox.Confirm(durable.IntentID); err != nil {
		t.Fatal(err)
	}
	outbox.Complete(durable)
	report := rt.Step(3, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("completion errors=%#v", report.CommandErrors)
	}
	if state, _ := rt.characters.State(oldFence.EntityID); state.ClassID != classid.Shadowblade {
		t.Fatalf("completed class=%q", state.ClassID)
	}

	newMessages := classProtocolMessages(drainReliable(newConn))
	if len(newMessages) == 0 {
		t.Fatal("replacement received no class state")
	}
	lastState := ""
	for _, message := range newMessages {
		switch m := message.(type) {
		case protocol.CharacterClassState:
			lastState = m.ClassID
		case protocol.InitialClassSelectionResult:
			t.Fatalf("replacement received old owner's correlated result=%#v", m)
		}
	}
	if lastState != string(classid.Shadowblade) {
		t.Fatalf("replacement final class state=%q messages=%#v", lastState, newMessages)
	}
	for _, message := range classProtocolMessages(drainReliable(oldConn)) {
		if result, ok := message.(protocol.InitialClassSelectionResult); ok && result.ClientActionSequence == 11 {
			t.Fatalf("stale owner received completion result=%#v", result)
		}
	}
}

func reliableConnectionForFence(t *testing.T, rt *Runtime, fence SessionOwnershipFence) *session.QueueConnection {
	t.Helper()
	s, ok := rt.sessions.Get(fence.SessionID)
	if !ok {
		t.Fatalf("session %d missing", fence.SessionID)
	}
	conn, ok := s.Connection().(*session.QueueConnection)
	if !ok {
		t.Fatalf("connection=%T", s.Connection())
	}
	return conn
}

func classProtocolMessages(envelopes []protocol.Envelope) []protocol.Message {
	out := make([]protocol.Message, 0)
	for _, envelope := range envelopes {
		switch envelope.Message.(type) {
		case protocol.CharacterClassState, protocol.InitialClassSelectionResult:
			out = append(out, envelope.Message)
		}
	}
	return out
}

func joinEphemeralClassSelectionCharacter(t *testing.T) (*Runtime, *characterstate.Outbox, *session.Session, *session.QueueConnection) {
	t.Helper()
	outbox, err := characterstate.NewOutbox(8)
	if err != nil {
		t.Fatal(err)
	}
	nav := navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100, Layer: 0}
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(nav, 0.1))
	cfg := DefaultConfig()
	cfg.SnapshotEveryTicks = 1000
	cfg.CharacterStateAutosaveEveryTicks = 0
	rt := New(sim, cfg, WithCharacterStateOutbox(outbox, characterStateTestWorld))
	conn := session.NewQueueConnection(32, 32)
	sess, err := session.New(1, 1, 64, conn)
	if err != nil {
		t.Fatal(err)
	}
	request := JoinRequest{
		Session: sess,
		Entity: world.EntityState{ID: 1, Kind: world.EntityPlayer, Transform: world.Transform{Position: world.Position{Layer: 0}}},
		Speed: 6, Radius: 0.35, MaxStepHeight: 0.5,
	}
	if err := rt.EnqueueJoin(request); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join errors=%#v", report.CommandErrors)
	}
	return rt, outbox, sess, conn
}

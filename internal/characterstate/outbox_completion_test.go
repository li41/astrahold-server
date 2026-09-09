package characterstate

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/classid"
)

func TestSaveOutboxCompletionReservationSurvivesJournalConfirmUntilWorldConfirm(t *testing.T) {
	outbox, err := NewOutbox(2)
	if err != nil {
		t.Fatal(err)
	}
	identity := trusted(t, "character:class-completion")
	snapshot := testSnapshot()
	snapshot.ClassID = classid.Oathguard
	intent, err := outbox.EnqueueWithCompletion(identity, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !intent.CompletionRequested || outbox.CompletionDepth() != 0 {
		t.Fatalf("intent=%#v completion_depth=%d", intent, outbox.CompletionDepth())
	}
	if reserved, ok := outbox.CompletionForCharacter(identity.ID); !ok || reserved != intent {
		t.Fatalf("reservation=%#v ok=%v", reserved, ok)
	}
	if _, err := outbox.EnqueueWithCompletion(identity, snapshot); !errors.Is(err, ErrSaveCompletionPending) {
		t.Fatalf("duplicate completion err=%v", err)
	}

	if err := outbox.Confirm(intent.IntentID); err != nil {
		t.Fatal(err)
	}
	if outbox.Depth() != 0 {
		t.Fatalf("pending depth=%d", outbox.Depth())
	}
	if reserved, ok := outbox.CompletionForCharacter(identity.ID); !ok || reserved != intent {
		t.Fatalf("reservation after journal confirm=%#v ok=%v", reserved, ok)
	}

	outbox.Complete(intent)
	completed := outbox.Completed(0)
	if len(completed) != 1 || completed[0] != intent || outbox.CompletionDepth() != 1 {
		t.Fatalf("completed=%#v depth=%d", completed, outbox.CompletionDepth())
	}
	if reserved, ok := outbox.CompletionForCharacter(identity.ID); !ok || reserved != intent {
		t.Fatalf("reservation before world confirm=%#v ok=%v", reserved, ok)
	}
	if err := outbox.ConfirmCompletion(intent.IntentID); err != nil {
		t.Fatal(err)
	}
	if outbox.CompletionDepth() != 0 {
		t.Fatalf("completion depth=%d", outbox.CompletionDepth())
	}
	if _, ok := outbox.CompletionForCharacter(identity.ID); ok {
		t.Fatal("world confirmation did not release character completion reservation")
	}
}

func TestSaveOutboxCompletionConfirmIsOrderedAndNormalSaveDoesNotReserve(t *testing.T) {
	outbox, err := NewOutbox(3)
	if err != nil {
		t.Fatal(err)
	}
	firstIdentity := trusted(t, "character:completion-first")
	secondIdentity := trusted(t, "character:completion-second")
	firstSnapshot := testSnapshot()
	firstSnapshot.ClassID = classid.Ranger
	secondSnapshot := testSnapshot()
	secondSnapshot.ClassID = classid.Breaker
	first, err := outbox.EnqueueWithCompletion(firstIdentity, firstSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	second, err := outbox.EnqueueWithCompletion(secondIdentity, secondSnapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := outbox.Confirm(first.IntentID); err != nil {
		t.Fatal(err)
	}
	if err := outbox.Confirm(second.IntentID); err != nil {
		t.Fatal(err)
	}
	outbox.Complete(first)
	outbox.Complete(second)
	if err := outbox.ConfirmCompletion(second.IntentID); !errors.Is(err, ErrCompletionConfirmOutOfOrder) {
		t.Fatalf("out-of-order completion err=%v", err)
	}
	if err := outbox.ConfirmCompletion(first.IntentID); err != nil {
		t.Fatal(err)
	}
	if err := outbox.ConfirmCompletion(second.IntentID); err != nil {
		t.Fatal(err)
	}

	normalIdentity := trusted(t, "character:normal-save")
	normal, err := outbox.Enqueue(normalIdentity, testSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if normal.CompletionRequested {
		t.Fatal("normal save unexpectedly requested completion")
	}
	if _, ok := outbox.CompletionForCharacter(normalIdentity.ID); ok {
		t.Fatal("normal save reserved completion transaction")
	}
}

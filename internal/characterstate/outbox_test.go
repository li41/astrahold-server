package characterstate

import (
	"errors"
	"testing"
)

func TestSaveOutboxPendingIsNonDestructiveAndConfirmOrdered(t *testing.T) {
	outbox, err := NewOutbox(2)
	if err != nil {
		t.Fatal(err)
	}
	first, err := outbox.Enqueue(trusted(t, "character:first"), testSnapshot())
	if err != nil || first.IntentID != 1 {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	secondSnapshot := testSnapshot()
	secondSnapshot.HP = 800
	second, err := outbox.Enqueue(trusted(t, "character:second"), secondSnapshot)
	if err != nil || second.IntentID != 2 {
		t.Fatalf("second=%#v err=%v", second, err)
	}
	pending := outbox.Pending(1)
	if len(pending) != 1 || pending[0] != first || outbox.Depth() != 2 {
		t.Fatalf("pending=%#v depth=%d", pending, outbox.Depth())
	}
	if err := outbox.Confirm(second.IntentID); !errors.Is(err, ErrSaveConfirmOutOfOrder) {
		t.Fatalf("out-of-order err=%v", err)
	}
	if err := outbox.Confirm(first.IntentID); err != nil {
		t.Fatal(err)
	}
	pending = outbox.Pending(0)
	if len(pending) != 1 || pending[0] != second {
		t.Fatalf("remaining=%#v", pending)
	}
}

func TestSaveOutboxFullAndInvalidIntentDoNotAdvanceIDs(t *testing.T) {
	outbox, _ := NewOutbox(1)
	first, err := outbox.Enqueue(trusted(t, "character:first"), testSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := outbox.Enqueue(trusted(t, "character:second"), testSnapshot()); !errors.Is(err, ErrSaveOutboxFull) {
		t.Fatalf("full err=%v", err)
	}
	if err := outbox.Confirm(first.IntentID); err != nil {
		t.Fatal(err)
	}
	second, err := outbox.Enqueue(trusted(t, "character:second"), testSnapshot())
	if err != nil || second.IntentID != 2 {
		t.Fatalf("second=%#v err=%v", second, err)
	}
}

func TestSaveOutboxRejectsEphemeralAndInvalidSnapshot(t *testing.T) {
	outbox, _ := NewOutbox(2)
	ephemeral, err := characteridentity.NewEphemeral()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := outbox.Enqueue(ephemeral, testSnapshot()); !errors.Is(err, ErrIdentityNotDurable) {
		t.Fatalf("ephemeral err=%v", err)
	}
	invalid := testSnapshot()
	invalid.HP = 0
	if _, err := outbox.Enqueue(trusted(t, "character:invalid"), invalid); !errors.Is(err, ErrInvalidSnapshot) {
		t.Fatalf("snapshot err=%v", err)
	}
	if outbox.Depth() != 0 {
		t.Fatalf("depth=%d", outbox.Depth())
	}
}

func TestNewSaveOutboxRejectsInvalidCapacity(t *testing.T) {
	if _, err := NewOutbox(0); !errors.Is(err, ErrInvalidOutboxCapacity) {
		t.Fatalf("err=%v", err)
	}
}

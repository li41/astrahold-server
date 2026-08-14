package deathoutcome

import (
	"errors"
	"testing"

	"github.com/li41/astrahold-server/internal/respawnpolicy"
	"github.com/li41/astrahold-server/internal/world"
)

func TestOutboxPendingIsNonDestructiveAndConfirmIsOrdered(t *testing.T) {
	outbox, err := NewOutbox(4)
	if err != nil {
		t.Fatal(err)
	}
	first, created, err := outbox.Enqueue(testEvent(7, 1))
	if err != nil || !created || first.EventID != 1 {
		t.Fatalf("first=%#v created=%v err=%v", first, created, err)
	}
	second, created, err := outbox.Enqueue(testEvent(8, 1))
	if err != nil || !created || second.EventID != 2 {
		t.Fatalf("second=%#v created=%v err=%v", second, created, err)
	}

	pending := outbox.Pending(1)
	if len(pending) != 1 || pending[0] != first || outbox.Depth() != 2 {
		t.Fatalf("pending=%#v depth=%d", pending, outbox.Depth())
	}
	if err := outbox.Confirm(second.EventID); !errors.Is(err, ErrConfirmOutOfOrder) {
		t.Fatalf("out of order confirm err=%v", err)
	}
	if err := outbox.Confirm(first.EventID); err != nil {
		t.Fatal(err)
	}
	pending = outbox.Pending(0)
	if len(pending) != 1 || pending[0] != second {
		t.Fatalf("after confirm=%#v", pending)
	}
}

func TestOutboxSameRevisionIsIdempotentAndConflictIsRejected(t *testing.T) {
	outbox, _ := NewOutbox(4)
	original, created, err := outbox.Enqueue(testEvent(7, 1))
	if err != nil || !created {
		t.Fatalf("original created=%v err=%v", created, err)
	}
	duplicate, created, err := outbox.Enqueue(testEvent(7, 1))
	if err != nil || created || duplicate != original || outbox.Depth() != 1 {
		t.Fatalf("duplicate=%#v created=%v depth=%d err=%v", duplicate, created, outbox.Depth(), err)
	}
	conflict := testEvent(7, 1)
	conflict.CheckpointForfeited = false
	if _, _, err := outbox.Enqueue(conflict); !errors.Is(err, ErrOutcomeConflict) {
		t.Fatalf("conflict err=%v", err)
	}
	if _, _, err := outbox.Enqueue(testEvent(7, 0)); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("invalid revision err=%v", err)
	}
}

func TestOutboxRevisionRegressionAndFullDoNotAdvanceTruth(t *testing.T) {
	outbox, _ := NewOutbox(1)
	if _, _, err := outbox.Enqueue(testEvent(7, 2)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := outbox.Enqueue(testEvent(7, 1)); !errors.Is(err, ErrRevisionRegression) {
		t.Fatalf("regression err=%v", err)
	}
	if _, _, err := outbox.Enqueue(testEvent(8, 1)); !errors.Is(err, ErrOutboxFull) {
		t.Fatalf("full err=%v", err)
	}
	first := outbox.Pending(1)[0]
	if err := outbox.Confirm(first.EventID); err != nil {
		t.Fatal(err)
	}
	stored, created, err := outbox.Enqueue(testEvent(8, 1))
	if err != nil || !created || stored.EventID != 2 {
		t.Fatalf("retry stored=%#v created=%v err=%v", stored, created, err)
	}
}

func TestOutboxResetEntityAllowsEntityIDReuseWithoutDroppingPending(t *testing.T) {
	outbox, _ := NewOutbox(4)
	old, _, err := outbox.Enqueue(testEvent(7, 1))
	if err != nil {
		t.Fatal(err)
	}
	outbox.ResetEntity(7)
	reused := testEvent(7, 1)
	reused.DefeatedTick = 20
	reused.Respawn.DueTick = 24
	fresh, created, err := outbox.Enqueue(reused)
	if err != nil || !created || fresh.EventID != 2 {
		t.Fatalf("reused=%#v created=%v err=%v", fresh, created, err)
	}
	pending := outbox.Pending(0)
	if len(pending) != 2 || pending[0] != old || pending[1] != fresh {
		t.Fatalf("pending=%#v", pending)
	}
}

func TestOutboxValidatesOutcomeShape(t *testing.T) {
	if _, err := NewOutbox(0); !errors.Is(err, ErrInvalidCapacity) {
		t.Fatalf("capacity err=%v", err)
	}
	outbox, _ := NewOutbox(4)
	invalid := testEvent(7, 1)
	invalid.Respawn.Scheduled = false
	if _, _, err := outbox.Enqueue(invalid); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("unscheduled binding err=%v", err)
	}
	invalid = testEvent(7, 1)
	invalid.PenaltyTransactionApplied = false
	if _, _, err := outbox.Enqueue(invalid); !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("penalty shape err=%v", err)
	}
}

func testEvent(entityID world.EntityID, revision uint64) Event {
	return Event{
		EntityID:                   entityID,
		DefeatRevision:             revision,
		Context:                    respawnpolicy.DeathContextPvE,
		DefeatedTick:               10,
		RespawnPolicyRevision:      "respawn-test",
		DeathPenaltyPolicyRevision: "penalty-test",
		Respawn: RespawnBinding{
			Scheduled:    true,
			SpawnPointID: "checkpoint",
			SpawnClass:   respawnpolicy.SpawnClassCheckpoint,
			Position:     world.Position{X: 12, Y: 1, Z: -5, Layer: 2},
			DueTick:      14,
		},
		PenaltyTransactionApplied: true,
		CheckpointForfeited:       true,
	}
}

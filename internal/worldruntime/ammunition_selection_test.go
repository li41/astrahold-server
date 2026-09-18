package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/ammunition"
	"github.com/li41/astrahold-server/internal/protocol"
)

func TestAmmunitionSelectionRequiresOwnedArrowAndPublishesState(t *testing.T) {
	rt, s, conn, _ := newBowAmmunitionRuntime(t, 6)

	if err := rt.EnqueueAmmunitionCommand(s.ID, 2, protocol.ClientAmmunitionCommand{
		Operation: protocol.AmmunitionOperationSelect,
		ItemArchetypeID: ammunition.ItemSilverArrow,
	}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(3, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("selection rejection command errors=%#v", report.CommandErrors)
	}
	rt.Step(4, 50*time.Millisecond) // flush queued Reliable result/state
	result := nextAmmunitionResult(t, conn)
	if result.Outcome != protocol.AmmunitionOutcomeRejected || result.Reason != protocol.AmmunitionRejectionInsufficientInventory {
		t.Fatalf("missing-arrow selection result=%#v", result)
	}

	inv := rt.inventories[s.CharacterIdentity.ID]
	if err := inv.Add(ammunition.ItemSilverArrow, 1); err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueueAmmunitionCommand(s.ID, 3, protocol.ClientAmmunitionCommand{
		Operation: protocol.AmmunitionOperationSelect,
		ItemArchetypeID: ammunition.ItemSilverArrow,
	}); err != nil {
		t.Fatal(err)
	}
	if report := rt.Step(5, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("selection command errors=%#v", report.CommandErrors)
	}
	rt.Step(6, 50*time.Millisecond) // flush queued Reliable result/state
	result = nextAmmunitionResult(t, conn)
	if result.Outcome != protocol.AmmunitionOutcomeSelected || result.ItemArchetypeID != ammunition.ItemSilverArrow {
		t.Fatalf("selected result=%#v", result)
	}
	state := nextAmmunitionState(t, conn, ammunition.ItemSilverArrow)
	if state.SelectedItemArchetypeID != ammunition.ItemSilverArrow {
		t.Fatalf("selected state=%#v", state)
	}
}

func nextAmmunitionResult(t *testing.T, conn interface{ Reliable() <-chan protocol.Envelope }) protocol.AmmunitionResult {
	t.Helper()
	for i := 0; i < 256; i++ {
		select {
		case envelope := <-conn.Reliable():
			if result, ok := envelope.Message.(protocol.AmmunitionResult); ok {
				return result
			}
		default:
			t.Fatal("ammunition result missing")
		}
	}
	t.Fatal("ammunition result missing")
	return protocol.AmmunitionResult{}
}

func nextAmmunitionState(t *testing.T, conn interface{ Reliable() <-chan protocol.Envelope }, selected string) protocol.AmmunitionState {
	t.Helper()
	for i := 0; i < 256; i++ {
		select {
		case envelope := <-conn.Reliable():
			if state, ok := envelope.Message.(protocol.AmmunitionState); ok && state.SelectedItemArchetypeID == selected {
				return state
			}
		default:
			t.Fatal("ammunition state missing")
		}
	}
	t.Fatal("ammunition state missing")
	return protocol.AmmunitionState{}
}

package skillloadout

import (
	"errors"
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/skillcatalog"
	"github.com/li41/astrahold-server/internal/world"
)

func TestSlotsCanonicalComparableForm(t *testing.T) {
	ids := []skillcatalog.ID{skillcatalog.HeavyStrike, skillcatalog.PiercingShot, skillcatalog.FireBolt}
	slots, err := NewSlots(ids)
	if err != nil {
		t.Fatalf("NewSlots() error = %v", err)
	}
	if err := slots.Validate(); err != nil {
		t.Fatalf("Slots.Validate() error = %v", err)
	}
	if got := slots.IDs(); !reflect.DeepEqual(got, ids) {
		t.Fatalf("Slots.IDs() = %v, want %v", got, ids)
	}
	if slots != slots {
		t.Fatal("Slots must remain comparable for durable snapshot equality")
	}

	nonCanonical := Slots{skillcatalog.HeavyStrike, "", skillcatalog.FireBolt}
	if err := nonCanonical.Validate(); !errors.Is(err, ErrNonCanonicalSlots) {
		t.Fatalf("non-canonical error = %v, want ErrNonCanonicalSlots", err)
	}
}

func TestStorePreservesOrderedMixedCombatLoadout(t *testing.T) {
	store := NewStore()
	entityID := world.EntityID(41)
	loadout := []skillcatalog.ID{
		skillcatalog.HeavyStrike,
		skillcatalog.PiercingShot,
		skillcatalog.FireBolt,
		skillcatalog.Flurry,
		skillcatalog.Volley,
		skillcatalog.Meteor,
	}

	if err := store.SetCombat(entityID, loadout); err != nil {
		t.Fatalf("SetCombat() error = %v", err)
	}
	if got := store.Combat(entityID); !reflect.DeepEqual(got, loadout) {
		t.Fatalf("Combat() = %v, want %v", got, loadout)
	}
	for _, id := range loadout {
		if !store.ContainsCombat(entityID, id) {
			t.Fatalf("ContainsCombat(%q) = false, want true", id)
		}
	}
}

func TestStoreCopiesInputAndOutput(t *testing.T) {
	store := NewStore()
	entityID := world.EntityID(42)
	input := []skillcatalog.ID{skillcatalog.HeavyStrike, skillcatalog.FireBolt}

	if err := store.SetCombat(entityID, input); err != nil {
		t.Fatalf("SetCombat() error = %v", err)
	}
	input[0] = skillcatalog.Meteor
	if got := store.Combat(entityID)[0]; got != skillcatalog.HeavyStrike {
		t.Fatalf("input mutation changed store: got %q", got)
	}

	output := store.Combat(entityID)
	output[0] = skillcatalog.Meteor
	if got := store.Combat(entityID)[0]; got != skillcatalog.HeavyStrike {
		t.Fatalf("output mutation changed store: got %q", got)
	}
}

func TestRejectedReplacementLeavesPreviousStateIntact(t *testing.T) {
	store := NewStore()
	entityID := world.EntityID(43)
	original := []skillcatalog.ID{skillcatalog.Cleave, skillcatalog.ArcaneLance}
	if err := store.SetCombat(entityID, original); err != nil {
		t.Fatalf("initial SetCombat() error = %v", err)
	}

	invalid := []skillcatalog.ID{skillcatalog.Cleave, skillcatalog.Guard}
	if err := store.SetCombat(entityID, invalid); !errors.Is(err, skillcatalog.ErrNotCombatLoadoutSkill) {
		t.Fatalf("replacement error = %v, want ErrNotCombatLoadoutSkill", err)
	}
	if got := store.Combat(entityID); !reflect.DeepEqual(got, original) {
		t.Fatalf("rejected replacement mutated state: got %v, want %v", got, original)
	}
}

func TestStoreRejectsInvalidEntityWithoutMutation(t *testing.T) {
	store := NewStore()
	if err := store.SetCombat(0, []skillcatalog.ID{skillcatalog.HeavyStrike}); !errors.Is(err, ErrInvalidEntity) {
		t.Fatalf("error = %v, want ErrInvalidEntity", err)
	}
	if got := store.Combat(0); got != nil {
		t.Fatalf("Combat(0) = %v, want nil", got)
	}
}

func TestEmptyLoadoutAndClearEntityRemoveRuntimeState(t *testing.T) {
	store := NewStore()
	entityA := world.EntityID(44)
	entityB := world.EntityID(45)
	if err := store.SetCombat(entityA, []skillcatalog.ID{skillcatalog.Execute}); err != nil {
		t.Fatalf("SetCombat(entityA) error = %v", err)
	}
	if err := store.SetCombat(entityB, []skillcatalog.ID{skillcatalog.RapidShot}); err != nil {
		t.Fatalf("SetCombat(entityB) error = %v", err)
	}

	if err := store.SetCombat(entityA, nil); err != nil {
		t.Fatalf("SetCombat(empty) error = %v", err)
	}
	if got := store.Combat(entityA); len(got) != 0 {
		t.Fatalf("entityA loadout after empty set = %v, want empty", got)
	}
	if !store.ContainsCombat(entityB, skillcatalog.RapidShot) {
		t.Fatal("clearing entityA affected entityB")
	}

	store.ClearEntity(entityB)
	if got := store.Combat(entityB); len(got) != 0 {
		t.Fatalf("entityB loadout after ClearEntity = %v, want empty", got)
	}
}

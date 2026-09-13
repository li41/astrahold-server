package learnedskills

import (
	"errors"
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/skillcatalog"
	"github.com/li41/astrahold-server/internal/world"
)

func TestStoreAcceptsWholeFormalCatalog(t *testing.T) {
	store := NewStore()
	entityID := world.EntityID(51)
	definitions := skillcatalog.All()
	input := make([]skillcatalog.ID, len(definitions))
	want := make([]skillcatalog.ID, len(definitions))
	for i, definition := range definitions {
		want[i] = definition.ID
		input[len(definitions)-1-i] = definition.ID
	}

	if err := store.SetLearned(entityID, input); err != nil {
		t.Fatalf("SetLearned() error = %v", err)
	}
	if got := store.Learned(entityID); !reflect.DeepEqual(got, want) {
		t.Fatalf("Learned() = %v, want catalog order %v", got, want)
	}
	if got := store.Count(entityID); got != len(definitions) {
		t.Fatalf("Count() = %d, want %d", got, len(definitions))
	}
	for _, id := range want {
		if !store.Contains(entityID, id) {
			t.Fatalf("Contains(%q) = false, want true", id)
		}
	}
}

func TestStoreDoesNotApplyCombatLoadoutEligibilityToLearnedSkills(t *testing.T) {
	store := NewStore()
	entityID := world.EntityID(52)
	input := []skillcatalog.ID{
		skillcatalog.Haste,
		skillcatalog.FireBolt,
		skillcatalog.RandomTeleport,
		skillcatalog.HeavyStrike,
		skillcatalog.StrongPhysique,
	}
	want := []skillcatalog.ID{
		skillcatalog.RandomTeleport,
		skillcatalog.HeavyStrike,
		skillcatalog.FireBolt,
		skillcatalog.StrongPhysique,
		skillcatalog.Haste,
	}

	if err := store.SetLearned(entityID, input); err != nil {
		t.Fatalf("SetLearned() error = %v", err)
	}
	if got := store.Learned(entityID); !reflect.DeepEqual(got, want) {
		t.Fatalf("Learned() = %v, want %v", got, want)
	}
}

func TestRejectedReplacementLeavesPreviousStateIntact(t *testing.T) {
	store := NewStore()
	entityID := world.EntityID(53)
	original := []skillcatalog.ID{skillcatalog.Heal, skillcatalog.Cleave, skillcatalog.SpiritConcentration}
	if err := store.SetLearned(entityID, original); err != nil {
		t.Fatalf("initial SetLearned() error = %v", err)
	}

	if err := store.SetLearned(entityID, []skillcatalog.ID{skillcatalog.Heal, "not-a-skill"}); !errors.Is(err, skillcatalog.ErrUnknownSkill) {
		t.Fatalf("unknown replacement error = %v, want ErrUnknownSkill", err)
	}
	if got := store.Learned(entityID); !reflect.DeepEqual(got, original) {
		t.Fatalf("unknown replacement mutated state: got %v, want %v", got, original)
	}

	if err := store.SetLearned(entityID, []skillcatalog.ID{skillcatalog.Heal, skillcatalog.Heal}); !errors.Is(err, skillcatalog.ErrDuplicateSkill) {
		t.Fatalf("duplicate replacement error = %v, want ErrDuplicateSkill", err)
	}
	if got := store.Learned(entityID); !reflect.DeepEqual(got, original) {
		t.Fatalf("duplicate replacement mutated state: got %v, want %v", got, original)
	}
}

func TestStoreIsolatesInputAndOutput(t *testing.T) {
	store := NewStore()
	entityID := world.EntityID(54)
	input := []skillcatalog.ID{skillcatalog.HeavyStrike, skillcatalog.FireBolt}
	if err := store.SetLearned(entityID, input); err != nil {
		t.Fatalf("SetLearned() error = %v", err)
	}

	input[0] = skillcatalog.Meteor
	if !store.Contains(entityID, skillcatalog.HeavyStrike) || store.Contains(entityID, skillcatalog.Meteor) {
		t.Fatal("input mutation changed authoritative state")
	}

	output := store.Learned(entityID)
	output[0] = skillcatalog.Meteor
	if !store.Contains(entityID, skillcatalog.HeavyStrike) || store.Contains(entityID, skillcatalog.Meteor) {
		t.Fatal("output mutation changed authoritative state")
	}
}

func TestStoreRejectsInvalidEntityAndNonEmptyNilStore(t *testing.T) {
	store := NewStore()
	if err := store.SetLearned(0, []skillcatalog.ID{skillcatalog.HeavyStrike}); !errors.Is(err, ErrInvalidEntity) {
		t.Fatalf("invalid entity error = %v, want ErrInvalidEntity", err)
	}

	var nilStore *Store
	if err := nilStore.SetLearned(world.EntityID(55), []skillcatalog.ID{skillcatalog.HeavyStrike}); !errors.Is(err, ErrNilStore) {
		t.Fatalf("nil store error = %v, want ErrNilStore", err)
	}
	if err := nilStore.SetLearned(world.EntityID(55), nil); err != nil {
		t.Fatalf("empty SetLearned() on nil store error = %v", err)
	}
}

func TestEmptySetAndClearEntityRemoveOnlyTargetRuntimeState(t *testing.T) {
	store := NewStore()
	entityA := world.EntityID(56)
	entityB := world.EntityID(57)
	if err := store.SetLearned(entityA, []skillcatalog.ID{skillcatalog.Execute}); err != nil {
		t.Fatalf("SetLearned(entityA) error = %v", err)
	}
	if err := store.SetLearned(entityB, []skillcatalog.ID{skillcatalog.Haste}); err != nil {
		t.Fatalf("SetLearned(entityB) error = %v", err)
	}

	if err := store.SetLearned(entityA, nil); err != nil {
		t.Fatalf("SetLearned(empty) error = %v", err)
	}
	if got := store.Learned(entityA); len(got) != 0 {
		t.Fatalf("entityA learned state after empty set = %v, want empty", got)
	}
	if !store.Contains(entityB, skillcatalog.Haste) {
		t.Fatal("clearing entityA affected entityB")
	}

	store.ClearEntity(entityB)
	if got := store.Count(entityB); got != 0 {
		t.Fatalf("entityB count after ClearEntity = %d, want 0", got)
	}
}

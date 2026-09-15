package learnedskills

import (
	"errors"
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/skillcatalog"
)

func TestSetCanonicalizesFormalCatalogOrder(t *testing.T) {
	set, err := NewSet([]skillcatalog.ID{
		skillcatalog.Haste,
		skillcatalog.FireBolt,
		skillcatalog.RandomTeleport,
		skillcatalog.HeavyStrike,
		skillcatalog.StrongPhysique,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []skillcatalog.ID{
		skillcatalog.RandomTeleport,
		skillcatalog.HeavyStrike,
		skillcatalog.FireBolt,
		skillcatalog.StrongPhysique,
		skillcatalog.Haste,
	}
	if got := set.IDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("IDs() = %v, want %v", got, want)
	}
	if set.Len() != len(want) {
		t.Fatalf("Len() = %d, want %d", set.Len(), len(want))
	}
	if !set.Contains(skillcatalog.FireBolt) || set.Contains(skillcatalog.Meteor) {
		t.Fatal("Contains() mismatch")
	}
	if err := set.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestSetAcceptsWholeFormalCatalog(t *testing.T) {
	definitions := skillcatalog.All()
	ids := make([]skillcatalog.ID, len(definitions))
	for i, definition := range definitions {
		ids[len(definitions)-1-i] = definition.ID
	}
	set, err := NewSet(ids)
	if err != nil {
		t.Fatal(err)
	}
	if set.Len() != len(definitions) {
		t.Fatalf("Len() = %d, want %d", set.Len(), len(definitions))
	}
	for _, definition := range definitions {
		if !set.Contains(definition.ID) {
			t.Fatalf("missing %q", definition.ID)
		}
	}
}

func TestSetRejectsUnknownDuplicateAndTooMany(t *testing.T) {
	if _, err := NewSet([]skillcatalog.ID{"unknown-skill"}); !errors.Is(err, skillcatalog.ErrUnknownSkill) {
		t.Fatalf("unknown error = %v", err)
	}
	if _, err := NewSet([]skillcatalog.ID{skillcatalog.HeavyStrike, skillcatalog.HeavyStrike}); !errors.Is(err, skillcatalog.ErrDuplicateSkill) {
		t.Fatalf("duplicate error = %v", err)
	}
	tooMany := make([]skillcatalog.ID, maxDurableSkills+1)
	if _, err := NewSet(tooMany); !errors.Is(err, ErrTooManySkills) {
		t.Fatalf("too many error = %v", err)
	}
}

func TestSetIDsDoesNotExposeInternalStorage(t *testing.T) {
	set, err := NewSet([]skillcatalog.ID{skillcatalog.HeavyStrike, skillcatalog.FireBolt})
	if err != nil {
		t.Fatal(err)
	}
	ids := set.IDs()
	ids[0] = skillcatalog.Meteor
	if !set.Contains(skillcatalog.HeavyStrike) || set.Contains(skillcatalog.Meteor) {
		t.Fatal("IDs() output mutation changed set")
	}
}

func TestZeroSetIsValidEmpty(t *testing.T) {
	var set Set
	if err := set.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if set.Len() != 0 || set.IDs() != nil {
		t.Fatalf("zero set = len %d ids %v", set.Len(), set.IDs())
	}
}

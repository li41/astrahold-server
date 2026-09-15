package equipmentstats

import (
	"errors"
	"math"
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

func TestAggregateStaticAndMergeWithRolledModifiers(t *testing.T) {
	fixed, err := AggregateStatic(
		equipmentcatalog.StaticModifier{ID: equipmentcatalog.StaticMagicPower, Value: 4},
		equipmentcatalog.StaticModifier{ID: equipmentcatalog.StaticStrength, Value: 2},
	)
	if err != nil {
		t.Fatal(err)
	}
	rolled := Modifiers{MagicPower: 3, CriticalRating: 1, Strength: 1}
	got, err := Merge(fixed, rolled)
	if err != nil {
		t.Fatal(err)
	}
	if got.MagicPower != 7 || got.Strength != 3 || got.CriticalRating != 1 {
		t.Fatalf("merged modifiers=%#v", got)
	}
}

func TestMergeRejectsModifierOverflow(t *testing.T) {
	_, err := Merge(Modifiers{MagicPower: math.MaxUint32}, Modifiers{MagicPower: 1})
	if !errors.Is(err, ErrOverflow) {
		t.Fatalf("err=%v", err)
	}
}

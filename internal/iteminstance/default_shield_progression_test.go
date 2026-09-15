package iteminstance

import (
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

func TestDefaultShieldProgressionCreatesAuthoritativeTierAffixes(t *testing.T) {
	catalog, err := equipmentcatalog.Default()
	if err != nil {
		t.Fatal(err)
	}

	midDefinition, ok := catalog.Resolve("item_mid_guard_shield")
	if !ok {
		t.Fatal("mid guard shield missing")
	}
	mid, err := Create("instance:mid-guard-shield", midDefinition, &sequenceRoller{values: []int{0, 0}})
	if err != nil {
		t.Fatal(err)
	}
	if len(mid.Affixes) != 1 {
		t.Fatalf("mid shield affix count=%d want=1", len(mid.Affixes))
	}

	highDefinition, ok := catalog.Resolve("item_high_runed_square_shield")
	if !ok {
		t.Fatal("high runed square shield missing")
	}
	high, err := Create("instance:high-runed-square-shield", highDefinition, &sequenceRoller{values: []int{0, 0, 0, 0}})
	if err != nil {
		t.Fatal(err)
	}
	if len(high.Affixes) != 2 {
		t.Fatalf("high shield affix count=%d want=2", len(high.Affixes))
	}
	if high.Affixes[0].ID == high.Affixes[1].ID {
		t.Fatalf("high shield duplicate affixes=%#v", high.Affixes)
	}
	if err := Validate(high, highDefinition); err != nil {
		t.Fatalf("high shield validation: %v", err)
	}
}

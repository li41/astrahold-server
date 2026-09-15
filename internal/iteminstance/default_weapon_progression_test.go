package iteminstance

import (
	"fmt"
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

func TestDefaultMidHighWeaponsCreateRequiredUniqueAffixes(t *testing.T) {
	catalog, err := equipmentcatalog.Default()
	if err != nil { t.Fatal(err) }
	weaponTypes := []equipmentcatalog.WeaponType{
		"one_hand_sword", "dagger", "one_hand_axe", "one_hand_spear", "warhammer", "morning_star", "mace",
		"two_hand_sword", "two_hand_axe", "two_hand_spear", "knuckles", "claw", "dual_blades", "bow", "crossbow", "sling", "staff",
	}
	for _, weaponType := range weaponTypes {
		for _, tc := range []struct{
			prefix string
			wantAffixes int
		}{
			{"item_mid_", 1},
			{"item_high_", 2},
		} {
			id := tc.prefix + string(weaponType)
			definition, ok := catalog.Resolve(id)
			if !ok { t.Fatalf("missing %s", id) }
			instance, err := Create(ID(fmt.Sprintf("instance:%s", id)), definition, &sequenceRoller{values: []int{0, 0, 0, 0}})
			if err != nil { t.Fatalf("Create %s: %v", id, err) }
			if len(instance.Affixes) != tc.wantAffixes { t.Fatalf("%s affix count=%d want=%d", id, len(instance.Affixes), tc.wantAffixes) }
			if err := Validate(instance, definition); err != nil { t.Fatalf("Validate %s: %v", id, err) }
		}
	}
}

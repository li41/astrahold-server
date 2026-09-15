package iteminstance

import (
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentaffix"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
)

type armorInstanceRoller struct{ calls int }

func (r *armorInstanceRoller) Intn(n int) int {
	r.calls++
	return 0
}

func TestCreateMidAndHighArmorInstances(t *testing.T) {
	catalog, err := equipmentcatalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		id        string
		wantCount int
	}{
		{"item_garrison_steel_helm", 1},
		{"item_starlight_robe", 2},
	}
	for _, tc := range cases {
		definition, ok := catalog.Resolve(tc.id)
		if !ok {
			t.Fatalf("missing %s", tc.id)
		}
		instance, err := Create(ID("instance:"+tc.id), definition, &armorInstanceRoller{})
		if err != nil {
			t.Fatalf("Create(%s): %v", tc.id, err)
		}
		if len(instance.Affixes) != tc.wantCount {
			t.Fatalf("%s affix count=%d want=%d", tc.id, len(instance.Affixes), tc.wantCount)
		}
		if err := Validate(instance, definition); err != nil {
			t.Fatalf("Validate(%s): %v", tc.id, err)
		}
		for _, affix := range instance.Affixes {
			if affix.ID == equipmentaffix.AffixPhysicalDamage {
				t.Fatalf("%s rolled forbidden physical damage affix", tc.id)
			}
		}
	}
}

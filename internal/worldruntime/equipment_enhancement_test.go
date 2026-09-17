package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/protocol"
)

func TestEnhancementScrollCompatibility(t *testing.T) {
	cases := []struct {
		name string
		scroll string
		kind equipmentcatalog.Kind
		want bool
	}{
		{"weapon weapon", WeaponEnhancementScrollItemArchetypeID, equipmentcatalog.KindWeapon, true},
		{"weapon armor", WeaponEnhancementScrollItemArchetypeID, equipmentcatalog.KindArmor, false},
		{"armor armor", ArmorEnhancementScrollItemArchetypeID, equipmentcatalog.KindArmor, true},
		{"armor shield", ArmorEnhancementScrollItemArchetypeID, equipmentcatalog.KindShield, true},
		{"armor weapon", ArmorEnhancementScrollItemArchetypeID, equipmentcatalog.KindWeapon, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := enhancementScrollAllows(tc.scroll, tc.kind); got != tc.want { t.Fatalf("got=%v want=%v", got, tc.want) }
		})
	}
}

func TestValidateEnhanceEquipmentIntentRejectsClientInventedScroll(t *testing.T) {
	if err := validateEnhanceEquipmentIntent(protocol.ClientEnhanceEquipment{ScrollItemArchetypeID: "item_fake_scroll", ItemInstanceID: "instance:weapon"}); err == nil {
		t.Fatal("fake scroll accepted")
	}
	if err := validateEnhanceEquipmentIntent(protocol.ClientEnhanceEquipment{ScrollItemArchetypeID: WeaponEnhancementScrollItemArchetypeID, ItemInstanceID: "instance:weapon"}); err != nil {
		t.Fatalf("valid intent rejected: %v", err)
	}
}

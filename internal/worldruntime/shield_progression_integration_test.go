package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentaffix"
	"github.com/li41/astrahold-server/internal/iteminstance"
)

func TestHighRunedShieldUniqueInstanceAppliesBaseAndRolledMitigation(t *testing.T) {
	runtime, s := runtimeWithJoinedPlayerForStaticModifierTest(t)
	definition, ok := defaultEquipmentCatalog.Resolve("item_high_runed_square_shield")
	if !ok {
		t.Fatal("high runed square shield missing")
	}
	instance := iteminstance.Instance{
		ID:              "item-instance:high-runed-shield-mitigation",
		ItemArchetypeID: definition.ItemArchetypeID,
		Affixes: []equipmentaffix.Affix{
			{ID: equipmentaffix.AffixMagicDefense, Strength: 2, Value: 2},
			{ID: equipmentaffix.AffixPhysicalDefense, Strength: 3, Value: 3},
		},
	}
	if err := iteminstance.Validate(instance, definition); err != nil {
		t.Fatal(err)
	}
	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		t.Fatal("inventory missing")
	}
	if err := inv.AddInstance(instance); err != nil {
		t.Fatal(err)
	}
	if err := inv.EquipOffHandInstance(instance.ID); err != nil {
		t.Fatal(err)
	}
	if got := inv.OffHand(); got != definition.ItemArchetypeID {
		t.Fatalf("off-hand archetype=%q want=%q", got, definition.ItemArchetypeID)
	}

	physical, err := runtime.resolveIncomingDamage(DamageRequest{
		TargetEntityID: s.EntityID,
		RawDamage:      100,
		DamageType:     combat.DamagePhysical,
		Blockable:      false,
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Shield physical defense 4 + rolled physical defense 3 = 7.
	// 100 * (1 - 7/(7+20)) = 74.07..., rounded once to 74.
	if physical.FinalDamage != 74 || physical.Blocked {
		t.Fatalf("physical result=%#v want damage=74 blocked=false", physical)
	}

	magic, err := runtime.resolveIncomingDamage(DamageRequest{
		TargetEntityID: s.EntityID,
		RawDamage:      100,
		DamageType:     combat.DamageMagic,
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Rolled magic defense 2 mitigates first, then the shield's independent 16%% magic reduction.
	// 100 * (1 - 2/(2+20)) * 0.84 = 76.36..., rounded once to 76.
	if magic.FinalDamage != 76 || magic.Blocked {
		t.Fatalf("magic result=%#v want damage=76 blocked=false", magic)
	}
}

package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentaffix"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/iteminstance"
)

func TestUniqueMainHandResolvesWeaponArchetypeForBasicAttackRules(t *testing.T) {
	runtime, s := runtimeWithJoinedPlayerForStaticModifierTest(t)
	definition, ok := defaultEquipmentCatalog.Resolve("item_mid_bow")
	if !ok { t.Fatal("mid bow missing") }
	instance := iteminstance.Instance{
		ID: "item-instance:mid-bow-basic-attack-test",
		ItemArchetypeID: definition.ItemArchetypeID,
		Affixes: []equipmentaffix.Affix{{ID: equipmentaffix.AffixMagicPower, Strength: 1, Value: 1}},
	}
	if err := iteminstance.Validate(instance, definition); err != nil { t.Fatal(err) }
	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil { t.Fatal("inventory missing") }
	if err := inv.AddInstance(instance); err != nil { t.Fatal(err) }
	if err := inv.EquipMainHandInstance(instance.ID); err != nil { t.Fatal(err) }
	if got := inv.MainHand(); got != "item_mid_bow" {
		t.Fatalf("unique main hand archetype=%q want=item_mid_bow", got)
	}

	resolved, ok := runtime.equippedCatalogWeapon(s.EntityID, s.ID)
	if !ok { t.Fatal("unique main-hand weapon did not resolve catalog archetype") }
	if resolved.ItemArchetypeID != "item_mid_bow" || resolved.Tier != equipmentcatalog.TierMid || resolved.Weapon == nil || resolved.Weapon.WeaponType != equipmentcatalog.WeaponTypeBow {
		t.Fatalf("resolved unique weapon=%#v", resolved)
	}
	if got := rollWeaponDamage(resolved, equipmentcatalog.BodySizeSmall, 0); got != 3 {
		t.Fatalf("mid bow base damage=%d want=3 before arrow damage", got)
	}
	if resolved.Weapon.AccuracyModifier != 1 {
		t.Fatalf("mid bow accuracy=%d want=1", resolved.Weapon.AccuracyModifier)
	}

	prepared := combat.PreparedAction{
		ActorEntityID: s.EntityID,
		Definition: combat.ActionDefinition{ID: basicAttackActionID, CooldownSeconds: .5, Range: 4.5},
		Target: combat.Target{Kind: combat.TargetEntity, ID: "999"},
	}
	runtime.applyEquippedBasicAttackTiming(&prepared, s.ID)
	runtime.applyEquippedBasicAttackRange(&prepared, s.ID)
	if prepared.Definition.CooldownSeconds != weaponAttackCooldownSeconds(1200) {
		t.Fatalf("mid bow cooldown=%v want=%v", prepared.Definition.CooldownSeconds, weaponAttackCooldownSeconds(1200))
	}
	if prepared.Definition.Range != 18 {
		t.Fatalf("mid bow range=%v want=18", prepared.Definition.Range)
	}
}

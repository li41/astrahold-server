package main

import (
	"testing"

	"github.com/li41/astrahold-server/internal/actionpolicy"
	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/characterstats"
	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

func TestE2ECharacterRestoreMatchesCurrentContract(t *testing.T) {
	identity, err := characteridentity.NewTrusted(e2eCharacterID)
	if err != nil {
		t.Fatal(err)
	}
	worldIdentity := e2eWorldIdentity()
	restore, err := e2eCharacterRestore(identity, worldIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if err := worldruntime.ValidateCharacterRestore(identity, restore, worldIdentity); err != nil {
		t.Fatalf("current browserwse2e restore rejected: %v", err)
	}
	if restore.PrimaryStats != characterstats.DefaultPrimary() {
		t.Fatalf("primary stats=%+v, want neutral current-schema stats=%+v", restore.PrimaryStats, characterstats.DefaultPrimary())
	}
	equipment, err := restore.Inventory.Equipment()
	if err != nil {
		t.Fatal(err)
	}
	if !restore.Inventory.Initialized || len(equipment) != 1 || equipment[0].Slot != "main_hand" || equipment[0].ItemArchetypeID != e2eBasicAttackWeaponID {
		t.Fatalf("inventory=%+v equipment=%+v, want equipped main hand %q", restore.Inventory, equipment, e2eBasicAttackWeaponID)
	}
}

func TestE2EBasicAttackWeaponUsesAuthoredOneHandSwordCadence(t *testing.T) {
	intervalMS, err := e2eBasicAttackIntervalMS()
	if err != nil {
		t.Fatal(err)
	}
	if intervalMS != 900 {
		t.Fatalf("basic-attack interval=%dms, want formally authored 900ms", intervalMS)
	}

	catalog, err := equipmentcatalog.Default()
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := catalog.Resolve(e2eBasicAttackWeaponID)
	if !ok || definition.Weapon == nil {
		t.Fatalf("missing weapon definition for %q", e2eBasicAttackWeaponID)
	}
	if definition.Weapon.WeaponType != equipmentcatalog.WeaponTypeOneHandSword {
		t.Fatalf("weapon type=%q, want %q", definition.Weapon.WeaponType, equipmentcatalog.WeaponTypeOneHandSword)
	}
}

func TestE2ECombatActionsPreserveShadowbladeAndBasicAttack(t *testing.T) {
	actions := e2eCombatActions()
	seenShadowblade := false
	seenBasicAttack := false
	for _, action := range actions {
		switch action.ID {
		case actionpolicy.ShadowbladeDualBladeStrike:
			seenShadowblade = true
		case e2eBasicAttackActionID:
			seenBasicAttack = true
		}
	}
	if !seenShadowblade || !seenBasicAttack {
		t.Fatalf("actions=%+v, shadowblade=%v basic-attack=%v", actions, seenShadowblade, seenBasicAttack)
	}
}

func TestE2EBasicAttackTargetIsIndependentFromShadowbladeTarget(t *testing.T) {
	spawn := e2eBasicAttackTargetSpawn()
	if spawn.Entity.ID != e2eBasicAttackTargetID || spawn.Entity.ID == e2eTargetID {
		t.Fatalf("basic-attack target id=%d shadowblade target id=%d", spawn.Entity.ID, e2eTargetID)
	}
	if spawn.BodySize != equipmentcatalog.BodySizeSmall || spawn.HP != 1000 || spawn.MaxHP != 1000 {
		t.Fatalf("basic-attack target spawn=%+v", spawn)
	}
}

package worldruntime

import (
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/equipmentaffix"
	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/movement"
	"github.com/li41/astrahold-server/internal/navigation"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/simulation"
	"github.com/li41/astrahold-server/internal/spatial"
	"github.com/li41/astrahold-server/internal/world"
)

func runtimeWithEquippedMidTierDamageAffix(t *testing.T, affix equipmentaffix.Affix) (*Runtime, *session.Session) {
	t.Helper()
	itemID := withMidTierRestoreCatalog(t)
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1))
	config := DefaultConfig()
	config.SnapshotEveryTicks = 1000
	runtime := New(sim, config)
	connection := session.NewQueueConnection(32, 8)
	s, err := session.New(41, 410, 42, connection)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtime.EnqueueJoin(JoinRequest{
		Session: s,
		Entity: world.EntityState{ID: 410, Kind: world.EntityPlayer},
		Speed: 6,
		Radius: 0.35,
		MaxStepHeight: 0.5,
	}); err != nil {
		t.Fatal(err)
	}
	if report := runtime.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 {
		t.Fatalf("join errors: %#v", report.CommandErrors)
	}
	<-connection.Reliable()
	<-connection.Reliable()

	instance := iteminstance.Instance{
		ID: "item-instance:flat-damage-test",
		ItemArchetypeID: itemID,
		Affixes: []equipmentaffix.Affix{affix},
	}
	definition, ok := defaultEquipmentCatalog.Resolve(itemID)
	if !ok {
		t.Fatal("test item missing")
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
	if err := inv.EquipMainHandInstance(instance.ID); err != nil {
		t.Fatal(err)
	}
	return runtime, s
}

func TestEquippedPhysicalDamageAffixAppliesToDirectPhysicalAction(t *testing.T) {
	runtime, s := runtimeWithEquippedMidTierDamageAffix(t, equipmentaffix.Affix{
		ID: equipmentaffix.AffixPhysicalDamage,
		Strength: 2,
		Value: 2,
	})
	prepared := combat.PreparedAction{
		Definition: combat.ActionDefinition{ID: "shatter-strike", Effect: combat.EffectDamage},
		Target: combat.Target{Kind: combat.TargetEntity, ID: "999"},
		Damage: combat.Damage{Type: combat.DamagePhysical, Amount: 100},
	}
	if got := runtime.resolveEquippedBasicAttackDamage(s.EntityID, s.ID, 999, prepared); got != 102 {
		t.Fatalf("physical damage with equipped affix=%d want=102", got)
	}
}

func TestEquippedMagicPowerAffixAppliesToDirectMagicAction(t *testing.T) {
	runtime, s := runtimeWithEquippedMidTierDamageAffix(t, equipmentaffix.Affix{
		ID: equipmentaffix.AffixMagicPower,
		Strength: 2,
		Value: 2,
	})
	prepared := combat.PreparedAction{
		Definition: combat.ActionDefinition{ID: "fireball", Effect: combat.EffectDamage},
		Target: combat.Target{Kind: combat.TargetEntity, ID: "999"},
		Damage: combat.Damage{Type: combat.DamageMagic, Amount: 150},
	}
	if got := runtime.resolveEquippedBasicAttackDamage(s.EntityID, s.ID, 999, prepared); got != 152 {
		t.Fatalf("magic damage with equipped affix=%d want=152", got)
	}
}

func TestEquippedFlatDamageAffixDoesNotCrossDamageTypes(t *testing.T) {
	runtime, s := runtimeWithEquippedMidTierDamageAffix(t, equipmentaffix.Affix{
		ID: equipmentaffix.AffixMagicPower,
		Strength: 2,
		Value: 2,
	})
	prepared := combat.PreparedAction{
		Definition: combat.ActionDefinition{ID: "shatter-strike", Effect: combat.EffectDamage},
		Target: combat.Target{Kind: combat.TargetEntity, ID: "999"},
		Damage: combat.Damage{Type: combat.DamagePhysical, Amount: 100},
	}
	if got := runtime.resolveEquippedBasicAttackDamage(s.EntityID, s.ID, 999, prepared); got != 100 {
		t.Fatalf("magic power changed physical damage: got=%d want=100", got)
	}
}

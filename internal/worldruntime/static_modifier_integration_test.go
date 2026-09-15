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

func runtimeWithJoinedPlayerForStaticModifierTest(t *testing.T) (*Runtime, *session.Session) {
	t.Helper()
	sim := simulation.New(spatial.NewGrid(16), movement.NewService(navigation.Plane{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, 0.1))
	config := DefaultConfig()
	config.SnapshotEveryTicks = 1000
	runtime := New(sim, config)
	connection := session.NewQueueConnection(32, 8)
	s, err := session.New(71, 710, 72, connection)
	if err != nil { t.Fatal(err) }
	if err := runtime.EnqueueJoin(JoinRequest{Session: s, Entity: world.EntityState{ID: 710, Kind: world.EntityPlayer}, Speed: 6, Radius: 0.35, MaxStepHeight: 0.5}); err != nil { t.Fatal(err) }
	if report := runtime.Step(1, 50*time.Millisecond); len(report.CommandErrors) != 0 { t.Fatalf("join errors: %#v", report.CommandErrors) }
	return runtime, s
}

func magicPreparedDamage(amount uint32) combat.PreparedAction {
	return combat.PreparedAction{
		Definition: combat.ActionDefinition{ID: "fireball", Effect: combat.EffectDamage},
		Target: combat.Target{Kind: combat.TargetEntity, ID: "999"},
		Damage: combat.Damage{Type: combat.DamageMagic, Amount: amount},
	}
}

func TestLowStaffStaticMagicPowerAppliesWithoutItemInstance(t *testing.T) {
	runtime, s := runtimeWithJoinedPlayerForStaticModifierTest(t)
	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil { t.Fatal("inventory missing") }
	if err := inv.Add("item_apprentice_wood_staff", 1); err != nil { t.Fatal(err) }
	if err := inv.EquipMainHand("item_apprentice_wood_staff"); err != nil { t.Fatal(err) }
	if got := runtime.resolveEquippedBasicAttackDamage(s.EntityID, s.ID, 999, magicPreparedDamage(150)); got != 151 {
		t.Fatalf("low staff magic damage=%d want=151", got)
	}
}

func TestMidStaffStaticMagicPowerStacksWithRolledAffix(t *testing.T) {
	runtime, s := runtimeWithJoinedPlayerForStaticModifierTest(t)
	definition, ok := defaultEquipmentCatalog.Resolve("item_mid_staff")
	if !ok { t.Fatal("mid staff missing") }
	instance := iteminstance.Instance{
		ID: "item-instance:mid-staff-static-test",
		ItemArchetypeID: definition.ItemArchetypeID,
		Affixes: []equipmentaffix.Affix{{ID: equipmentaffix.AffixMagicPower, Strength: 2, Value: 2}},
	}
	if err := iteminstance.Validate(instance, definition); err != nil { t.Fatal(err) }
	inv := runtime.inventories[s.CharacterIdentity.ID]
	if inv == nil { t.Fatal("inventory missing") }
	if err := inv.AddInstance(instance); err != nil { t.Fatal(err) }
	if err := inv.EquipMainHandInstance(instance.ID); err != nil { t.Fatal(err) }
	if got := runtime.resolveEquippedBasicAttackDamage(s.EntityID, s.ID, 999, magicPreparedDamage(150)); got != 154 {
		t.Fatalf("mid staff static+rolled magic damage=%d want=154", got)
	}
}

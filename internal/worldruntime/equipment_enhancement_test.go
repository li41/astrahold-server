package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/inventory"
	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/protocol"
)

func TestEnhancementScrollCompatibility(t *testing.T) {
	cases := []struct {
		name   string
		scroll string
		kind   equipmentcatalog.Kind
		want   bool
	}{
		{"weapon weapon", WeaponEnhancementScrollItemArchetypeID, equipmentcatalog.KindWeapon, true},
		{"weapon armor", WeaponEnhancementScrollItemArchetypeID, equipmentcatalog.KindArmor, false},
		{"armor armor", ArmorEnhancementScrollItemArchetypeID, equipmentcatalog.KindArmor, true},
		{"armor shield", ArmorEnhancementScrollItemArchetypeID, equipmentcatalog.KindShield, true},
		{"armor weapon", ArmorEnhancementScrollItemArchetypeID, equipmentcatalog.KindWeapon, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := enhancementScrollAllows(tc.scroll, tc.kind); got != tc.want {
				t.Fatalf("got=%v want=%v", got, tc.want)
			}
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

func TestEquipmentEnhancementWeaponProbabilityTable(t *testing.T) {
	cases := []struct {
		level                  uint16
		success, nochange, brk int
	}{
		{0, 100, 0, 0},
		{5, 100, 0, 0},
		{6, 60, 20, 20},
		{7, 50, 25, 25},
		{8, 40, 30, 30},
		{9, 30, 35, 35},
		{10, 20, 40, 40},
		{11, 10, 45, 45},
		{12, 1, 39, 60},
		{13, 1, 39, 60},
		{500, 1, 39, 60},
	}
	for _, tc := range cases {
		got, ok := equipmentEnhancementChanceFor(equipmentcatalog.KindWeapon, tc.level)
		if !ok {
			t.Fatalf("level %d: no chance table", tc.level)
		}
		if got.Success != tc.success || got.NoChange != tc.nochange || got.Break != tc.brk {
			t.Fatalf("level %d: got=%+v want=%d/%d/%d", tc.level, got, tc.success, tc.nochange, tc.brk)
		}
		if got.Success+got.NoChange+got.Break != 100 {
			t.Fatalf("level %d: chance sum=%d", tc.level, got.Success+got.NoChange+got.Break)
		}
	}
}

func TestEquipmentEnhancementArmorAndShieldProbabilityTable(t *testing.T) {
	cases := []struct {
		level                  uint16
		success, nochange, brk int
	}{
		{0, 100, 0, 0},
		{3, 100, 0, 0},
		{4, 60, 15, 25},
		{5, 50, 20, 30},
		{6, 40, 25, 35},
		{7, 30, 30, 40},
		{8, 20, 35, 45},
		{9, 10, 40, 50},
		{10, 1, 39, 60},
		{11, 1, 39, 60},
		{500, 1, 39, 60},
	}
	for _, kind := range []equipmentcatalog.Kind{equipmentcatalog.KindArmor, equipmentcatalog.KindShield} {
		for _, tc := range cases {
			got, ok := equipmentEnhancementChanceFor(kind, tc.level)
			if !ok {
				t.Fatalf("kind %q level %d: no chance table", kind, tc.level)
			}
			if got.Success != tc.success || got.NoChange != tc.nochange || got.Break != tc.brk {
				t.Fatalf("kind %q level %d: got=%+v want=%d/%d/%d", kind, tc.level, got, tc.success, tc.nochange, tc.brk)
			}
			if got.Success+got.NoChange+got.Break != 100 {
				t.Fatalf("kind %q level %d: chance sum=%d", kind, tc.level, got.Success+got.NoChange+got.Break)
			}
		}
	}
}

func TestResolveEquipmentEnhancementRollBoundaries(t *testing.T) {
	cases := []struct {
		name    string
		kind    equipmentcatalog.Kind
		level   uint16
		roll    int
		outcome protocol.EquipmentEnhancementOutcome
	}{
		{"safe weapon always succeeds", equipmentcatalog.KindWeapon, 5, 100, protocol.EquipmentEnhancementOutcomeEnhanced},
		{"weapon risk success upper", equipmentcatalog.KindWeapon, 6, 60, protocol.EquipmentEnhancementOutcomeEnhanced},
		{"weapon risk nochange lower", equipmentcatalog.KindWeapon, 6, 61, protocol.EquipmentEnhancementOutcomeNoChange},
		{"weapon risk nochange upper", equipmentcatalog.KindWeapon, 6, 80, protocol.EquipmentEnhancementOutcomeNoChange},
		{"weapon risk break lower", equipmentcatalog.KindWeapon, 6, 81, protocol.EquipmentEnhancementOutcomeBroken},
		{"weapon risk break upper", equipmentcatalog.KindWeapon, 6, 100, protocol.EquipmentEnhancementOutcomeBroken},
		{"armor risk nochange upper", equipmentcatalog.KindArmor, 4, 75, protocol.EquipmentEnhancementOutcomeNoChange},
		{"armor risk break lower", equipmentcatalog.KindArmor, 4, 76, protocol.EquipmentEnhancementOutcomeBroken},
		{"weapon tail success", equipmentcatalog.KindWeapon, 120, 1, protocol.EquipmentEnhancementOutcomeEnhanced},
		{"weapon tail nochange lower", equipmentcatalog.KindWeapon, 120, 2, protocol.EquipmentEnhancementOutcomeNoChange},
		{"weapon tail nochange upper", equipmentcatalog.KindWeapon, 120, 40, protocol.EquipmentEnhancementOutcomeNoChange},
		{"weapon tail break lower", equipmentcatalog.KindWeapon, 120, 41, protocol.EquipmentEnhancementOutcomeBroken},
		{"shield tail break", equipmentcatalog.KindShield, 120, 100, protocol.EquipmentEnhancementOutcomeBroken},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := resolveEquipmentEnhancementRoll(tc.kind, tc.level, tc.roll)
			if !ok {
				t.Fatal("roll rejected")
			}
			if got != tc.outcome {
				t.Fatalf("got=%q want=%q", got, tc.outcome)
			}
		})
	}
}

func TestResolveEquipmentEnhancementRollRejectsInvalidInput(t *testing.T) {
	if _, ok := resolveEquipmentEnhancementRoll(equipmentcatalog.KindWeapon, 6, 0); ok {
		t.Fatal("roll 0 accepted")
	}
	if _, ok := resolveEquipmentEnhancementRoll(equipmentcatalog.KindWeapon, 6, 101); ok {
		t.Fatal("roll 101 accepted")
	}
	if _, ok := resolveEquipmentEnhancementRoll(equipmentcatalog.Kind("unknown"), 0, 1); ok {
		t.Fatal("unknown kind accepted")
	}
}

func newEnhancementMutationInventory(t *testing.T, scrollID string, level uint16) (*inventory.Inventory, iteminstance.Instance) {
	t.Helper()
	inv := inventory.NewWithWeightPolicy(8, inventory.WeightPolicy{DefaultUnitWeight: 1})
	if err := inv.Add(scrollID, 1); err != nil {
		t.Fatalf("Add scroll: %v", err)
	}
	instance := iteminstance.Instance{ID: "instance:enhancement-target", ItemArchetypeID: "item_test_equipment", EnhancementLevel: level}
	if err := inv.AddInstance(instance); err != nil {
		t.Fatalf("AddInstance: %v", err)
	}
	return inv, instance
}

func TestCommitEquipmentEnhancementOutcomeEnhancedConsumesOneScrollAndKeepsIdentity(t *testing.T) {
	inv, instance := newEnhancementMutationInventory(t, WeaponEnhancementScrollItemArchetypeID, 11)
	current, err := commitEquipmentEnhancementOutcome(inv, WeaponEnhancementScrollItemArchetypeID, instance, protocol.EquipmentEnhancementOutcomeEnhanced)
	if err != nil {
		t.Fatalf("commitEquipmentEnhancementOutcome: %v", err)
	}
	if current != 12 {
		t.Fatalf("current=%d want=12", current)
	}
	if got := inv.Quantity(WeaponEnhancementScrollItemArchetypeID); got != 0 {
		t.Fatalf("scroll quantity=%d want=0", got)
	}
	updated, _, ok := inv.OwnedInstance(instance.ID)
	if !ok {
		t.Fatal("enhanced instance disappeared")
	}
	if updated.ID != instance.ID || updated.EnhancementLevel != 12 {
		t.Fatalf("updated=%+v", updated)
	}
}

func TestCommitEquipmentEnhancementOutcomeNoChangeConsumesScrollOnly(t *testing.T) {
	inv, instance := newEnhancementMutationInventory(t, WeaponEnhancementScrollItemArchetypeID, 12)
	current, err := commitEquipmentEnhancementOutcome(inv, WeaponEnhancementScrollItemArchetypeID, instance, protocol.EquipmentEnhancementOutcomeNoChange)
	if err != nil {
		t.Fatalf("commitEquipmentEnhancementOutcome: %v", err)
	}
	if current != 12 {
		t.Fatalf("current=%d want=12", current)
	}
	if got := inv.Quantity(WeaponEnhancementScrollItemArchetypeID); got != 0 {
		t.Fatalf("scroll quantity=%d want=0", got)
	}
	updated, _, ok := inv.OwnedInstance(instance.ID)
	if !ok || updated.EnhancementLevel != 12 {
		t.Fatalf("nochange target=%+v owned=%v", updated, ok)
	}
}

func TestCommitEquipmentEnhancementOutcomeBrokenConsumesScrollAndDestroysExactInstance(t *testing.T) {
	inv, instance := newEnhancementMutationInventory(t, WeaponEnhancementScrollItemArchetypeID, 12)
	current, err := commitEquipmentEnhancementOutcome(inv, WeaponEnhancementScrollItemArchetypeID, instance, protocol.EquipmentEnhancementOutcomeBroken)
	if err != nil {
		t.Fatalf("commitEquipmentEnhancementOutcome: %v", err)
	}
	if current != 12 {
		t.Fatalf("current=%d want=12", current)
	}
	if got := inv.Quantity(WeaponEnhancementScrollItemArchetypeID); got != 0 {
		t.Fatalf("scroll quantity=%d want=0", got)
	}
	if _, _, ok := inv.OwnedInstance(instance.ID); ok {
		t.Fatal("broken target still owned")
	}
}

func TestCommitEquipmentEnhancementOutcomeBrokenEquippedTargetClearsSlot(t *testing.T) {
	inv, instance := newEnhancementMutationInventory(t, WeaponEnhancementScrollItemArchetypeID, 12)
	if err := inv.EquipInstance(inventory.SlotMainHand, instance.ID); err != nil {
		t.Fatalf("EquipInstance: %v", err)
	}
	beforeEquipmentRevision := inv.EquipmentRevision()
	if _, err := commitEquipmentEnhancementOutcome(inv, WeaponEnhancementScrollItemArchetypeID, instance, protocol.EquipmentEnhancementOutcomeBroken); err != nil {
		t.Fatalf("commitEquipmentEnhancementOutcome: %v", err)
	}
	if _, ok := inv.EquippedInstance(inventory.SlotMainHand); ok {
		t.Fatal("broken target remained equipped")
	}
	if got := inv.EquipmentRevision(); got != beforeEquipmentRevision+1 {
		t.Fatalf("equipment revision=%d want=%d", got, beforeEquipmentRevision+1)
	}
}

func TestCommitEquipmentEnhancementOutcomeRejectsInvalidOutcomeWithoutConsuming(t *testing.T) {
	inv, instance := newEnhancementMutationInventory(t, WeaponEnhancementScrollItemArchetypeID, 12)
	if _, err := commitEquipmentEnhancementOutcome(inv, WeaponEnhancementScrollItemArchetypeID, instance, protocol.EquipmentEnhancementOutcomeRejected); err == nil {
		t.Fatal("rejected outcome unexpectedly committed")
	}
	if got := inv.Quantity(WeaponEnhancementScrollItemArchetypeID); got != 1 {
		t.Fatalf("scroll quantity=%d want=1", got)
	}
	if _, _, ok := inv.OwnedInstance(instance.ID); !ok {
		t.Fatal("invalid outcome removed target")
	}
}

package worldruntime

import (
	"reflect"
	"testing"

	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/loot"
	"github.com/li41/astrahold-server/internal/world"
)

func TestResolveMonsterLootQuantityIsDeterministicAndWithinClosedRange(t *testing.T) {
	var secret monsterLootRollSecret
	secret[0] = 0x71
	secret[31] = 0x19
	drops := []loot.Drop{{
		Kind:              loot.DropKindStack,
		ItemArchetypeID:   "item_low_arrow",
		ChanceBasisPoints: loot.ChanceBasisPointsScale,
		QuantityMin:       4,
		QuantityMax:       8,
	}}
	first := resolveMonsterLootDropsWithSecret(secret, world.EntityID(9001), 5, drops)
	second := resolveMonsterLootDropsWithSecret(secret, world.EntityID(9001), 5, drops)
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("resolved first=%#v second=%#v", first, second)
	}
	if first[0].quantity < 4 || first[0].quantity > 8 {
		t.Fatalf("quantity=%d outside 4..8", first[0].quantity)
	}
	if first[0].quantity != second[0].quantity {
		t.Fatalf("same Server secret/context changed quantity: %d vs %d", first[0].quantity, second[0].quantity)
	}
}

func TestMaterializeMonsterLootInstanceIsStableForRetryAndRollsOneMidAffix(t *testing.T) {
	var secret monsterLootRollSecret
	secret[0] = 0x22
	secret[31] = 0xa4
	resolved := monsterLootResolvedDrop{
		drop: loot.Drop{
			Kind:              loot.DropKindEquipmentInstance,
			ItemArchetypeID:   "item_mid_guard_shield",
			ChanceBasisPoints: 167,
			QuantityMin:       1,
			QuantityMax:       1,
		},
		authoredIndex: 11,
		quantity:      1,
	}
	first, err := materializeMonsterLootPayloadWithSecret(secret, 9022, 4, resolved)
	if err != nil {
		t.Fatal(err)
	}
	second, err := materializeMonsterLootPayloadWithSecret(secret, 9022, 4, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("retry rematerialized different payload: first=%+v second=%+v", first, second)
	}
	if first.Instance.ID == "" || len(first.Instance.Affixes) != 1 {
		t.Fatalf("mid instance=%+v", first.Instance)
	}
	definition, ok := defaultEquipmentCatalog.Resolve(first.ItemArchetypeID)
	if !ok {
		t.Fatalf("missing equipment %s", first.ItemArchetypeID)
	}
	if err := iteminstance.Validate(first.Instance, definition); err != nil {
		t.Fatalf("instance validation: %v", err)
	}

	other := resolved
	other.authoredIndex++
	third, err := materializeMonsterLootPayloadWithSecret(secret, 9022, 4, other)
	if err != nil {
		t.Fatal(err)
	}
	if third.Instance.ID == first.Instance.ID {
		t.Fatalf("different authored drop indices reused instance id %q", first.Instance.ID)
	}
}

func TestMonsterLootAutoGrantUsesResolvedStackQuantity(t *testing.T) {
	rt, sim, s := newItemDropTestRuntime(t, world.Position{})
	payload := stackItemDropPayload("item_gold_coin", 7)
	dropID, err := rt.spawnExpiringItemDropPayload(payload, world.Position{X: 1, Layer: 0}, 1)
	if err != nil {
		t.Fatal(err)
	}
	report := StepReport{Tick: 2}
	ok := rt.tryAutoGrantMonsterLoot(monsterLootCandidate{
		sessionID: s.ID, characterID: s.CharacterIdentity.ID, damage: 1,
	}, payload, dropID, &report)
	if !ok || len(report.CommandErrors) != 0 {
		t.Fatalf("auto grant ok=%v errors=%#v", ok, report.CommandErrors)
	}
	if got := rt.inventories[s.CharacterIdentity.ID].Quantity("item_gold_coin"); got != 7 {
		t.Fatalf("gold quantity=%d want=7", got)
	}
	if _, exists := sim.Entity(dropID); exists {
		t.Fatal("auto-granted quantity drop still exists")
	}
	if _, exists := rt.itemDropPayloads[dropID]; exists {
		t.Fatal("auto-granted quantity payload leaked")
	}
}

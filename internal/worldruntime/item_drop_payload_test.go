package worldruntime

import (
	"reflect"
	"testing"
	"time"

	"github.com/li41/astrahold-server/internal/equipmentcatalog"
	"github.com/li41/astrahold-server/internal/iteminstance"
	"github.com/li41/astrahold-server/internal/loot"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/world"
)

func TestPickupStackPayloadGrantsWholeQuantityAndClearsPrivatePayload(t *testing.T) {
	rt, sim, s := newItemDropTestRuntime(t, world.Position{})
	inv := rt.inventories[s.CharacterIdentity.ID]
	beforeWeight := inv.CurrentWeight()

	dropID, err := rt.spawnExpiringItemDropPayload(
		stackItemDropPayload("item_gold_coin", 3),
		world.Position{X: 1, Layer: 0},
		1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := rt.itemDropPayloads[dropID]; !ok {
		t.Fatal("private stack payload missing")
	}
	if _, ok := rt.itemDropExpireTick[dropID]; !ok {
		t.Fatal("stack payload missing expiry")
	}

	if err := rt.EnqueuePickupItem(s.ID, 1, protocol.ClientPickupItem{DropEntityID: dropID}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("pickup errors=%#v", report.CommandErrors)
	}
	if _, ok := sim.Entity(dropID); ok {
		t.Fatal("picked stack drop still exists")
	}
	if got := inv.Quantity("item_gold_coin"); got != 3 {
		t.Fatalf("gold quantity=%d want=3", got)
	}
	if got := inv.CurrentWeight(); got != beforeWeight {
		t.Fatalf("gold changed carry weight: got=%d want=%d", got, beforeWeight)
	}
	if _, ok := rt.itemDropPayloads[dropID]; ok {
		t.Fatal("picked stack payload leaked")
	}
	if _, ok := rt.itemDropExpireTick[dropID]; ok {
		t.Fatal("picked stack expiry leaked")
	}
}

func TestPickupUniquePayloadPreservesExactInstanceIdentityAndAffix(t *testing.T) {
	rt, sim, s := newItemDropTestRuntime(t, world.Position{})
	var secret monsterLootRollSecret
	secret[0] = 0x42
	resolved := monsterLootResolvedDrop{
		drop: loot.Drop{
			Kind:              loot.DropKindEquipmentInstance,
			ItemArchetypeID:   "item_mid_one_hand_sword",
			ChanceBasisPoints: loot.ChanceBasisPointsScale,
			QuantityMin:       1,
			QuantityMax:       1,
		},
		authoredIndex: 7,
		quantity:      1,
	}
	payload, err := materializeMonsterLootPayloadWithSecret(secret, 9001, 3, resolved)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Kind != loot.DropKindEquipmentInstance || payload.Instance.ID == "" || len(payload.Instance.Affixes) != 1 {
		t.Fatalf("payload=%+v", payload)
	}
	if err := iteminstance.Validate(payload.Instance, mustEquipmentDefinitionForLootPayloadTest(t, payload.ItemArchetypeID)); err != nil {
		t.Fatalf("materialized instance invalid: %v", err)
	}

	dropID, err := rt.spawnExpiringItemDropPayload(payload, world.Position{X: 1, Layer: 0}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := rt.EnqueuePickupItem(s.ID, 1, protocol.ClientPickupItem{DropEntityID: dropID}); err != nil {
		t.Fatal(err)
	}
	report := rt.Step(2, 50*time.Millisecond)
	if len(report.CommandErrors) != 0 {
		t.Fatalf("pickup errors=%#v", report.CommandErrors)
	}
	if _, ok := sim.Entity(dropID); ok {
		t.Fatal("picked unique drop still exists")
	}
	inv := rt.inventories[s.CharacterIdentity.ID]
	got, ok := inv.Instance(payload.Instance.ID)
	if !ok {
		t.Fatalf("inventory missing exact instance %q", payload.Instance.ID)
	}
	if !reflect.DeepEqual(got, payload.Instance) {
		t.Fatalf("instance changed across ground pickup: got=%+v want=%+v", got, payload.Instance)
	}
}

func mustEquipmentDefinitionForLootPayloadTest(t *testing.T, itemArchetypeID string) equipmentcatalog.Definition {
	t.Helper()
	definition, ok := defaultEquipmentCatalog.Resolve(itemArchetypeID)
	if !ok {
		t.Fatalf("missing equipment definition %s", itemArchetypeID)
	}
	return definition
}

package inventory

import (
	"testing"

	"github.com/li41/astrahold-server/internal/iteminstance"
)

func TestReplaceOwnedInstancePreservesIdentityInInventory(t *testing.T) {
	inv := New(8)
	item := iteminstance.Instance{ID: iteminstance.ID("instance:weapon"), ItemArchetypeID: "item_weapon"}
	if err := inv.AddInstance(item); err != nil { t.Fatal(err) }
	before := inv.Revision()
	item.EnhancementLevel = 3
	equipped, err := inv.ReplaceOwnedInstance(item)
	if err != nil { t.Fatal(err) }
	if equipped { t.Fatal("unequipped item reported equipped") }
	got, ok := inv.Instance(item.ID)
	if !ok || got.EnhancementLevel != 3 || got.ID != item.ID { t.Fatalf("item=%#v ok=%v", got, ok) }
	if inv.Revision() != before+1 { t.Fatalf("revision=%d want=%d", inv.Revision(), before+1) }
}

func TestReplaceOwnedInstanceAdvancesEquipmentRevision(t *testing.T) {
	inv := New(8)
	item := iteminstance.Instance{ID: iteminstance.ID("instance:weapon"), ItemArchetypeID: "item_weapon"}
	if err := inv.AddInstance(item); err != nil { t.Fatal(err) }
	if err := inv.EquipInstance(SlotMainHand, item.ID); err != nil { t.Fatal(err) }
	before := inv.EquipmentRevision()
	item.EnhancementLevel = 4
	equipped, err := inv.ReplaceOwnedInstance(item)
	if err != nil { t.Fatal(err) }
	if !equipped { t.Fatal("equipped item reported unequipped") }
	got, ok := inv.EquippedInstance(SlotMainHand)
	if !ok || got.EnhancementLevel != 4 || got.ID != item.ID { t.Fatalf("item=%#v ok=%v", got, ok) }
	if inv.EquipmentRevision() != before+1 { t.Fatalf("equipment revision=%d want=%d", inv.EquipmentRevision(), before+1) }
}

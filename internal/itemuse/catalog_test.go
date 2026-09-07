package itemuse

import (
	"errors"
	"testing"
)

func TestDefaultCatalogAuthorsSharedPotionCooldown(t *testing.T) {
	catalog, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	if catalog.Revision() != "item-use-2" {
		t.Fatalf("revision=%q", catalog.Revision())
	}
	heal, ok := catalog.Resolve("item_minor_healing_potion")
	if !ok {
		t.Fatal("healing potion missing")
	}
	mana, ok := catalog.Resolve("item_minor_mana_potion")
	if !ok {
		t.Fatal("mana potion missing")
	}
	if heal.CooldownGroup != "resource_potion" || mana.CooldownGroup != heal.CooldownGroup {
		t.Fatalf("cooldown groups heal=%q mana=%q", heal.CooldownGroup, mana.CooldownGroup)
	}
	if heal.CooldownTicks != 40 || mana.CooldownTicks != 40 {
		t.Fatalf("cooldown ticks heal=%d mana=%d", heal.CooldownTicks, mana.CooldownTicks)
	}
}

func TestCatalogRejectsMissingCooldownPolicy(t *testing.T) {
	for _, definition := range []Definition{
		{ItemArchetypeID: "item_test", Resource: ResourceHP, RestoreAmount: 1, CooldownTicks: 40},
		{ItemArchetypeID: "item_test", Resource: ResourceHP, RestoreAmount: 1, CooldownGroup: "resource_potion"},
	} {
		_, err := New(CatalogDefinition{Revision: "test", Items: []Definition{definition}})
		if !errors.Is(err, ErrInvalidCatalog) {
			t.Fatalf("definition=%#v err=%v", definition, err)
		}
	}
}

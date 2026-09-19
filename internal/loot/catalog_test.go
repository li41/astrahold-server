package loot

import (
	"errors"
	"testing"
)

func TestCatalogLookupReturnsOwnedDrops(t *testing.T) {
	catalog, err := New(Definition{
		Revision: "test-v2",
		Tables: []Table{{
			SourceArchetypeID: "monster-a",
			Drops: []Drop{
				{ItemArchetypeID: "item-a"},
				{ItemArchetypeID: "item-b", ChanceBasisPoints: 2_500, QuantityMin: 2, QuantityMax: 5},
				{Kind: DropKindEquipmentInstance, ItemArchetypeID: "item-mid", ChanceBasisPoints: 100},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.Revision(); got != "test-v2" {
		t.Fatalf("revision=%q", got)
	}
	drops, ok := catalog.DropsFor("monster-a")
	if !ok || len(drops) != 3 {
		t.Fatalf("drops=%#v ok=%v", drops, ok)
	}
	if drops[0].Kind != DropKindStack || drops[0].ItemArchetypeID != "item-a" || drops[0].ChanceBasisPoints != ChanceBasisPointsScale || drops[0].QuantityMin != 1 || drops[0].QuantityMax != 1 {
		t.Fatalf("default drop not normalized: %#v", drops[0])
	}
	if drops[1].ItemArchetypeID != "item-b" || drops[1].ChanceBasisPoints != 2_500 || drops[1].QuantityMin != 2 || drops[1].QuantityMax != 5 {
		t.Fatalf("explicit stack changed: %#v", drops[1])
	}
	if drops[2].Kind != DropKindEquipmentInstance || drops[2].QuantityMin != 1 || drops[2].QuantityMax != 1 {
		t.Fatalf("instance default quantity not normalized: %#v", drops[2])
	}
	if !drops[1].IncludesRoll(2_499) || drops[1].IncludesRoll(2_500) {
		t.Fatalf("chance threshold semantics are not [0,chance): %#v", drops[1])
	}
	drops[0].ItemArchetypeID = "mutated"
	again, ok := catalog.DropsFor("monster-a")
	if !ok || again[0].ItemArchetypeID != "item-a" {
		t.Fatalf("catalog mutated through lookup: %#v", again)
	}
	if _, ok := catalog.DropsFor("monster-b"); ok {
		t.Fatal("unexpected table for unconfigured archetype")
	}
}

func TestCatalogRejectsInvalidAndDuplicateTables(t *testing.T) {
	cases := []struct {
		name string
		def  Definition
		want error
	}{
		{name: "empty revision", def: Definition{Tables: []Table{{SourceArchetypeID: "a", Drops: []Drop{{ItemArchetypeID: "x"}}}}}, want: ErrInvalidDefinition},
		{name: "empty tables", def: Definition{Revision: "v1"}, want: ErrInvalidDefinition},
		{name: "empty source", def: Definition{Revision: "v1", Tables: []Table{{Drops: []Drop{{ItemArchetypeID: "x"}}}}}, want: ErrInvalidDefinition},
		{name: "empty drops", def: Definition{Revision: "v1", Tables: []Table{{SourceArchetypeID: "a"}}}, want: ErrInvalidDefinition},
		{name: "empty item", def: Definition{Revision: "v1", Tables: []Table{{SourceArchetypeID: "a", Drops: []Drop{{}}}}}, want: ErrInvalidDefinition},
		{name: "chance above scale", def: Definition{Revision: "v1", Tables: []Table{{SourceArchetypeID: "a", Drops: []Drop{{ItemArchetypeID: "x", ChanceBasisPoints: ChanceBasisPointsScale + 1}}}}}, want: ErrInvalidDefinition},
		{name: "bad quantity zero min", def: Definition{Revision: "v1", Tables: []Table{{SourceArchetypeID: "a", Drops: []Drop{{ItemArchetypeID: "x", QuantityMax: 2}}}}}, want: ErrInvalidDefinition},
		{name: "bad quantity inverted", def: Definition{Revision: "v1", Tables: []Table{{SourceArchetypeID: "a", Drops: []Drop{{ItemArchetypeID: "x", QuantityMin: 3, QuantityMax: 2}}}}}, want: ErrInvalidDefinition},
		{name: "unknown kind", def: Definition{Revision: "v1", Tables: []Table{{SourceArchetypeID: "a", Drops: []Drop{{Kind: DropKind("future"), ItemArchetypeID: "x"}}}}}, want: ErrInvalidDefinition},
		{name: "instance quantity range", def: Definition{Revision: "v1", Tables: []Table{{SourceArchetypeID: "a", Drops: []Drop{{Kind: DropKindEquipmentInstance, ItemArchetypeID: "x", QuantityMin: 1, QuantityMax: 2}}}}}, want: ErrInvalidDefinition},
		{name: "duplicate source", def: Definition{Revision: "v1", Tables: []Table{
			{SourceArchetypeID: "a", Drops: []Drop{{ItemArchetypeID: "x"}}},
			{SourceArchetypeID: "a", Drops: []Drop{{ItemArchetypeID: "y"}}},
		}}, want: ErrDuplicateSourceArchetype},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(tc.def)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want=%v", err, tc.want)
			}
		})
	}
}

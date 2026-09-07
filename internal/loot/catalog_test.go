package loot

import (
	"errors"
	"testing"
)

func TestCatalogLookupReturnsOwnedDrops(t *testing.T) {
	catalog, err := New(Definition{
		Revision: "test-v1",
		Tables: []Table{{
			SourceArchetypeID: "monster-a",
			Drops: []Drop{
				{ItemArchetypeID: "item-a"},
				{ItemArchetypeID: "item-b", ChanceBasisPoints: 2_500},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.Revision(); got != "test-v1" {
		t.Fatalf("revision=%q", got)
	}
	drops, ok := catalog.DropsFor("monster-a")
	if !ok || len(drops) != 2 {
		t.Fatalf("drops=%#v ok=%v", drops, ok)
	}
	if drops[0].ItemArchetypeID != "item-a" || drops[0].ChanceBasisPoints != ChanceBasisPointsScale {
		t.Fatalf("default chance not normalized to guaranteed: %#v", drops[0])
	}
	if drops[1].ItemArchetypeID != "item-b" || drops[1].ChanceBasisPoints != 2_500 {
		t.Fatalf("explicit chance changed: %#v", drops[1])
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

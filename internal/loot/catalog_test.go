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
			Drops:             []Drop{{ItemArchetypeID: "item-a"}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := catalog.Revision(); got != "test-v1" {
		t.Fatalf("revision=%q", got)
	}
	drops, ok := catalog.DropsFor("monster-a")
	if !ok || len(drops) != 1 || drops[0].ItemArchetypeID != "item-a" {
		t.Fatalf("drops=%#v ok=%v", drops, ok)
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

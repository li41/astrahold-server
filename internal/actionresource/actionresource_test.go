package actionresource

import "testing"

func TestDefinitionsRemainStable(t *testing.T) {
	cases := []Definition{
		{ID: Resolve, Max: 100},
		{ID: Momentum, Max: 100},
		{ID: HuntMomentum, Max: 100},
		{ID: StarHeat, Max: 100},
		{ID: OathSeal, Max: 3, ProgressThreshold: 100},
	}
	for _, want := range cases {
		got, ok := DefinitionForID(want.ID)
		if !ok {
			t.Fatalf("missing definition for %q", want.ID)
		}
		if got != want {
			t.Fatalf("definition for %q = %#v, want %#v", want.ID, got, want)
		}
	}
	if _, ok := DefinitionForID(Empty); ok {
		t.Fatal("empty resource unexpectedly has a definition")
	}
}

package character

import (
	"testing"

	"github.com/li41/astrahold-server/internal/actionresource"
)

func TestStateActionResourceReturnsGenericSnapshot(t *testing.T) {
	state := State{
		ActionResourceID:       actionresource.Resolve,
		ActionResourceCurrent:  3,
		MaxActionResource:      5,
		ActionResourceProgress: 40,
	}

	got := state.ActionResource()
	if got.ID != actionresource.Resolve || got.Current != 3 || got.Max != 5 || got.Progress != 40 {
		t.Fatalf("ActionResource()=%#v", got)
	}

	got.Current = 1
	if state.ActionResourceCurrent != 3 {
		t.Fatalf("ActionResource() exposed mutable state: current=%d", state.ActionResourceCurrent)
	}
}

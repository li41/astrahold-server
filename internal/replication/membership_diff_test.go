package replication

import (
	"testing"

	"github.com/li41/astrahold-server/internal/world"
)

func TestDesiredMembershipRemovedDistinguishesGrowthFromChurn(t *testing.T) {
	grown, grownVisible := lifecycleFrameIDs(1, []world.EntityID{1, 2, 3, 4, 5})
	if desiredMembershipRemoved([]world.EntityID{1, 2, 3, 4}, grown, grownVisible) {
		t.Fatal("pure desired growth must not be treated as departed membership")
	}

	changed, changedVisible := lifecycleFrameIDs(2, []world.EntityID{1, 2, 5, 6})
	if !desiredMembershipRemoved([]world.EntityID{1, 2, 3, 4}, changed, changedVisible) {
		t.Fatal("removed desired IDs must trigger departed membership diff")
	}

	if desiredMembershipRemoved(nil, grown, grownVisible) {
		t.Fatal("empty previous desired cannot have departed membership")
	}
}

package worldruntime

import (
	"math"
	"testing"

	"github.com/li41/astrahold-server/internal/protocol"
)

func TestFencedPointActionAcceptsCoordinatesWithoutTargetID(t *testing.T) {
	rt, fence, _ := joinOwnedIdentitySession(t)
	x := float32(4.5)
	z := float32(12.25)
	action := protocol.ClientUseAction{
		ActionID:   "meteor-strike",
		TargetKind: protocol.ActionTargetPoint,
		TargetX:    &x,
		TargetZ:    &z,
	}
	if err := rt.EnqueueFencedUseAction(fence, 1, action); err != nil {
		t.Fatalf("legal fenced point action rejected: %v", err)
	}
}

func TestFencedPointActionRejectsMissingOrNonFiniteCoordinates(t *testing.T) {
	rt, fence, _ := joinOwnedIdentitySession(t)
	z := float32(1)
	if err := rt.EnqueueFencedUseAction(fence, 1, protocol.ClientUseAction{
		ActionID:   "meteor-strike",
		TargetKind: protocol.ActionTargetPoint,
		TargetZ:    &z,
	}); err == nil {
		t.Fatal("missing point X was accepted")
	}

	nan := float32(math.NaN())
	if err := rt.EnqueueFencedUseAction(fence, 1, protocol.ClientUseAction{
		ActionID:   "meteor-strike",
		TargetKind: protocol.ActionTargetPoint,
		TargetX:    &nan,
		TargetZ:    &z,
	}); err == nil {
		t.Fatal("non-finite point X was accepted")
	}
}

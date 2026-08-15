package tcpudp

import (
	"context"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/world"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

type trustedConnectionPlan struct {
	entityID       world.EntityID
	admissionLease *worldruntime.CharacterAdmissionLease
	takeover       worldruntime.SessionOwnershipFence
}

func (s *Server) prepareTrustedConnection(ctx context.Context, identity characteridentity.Binding, allocatedEntityID world.EntityID) (trustedConnectionPlan, error) {
	plan, err := s.runtime.AwaitCharacterConnectionPlan(ctx, identity)
	if err != nil {
		return trustedConnectionPlan{}, err
	}
	if !plan.Valid() {
		return trustedConnectionPlan{}, ErrInvalidCharacterConnectionPlan
	}
	if plan.Takeover() {
		if plan.Ownership.CharacterID != identity.ID || plan.Ownership.EntityID == 0 {
			return trustedConnectionPlan{}, ErrInvalidCharacterConnectionPlan
		}
		return trustedConnectionPlan{entityID: plan.Ownership.EntityID, takeover: plan.Ownership}, nil
	}
	if !plan.AdmissionLease.Valid() || plan.AdmissionLease.CharacterID != identity.ID {
		return trustedConnectionPlan{}, ErrInvalidCharacterConnectionPlan
	}
	lease := plan.AdmissionLease
	return trustedConnectionPlan{entityID: allocatedEntityID, admissionLease: &lease}, nil
}

// retireTakenOverPeer runs only after the F.19 transfer has committed. The old Session has
// already been removed from worldruntime, so its normal transport-close Leave must be
// suppressed. A concurrent close that already enqueued its fenced Leave remains safe: if the
// transfer committed, that command is necessarily stale at world-owner execution.
func (s *Server) retireTakenOverPeer(expected worldruntime.SessionOwnershipFence) {
	if !expected.Valid() {
		return
	}
	s.mu.RLock()
	var old *peer
	for _, candidate := range s.peers {
		// joined is the publication barrier for ownership. Load it before reading the
		// immutable fence; the F.18/F.20 handle path writes ownership and only then stores
		// joined=true. Reversing this order creates a race with a peer still publishing.
		if !candidate.joined.Load() {
			continue
		}
		if candidate.ownership == expected {
			old = candidate
			break
		}
	}
	s.mu.RUnlock()
	if old == nil {
		return
	}
	old.leaveOnce.Do(func() {})
	s.closePeer(old, "ownership_takeover", nil)
}

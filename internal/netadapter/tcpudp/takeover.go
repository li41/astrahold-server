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
// suppressed. SessionID and EntityID are immutable from peer creation, unlike ownership,
// which is published only after world-owner join completion. Looking up by those immutable
// IDs lets takeover retire even an old peer whose join committed but whose transport goroutine
// has not yet published ownership/joined. A concurrent close that already enqueued its fenced
// Leave remains safe because F.18 makes that command stale after transfer commit.
func (s *Server) retireTakenOverPeer(expected worldruntime.SessionOwnershipFence) {
	if !expected.Valid() {
		return
	}
	s.mu.RLock()
	var old *peer
	for _, candidate := range s.peers {
		if candidate.sessionID == expected.SessionID && candidate.entityID == expected.EntityID {
			old = candidate
			break
		}
	}
	s.mu.RUnlock()
	if old == nil {
		return
	}
	// Consume the one-shot Leave even when joined is not yet published. If the old handle
	// later observes its already-committed join and calls closePeer again, it cannot enqueue
	// a Leave that would target the transferred Entity.
	old.leaveOnce.Do(func() {})
	s.closePeer(old, "ownership_takeover", nil)
}

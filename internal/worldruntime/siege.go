package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/combat"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/siege"
)

const legacyGateActionID = "__legacy_gate_attack__"

var (
	ErrSiegeUnavailable  = errors.New("worldruntime: siege unavailable")
	ErrCombatUnavailable = errors.New("worldruntime: combat unavailable")
)

func WithSiegeGates(gates []gameplayworld.Gate) Option {
	return func(r *Runtime) {
		if len(gates) == 0 {
			return
		}
		r.siege = siege.NewService(gates)
		profile := gates[0].Attack
		legacy, err := combat.NewService([]combat.ActionDefinition{{
			ID:              legacyGateActionID,
			Targets:         []combat.TargetKind{combat.TargetGate},
			Range:           profile.Range,
			BaseDamage:      profile.Damage,
			DamageType:      combat.DamagePhysical,
			CooldownSeconds: profile.CooldownSeconds,
		}})
		if err == nil {
			r.combat = legacy
		}
	}
}

// WithSiegeMatch configures the authoritative siege match state after WithSiegeGates.
// It is a startup-only option; invalid static configuration fails fast.
func WithSiegeMatch(definition siege.MatchDefinition) Option {
	return func(r *Runtime) {
		if r.siege == nil {
			panic("worldruntime: siege gates must be configured before siege match")
		}
		if err := r.siege.ConfigureMatch(definition); err != nil {
			panic(err)
		}
	}
}

func WithCombatService(service *combat.Service) Option {
	return func(r *Runtime) { r.combat = service }
}

// SiegeMatchState exposes a read-only snapshot for replication/observability seams.
func (r *Runtime) SiegeMatchState() (siege.MatchState, bool) {
	if r == nil || r.siege == nil {
		return siege.MatchState{}, false
	}
	return r.siege.MatchState()
}

// SiegeCastleOwnershipState exposes the process-local Server-authoritative castle owner.
// D.3A does not make this durable or add it to Protocol v7.
func (r *Runtime) SiegeCastleOwnershipState() (siege.CastleOwnershipState, bool) {
	if r == nil || r.siege == nil {
		return siege.CastleOwnershipState{}, false
	}
	return r.siege.CastleOwnershipState()
}

// SiegeThronePresenceState exposes Server-observed throne occupancy/contest eligibility.
// It is intentionally not a network contract.
func (r *Runtime) SiegeThronePresenceState() (siege.ThronePresenceState, bool) {
	if r == nil || r.siege == nil {
		return siege.ThronePresenceState{}, false
	}
	return r.siege.ThronePresenceState()
}

// SiegeThroneCaptureState exposes Server-clock capture progress for observability/tests.
// D.3A consumes ReadyForResolution internally; the progress itself remains off-wire.
func (r *Runtime) SiegeThroneCaptureState() (siege.ThroneCaptureState, bool) {
	if r == nil || r.siege == nil {
		return siege.ThroneCaptureState{}, false
	}
	return r.siege.ThroneCaptureState()
}

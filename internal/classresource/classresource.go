// Package classresource retains the stable combat-resource IDs used by shipped actions and the
// Protocol v27 compatibility resource lane. The package name is historical: current classless
// gameplay may use these resources through learned actions without a fixed-profession gate.
package classresource

import (
	"errors"

	"github.com/li41/astrahold-server/internal/classid"
)

type ID string

const (
	Empty        ID = ""
	Resolve      ID = "resolve"
	Momentum     ID = "momentum"
	HuntMomentum ID = "hunt_momentum"
	StarHeat     ID = "star_heat"
	OathSeal     ID = "oath_seal"
)

var ErrResourceMismatch = errors.New("classresource: resource mismatch")

type Definition struct {
	ID                ID
	Max               uint32
	ProgressThreshold uint32
}

// PrimaryForClass is a fixed-profession compatibility lookup for legacy v27 resource state.
// It must not be used to decide current classless action, equipment, or skill legality.
func PrimaryForClass(id classid.ID) (Definition, bool) {
	switch id {
	case classid.Oathguard:
		return Definition{ID: Resolve, Max: 100}, true
	case classid.Breaker:
		return Definition{ID: Momentum, Max: 100}, true
	case classid.Ranger:
		return Definition{ID: HuntMomentum, Max: 100}, true
	case classid.StarfireMage:
		return Definition{ID: StarHeat, Max: 100}, true
	case classid.Oathhealer:
		return Definition{ID: OathSeal, Max: 3, ProgressThreshold: 100}, true
	default:
		return Definition{}, false
	}
}

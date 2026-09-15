// Package actionresource owns the stable Server-authoritative combat/action resource definitions.
// Resource IDs are independent of fixed profession identity; learned actions may consume or produce
// them without a ClassID gate.
package actionresource

import "errors"

type ID string

const (
	Empty        ID = ""
	Resolve      ID = "resolve"
	Momentum     ID = "momentum"
	HuntMomentum ID = "hunt_momentum"
	StarHeat     ID = "star_heat"
	OathSeal     ID = "oath_seal"
)

var ErrResourceMismatch = errors.New("actionresource: resource mismatch")

type Definition struct {
	ID                ID
	Max               uint32
	ProgressThreshold uint32
}

// DefinitionForID returns the authoritative runtime definition for a stable action resource ID.
func DefinitionForID(id ID) (Definition, bool) {
	switch id {
	case Resolve:
		return Definition{ID: Resolve, Max: 100}, true
	case Momentum:
		return Definition{ID: Momentum, Max: 100}, true
	case HuntMomentum:
		return Definition{ID: HuntMomentum, Max: 100}, true
	case StarHeat:
		return Definition{ID: StarHeat, Max: 100}, true
	case OathSeal:
		return Definition{ID: OathSeal, Max: 3, ProgressThreshold: 100}, true
	default:
		return Definition{}, false
	}
}

// Package classresource is the fixed-profession compatibility adapter for Protocol v27 and
// legacy fixtures. Current classless gameplay definitions live in internal/actionresource.
package classresource

import (
	"github.com/li41/astrahold-server/internal/actionresource"
	"github.com/li41/astrahold-server/internal/classid"
)

type ID = actionresource.ID

const (
	Empty        = actionresource.Empty
	Resolve      = actionresource.Resolve
	Momentum     = actionresource.Momentum
	HuntMomentum = actionresource.HuntMomentum
	StarHeat     = actionresource.StarHeat
	OathSeal     = actionresource.OathSeal
)

var ErrResourceMismatch = actionresource.ErrResourceMismatch

type Definition = actionresource.Definition

// DefinitionForID is retained for source compatibility while callers migrate to actionresource.
func DefinitionForID(id ID) (Definition, bool) {
	return actionresource.DefinitionForID(id)
}

// PrimaryForClass is a fixed-profession compatibility lookup for legacy v27 resource state.
// It must not be used to decide current classless action, equipment, or skill legality.
func PrimaryForClass(id classid.ID) (Definition, bool) {
	switch id {
	case classid.Oathguard:
		return actionresource.DefinitionForID(actionresource.Resolve)
	case classid.Breaker:
		return actionresource.DefinitionForID(actionresource.Momentum)
	case classid.Ranger:
		return actionresource.DefinitionForID(actionresource.HuntMomentum)
	case classid.StarfireMage:
		return actionresource.DefinitionForID(actionresource.StarHeat)
	case classid.Oathhealer:
		return actionresource.DefinitionForID(actionresource.OathSeal)
	default:
		return Definition{}, false
	}
}

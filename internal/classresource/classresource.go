// Package classresource is the Protocol v27 source-compatibility adapter for legacy class-resource
// names. Current classless gameplay definitions and ownership live in internal/actionresource.
package classresource

import "github.com/li41/astrahold-server/internal/actionresource"

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

// DefinitionForID is retained for source compatibility while legacy Protocol v27 callers migrate
// to actionresource. It does not imply fixed-profession gameplay ownership.
func DefinitionForID(id ID) (Definition, bool) {
	return actionresource.DefinitionForID(id)
}

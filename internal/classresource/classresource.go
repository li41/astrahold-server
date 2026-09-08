// Package classresource defines stable Server-side class combat resource identities and caps.
package classresource

import (
	"errors"

	"github.com/li41/astrahold-server/internal/classid"
)

type ID string

const (
	Empty   ID = ""
	Resolve ID = "resolve"
)

var ErrResourceMismatch = errors.New("classresource: resource mismatch")

type Definition struct {
	ID  ID
	Max uint32
}

// PrimaryForClass returns the authored primary combat resource for a class. A missing definition
// means that class has not yet shipped an authoritative class-resource contract.
func PrimaryForClass(id classid.ID) (Definition, bool) {
	switch id {
	case classid.Oathguard:
		return Definition{ID: Resolve, Max: 100}, true
	default:
		return Definition{}, false
	}
}

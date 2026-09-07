// Package loot defines server-authored loot tables independently from combat, monster lifecycle,
// inventory, and pickup transport.
package loot

import "errors"

var (
	ErrInvalidDefinition        = errors.New("loot: invalid definition")
	ErrDuplicateSourceArchetype = errors.New("loot: duplicate source archetype")
)

// Drop describes one authoritative item-drop entity to materialize when a source table resolves.
// Quantity is intentionally not encoded here because the current item-drop protocol represents one
// item entity per pickup. Multiple guaranteed units can be authored as multiple Drop entries.
type Drop struct {
	ItemArchetypeID string
}

// Table maps one gameplay source archetype to its guaranteed first-slice drops. Chance/weight
// policy can be added inside this package later without coupling worldruntime to monster types.
type Table struct {
	SourceArchetypeID string
	Drops             []Drop
}

type Definition struct {
	Revision string
	Tables   []Table
}

// Catalog is immutable after construction. Lookup returns owned copies so callers cannot mutate
// server-authored loot truth across world ticks.
type Catalog struct {
	revision string
	bySource map[string][]Drop
}

func New(def Definition) (*Catalog, error) {
	if def.Revision == "" || len(def.Tables) == 0 {
		return nil, ErrInvalidDefinition
	}
	catalog := &Catalog{
		revision: def.Revision,
		bySource: make(map[string][]Drop, len(def.Tables)),
	}
	for _, table := range def.Tables {
		if table.SourceArchetypeID == "" || len(table.Drops) == 0 {
			return nil, ErrInvalidDefinition
		}
		if _, exists := catalog.bySource[table.SourceArchetypeID]; exists {
			return nil, ErrDuplicateSourceArchetype
		}
		drops := make([]Drop, len(table.Drops))
		for i, drop := range table.Drops {
			if drop.ItemArchetypeID == "" {
				return nil, ErrInvalidDefinition
			}
			drops[i] = drop
		}
		catalog.bySource[table.SourceArchetypeID] = drops
	}
	return catalog, nil
}

func (c *Catalog) Revision() string {
	if c == nil {
		return ""
	}
	return c.revision
}

func (c *Catalog) DropsFor(sourceArchetypeID string) ([]Drop, bool) {
	if c == nil || sourceArchetypeID == "" {
		return nil, false
	}
	drops, ok := c.bySource[sourceArchetypeID]
	if !ok {
		return nil, false
	}
	return append([]Drop(nil), drops...), true
}

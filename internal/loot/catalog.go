// Package loot defines server-authored loot tables independently from combat, monster lifecycle,
// inventory, and pickup transport.
package loot

import "errors"

const ChanceBasisPointsScale uint16 = 10_000

var (
	ErrInvalidDefinition        = errors.New("loot: invalid definition")
	ErrDuplicateSourceArchetype = errors.New("loot: duplicate source archetype")
)

// Drop describes one authoritative item-drop entity candidate when a source table resolves.
// ChanceBasisPoints uses 1..10_000 after Catalog construction. Definition input may use zero as
// a backwards-compatible shorthand for guaranteed (10_000). A 0% entry should simply be omitted.
// Quantity is intentionally not encoded here because the current item-drop protocol represents one
// item entity per pickup. Multiple guaranteed units can still be authored as multiple Drop entries.
type Drop struct {
	ItemArchetypeID   string
	ChanceBasisPoints uint16
}

func (d Drop) IncludesRoll(roll uint16) bool {
	return d.ChanceBasisPoints > 0 && roll < d.ChanceBasisPoints
}

// Table maps one gameplay source archetype to its Server-authored drop candidates. Runtime supplies
// unpredictable Server-private rolls; this package owns the authored probability threshold semantics.
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
			if drop.ItemArchetypeID == "" || drop.ChanceBasisPoints > ChanceBasisPointsScale {
				return nil, ErrInvalidDefinition
			}
			if drop.ChanceBasisPoints == 0 {
				drop.ChanceBasisPoints = ChanceBasisPointsScale
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

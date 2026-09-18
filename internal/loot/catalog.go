// Package loot defines server-authored loot tables independently from combat, monster lifecycle,
// inventory, and pickup transport.
package loot

import "errors"

const ChanceBasisPointsScale uint16 = 10_000

var (
	ErrInvalidDefinition        = errors.New("loot: invalid definition")
	ErrDuplicateSourceArchetype = errors.New("loot: duplicate source archetype")
)

type DropKind string

const (
	DropKindStack             DropKind = "stack"
	DropKindEquipmentInstance DropKind = "equipment_instance"
)

// Drop describes one authoritative loot candidate when a source table resolves.
//
// ChanceBasisPoints uses 1..10_000 after Catalog construction. Definition input may use zero as
// backwards-compatible shorthand for guaranteed (10_000). A 0% entry should be omitted.
//
// QuantityMin/QuantityMax are inclusive. Both zero normalize to 1..1 for backwards compatibility.
// Equipment-instance drops are always exactly one unique instance; stack drops may resolve quantity
// greater than one while remaining a single public ground entity until pickup.
type Drop struct {
	Kind              DropKind
	ItemArchetypeID   string
	ChanceBasisPoints uint16
	QuantityMin       uint32
	QuantityMax       uint32
}

func (d Drop) IncludesRoll(roll uint16) bool {
	return d.ChanceBasisPoints > 0 && roll < d.ChanceBasisPoints
}

// Table maps one gameplay source archetype to its Server-authored drop candidates. Runtime supplies
// unpredictable Server-private rolls; this package owns authored threshold and quantity semantics.
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
			if drop.Kind == "" {
				drop.Kind = DropKindStack
			}
			if drop.QuantityMin == 0 && drop.QuantityMax == 0 {
				drop.QuantityMin = 1
				drop.QuantityMax = 1
			}
			if drop.QuantityMin == 0 || drop.QuantityMax < drop.QuantityMin {
				return nil, ErrInvalidDefinition
			}
			switch drop.Kind {
			case DropKindStack:
			case DropKindEquipmentInstance:
				if drop.QuantityMin != 1 || drop.QuantityMax != 1 {
					return nil, ErrInvalidDefinition
				}
			default:
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

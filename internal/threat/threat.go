// Package threat owns reusable Server-authoritative threat accumulation and selection.
// It intentionally stores stable gameplay EntityID values rather than session identities.
package threat

import "github.com/li41/astrahold-server/internal/world"

// Table accumulates threat for one encounter owner. It is expected to be mutated only from the
// authoritative world-owner path; callers decide which gameplay facts are eligible to add threat.
type Table struct {
	values map[world.EntityID]uint64
}

func New() *Table {
	return &Table{values: make(map[world.EntityID]uint64)}
}

// Add saturates at MaxUint64 so long-running encounters cannot wrap threat and invert ordering.
func (t *Table) Add(entityID world.EntityID, amount uint64) {
	if t == nil || entityID == 0 || amount == 0 {
		return
	}
	if t.values == nil {
		t.values = make(map[world.EntityID]uint64)
	}
	current := t.values[entityID]
	if amount > ^uint64(0)-current {
		t.values[entityID] = ^uint64(0)
		return
	}
	t.values[entityID] = current + amount
}

func (t *Table) Remove(entityID world.EntityID) {
	if t == nil || entityID == 0 {
		return
	}
	delete(t.values, entityID)
}

func (t *Table) Clear() {
	if t == nil || len(t.values) == 0 {
		return
	}
	clear(t.values)
}

func (t *Table) Value(entityID world.EntityID) uint64 {
	if t == nil {
		return 0
	}
	return t.values[entityID]
}

func (t *Table) Len() int {
	if t == nil {
		return 0
	}
	return len(t.values)
}

// HighestValid returns the highest-threat currently valid target and prunes invalid targets while
// scanning. Equal threat is resolved by lower EntityID so target selection never depends on map
// iteration order. A nil validator treats every entry as valid.
func (t *Table) HighestValid(valid func(world.EntityID) bool) (world.EntityID, uint64, bool) {
	if t == nil || len(t.values) == 0 {
		return 0, 0, false
	}
	var bestID world.EntityID
	var bestThreat uint64
	found := false
	for entityID, value := range t.values {
		if entityID == 0 || value == 0 || (valid != nil && !valid(entityID)) {
			delete(t.values, entityID)
			continue
		}
		if !found || value > bestThreat || (value == bestThreat && entityID < bestID) {
			bestID = entityID
			bestThreat = value
			found = true
		}
	}
	return bestID, bestThreat, found
}

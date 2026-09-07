// Package weaponeffect owns Server-authoritative equipped weapon gameplay effects.
// Presentation metadata stays client-side; this package resolves stable item archetype IDs
// into deterministic effects that may run only after authoritative combat resolution.
package weaponeffect

import (
	_ "embed"
	"encoding/json"
	"errors"
	"strings"
)

//go:embed default.json
var defaultCatalogJSON []byte

var ErrInvalidCatalog = errors.New("weaponeffect: invalid catalog")

type Definition struct {
	ItemArchetypeID        string `json:"item_archetype_id"`
	ManaRestoreOnDamageHit uint32 `json:"mana_restore_on_damage_hit"`
}

type CatalogDefinition struct {
	Revision string       `json:"revision"`
	Weapons  []Definition `json:"weapons"`
}

type Catalog struct {
	revision string
	byItem   map[string]Definition
}

func Default() (*Catalog, error) {
	return Load(defaultCatalogJSON)
}

func Load(data []byte) (*Catalog, error) {
	var definition CatalogDefinition
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&definition); err != nil {
		return nil, err
	}
	return New(definition)
}

func New(definition CatalogDefinition) (*Catalog, error) {
	definition.Revision = strings.TrimSpace(definition.Revision)
	if definition.Revision == "" || len(definition.Weapons) == 0 {
		return nil, ErrInvalidCatalog
	}

	catalog := &Catalog{
		revision: definition.Revision,
		byItem:   make(map[string]Definition, len(definition.Weapons)),
	}
	for _, weapon := range definition.Weapons {
		weapon.ItemArchetypeID = strings.TrimSpace(weapon.ItemArchetypeID)
		if weapon.ItemArchetypeID == "" || weapon.ManaRestoreOnDamageHit == 0 {
			return nil, ErrInvalidCatalog
		}
		if _, exists := catalog.byItem[weapon.ItemArchetypeID]; exists {
			return nil, ErrInvalidCatalog
		}
		catalog.byItem[weapon.ItemArchetypeID] = weapon
	}
	return catalog, nil
}

func (c *Catalog) Revision() string {
	if c == nil {
		return ""
	}
	return c.revision
}

func (c *Catalog) Resolve(itemArchetypeID string) (Definition, bool) {
	if c == nil {
		return Definition{}, false
	}
	definition, ok := c.byItem[strings.TrimSpace(itemArchetypeID)]
	return definition, ok
}

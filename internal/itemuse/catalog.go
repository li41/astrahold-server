// Package itemuse owns Server-authoritative consumable item gameplay definitions.
// Presentation metadata stays client-side; this package only resolves stable item archetype IDs
// into gameplay resource effects.
package itemuse

import (
	_ "embed"
	"encoding/json"
	"errors"
	"strings"
)

//go:embed default.json
var defaultCatalogJSON []byte

var ErrInvalidCatalog = errors.New("itemuse: invalid catalog")

type Resource string

const (
	ResourceHP Resource = "hp"
	ResourceMP Resource = "mp"
)

type Definition struct {
	ItemArchetypeID string   `json:"item_archetype_id"`
	Resource        Resource `json:"resource"`
	RestoreAmount   uint32   `json:"restore_amount"`
}

type CatalogDefinition struct {
	Revision string       `json:"revision"`
	Items    []Definition `json:"items"`
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
	if definition.Revision == "" || len(definition.Items) == 0 {
		return nil, ErrInvalidCatalog
	}

	catalog := &Catalog{
		revision: definition.Revision,
		byItem:   make(map[string]Definition, len(definition.Items)),
	}
	for _, item := range definition.Items {
		item.ItemArchetypeID = strings.TrimSpace(item.ItemArchetypeID)
		if item.ItemArchetypeID == "" || item.RestoreAmount == 0 {
			return nil, ErrInvalidCatalog
		}
		switch item.Resource {
		case ResourceHP, ResourceMP:
		default:
			return nil, ErrInvalidCatalog
		}
		if _, exists := catalog.byItem[item.ItemArchetypeID]; exists {
			return nil, ErrInvalidCatalog
		}
		catalog.byItem[item.ItemArchetypeID] = item
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

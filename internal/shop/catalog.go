package shop

import (
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"strings"
)

//go:embed default.json
var defaultCatalogJSON []byte

var ErrInvalidCatalog = errors.New("shop: invalid catalog")

type Offer struct {
	ID              string `json:"id"`
	ItemArchetypeID string `json:"item_archetype_id"`
	Quantity        uint32 `json:"quantity"`
	CostArchetypeID string `json:"cost_archetype_id"`
	CostQuantity    uint32 `json:"cost_quantity"`
}

type Shop struct {
	ID             string  `json:"id"`
	NPCArchetypeID string  `json:"npc_archetype_id"`
	Offers         []Offer `json:"offers"`
}

type Definition struct {
	Revision string `json:"revision"`
	Shops    []Shop `json:"shops"`
}

type Catalog struct {
	revision string
	byNPC    map[string]Shop
}

func Default() (*Catalog, error) {
	return Load(defaultCatalogJSON)
}

func LoadFile(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Load(data)
}

func Load(data []byte) (*Catalog, error) {
	var definition Definition
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&definition); err != nil {
		return nil, err
	}
	return New(definition)
}

func New(definition Definition) (*Catalog, error) {
	definition.Revision = strings.TrimSpace(definition.Revision)
	if definition.Revision == "" || len(definition.Shops) == 0 {
		return nil, ErrInvalidCatalog
	}

	catalog := &Catalog{revision: definition.Revision, byNPC: make(map[string]Shop, len(definition.Shops))}
	shopIDs := make(map[string]struct{}, len(definition.Shops))
	for _, entry := range definition.Shops {
		entry.ID = strings.TrimSpace(entry.ID)
		entry.NPCArchetypeID = strings.TrimSpace(entry.NPCArchetypeID)
		if entry.ID == "" || entry.NPCArchetypeID == "" || len(entry.Offers) == 0 {
			return nil, ErrInvalidCatalog
		}
		if _, exists := shopIDs[entry.ID]; exists {
			return nil, ErrInvalidCatalog
		}
		if _, exists := catalog.byNPC[entry.NPCArchetypeID]; exists {
			return nil, ErrInvalidCatalog
		}
		shopIDs[entry.ID] = struct{}{}

		offerIDs := make(map[string]struct{}, len(entry.Offers))
		for index := range entry.Offers {
			offer := &entry.Offers[index]
			offer.ID = strings.TrimSpace(offer.ID)
			offer.ItemArchetypeID = strings.TrimSpace(offer.ItemArchetypeID)
			offer.CostArchetypeID = strings.TrimSpace(offer.CostArchetypeID)
			if offer.ID == "" || offer.ItemArchetypeID == "" || offer.CostArchetypeID == "" || offer.Quantity == 0 || offer.CostQuantity == 0 {
				return nil, ErrInvalidCatalog
			}
			if _, exists := offerIDs[offer.ID]; exists {
				return nil, ErrInvalidCatalog
			}
			offerIDs[offer.ID] = struct{}{}
		}
		catalog.byNPC[entry.NPCArchetypeID] = entry
	}
	return catalog, nil
}

func (c *Catalog) Revision() string {
	if c == nil {
		return ""
	}
	return c.revision
}

func (c *Catalog) ShopForNPC(npcArchetypeID string) (Shop, bool) {
	if c == nil {
		return Shop{}, false
	}
	entry, ok := c.byNPC[npcArchetypeID]
	if !ok {
		return Shop{}, false
	}
	entry.Offers = append([]Offer(nil), entry.Offers...)
	return entry, true
}

func FindOffer(entry Shop, offerID string) (Offer, bool) {
	for _, offer := range entry.Offers {
		if offer.ID == offerID {
			return offer, true
		}
	}
	return Offer{}, false
}

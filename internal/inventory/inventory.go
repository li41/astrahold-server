// Package inventory owns authoritative character item-stack and minimal equipment state.
// Presentation metadata (mesh/icon/name) stays outside the Server; the Server stores stable item archetype IDs only.
package inventory

import (
	"errors"
	"math"
	"sort"
	"strings"
)

var (
	ErrInvalidArchetype      = errors.New("inventory: invalid archetype")
	ErrInvalidQuantity       = errors.New("inventory: invalid quantity")
	ErrFull                  = errors.New("inventory: full")
	ErrInsufficient          = errors.New("inventory: insufficient quantity")
	ErrQuantityOverflow      = errors.New("inventory: quantity overflow")
	ErrEquipmentSlotOccupied = errors.New("inventory: equipment slot occupied")
	ErrEquipmentSlotEmpty    = errors.New("inventory: equipment slot empty")
)

// Stack is an authoritative aggregate for one stackable item archetype.
type Stack struct {
	ArchetypeID string
	Quantity    uint32
}

// Inventory intentionally starts small: one stack per archetype plus the first MainHand equipment slot.
// The aggregate owns inventory/equipment transfer atomically so no caller can commit only half of an equip transaction.
type Inventory struct {
	maxStacks int
	stacks    map[string]uint32
	revision  uint64

	mainHand          string
	equipmentRevision uint64
}

func New(maxStacks int) *Inventory {
	if maxStacks < 0 {
		maxStacks = 0
	}
	return &Inventory{maxStacks: maxStacks, stacks: make(map[string]uint32)}
}

func (i *Inventory) Revision() uint64 {
	if i == nil {
		return 0
	}
	return i.revision
}

func (i *Inventory) EquipmentRevision() uint64 {
	if i == nil {
		return 0
	}
	return i.equipmentRevision
}

func (i *Inventory) MainHand() string {
	if i == nil {
		return ""
	}
	return i.mainHand
}

func (i *Inventory) Add(archetypeID string, quantity uint32) error {
	if i == nil {
		return ErrFull
	}
	archetypeID = strings.TrimSpace(archetypeID)
	if archetypeID == "" {
		return ErrInvalidArchetype
	}
	if quantity == 0 {
		return ErrInvalidQuantity
	}

	current, exists := i.stacks[archetypeID]
	if !exists && len(i.stacks) >= i.maxStacks {
		return ErrFull
	}
	if uint64(current)+uint64(quantity) > math.MaxUint32 {
		return ErrQuantityOverflow
	}

	i.stacks[archetypeID] = current + quantity
	i.revision++
	return nil
}

func (i *Inventory) Remove(archetypeID string, quantity uint32) error {
	if i == nil {
		return ErrInsufficient
	}
	archetypeID = strings.TrimSpace(archetypeID)
	if archetypeID == "" {
		return ErrInvalidArchetype
	}
	if quantity == 0 {
		return ErrInvalidQuantity
	}

	current := i.stacks[archetypeID]
	if current < quantity {
		return ErrInsufficient
	}
	remaining := current - quantity
	if remaining == 0 {
		delete(i.stacks, archetypeID)
	} else {
		i.stacks[archetypeID] = remaining
	}
	i.revision++
	return nil
}

// EquipMainHand atomically moves one item from Inventory into MainHand.
func (i *Inventory) EquipMainHand(archetypeID string) error {
	if i == nil {
		return ErrInsufficient
	}
	archetypeID = strings.TrimSpace(archetypeID)
	if archetypeID == "" {
		return ErrInvalidArchetype
	}
	if i.mainHand != "" {
		return ErrEquipmentSlotOccupied
	}
	current := i.stacks[archetypeID]
	if current == 0 {
		return ErrInsufficient
	}

	if current == 1 {
		delete(i.stacks, archetypeID)
	} else {
		i.stacks[archetypeID] = current - 1
	}
	i.mainHand = archetypeID
	i.revision++
	i.equipmentRevision++
	return nil
}

// UnequipMainHand atomically moves the equipped MainHand item back into Inventory.
func (i *Inventory) UnequipMainHand() (string, error) {
	if i == nil || i.mainHand == "" {
		return "", ErrEquipmentSlotEmpty
	}
	archetypeID := i.mainHand
	current, exists := i.stacks[archetypeID]
	if !exists && len(i.stacks) >= i.maxStacks {
		return "", ErrFull
	}
	if current == math.MaxUint32 {
		return "", ErrQuantityOverflow
	}

	i.stacks[archetypeID] = current + 1
	i.mainHand = ""
	i.revision++
	i.equipmentRevision++
	return archetypeID, nil
}

func (i *Inventory) Quantity(archetypeID string) uint32 {
	if i == nil {
		return 0
	}
	return i.stacks[archetypeID]
}

// Snapshot returns a deterministic copy safe for persistence/protocol translation.
func (i *Inventory) Snapshot() []Stack {
	if i == nil || len(i.stacks) == 0 {
		return nil
	}
	out := make([]Stack, 0, len(i.stacks))
	for archetypeID, quantity := range i.stacks {
		out = append(out, Stack{ArchetypeID: archetypeID, Quantity: quantity})
	}
	sort.Slice(out, func(a, b int) bool { return out[a].ArchetypeID < out[b].ArchetypeID })
	return out
}

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
	ErrWeightExceeded        = errors.New("inventory: carry weight exceeded")
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

// WeightPolicy is fixed-point Server gameplay data. MaxWeight == 0 disables weight enforcement.
// UnitWeights may override DefaultUnitWeight for specific ItemArchetypeIDs. A zero default falls
// back to one unit so newly-authored items cannot silently become weightless.
type WeightPolicy struct {
	MaxWeight         uint64
	DefaultUnitWeight uint32
	UnitWeights       map[string]uint32
}

// Inventory intentionally starts small: one stack per archetype plus the first MainHand equipment slot.
// The aggregate owns inventory/equipment transfer atomically so no caller can commit only half of an equip transaction.
type Inventory struct {
	maxStacks int
	stacks    map[string]uint32
	revision  uint64

	maxWeight         uint64
	currentWeight     uint64
	defaultUnitWeight uint32
	unitWeights       map[string]uint32

	mainHand          string
	equipmentRevision uint64
}

func New(maxStacks int) *Inventory {
	return NewWithWeightPolicy(maxStacks, WeightPolicy{})
}

func NewWithWeightPolicy(maxStacks int, policy WeightPolicy) *Inventory {
	if maxStacks < 0 {
		maxStacks = 0
	}
	defaultWeight := policy.DefaultUnitWeight
	if defaultWeight == 0 {
		defaultWeight = 1
	}
	weights := make(map[string]uint32, len(policy.UnitWeights))
	for archetypeID, weight := range policy.UnitWeights {
		archetypeID = strings.TrimSpace(archetypeID)
		if archetypeID == "" || weight == 0 {
			continue
		}
		weights[archetypeID] = weight
	}
	return &Inventory{
		maxStacks:         maxStacks,
		stacks:            make(map[string]uint32),
		maxWeight:         policy.MaxWeight,
		defaultUnitWeight: defaultWeight,
		unitWeights:       weights,
	}
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

func (i *Inventory) CurrentWeight() uint64 {
	if i == nil {
		return 0
	}
	return i.currentWeight
}

func (i *Inventory) MaxWeight() uint64 {
	if i == nil {
		return 0
	}
	return i.maxWeight
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
	addedWeight := i.weightFor(archetypeID, quantity)
	if i.maxWeight > 0 && (addedWeight > i.maxWeight || i.currentWeight > i.maxWeight-addedWeight) {
		return ErrWeightExceeded
	}

	i.stacks[archetypeID] = current + quantity
	i.currentWeight += addedWeight
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
	i.currentWeight -= i.weightFor(archetypeID, quantity)
	i.revision++
	return nil
}

// Exchange atomically consumes one authoritative stack quantity and grants another.
// Validation is completed before mutation, and a successful exchange advances inventory revision exactly once.
func (i *Inventory) Exchange(removeArchetypeID string, removeQuantity uint32, addArchetypeID string, addQuantity uint32) error {
	if i == nil {
		return ErrInsufficient
	}
	removeArchetypeID = strings.TrimSpace(removeArchetypeID)
	addArchetypeID = strings.TrimSpace(addArchetypeID)
	if removeArchetypeID == "" || addArchetypeID == "" {
		return ErrInvalidArchetype
	}
	if removeQuantity == 0 || addQuantity == 0 {
		return ErrInvalidQuantity
	}

	removeCurrent := i.stacks[removeArchetypeID]
	if removeCurrent < removeQuantity {
		return ErrInsufficient
	}

	if removeArchetypeID == addArchetypeID {
		remaining := uint64(removeCurrent - removeQuantity)
		final := remaining + uint64(addQuantity)
		if final > math.MaxUint32 {
			return ErrQuantityOverflow
		}
		removedWeight := i.weightFor(removeArchetypeID, removeQuantity)
		addedWeight := i.weightFor(addArchetypeID, addQuantity)
		if i.maxWeight > 0 {
			baseWeight := i.currentWeight - removedWeight
			if addedWeight > i.maxWeight || baseWeight > i.maxWeight-addedWeight {
				return ErrWeightExceeded
			}
		}
		i.stacks[removeArchetypeID] = uint32(final)
		i.currentWeight = i.currentWeight - removedWeight + addedWeight
		i.revision++
		return nil
	}

	addCurrent, addExists := i.stacks[addArchetypeID]
	if uint64(addCurrent)+uint64(addQuantity) > math.MaxUint32 {
		return ErrQuantityOverflow
	}

	stackCountAfterRemove := len(i.stacks)
	if removeCurrent == removeQuantity {
		stackCountAfterRemove--
	}
	if !addExists && stackCountAfterRemove >= i.maxStacks {
		return ErrFull
	}

	removedWeight := i.weightFor(removeArchetypeID, removeQuantity)
	addedWeight := i.weightFor(addArchetypeID, addQuantity)
	if i.maxWeight > 0 {
		baseWeight := i.currentWeight - removedWeight
		if addedWeight > i.maxWeight || baseWeight > i.maxWeight-addedWeight {
			return ErrWeightExceeded
		}
	}

	remaining := removeCurrent - removeQuantity
	if remaining == 0 {
		delete(i.stacks, removeArchetypeID)
	} else {
		i.stacks[removeArchetypeID] = remaining
	}
	i.stacks[addArchetypeID] = addCurrent + addQuantity
	i.currentWeight = i.currentWeight - removedWeight + addedWeight
	i.revision++
	return nil
}

// EquipMainHand atomically moves one item from Inventory into MainHand. Carry weight is unchanged:
// equipped items remain part of the character's authoritative carried load.
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

// UnequipMainHand atomically moves the equipped MainHand item back into Inventory. Carry weight is
// unchanged because the item was already counted while equipped.
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

func (i *Inventory) weightFor(archetypeID string, quantity uint32) uint64 {
	if i == nil || i.maxWeight == 0 || quantity == 0 {
		return 0
	}
	unitWeight := i.defaultUnitWeight
	if override := i.unitWeights[archetypeID]; override > 0 {
		unitWeight = override
	}
	return uint64(unitWeight) * uint64(quantity)
}

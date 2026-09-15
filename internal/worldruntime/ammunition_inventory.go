package worldruntime

import "github.com/li41/astrahold-server/internal/ammunition"

func init() {
	for _, definition := range ammunition.Definitions() {
		if _, exists := defaultInventoryUnitWeights[definition.ItemArchetypeID]; exists {
			panic("worldruntime: ammunition inventory weight collision")
		}
		defaultInventoryUnitWeights[definition.ItemArchetypeID] = definition.UnitWeight
	}
}

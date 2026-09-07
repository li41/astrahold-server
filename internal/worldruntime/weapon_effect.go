package worldruntime

import (
	"errors"

	"github.com/li41/astrahold-server/internal/character"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/weaponeffect"
	"github.com/li41/astrahold-server/internal/world"
)

var defaultWeaponEffectCatalog = mustDefaultWeaponEffectCatalog()

func mustDefaultWeaponEffectCatalog() *weaponeffect.Catalog {
	catalog, err := weaponeffect.Default()
	if err != nil {
		panic(err)
	}
	return catalog
}

// applyEquippedWeaponDamageHit runs only after authoritative target legality and actual damage
// resolution have succeeded. Client equipment presentation and predicted hits never invoke this path.
func (r *Runtime) applyEquippedWeaponDamageHit(actorID world.EntityID, sourceSessionID session.ID, actualDamage uint32, report *StepReport) {
	if r == nil || report == nil || sourceSessionID == 0 || actualDamage == 0 {
		return
	}
	s, ok := r.sessions.Get(sourceSessionID)
	if !ok || s.EntityID != actorID || !s.CharacterIdentity.Valid() {
		return
	}
	inv := r.inventories[s.CharacterIdentity.ID]
	if inv == nil {
		return
	}
	definition, ok := defaultWeaponEffectCatalog.Resolve(inv.MainHand())
	if !ok || definition.ManaRestoreOnDamageHit == 0 {
		return
	}
	before, ok := r.characters.State(actorID)
	if !ok || before.Defeated || before.MP >= before.MaxMP {
		return
	}
	after, err := r.characters.RestoreMP(actorID, definition.ManaRestoreOnDamageHit)
	if err != nil {
		if errors.Is(err, character.ErrResourceFull) || errors.Is(err, character.ErrCharacterDefeated) {
			return
		}
		report.CommandErrors = append(report.CommandErrors, CommandError{Command: "weapon_damage_hit_effect", SessionID: sourceSessionID, Err: err})
		return
	}
	if after.MP != before.MP {
		r.markEntityVitalsDirty(actorID)
	}
}

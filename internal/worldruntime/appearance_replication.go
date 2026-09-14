package worldruntime

import (
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
)

// appearanceSnapshotForSession projects presentation state from the same authoritative character
// appearance and equipped WeaponType truth used by basic-attack damage. The Client must not
// recompute BasicAttackAffinityBonus from cosmetic assets or local equipment guesses.
func (r *Runtime) appearanceSnapshotForSession(s *session.Session) protocol.AppearanceSnapshot {
	if r == nil || s == nil || s.EntityID == 0 {
		return protocol.AppearanceSnapshot{}
	}

	snapshot := protocol.AppearanceSnapshot{SkinID: r.characterSkills.appearanceID(s.EntityID)}
	definition, ok := r.equippedCatalogWeapon(s.EntityID, s.ID)
	if !ok || definition.Weapon == nil {
		return snapshot
	}
	snapshot.BasicAttackAffinityBonus = r.matchingSkinBasicAttackDamageBonus(s.EntityID, definition.Weapon.WeaponType)
	return snapshot
}

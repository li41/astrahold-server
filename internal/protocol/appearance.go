package protocol

import "github.com/li41/astrahold-server/internal/appearance"

const MessageAppearanceSnapshot MessageType = 119

// AppearanceSnapshot is the owning character's complete Server-authoritative appearance gameplay
// state introduced by Protocol v28. SkinID is a stable Server-owned gameplay identity, never a
// Client asset path or UUID. BasicAttackAffinityBonus is the currently active flat raw physical
// basic-attack bonus derived from the selected skin and authoritative equipped main-hand WeaponType.
// Clients must present this value but must not recompute or use it to decide gameplay outcomes.
type AppearanceSnapshot struct {
	SkinID                   appearance.SkinID
	BasicAttackAffinityBonus uint32
}

func (AppearanceSnapshot) Type() MessageType { return MessageAppearanceSnapshot }

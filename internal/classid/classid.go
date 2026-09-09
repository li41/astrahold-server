// Package classid owns the stable Server-side vocabulary for character profession identity.
//
// Empty ID means the character has not been assigned a class yet. It is deliberately not a
// seventh class and must never be accepted by class-scoped gameplay policies.
package classid

type ID string

const (
	Oathguard     ID = "class_oathguard"
	Breaker       ID = "class_breaker"
	Ranger        ID = "class_ranger"
	StarfireMage  ID = "class_starfire_mage"
	Oathhealer    ID = "class_oathhealer"
	Shadowblade   ID = "class_shadowblade"
)

var canonical = [...]ID{
	Oathguard,
	Breaker,
	Ranger,
	StarfireMage,
	Oathhealer,
	Shadowblade,
}

// IsCanonical reports whether id is one of the six locked Astrahold ClassIDs.
// Empty/unassigned and unknown future values return false.
func IsCanonical(id ID) bool {
	switch id {
	case Oathguard, Breaker, Ranger, StarfireMage, Oathhealer, Shadowblade:
		return true
	default:
		return false
	}
}

// Parse accepts only an exact canonical wire/storage spelling. It intentionally does not trim
// or case-fold IDs so persistence and authored data cannot silently normalize a typo.
func Parse(raw string) (ID, bool) {
	id := ID(raw)
	return id, IsCanonical(id)
}

// All returns a defensive copy in the stable design order.
func All() []ID {
	out := make([]ID, len(canonical))
	copy(out, canonical[:])
	return out
}

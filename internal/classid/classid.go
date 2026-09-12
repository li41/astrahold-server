// Package classid retains the retired fixed-profession vocabulary needed by Protocol v27
// compatibility, legacy durable-data validation, and explicit compatibility fixtures.
//
// ClassID is not current classless gameplay truth and must not be used to gate new actions,
// equipment, skills, or durable character state. Keep the exact historical spellings stable
// until the coordinated compatibility removal is safe.
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

// IsCanonical reports whether id is one of the six retired fixed-profession IDs that remain
// valid compatibility input. Empty and unknown values return false.
func IsCanonical(id ID) bool {
	switch id {
	case Oathguard, Breaker, Ranger, StarfireMage, Oathhealer, Shadowblade:
		return true
	default:
		return false
	}
}

// Parse accepts only an exact historical wire/storage spelling. It intentionally does not trim
// or case-fold IDs so legacy persistence and compatibility fixtures cannot silently normalize a typo.
func Parse(raw string) (ID, bool) {
	id := ID(raw)
	return id, IsCanonical(id)
}

// All returns a defensive copy in the historical stable design order.
func All() []ID {
	out := make([]ID, len(canonical))
	copy(out, canonical[:])
	return out
}

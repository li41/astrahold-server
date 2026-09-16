package gameplayworld

// MapExitPolicy is Server-owned policy for transferring a character out of a map.
// Restricted item IDs are stable ItemArchetypeIDs; callers cannot supply or override them.
type MapExitPolicy struct {
	RestrictedItemArchetypeIDs []string
}

// MapExitPolicyFor returns the authoritative exit policy for a map.
// map0 currently has no formally approved restricted ItemArchetypeIDs, so its policy is explicit but empty.
func MapExitPolicyFor(id MapID) (MapExitPolicy, bool) {
	switch id {
	case MapIDGMRoom:
		return MapExitPolicy{}, true
	default:
		return MapExitPolicy{}, false
	}
}

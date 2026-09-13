package character

import "github.com/li41/astrahold-server/internal/actionresource"

// ActionResourceState is the classless read view of a character's current authoritative action resource.
// State still carries legacy field names while Protocol v27 compatibility remains, but production callers
// should consume this view instead of depending on those compatibility-era names directly.
type ActionResourceState struct {
	ID       actionresource.ID
	Current  uint32
	Max      uint32
	Progress uint32
}

func (state State) ActionResource() ActionResourceState {
	return ActionResourceState{
		ID:       state.ClassResourceID,
		Current:  state.ClassResource,
		Max:      state.MaxClassResource,
		Progress: state.ClassResourceProgress,
	}
}

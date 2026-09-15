package character

import "github.com/li41/astrahold-server/internal/actionresource"

// ActionResourceState is the classless read view of a character's current authoritative action resource.
type ActionResourceState struct {
	ID       actionresource.ID
	Current  uint32
	Max      uint32
	Progress uint32
}

func (state State) ActionResource() ActionResourceState {
	return ActionResourceState{
		ID:       state.ActionResourceID,
		Current:  state.ActionResourceCurrent,
		Max:      state.MaxActionResource,
		Progress: state.ActionResourceProgress,
	}
}

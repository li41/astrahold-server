package worldruntime

import "github.com/li41/astrahold-server/internal/siege"

// WithSiegeOwnershipPersistence restores the durable castle owner after WithSiegeMatch and
// installs the one-shot CAS commit barrier used by authoritative throne resolution.
func WithSiegeOwnershipPersistence(state siege.CastleOwnershipState, committer siege.CastleOwnershipCommitter) Option {
	return func(r *Runtime) {
		if r.siege == nil {
			panic("worldruntime: siege match must be configured before siege ownership persistence")
		}
		if err := r.siege.ConfigureCastleOwnershipPersistence(state, committer); err != nil {
			panic(err)
		}
	}
}

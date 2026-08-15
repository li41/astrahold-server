package tcpudp

import (
	"context"

	"github.com/li41/astrahold-server/internal/worldruntime"
)

// F.18 transport joins through AwaitJoinOwned, so the controlled runtime must keep the
// existing join-failure tests on the same rejection path rather than inheriting fake success.
func (r *controlledRuntime) AwaitJoinOwned(_ context.Context, request worldruntime.JoinRequest) (worldruntime.SessionOwnershipFence, error) {
	if r.joinErr != nil {
		return worldruntime.SessionOwnershipFence{}, r.joinErr
	}
	return r.fakeRuntime.AwaitJoinOwned(context.Background(), request)
}

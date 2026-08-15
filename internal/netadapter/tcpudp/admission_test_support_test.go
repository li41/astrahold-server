package tcpudp

import (
	"context"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/protocol"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

func (f *fakeRuntime) AwaitCharacterAdmission(_ context.Context, identity characteridentity.Binding) (worldruntime.CharacterAdmissionLease, error) {
	return worldruntime.CharacterAdmissionLease{CharacterID: identity.ID, Generation: 1, ExpiresAt: time.Now().Add(time.Minute)}, nil
}

func (f *fakeRuntime) AwaitCharacterConnectionPlan(_ context.Context, identity characteridentity.Binding) (worldruntime.CharacterConnectionPlan, error) {
	return worldruntime.CharacterConnectionPlan{
		AdmissionLease: worldruntime.CharacterAdmissionLease{CharacterID: identity.ID, Generation: 1, ExpiresAt: time.Now().Add(time.Minute)},
	}, nil
}

func (f *fakeRuntime) ReleaseCharacterAdmission(context.Context, worldruntime.CharacterAdmissionLease) error {
	return nil
}

func (f *fakeRuntime) AwaitJoin(_ context.Context, request worldruntime.JoinRequest) error {
	return f.EnqueueJoin(request)
}

func (f *fakeRuntime) AwaitJoinOwned(_ context.Context, request worldruntime.JoinRequest) (worldruntime.SessionOwnershipFence, error) {
	if err := f.EnqueueJoin(request); err != nil {
		return worldruntime.SessionOwnershipFence{}, err
	}
	if request.Session.CharacterIdentity.Assurance != characteridentity.AssuranceTrusted {
		return worldruntime.SessionOwnershipFence{}, nil
	}
	return worldruntime.SessionOwnershipFence{
		SessionID: request.Session.ID,
		EntityID: request.Session.EntityID,
		CharacterID: request.Session.CharacterIdentity.ID,
		Epoch: 1,
	}, nil
}

func (f *fakeRuntime) AwaitOwnershipTransfer(_ context.Context, expected worldruntime.SessionOwnershipFence, replacement *session.Session) (worldruntime.SessionOwnershipFence, error) {
	return worldruntime.SessionOwnershipFence{
		SessionID: replacement.ID,
		EntityID: replacement.EntityID,
		CharacterID: replacement.CharacterIdentity.ID,
		Epoch: expected.Epoch + 1,
	}, nil
}

func (f *fakeRuntime) EnqueueFencedLeave(fence worldruntime.SessionOwnershipFence) error {
	return f.EnqueueLeave(fence.SessionID)
}

func (f *fakeRuntime) EnqueueFencedMove(fence worldruntime.SessionOwnershipFence, sequence uint32, input protocol.ClientMoveInput) error {
	return f.EnqueueMove(fence.SessionID, sequence, input)
}

func (f *fakeRuntime) EnqueueFencedUseAction(fence worldruntime.SessionOwnershipFence, sequence uint32, action protocol.ClientUseAction) error {
	return f.EnqueueUseAction(fence.SessionID, sequence, action)
}

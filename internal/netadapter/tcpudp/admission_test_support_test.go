package tcpudp

import (
	"context"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

func (f *fakeRuntime) AwaitCharacterAdmission(_ context.Context, identity characteridentity.Binding) (worldruntime.CharacterAdmissionLease, error) {
	return worldruntime.CharacterAdmissionLease{CharacterID: identity.ID, Generation: 1, ExpiresAt: time.Now().Add(time.Minute)}, nil
}

func (f *fakeRuntime) ReleaseCharacterAdmission(context.Context, worldruntime.CharacterAdmissionLease) error {
	return nil
}

func (f *fakeRuntime) AwaitJoin(_ context.Context, request worldruntime.JoinRequest) error {
	return f.EnqueueJoin(request)
}

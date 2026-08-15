package tcpudp

import (
	"context"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/worldruntime"
)

func (f *fakeRuntime) AwaitCharacterAdmission(context.Context, characteridentity.Binding) error {
	return nil
}

func (f *fakeRuntime) AwaitJoin(_ context.Context, request worldruntime.JoinRequest) error {
	return f.EnqueueJoin(request)
}

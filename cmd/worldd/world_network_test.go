package main

import (
	"testing"

	"github.com/li41/astrahold-server/internal/netadapter/tcpudp"
	"github.com/li41/astrahold-server/internal/session"
	"github.com/li41/astrahold-server/internal/world"
)

func TestValidateWorldNetworkMode(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{worldNetworkTCPUDP, worldNetworkBrowserWSDev} {
		if err := validateWorldNetworkMode(mode); err != nil {
			t.Fatalf("mode %q rejected: %v", mode, err)
		}
	}
	if err := validateWorldNetworkMode("both"); err == nil {
		t.Fatal("expected simultaneous/unknown network mode to be rejected")
	}
}

func TestBrowserWSDevRequiresLoopback(t *testing.T) {
	t.Parallel()
	for _, address := range []string{"127.0.0.1:7779", "[::1]:7779", "localhost:7779"} {
		if err := validateBrowserWSLoopbackAddress(address); err != nil {
			t.Fatalf("loopback %q rejected: %v", address, err)
		}
	}
	for _, address := range []string{"0.0.0.0:7779", "192.0.2.10:7779", ":7779"} {
		if err := validateBrowserWSLoopbackAddress(address); err == nil {
			t.Fatalf("non-loopback %q unexpectedly accepted", address)
		}
	}
}

func TestAdaptBrowserPlayerFactoryPreservesGameplayBootstrap(t *testing.T) {
	t.Parallel()
	factory := tcpudp.PlayerFactory(func(_ session.ID, entityID world.EntityID) tcpudp.PlayerSpec {
		return tcpudp.PlayerSpec{
			Entity: world.EntityState{ID: entityID, Kind: world.EntityPlayer},
			Speed: 5.25, Radius: 0.4, MaxStepHeight: 0.6, AOIRadius: 72,
		}
	})
	adapted := adaptBrowserPlayerFactory(factory)
	if adapted == nil {
		t.Fatal("adapted factory is nil")
	}
	got := adapted(7, 19)
	if got.Entity.ID != 19 || got.Speed != 5.25 || got.Radius != 0.4 || got.MaxStepHeight != 0.6 || got.AOIRadius != 72 {
		t.Fatalf("adapted player spec changed gameplay bootstrap: %#v", got)
	}
}

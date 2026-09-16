package gameplayworld

import (
	"math"
	"testing"
)

func TestGMRoomTransferTargetMatchesAuthoredRoomCenterFacingStockKeeper(t *testing.T) {
	loaded, err := LoadFile("../../worlds/gm-room/gameplay.json")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.SHA256 != GMRoomGameplaySHA256 {
		t.Fatalf("gm-room gameplay sha=%q want=%q", loaded.SHA256, GMRoomGameplaySHA256)
	}
	if loaded.Definition.Revision != GMRoomWorldRevision {
		t.Fatalf("gm-room revision=%q want=%q", loaded.Definition.Revision, GMRoomWorldRevision)
	}

	derived, err := GMRoomArrivalTransform(loaded.Definition)
	if err != nil {
		t.Fatal(err)
	}
	target := GMRoomTransferTarget()
	if target.MapID != MapIDGMRoom || target.WorldID != GMRoomWorldID || target.Revision != GMRoomWorldRevision || target.GameplaySHA256 != loaded.SHA256 {
		t.Fatalf("target identity=%#v", target)
	}
	if derived.Position != target.Transform.Position {
		t.Fatalf("arrival position=%#v target=%#v", derived.Position, target.Transform.Position)
	}
	if math.Abs(float64(derived.Yaw-target.Transform.Yaw)) > 0.0001 {
		t.Fatalf("arrival yaw=%f target=%f", derived.Yaw, target.Transform.Yaw)
	}
	if target.Transform.Position.X != 0 || target.Transform.Position.Y != 0 || target.Transform.Position.Z != 0 || target.Transform.Position.Layer != 0 {
		t.Fatalf("map0 arrival is not room center: %#v", target.Transform.Position)
	}
}

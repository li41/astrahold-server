package worldruntime

import (
	"testing"

	"github.com/li41/astrahold-server/internal/world"
)

func TestShadowbladeSideBackClassifierUsesAuthoritativeTargetYaw(t *testing.T) {
	target := world.EntityState{ID: 20, Transform: world.Transform{Position: world.Position{}, Yaw: 0}}
	front := world.EntityState{ID: 10, Transform: world.Transform{Position: world.Position{Z: 2}}}
	back := world.EntityState{ID: 11, Transform: world.Transform{Position: world.Position{Z: -2}}}
	side := world.EntityState{ID: 12, Transform: world.Transform{Position: world.Position{X: 2}}}
	if isSideOrBackAttackPosition(front, target) { t.Fatal("front position classified side/back") }
	if !isSideOrBackAttackPosition(back, target) { t.Fatal("back position not classified side/back") }
	if !isSideOrBackAttackPosition(side, target) { t.Fatal("side position not classified side/back") }

	target.Transform.Yaw = 90
	if isSideOrBackAttackPosition(side, target) { t.Fatal("+X should be front when target yaw is 90") }
	if !isSideOrBackAttackPosition(front, target) { t.Fatal("+Z should be side when target yaw is 90") }
}

func TestShadowbladeClassifierRejectsCoincidentPosition(t *testing.T) {
	actor := world.EntityState{ID: 10}
	target := world.EntityState{ID: 20}
	if isSideOrBackAttackPosition(actor, target) { t.Fatal("coincident position must not build Flaw") }
}

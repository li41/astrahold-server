package worldruntime

import "testing"

func TestReviveProtectionStatusUsesServerUntilAndCancelMarksVitalsDirty(t *testing.T) {
	rt, _ := makeResurrectionRuntime(t)
	rt.config.PostReviveProtectionTicks = 5
	report := &StepReport{}

	rt.grantReviveProtection(2, 10, report)
	if got := rt.reviveProtectionUntilTick(2, 12); got != 15 {
		t.Fatalf("until=%d want=15", got)
	}
	if report.Metrics.ReviveProtectionsGranted != 1 {
		t.Fatalf("grant metrics=%#v", report.Metrics)
	}

	beforeRevision := rt.entityVitalsRevision[2]
	rt.cancelReviveProtectionByDamageAction(2, report)
	if got := rt.reviveProtectionUntilTick(2, 12); got != 0 {
		t.Fatalf("cancelled protection until=%d want=0", got)
	}
	if rt.entityVitalsRevision[2] != beforeRevision+1 {
		t.Fatalf("vitals revision=%d want=%d", rt.entityVitalsRevision[2], beforeRevision+1)
	}
	if _, dirty := rt.dirtyVitalsEntities[2]; !dirty {
		t.Fatal("protection cancellation did not mark EntityVitalsState dirty")
	}
	if report.Metrics.ReviveProtectionsCancelledByDamageAction != 1 {
		t.Fatalf("cancel metrics=%#v", report.Metrics)
	}
}

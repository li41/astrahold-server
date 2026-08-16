package main

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestWaitRecordsControllerObservationTiming(t *testing.T) {
	now := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	plan, candidate := makeTestPlanAndCandidate(t, now, 27, time.Second)
	if _, err := publishRollout(plan, candidate, now, writeAtomicFile); err != nil {
		t.Fatal(err)
	}
	clock := now
	wroteA := false
	wroteB := false
	result, err := waitForAcks(plan, candidate, func() time.Time { return clock }, func(delay time.Duration) {
		clock = clock.Add(delay)
		if !wroteA && !clock.Before(now.Add(100*time.Millisecond)) {
			writeExactAck(t, plan.members[0], plan, candidate, clock)
			wroteA = true
		}
		if !wroteB && !clock.Before(now.Add(300*time.Millisecond)) {
			writeExactAck(t, plan.members[1], plan, candidate, clock)
			wroteB = true
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "converged" || result.Observation == nil {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Observation.TimingSource != "controller" || result.Observation.ElapsedMS != 300 {
		t.Fatalf("unexpected evidence: %+v", result.Observation)
	}
	want := []rolloutAckObservation{
		{InstanceID: "instance-a", FirstObservedAt: now.Add(100 * time.Millisecond).Format(time.RFC3339Nano), ObservedElapsedMS: 100},
		{InstanceID: "instance-b", FirstObservedAt: now.Add(300 * time.Millisecond).Format(time.RFC3339Nano), ObservedElapsedMS: 300},
	}
	if !reflect.DeepEqual(result.Observation.Acks, want) {
		t.Fatalf("acks=%+v want=%+v", result.Observation.Acks, want)
	}
}

func TestWaitIncompleteKeepsObservedMemberTiming(t *testing.T) {
	now := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	plan, candidate := makeTestPlanAndCandidate(t, now, 28, 200*time.Millisecond)
	if _, err := publishRollout(plan, candidate, now, writeAtomicFile); err != nil {
		t.Fatal(err)
	}
	clock := now
	wroteA := false
	result, err := waitForAcks(plan, candidate, func() time.Time { return clock }, func(delay time.Duration) {
		clock = clock.Add(delay)
		if !wroteA && !clock.Before(now.Add(50*time.Millisecond)) {
			writeExactAck(t, plan.members[0], plan, candidate, clock)
			wroteA = true
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "incomplete" || result.Reason != "timeout" || result.Observation == nil {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Observation.ElapsedMS != 200 || len(result.Observation.Acks) != 1 || result.Observation.Acks[0].InstanceID != "instance-a" {
		t.Fatalf("unexpected evidence: %+v", result.Observation)
	}
	if !reflect.DeepEqual(result.PendingInstances, []string{"instance-b"}) {
		t.Fatalf("pending=%v", result.PendingInstances)
	}
}

func TestWaitRejectsBackwardControllerClock(t *testing.T) {
	now := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	plan, candidate := makeTestPlanAndCandidate(t, now, 29, 200*time.Millisecond)
	if _, err := publishRollout(plan, candidate, now, writeAtomicFile); err != nil {
		t.Fatal(err)
	}
	clock := now
	_, err := waitForAcks(plan, candidate, func() time.Time { return clock }, func(time.Duration) {
		clock = now.Add(-time.Millisecond)
	})
	if err == nil || !strings.Contains(err.Error(), "clock moved backwards") {
		t.Fatalf("expected controller clock rejection, got %v", err)
	}
}

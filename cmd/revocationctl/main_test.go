package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRevocationDigestMatchesF24Contract(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "candidate.json")
	writeTestJSON(t, path, revocationDefinition{
		SchemaVersion: revocationSchemaVersion,
		Revision:      "leaf-002",
		RevokedSPKISHA256: []string{
			strings.Repeat("f", 64),
			strings.Repeat("0", 64),
			strings.Repeat("0", 64),
		},
	})
	candidate, err := loadRevocationCandidate(path)
	if err != nil {
		t.Fatal(err)
	}
	wantIDs := []string{strings.Repeat("0", 64), strings.Repeat("f", 64)}
	if !reflect.DeepEqual(candidate.ids, wantIDs) {
		t.Fatalf("ids=%v want=%v", candidate.ids, wantIDs)
	}
	const wantDigest = "4ce1f1107d60491c194cf64acb9438db78a1a3eeb7c05edb40a07dd33a7a3469"
	if got := hex.EncodeToString(candidate.digest[:]); got != wantDigest {
		t.Fatalf("digest=%s want=%s", got, wantDigest)
	}
}

func TestPublishStagesAllRevocationsBeforeManifestCommit(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 0, 0, 0, time.UTC)
	plan, candidate := makeTestPlanAndCandidate(t, now, 2, 5*time.Second)
	var writes []string
	writer := func(path string, data []byte, mode fs.FileMode) error {
		writes = append(writes, path)
		return writeAtomicFile(path, data, mode)
	}
	result, err := publishRollout(plan, candidate, now, writer)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "published" {
		t.Fatalf("status=%q", result.Status)
	}
	if len(writes) != 4 {
		t.Fatalf("writes=%v", writes)
	}
	for _, path := range writes[:2] {
		if filepath.Base(path) != "revocations.json" {
			t.Fatalf("first phase wrote non-revocation target: %v", writes)
		}
	}
	for _, path := range writes[2:] {
		if filepath.Base(path) != "distribution.json" {
			t.Fatalf("commit phase wrote non-distribution target: %v", writes)
		}
	}
}

func TestPublishPartialCommitIsFailForwardAndIdempotent(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 0, 0, 0, time.UTC)
	plan, candidate := makeTestPlanAndCandidate(t, now, 2, 5*time.Second)
	oldDigest := strings.Repeat("0", 64)
	for _, member := range plan.members {
		writeTestJSON(t, member.distributionFile, distributionDefinition{
			SchemaVersion:             1,
			Epoch:                     1,
			RevocationAuthoritySHA256: oldDigest,
			ValidUntil:                now.Add(30 * time.Second).Format(time.RFC3339),
		})
	}
	bDist := plan.members[1].distributionFile
	writer := func(path string, data []byte, mode fs.FileMode) error {
		if path == bDist {
			return errors.New("synthetic manifest failure")
		}
		return writeAtomicFile(path, data, mode)
	}
	result, err := publishRollout(plan, candidate, now, writer)
	if err == nil {
		t.Fatal("expected publish failure")
	}
	if result.Status != "partial" || !reflect.DeepEqual(result.PublishedInstances, []string{"instance-a"}) || !reflect.DeepEqual(result.FailedInstances, []string{"instance-b"}) {
		t.Fatalf("unexpected partial result: %+v", result)
	}
	for _, member := range plan.members {
		rev, err := loadRevocationCandidate(member.revocationFile)
		if err != nil {
			t.Fatal(err)
		}
		if rev.digest != candidate.digest {
			t.Fatalf("%s did not receive staged revocation", member.instanceID)
		}
	}
	aDist, err := loadDistributionIfExists(plan.members[0].distributionFile)
	if err != nil {
		t.Fatal(err)
	}
	bSnapshot, err := loadDistributionIfExists(plan.members[1].distributionFile)
	if err != nil {
		t.Fatal(err)
	}
	if aDist == nil || aDist.epoch != 2 || bSnapshot == nil || bSnapshot.epoch != 1 {
		t.Fatalf("unexpected manifest epochs: a=%+v b=%+v", aDist, bSnapshot)
	}
	result, err = publishRollout(plan, candidate, now, writeAtomicFile)
	if err != nil || result.Status != "published" || len(result.PublishedInstances) != 2 {
		t.Fatalf("idempotent retry failed: result=%+v err=%v", result, err)
	}
}

func TestPublishRejectsRollbackBeforeAnyWrite(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 0, 0, 0, time.UTC)
	plan, candidate := makeTestPlanAndCandidate(t, now, 2, 5*time.Second)
	for _, member := range plan.members {
		writeTestJSON(t, member.distributionFile, distributionDefinition{
			SchemaVersion:             1,
			Epoch:                     3,
			RevocationAuthoritySHA256: hex.EncodeToString(candidate.digest[:]),
			ValidUntil:                plan.validUntil.Format(time.RFC3339),
		})
	}
	writes := 0
	writer := func(path string, data []byte, mode fs.FileMode) error {
		writes++
		return writeAtomicFile(path, data, mode)
	}
	result, err := publishRollout(plan, candidate, now, writer)
	if err == nil || result.Status != "rejected" {
		t.Fatalf("expected rejected rollback, result=%+v err=%v", result, err)
	}
	if writes != 0 {
		t.Fatalf("rollback preflight performed %d writes", writes)
	}
}

func TestWaitConvergesOnlyOnExactRequiredAckSet(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 0, 0, 0, time.UTC)
	plan, candidate := makeTestPlanAndCandidate(t, now, 7, 2*time.Second)
	if _, err := publishRollout(plan, candidate, now, writeAtomicFile); err != nil {
		t.Fatal(err)
	}
	writeExactAck(t, plan.members[0], plan, candidate, now.Add(100*time.Millisecond))
	writeExactAck(t, plan.members[1], plan, candidate, now.Add(200*time.Millisecond))
	clock := now
	result, err := waitForAcks(plan, candidate, func() time.Time { return clock }, func(delay time.Duration) { clock = clock.Add(delay) })
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "converged" || !reflect.DeepEqual(result.AcknowledgedInstances, []string{"instance-a", "instance-b"}) || len(result.PendingInstances) != 0 {
		t.Fatalf("unexpected convergence result: %+v", result)
	}
}

func TestWaitReturnsIncompleteForMissingRequiredMember(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 0, 0, 0, time.UTC)
	plan, candidate := makeTestPlanAndCandidate(t, now, 8, 500*time.Millisecond)
	if _, err := publishRollout(plan, candidate, now, writeAtomicFile); err != nil {
		t.Fatal(err)
	}
	writeExactAck(t, plan.members[0], plan, candidate, now.Add(50*time.Millisecond))
	clock := now
	result, err := waitForAcks(plan, candidate, func() time.Time { return clock }, func(delay time.Duration) { clock = clock.Add(delay) })
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "incomplete" || result.Reason != "timeout" || !reflect.DeepEqual(result.AcknowledgedInstances, []string{"instance-a"}) || !reflect.DeepEqual(result.PendingInstances, []string{"instance-b"}) {
		t.Fatalf("unexpected incomplete result: %+v", result)
	}
}

func TestWaitRejectsSupersededAck(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 0, 0, 0, time.UTC)
	plan, candidate := makeTestPlanAndCandidate(t, now, 9, 500*time.Millisecond)
	if _, err := publishRollout(plan, candidate, now, writeAtomicFile); err != nil {
		t.Fatal(err)
	}
	writeExactAck(t, plan.members[0], plan, candidate, now.Add(50*time.Millisecond))
	writeTestJSON(t, plan.members[1].ackFile, ackDefinition{
		SchemaVersion:             1,
		InstanceID:                plan.members[1].instanceID,
		Epoch:                     plan.epoch + 1,
		RevocationRevision:        candidate.revision,
		RevocationAuthoritySHA256: hex.EncodeToString(candidate.digest[:]),
		ValidUntil:                plan.validUntil.Format(time.RFC3339),
		AcknowledgedAt:            now.Add(50 * time.Millisecond).Format(time.RFC3339),
	})
	clock := now
	_, err := waitForAcks(plan, candidate, func() time.Time { return clock }, func(delay time.Duration) { clock = clock.Add(delay) })
	if err == nil || !strings.Contains(err.Error(), "superseded") {
		t.Fatalf("expected superseded error, got %v", err)
	}
}

func TestLoadRolloutPlanRejectsSharedAckFile(t *testing.T) {
	dir := t.TempDir()
	planPath := filepath.Join(dir, "rollout.json")
	writeTestJSON(t, planPath, rolloutPlanDefinition{
		SchemaVersion:        1,
		Epoch:                1,
		ValidUntil:           "2026-08-16T16:00:00Z",
		AckTimeout:           "1s",
		PollInterval:         "100ms",
		RevocationSourceFile: "candidate.json",
		RequiredInstances: []rolloutMemberDefinition{
			{InstanceID: "instance-a", RevocationFile: "a/rev.json", DistributionFile: "a/dist.json", AckFile: "shared.ack"},
			{InstanceID: "instance-b", RevocationFile: "b/rev.json", DistributionFile: "b/dist.json", AckFile: "shared.ack"},
		},
	})
	if _, err := loadRolloutPlan(planPath); err == nil || !strings.Contains(err.Error(), "share one ack_file") {
		t.Fatalf("expected shared ack rejection, got %v", err)
	}
}

func makeTestPlanAndCandidate(t *testing.T, now time.Time, epoch uint64, timeout time.Duration) (*rolloutPlan, *revocationCandidate) {
	t.Helper()
	root := t.TempDir()
	a := filepath.Join(root, "a")
	b := filepath.Join(root, "b")
	if err := os.MkdirAll(a, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(b, 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "candidate.json")
	writeTestJSON(t, source, revocationDefinition{
		SchemaVersion:     1,
		Revision:          "leaf-target",
		RevokedSPKISHA256: []string{strings.Repeat("a", 64)},
	})
	candidate, err := loadRevocationCandidate(source)
	if err != nil {
		t.Fatal(err)
	}
	plan := &rolloutPlan{
		path:                 filepath.Join(root, "plan.json"),
		epoch:                epoch,
		validUntil:           now.Add(30 * time.Second),
		ackTimeout:           timeout,
		pollInterval:         50 * time.Millisecond,
		revocationSourceFile: source,
		members: []rolloutMember{
			{instanceID: "instance-a", revocationFile: filepath.Join(a, "revocations.json"), distributionFile: filepath.Join(a, "distribution.json"), ackFile: filepath.Join(a, "ack.json")},
			{instanceID: "instance-b", revocationFile: filepath.Join(b, "revocations.json"), distributionFile: filepath.Join(b, "distribution.json"), ackFile: filepath.Join(b, "ack.json")},
		},
	}
	return plan, candidate
}

func writeExactAck(t *testing.T, member rolloutMember, plan *rolloutPlan, candidate *revocationCandidate, at time.Time) {
	t.Helper()
	writeTestJSON(t, member.ackFile, ackDefinition{
		SchemaVersion:             1,
		InstanceID:                member.instanceID,
		Epoch:                     plan.epoch,
		RevocationRevision:        candidate.revision,
		RevocationAuthoritySHA256: hex.EncodeToString(candidate.digest[:]),
		ValidUntil:                plan.validUntil.Format(time.RFC3339),
		AcknowledgedAt:            at.UTC().Format(time.RFC3339),
	})
}

func writeTestJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCLIWaitUsesExitCodeTwoForIncomplete(t *testing.T) {
	now := time.Date(2026, 8, 16, 15, 0, 0, 0, time.UTC)
	root := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	source := filepath.Join(root, "candidate.json")
	writeTestJSON(t, source, revocationDefinition{SchemaVersion: 1, Revision: "leaf-cli", RevokedSPKISHA256: []string{strings.Repeat("b", 64)}})
	planPath := filepath.Join(root, "plan.json")
	writeTestJSON(t, planPath, rolloutPlanDefinition{
		SchemaVersion:        1,
		Epoch:                11,
		ValidUntil:           now.Add(30 * time.Second).Format(time.RFC3339),
		AckTimeout:           "200ms",
		PollInterval:         "50ms",
		RevocationSourceFile: source,
		RequiredInstances: []rolloutMemberDefinition{
			{InstanceID: "instance-a", RevocationFile: "a/revocations.json", DistributionFile: "a/distribution.json", AckFile: "a/ack.json"},
			{InstanceID: "instance-b", RevocationFile: "b/revocations.json", DistributionFile: "b/distribution.json", AckFile: "b/ack.json"},
		},
	})
	plan, err := loadRolloutPlan(planPath)
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := loadRevocationCandidate(source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := publishRollout(plan, candidate, now, writeAtomicFile); err != nil {
		t.Fatal(err)
	}
	writeExactAck(t, plan.members[0], plan, candidate, now)
	clock := now
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"wait", "-plan", planPath}, &stdout, &stderr, func() time.Time { return clock }, func(delay time.Duration) { clock = clock.Add(delay) })
	if code != 2 {
		t.Fatalf("code=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var result rolloutResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "incomplete" || !reflect.DeepEqual(result.PendingInstances, []string{"instance-b"}) {
		t.Fatalf("unexpected result: %+v", result)
	}
}

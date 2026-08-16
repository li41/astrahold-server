package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEvidenceJournalRoundTripAndDecisionMetrics(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	converged := testEvidenceResult("converged", "", 300,
		[]string{"instance-a", "instance-b"},
		[]string{"instance-a", "instance-b"}, nil,
		[]rolloutAckObservation{
			{InstanceID: "instance-a", FirstObservedAt: "2026-08-17T00:00:00.1Z", ObservedElapsedMS: 100},
			{InstanceID: "instance-b", FirstObservedAt: "2026-08-17T00:00:00.3Z", ObservedElapsedMS: 300},
		})
	path1, err := writeRolloutEvidenceWithReader(root, "rollout", converged, bytes.NewReader(bytes.Repeat([]byte{0x11}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path1)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("record mode=%#o want=0600", got)
	}
	dirInfo, err := os.Stat(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("dir mode=%#o want=0700", got)
	}

	incomplete := testEvidenceResult("incomplete", "timeout", 500,
		[]string{"instance-a", "instance-b"},
		[]string{"instance-a"}, []string{"instance-b"},
		[]rolloutAckObservation{{InstanceID: "instance-a", FirstObservedAt: "2026-08-17T00:00:00.05Z", ObservedElapsedMS: 50}})
	if _, err := writeRolloutEvidenceWithReader(root, "wait", incomplete, bytes.NewReader(bytes.Repeat([]byte{0x22}, 16))); err != nil {
		t.Fatal(err)
	}

	summary, err := loadRolloutEvidenceSummary(root)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Records != 2 || summary.ConvergedRecords != 1 || summary.IncompleteRecords != 1 || summary.TimeoutRecords != 1 || summary.LeaseExpiredRecords != 0 || summary.MaxElapsedMS != 500 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if len(summary.Instances) != 2 {
		t.Fatalf("instances=%+v", summary.Instances)
	}
	a, b := summary.Instances[0], summary.Instances[1]
	if a.InstanceID != "instance-a" || a.RequiredRecords != 2 || a.ObservedRecords != 2 || a.PendingRecords != 0 || a.MaxObservedElapsedMS != 100 {
		t.Fatalf("instance-a=%+v", a)
	}
	if b.InstanceID != "instance-b" || b.RequiredRecords != 2 || b.ObservedRecords != 1 || b.PendingRecords != 1 || b.MaxObservedElapsedMS != 300 {
		t.Fatalf("instance-b=%+v", b)
	}
}

func TestEvidenceJournalRejectsBroadDirectoryPermissions(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(root, 0o750); err != nil {
		t.Fatal(err)
	}
	result := testEvidenceResult("converged", "", 0, []string{"instance-a"}, []string{"instance-a"}, nil,
		[]rolloutAckObservation{{InstanceID: "instance-a", FirstObservedAt: "2026-08-17T00:00:00Z", ObservedElapsedMS: 0}})
	if _, err := writeRolloutEvidenceWithReader(root, "wait", result, bytes.NewReader(bytes.Repeat([]byte{0x33}, 16))); err == nil || !strings.Contains(err.Error(), "owner-only") {
		t.Fatalf("expected owner-only directory rejection, got %v", err)
	}
}

func TestEvidenceReportRejectsTamperedAckPartition(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	result := testEvidenceResult("incomplete", "timeout", 200,
		[]string{"instance-a", "instance-b"}, []string{"instance-a"}, []string{"instance-b"},
		[]rolloutAckObservation{{InstanceID: "instance-a", FirstObservedAt: "2026-08-17T00:00:00.1Z", ObservedElapsedMS: 100}})
	path, err := writeRolloutEvidenceWithReader(root, "wait", result, bytes.NewReader(bytes.Repeat([]byte{0x44}, 16)))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record rolloutEvidenceRecord
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatal(err)
	}
	record.Result.PendingInstances = nil
	data, err = json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRolloutEvidenceSummary(root); err == nil || !strings.Contains(err.Error(), "partition required instances") {
		t.Fatalf("expected tamper rejection, got %v", err)
	}
}

func TestEvidenceReportIgnoresUnrelatedFilesButRejectsMatchingSymlink(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "README.txt"), []byte("operator note\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	summary, err := loadRolloutEvidenceSummary(root)
	if err != nil || summary.Records != 0 {
		t.Fatalf("empty summary=%+v err=%v", summary, err)
	}
	target := filepath.Join(root, "target.json")
	if err := os.WriteFile(target, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, evidenceRecordPrefix+strings.Repeat("5", 32)+evidenceRecordSuffix)
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRolloutEvidenceSummary(root); err == nil || !strings.Contains(err.Error(), "non-symlink") {
		t.Fatalf("expected matching symlink rejection, got %v", err)
	}
}

func TestCLIWaitPersistsIncompleteEvidenceAndKeepsExitTwo(t *testing.T) {
	now := time.Date(2026, 8, 17, 1, 0, 0, 0, time.UTC)
	root := t.TempDir()
	for _, name := range []string{"a", "b"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	source := filepath.Join(root, "candidate.json")
	writeTestJSON(t, source, revocationDefinition{SchemaVersion: 1, Revision: "leaf-f28", RevokedSPKISHA256: []string{strings.Repeat("c", 64)}})
	planPath := filepath.Join(root, "plan.json")
	writeTestJSON(t, planPath, rolloutPlanDefinition{
		SchemaVersion:        1,
		Epoch:                28,
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
	writeExactAck(t, plan.members[0], plan, candidate, now.Add(50*time.Millisecond))
	clock := now
	var stdout, stderr bytes.Buffer
	evidenceDir := filepath.Join(root, "evidence")
	code := run([]string{"wait", "-plan", planPath, "-evidence-dir", evidenceDir}, &stdout, &stderr,
		func() time.Time { return clock }, func(delay time.Duration) { clock = clock.Add(delay) })
	if code != 2 {
		t.Fatalf("exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	var result rolloutResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "incomplete" || result.Reason != "timeout" {
		t.Fatalf("result=%+v", result)
	}
	summary, err := loadRolloutEvidenceSummary(evidenceDir)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Records != 1 || summary.IncompleteRecords != 1 || summary.TimeoutRecords != 1 || summary.Instances[1].InstanceID != "instance-b" || summary.Instances[1].PendingRecords != 1 {
		t.Fatalf("summary=%+v", summary)
	}
}

func TestCLIReportReturnsStrictAggregateWithoutPlan(t *testing.T) {
	root := filepath.Join(t.TempDir(), "evidence")
	result := testEvidenceResult("converged", "", 120, []string{"instance-a"}, []string{"instance-a"}, nil,
		[]rolloutAckObservation{{InstanceID: "instance-a", FirstObservedAt: "2026-08-17T00:00:00.12Z", ObservedElapsedMS: 120}})
	if _, err := writeRolloutEvidenceWithReader(root, "wait", result, bytes.NewReader(bytes.Repeat([]byte{0x66}, 16))); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := run([]string{"report", "-evidence-dir", root}, &stdout, &stderr, time.Now, time.Sleep)
	if code != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var summary rolloutEvidenceSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if summary.Records != 1 || summary.ConvergedRecords != 1 || summary.MaxElapsedMS != 120 {
		t.Fatalf("summary=%+v", summary)
	}
}

func testEvidenceResult(status, reason string, elapsed int64, required, acknowledged, pending []string, acks []rolloutAckObservation) rolloutResult {
	started := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	completed := started.Add(time.Duration(elapsed) * time.Millisecond)
	return rolloutResult{
		SchemaVersion:             1,
		Status:                    status,
		Epoch:                     28,
		RevocationRevision:        "leaf-028",
		RevocationAuthoritySHA256: strings.Repeat("a", 64),
		ValidUntil:                "2026-08-17T00:10:00Z",
		RequiredInstances:         append([]string(nil), required...),
		AcknowledgedInstances:     append([]string(nil), acknowledged...),
		PendingInstances:          append([]string(nil), pending...),
		Observation: &rolloutObservationEvidence{
			TimingSource: "controller",
			StartedAt:    started.Format(time.RFC3339Nano),
			CompletedAt:  completed.Format(time.RFC3339Nano),
			ElapsedMS:    elapsed,
			Acks:         append([]rolloutAckObservation(nil), acks...),
		},
		Reason: reason,
	}
}

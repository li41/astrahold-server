package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	rolloutEvidenceSchemaVersion uint16 = 1
	maxEvidenceRecordBytes              = 64 * 1024
	maxEvidenceRecords                  = 1024
	evidenceRecordPrefix                = "rollout-evidence-v1-"
	evidenceRecordSuffix                = ".json"
)

type rolloutEvidenceRecord struct {
	SchemaVersion uint16        `json:"schema_version"`
	RecordID      string        `json:"record_id"`
	Command       string        `json:"command"`
	RecordedAt    string        `json:"recorded_at"`
	Result        rolloutResult `json:"result"`
}

type rolloutEvidenceInstanceSummary struct {
	InstanceID           string `json:"instance_id"`
	RequiredRecords      int    `json:"required_records"`
	ObservedRecords      int    `json:"observed_records"`
	PendingRecords       int    `json:"pending_records"`
	MaxObservedElapsedMS int64  `json:"max_observed_elapsed_ms"`
}

type rolloutEvidenceSummary struct {
	SchemaVersion       uint16                            `json:"schema_version"`
	TimingSource        string                            `json:"timing_source"`
	Records             int                               `json:"records"`
	ConvergedRecords    int                               `json:"converged_records"`
	IncompleteRecords   int                               `json:"incomplete_records"`
	TimeoutRecords      int                               `json:"timeout_records"`
	LeaseExpiredRecords int                               `json:"lease_expired_records"`
	MaxElapsedMS        int64                             `json:"max_elapsed_ms"`
	Instances           []rolloutEvidenceInstanceSummary `json:"instances,omitempty"`
}

func writeRolloutEvidence(dir, command string, result rolloutResult) (string, error) {
	return writeRolloutEvidenceWithReader(dir, command, result, rand.Reader)
}

func writeRolloutEvidenceWithReader(dir, command string, result rolloutResult, random io.Reader) (string, error) {
	if command != "wait" && command != "rollout" {
		return "", fmt.Errorf("%w: evidence command must be wait or rollout", errConfig)
	}
	if random == nil {
		return "", fmt.Errorf("%w: evidence random source is unavailable", errConfig)
	}
	if err := validateEvidenceResult(result); err != nil {
		return "", err
	}
	absolute, err := prepareEvidenceDir(dir, true)
	if err != nil {
		return "", err
	}
	for attempt := 0; attempt < 4; attempt++ {
		var idBytes [16]byte
		if _, err := io.ReadFull(random, idBytes[:]); err != nil {
			return "", fmt.Errorf("%w: generate evidence record id: %v", errConfig, err)
		}
		recordID := hex.EncodeToString(idBytes[:])
		record := rolloutEvidenceRecord{
			SchemaVersion: rolloutEvidenceSchemaVersion,
			RecordID:      recordID,
			Command:       command,
			RecordedAt:    result.Observation.CompletedAt,
			Result:        result,
		}
		data, err := json.MarshalIndent(record, "", "  ")
		if err != nil {
			return "", fmt.Errorf("%w: encode evidence record: %v", errConfig, err)
		}
		data = append(data, '\n')
		if len(data) > maxEvidenceRecordBytes {
			return "", fmt.Errorf("%w: evidence record exceeds %d bytes", errConfig, maxEvidenceRecordBytes)
		}
		finalPath := filepath.Join(absolute, evidenceRecordPrefix+recordID+evidenceRecordSuffix)
		created, err := writeImmutableEvidenceFile(absolute, finalPath, data)
		if err == nil {
			return created, nil
		}
		if os.IsExist(err) {
			continue
		}
		return "", err
	}
	return "", fmt.Errorf("%w: could not allocate a unique evidence record id", errConfig)
}

func writeImmutableEvidenceFile(dir, finalPath string, data []byte) (string, error) {
	temp, err := os.CreateTemp(dir, ".rollout-evidence.tmp-*")
	if err != nil {
		return "", fmt.Errorf("%w: create evidence temp file: %v", errConfig, err)
	}
	tempPath := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}
	if err := temp.Chmod(0o600); err != nil {
		cleanup()
		return "", fmt.Errorf("%w: chmod evidence temp file: %v", errConfig, err)
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return "", fmt.Errorf("%w: write evidence temp file: %v", errConfig, err)
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return "", fmt.Errorf("%w: fsync evidence temp file: %v", errConfig, err)
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return "", fmt.Errorf("%w: close evidence temp file: %v", errConfig, err)
	}
	if err := os.Link(tempPath, finalPath); err != nil {
		_ = os.Remove(tempPath)
		if os.IsExist(err) {
			return "", err
		}
		return "", fmt.Errorf("%w: commit evidence record: %v", errConfig, err)
	}
	if err := os.Remove(tempPath); err != nil {
		return "", fmt.Errorf("%w: remove evidence temp link: %v", errConfig, err)
	}
	if err := syncDirectory(dir); err != nil {
		return "", fmt.Errorf("%w: fsync evidence directory: %v", errConfig, err)
	}
	return finalPath, nil
}

func loadRolloutEvidenceSummary(dir string) (rolloutEvidenceSummary, error) {
	summary := rolloutEvidenceSummary{SchemaVersion: rolloutEvidenceSchemaVersion, TimingSource: "controller"}
	absolute, err := prepareEvidenceDir(dir, false)
	if err != nil {
		return summary, err
	}
	entries, err := os.ReadDir(absolute)
	if err != nil {
		return summary, fmt.Errorf("%w: read evidence directory: %v", errConfig, err)
	}
	instanceStats := make(map[string]*rolloutEvidenceInstanceSummary)
	matching := 0
	for _, entry := range entries {
		recordID, ok := evidenceRecordIDFromName(entry.Name())
		if !ok {
			continue
		}
		matching++
		if matching > maxEvidenceRecords {
			return summary, fmt.Errorf("%w: evidence directory exceeds %d records", errConfig, maxEvidenceRecords)
		}
		path := filepath.Join(absolute, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return summary, fmt.Errorf("%w: inspect evidence record %q: %v", errConfig, entry.Name(), err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return summary, fmt.Errorf("%w: evidence record %q is not a regular non-symlink file", errConfig, entry.Name())
		}
		if info.Mode().Perm()&0o077 != 0 {
			return summary, fmt.Errorf("%w: evidence record %q must be owner-only", errConfig, entry.Name())
		}
		data, err := readBoundedRegularFile(path, maxEvidenceRecordBytes)
		if err != nil {
			return summary, fmt.Errorf("%w: read evidence record %q: %v", errConfig, entry.Name(), err)
		}
		var record rolloutEvidenceRecord
		if err := decodeStrictJSON(data, &record); err != nil {
			return summary, fmt.Errorf("%w: decode evidence record %q: %v", errConfig, entry.Name(), err)
		}
		if err := validateEvidenceRecord(record, recordID); err != nil {
			return summary, fmt.Errorf("%w: evidence record %q: %v", errConfig, entry.Name(), err)
		}
		summary.Records++
		if record.Result.Status == "converged" {
			summary.ConvergedRecords++
		} else {
			summary.IncompleteRecords++
			if record.Result.Reason == "timeout" {
				summary.TimeoutRecords++
			} else if record.Result.Reason == "lease_expired" {
				summary.LeaseExpiredRecords++
			}
		}
		if record.Result.Observation.ElapsedMS > summary.MaxElapsedMS {
			summary.MaxElapsedMS = record.Result.Observation.ElapsedMS
		}
		observedByID := make(map[string]int64, len(record.Result.Observation.Acks))
		for _, ack := range record.Result.Observation.Acks {
			observedByID[ack.InstanceID] = ack.ObservedElapsedMS
		}
		pendingSet := make(map[string]struct{}, len(record.Result.PendingInstances))
		for _, instanceID := range record.Result.PendingInstances {
			pendingSet[instanceID] = struct{}{}
		}
		for _, instanceID := range record.Result.RequiredInstances {
			stat := instanceStats[instanceID]
			if stat == nil {
				stat = &rolloutEvidenceInstanceSummary{InstanceID: instanceID}
				instanceStats[instanceID] = stat
			}
			stat.RequiredRecords++
			if elapsed, ok := observedByID[instanceID]; ok {
				stat.ObservedRecords++
				if elapsed > stat.MaxObservedElapsedMS {
					stat.MaxObservedElapsedMS = elapsed
				}
			}
			if _, ok := pendingSet[instanceID]; ok {
				stat.PendingRecords++
			}
		}
	}
	for _, stat := range instanceStats {
		summary.Instances = append(summary.Instances, *stat)
	}
	sort.Slice(summary.Instances, func(i, j int) bool { return summary.Instances[i].InstanceID < summary.Instances[j].InstanceID })
	return summary, nil
}

func validateEvidenceRecord(record rolloutEvidenceRecord, expectedID string) error {
	if record.SchemaVersion != rolloutEvidenceSchemaVersion {
		return fmt.Errorf("schema_version must be %d", rolloutEvidenceSchemaVersion)
	}
	if record.RecordID != expectedID || !isLowerHex(record.RecordID, 32) {
		return fmt.Errorf("record_id does not match the immutable filename")
	}
	if record.Command != "wait" && record.Command != "rollout" {
		return fmt.Errorf("command must be wait or rollout")
	}
	if _, err := parseCanonicalNanoTime(record.RecordedAt, "recorded_at"); err != nil {
		return err
	}
	if err := validateEvidenceResult(record.Result); err != nil {
		return err
	}
	if record.RecordedAt != record.Result.Observation.CompletedAt {
		return fmt.Errorf("recorded_at must equal observation completed_at")
	}
	return nil
}

func validateEvidenceResult(result rolloutResult) error {
	if result.SchemaVersion != 1 || result.Epoch == 0 {
		return fmt.Errorf("%w: evidence result has invalid schema or epoch", errConfig)
	}
	if result.RevocationRevision == "" || result.RevocationRevision != strings.TrimSpace(result.RevocationRevision) || len(result.RevocationRevision) > maxRevisionBytes {
		return fmt.Errorf("%w: evidence result has invalid revocation_revision", errConfig)
	}
	if _, err := parseDigestHex(result.RevocationAuthoritySHA256, "evidence revocation_authority_sha256"); err != nil {
		return err
	}
	if _, err := parseCanonicalTime(result.ValidUntil, "evidence valid_until"); err != nil {
		return err
	}
	if err := validateSortedInstanceIDs(result.RequiredInstances, true); err != nil {
		return fmt.Errorf("%w: required_instances: %v", errConfig, err)
	}
	if len(result.RequiredInstances) > maxRequiredInstances {
		return fmt.Errorf("%w: required_instances exceeds %d", errConfig, maxRequiredInstances)
	}
	if err := validateSortedInstanceIDs(result.AcknowledgedInstances, false); err != nil {
		return fmt.Errorf("%w: acknowledged_instances: %v", errConfig, err)
	}
	if err := validateSortedInstanceIDs(result.PendingInstances, false); err != nil {
		return fmt.Errorf("%w: pending_instances: %v", errConfig, err)
	}
	if len(result.PublishedInstances) != 0 || len(result.FailedInstances) != 0 {
		return fmt.Errorf("%w: evidence records only accept final wait results", errConfig)
	}
	if result.Observation == nil || result.Observation.TimingSource != "controller" {
		return fmt.Errorf("%w: evidence result requires controller observation", errConfig)
	}
	if _, err := parseCanonicalNanoTime(result.Observation.StartedAt, "observation started_at"); err != nil {
		return err
	}
	if _, err := parseCanonicalNanoTime(result.Observation.CompletedAt, "observation completed_at"); err != nil {
		return err
	}
	if result.Observation.ElapsedMS < 0 {
		return fmt.Errorf("%w: observation elapsed_ms must be non-negative", errConfig)
	}
	observedIDs := make([]string, 0, len(result.Observation.Acks))
	for index, ack := range result.Observation.Acks {
		if err := validateInstanceID(ack.InstanceID); err != nil {
			return fmt.Errorf("%w: observation acks[%d]: %v", errConfig, index, err)
		}
		if _, err := parseCanonicalNanoTime(ack.FirstObservedAt, fmt.Sprintf("observation acks[%d].first_observed_at", index)); err != nil {
			return err
		}
		if ack.ObservedElapsedMS < 0 || ack.ObservedElapsedMS > result.Observation.ElapsedMS {
			return fmt.Errorf("%w: observation acks[%d] elapsed is outside rollout window", errConfig, index)
		}
		observedIDs = append(observedIDs, ack.InstanceID)
	}
	if err := validateSortedInstanceIDs(observedIDs, false); err != nil {
		return fmt.Errorf("%w: observation acks: %v", errConfig, err)
	}
	if !equalStrings(observedIDs, result.AcknowledgedInstances) {
		return fmt.Errorf("%w: observation ack set must equal acknowledged_instances", errConfig)
	}
	if result.Status == "converged" {
		if result.Reason != "" || len(result.PendingInstances) != 0 || !equalStrings(result.AcknowledgedInstances, result.RequiredInstances) {
			return fmt.Errorf("%w: converged evidence does not match all required instances", errConfig)
		}
		return nil
	}
	if result.Status != "incomplete" || (result.Reason != "timeout" && result.Reason != "lease_expired") {
		return fmt.Errorf("%w: evidence status must be converged or incomplete with a known reason", errConfig)
	}
	combined := append(append([]string(nil), result.AcknowledgedInstances...), result.PendingInstances...)
	sort.Strings(combined)
	if !equalStrings(combined, result.RequiredInstances) || hasOverlap(result.AcknowledgedInstances, result.PendingInstances) {
		return fmt.Errorf("%w: incomplete evidence must partition required instances into acknowledged and pending", errConfig)
	}
	return nil
}

func prepareEvidenceDir(path string, create bool) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" || trimmed != path {
		return "", fmt.Errorf("%w: evidence directory must be a non-empty trimmed path", errConfig)
	}
	absolute, err := filepath.Abs(filepath.Clean(trimmed))
	if err != nil {
		return "", fmt.Errorf("%w: evidence directory path: %v", errConfig, err)
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		if !os.IsNotExist(err) || !create {
			return "", fmt.Errorf("%w: evidence directory: %v", errConfig, err)
		}
		if err := os.MkdirAll(absolute, 0o700); err != nil {
			return "", fmt.Errorf("%w: create evidence directory: %v", errConfig, err)
		}
		if err := os.Chmod(absolute, 0o700); err != nil {
			return "", fmt.Errorf("%w: chmod evidence directory: %v", errConfig, err)
		}
		info, err = os.Lstat(absolute)
		if err != nil {
			return "", fmt.Errorf("%w: inspect evidence directory: %v", errConfig, err)
		}
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return "", fmt.Errorf("%w: evidence directory must be a real directory, not a symlink", errConfig)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("%w: evidence directory must be owner-only (0700 or stricter)", errConfig)
	}
	return absolute, nil
}

func evidenceRecordIDFromName(name string) (string, bool) {
	if !strings.HasPrefix(name, evidenceRecordPrefix) || !strings.HasSuffix(name, evidenceRecordSuffix) {
		return "", false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(name, evidenceRecordPrefix), evidenceRecordSuffix)
	if !isLowerHex(id, 32) {
		return "", false
	}
	return id, true
}

func isLowerHex(value string, length int) bool {
	if len(value) != length || value != strings.ToLower(value) {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func parseCanonicalNanoTime(value, field string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil || value != parsed.UTC().Format(time.RFC3339Nano) {
		return time.Time{}, fmt.Errorf("%w: %s must be canonical UTC RFC3339/RFC3339Nano", errConfig, field)
	}
	return parsed.UTC(), nil
}

func validateSortedInstanceIDs(values []string, requireNonEmpty bool) error {
	if requireNonEmpty && len(values) == 0 {
		return fmt.Errorf("must not be empty")
	}
	for i, value := range values {
		if err := validateInstanceID(value); err != nil {
			return err
		}
		if i > 0 && values[i-1] >= value {
			return fmt.Errorf("must be strictly sorted and unique")
		}
	}
	return nil
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasOverlap(a, b []string) bool {
	seen := make(map[string]struct{}, len(a))
	for _, value := range a {
		seen[value] = struct{}{}
	}
	for _, value := range b {
		if _, ok := seen[value]; ok {
			return true
		}
	}
	return false
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

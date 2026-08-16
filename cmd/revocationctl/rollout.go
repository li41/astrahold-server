package main

import (
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func preflightTargets(plan *rolloutPlan, candidate *revocationCandidate) ([]targetPair, error) {
	pairsByKey := make(map[string]*targetPair)
	for _, member := range plan.members {
		for _, path := range []string{member.revocationFile, member.distributionFile} {
			parent := filepath.Dir(path)
			info, err := os.Stat(parent)
			if err != nil || !info.IsDir() {
				return nil, fmt.Errorf("%w: target parent %q is unavailable", errConfig, parent)
			}
			if info, err := os.Stat(path); err == nil && !info.Mode().IsRegular() {
				return nil, fmt.Errorf("%w: target %q is not a regular file", errConfig, path)
			} else if err != nil && !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("%w: inspect target %q: %v", errConfig, path, err)
			}
		}
		if existing, err := loadDistributionIfExists(member.distributionFile); err != nil {
			return nil, err
		} else if existing != nil {
			if existing.epoch > plan.epoch {
				return nil, fmt.Errorf("%w: target %q is already at newer epoch %d", errConfig, member.instanceID, existing.epoch)
			}
			if existing.epoch == plan.epoch && (existing.digest != candidate.digest || !existing.validUntil.Equal(plan.validUntil)) {
				return nil, fmt.Errorf("%w: target %q reuses epoch %d with conflicting digest or lease", errConfig, member.instanceID, plan.epoch)
			}
		}
		key := member.revocationFile + "\x00" + member.distributionFile
		pair := pairsByKey[key]
		if pair == nil {
			pair = &targetPair{revocationFile: member.revocationFile, distributionFile: member.distributionFile}
			pairsByKey[key] = pair
		}
		pair.instances = append(pair.instances, member.instanceID)
	}
	pairs := make([]targetPair, 0, len(pairsByKey))
	for _, pair := range pairsByKey {
		sort.Strings(pair.instances)
		pairs = append(pairs, *pair)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].distributionFile == pairs[j].distributionFile {
			return pairs[i].revocationFile < pairs[j].revocationFile
		}
		return pairs[i].distributionFile < pairs[j].distributionFile
	})
	return pairs, nil
}

func waitForAcks(plan *rolloutPlan, candidate *revocationCandidate, now func() time.Time, sleep func(time.Duration)) (rolloutResult, error) {
	result := baseResult(plan, candidate)
	if plan == nil || candidate == nil || now == nil || sleep == nil {
		result.Status = "rejected"
		return result, errConfig
	}
	start := now().UTC()
	deadline := start.Add(plan.ackTimeout)
	if plan.validUntil.Before(deadline) {
		deadline = plan.validUntil
	}
	for {
		if err := validatePublishedTargets(plan, candidate); err != nil {
			result.Status = "rejected"
			return result, err
		}
		acknowledged, pending, err := collectAcks(plan, candidate)
		if err != nil {
			result.Status = "rejected"
			return result, err
		}
		result.AcknowledgedInstances = acknowledged
		result.PendingInstances = pending
		if len(pending) == 0 {
			result.Status = "converged"
			return result, nil
		}
		current := now().UTC()
		if !current.Before(deadline) {
			result.Status = "incomplete"
			if !current.Before(plan.validUntil) {
				result.Reason = "lease_expired"
			} else {
				result.Reason = "timeout"
			}
			return result, nil
		}
		delay := plan.pollInterval
		if remaining := deadline.Sub(current); remaining < delay {
			delay = remaining
		}
		if delay > 0 {
			sleep(delay)
		}
	}
}

func validatePublishedTargets(plan *rolloutPlan, candidate *revocationCandidate) error {
	seen := make(map[string]struct{})
	for _, member := range plan.members {
		key := member.revocationFile + "\x00" + member.distributionFile
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		revocation, err := loadRevocationCandidate(member.revocationFile)
		if err != nil {
			return fmt.Errorf("%w: published revocation for %q: %v", errConfig, member.instanceID, err)
		}
		if revocation.revision != candidate.revision || revocation.digest != candidate.digest {
			return fmt.Errorf("%w: published revocation for %q drifted from target", errConfig, member.instanceID)
		}
		distribution, err := loadDistributionIfExists(member.distributionFile)
		if err != nil {
			return err
		}
		if distribution == nil || distribution.epoch != plan.epoch || distribution.digest != candidate.digest || !distribution.validUntil.Equal(plan.validUntil) {
			return fmt.Errorf("%w: published distribution for %q does not match target epoch", errConfig, member.instanceID)
		}
	}
	return nil
}

func collectAcks(plan *rolloutPlan, candidate *revocationCandidate) ([]string, []string, error) {
	acknowledged := make([]string, 0, len(plan.members))
	pending := make([]string, 0, len(plan.members))
	for _, member := range plan.members {
		ack, err := loadAck(member.ackFile)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				pending = append(pending, member.instanceID)
				continue
			}
			return nil, nil, fmt.Errorf("%w: ack for %q: %v", errConfig, member.instanceID, err)
		}
		if ack.InstanceID != member.instanceID {
			return nil, nil, fmt.Errorf("%w: ack file for %q contains instance_id %q", errConfig, member.instanceID, ack.InstanceID)
		}
		if ack.Epoch < plan.epoch {
			pending = append(pending, member.instanceID)
			continue
		}
		if ack.Epoch > plan.epoch {
			return nil, nil, fmt.Errorf("%w: ack for %q superseded target epoch %d with epoch %d", errConfig, member.instanceID, plan.epoch, ack.Epoch)
		}
		digest, err := parseDigestHex(ack.RevocationAuthoritySHA256, "ack revocation_authority_sha256")
		if err != nil {
			return nil, nil, err
		}
		validUntil, err := parseCanonicalTime(ack.ValidUntil, "ack valid_until")
		if err != nil {
			return nil, nil, err
		}
		acknowledgedAt, err := parseCanonicalTime(ack.AcknowledgedAt, "ack acknowledged_at")
		if err != nil {
			return nil, nil, err
		}
		if digest != candidate.digest || !validUntil.Equal(plan.validUntil) || ack.RevocationRevision != candidate.revision {
			return nil, nil, fmt.Errorf("%w: ack for %q conflicts with target epoch metadata", errConfig, member.instanceID)
		}
		if acknowledgedAt.After(validUntil) {
			return nil, nil, fmt.Errorf("%w: ack for %q was acknowledged after lease expiry", errConfig, member.instanceID)
		}
		acknowledged = append(acknowledged, member.instanceID)
	}
	sort.Strings(acknowledged)
	sort.Strings(pending)
	return acknowledged, pending, nil
}

func loadAck(path string) (ackDefinition, error) {
	data, err := readBoundedRegularFile(path, maxAckBytes)
	if err != nil {
		return ackDefinition{}, err
	}
	var ack ackDefinition
	if err := decodeStrictJSON(data, &ack); err != nil {
		return ackDefinition{}, err
	}
	if ack.SchemaVersion != ackSchemaVersion || ack.Epoch == 0 {
		return ackDefinition{}, fmt.Errorf("invalid ack schema or epoch")
	}
	if err := validateInstanceID(ack.InstanceID); err != nil {
		return ackDefinition{}, err
	}
	if ack.RevocationRevision == "" || ack.RevocationRevision != strings.TrimSpace(ack.RevocationRevision) || len(ack.RevocationRevision) > maxRevisionBytes {
		return ackDefinition{}, fmt.Errorf("invalid ack revocation_revision")
	}
	if _, err := parseDigestHex(ack.RevocationAuthoritySHA256, "ack revocation_authority_sha256"); err != nil {
		return ackDefinition{}, err
	}
	if _, err := parseCanonicalTime(ack.ValidUntil, "ack valid_until"); err != nil {
		return ackDefinition{}, err
	}
	if _, err := parseCanonicalTime(ack.AcknowledgedAt, "ack acknowledged_at"); err != nil {
		return ackDefinition{}, err
	}
	return ack, nil
}

func baseResult(plan *rolloutPlan, candidate *revocationCandidate) rolloutResult {
	result := rolloutResult{SchemaVersion: 1}
	if plan != nil {
		result.Epoch = plan.epoch
		result.ValidUntil = plan.validUntil.Format(time.RFC3339)
		for _, member := range plan.members {
			result.RequiredInstances = append(result.RequiredInstances, member.instanceID)
		}
		sort.Strings(result.RequiredInstances)
	}
	if candidate != nil {
		result.RevocationRevision = candidate.revision
		result.RevocationAuthoritySHA256 = hex.EncodeToString(candidate.digest[:])
	}
	return result
}

func writeAtomicFile(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("target directory %q is unavailable", dir)
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}
	if err := temp.Chmod(mode); err != nil {
		cleanup()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Chmod(path, mode); err != nil {
		return err
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

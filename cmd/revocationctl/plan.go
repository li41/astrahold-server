package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func loadRolloutPlan(path string) (*rolloutPlan, error) {
	absolute, err := filepath.Abs(filepath.Clean(strings.TrimSpace(path)))
	if err != nil || strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("%w: invalid plan path", errConfig)
	}
	data, err := readBoundedRegularFile(absolute, maxPlanBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: plan file: %v", errConfig, err)
	}
	var definition rolloutPlanDefinition
	if err := decodeStrictJSON(data, &definition); err != nil {
		return nil, fmt.Errorf("%w: decode plan: %v", errConfig, err)
	}
	if definition.SchemaVersion != rolloutPlanSchemaVersion {
		return nil, fmt.Errorf("%w: schema_version must be %d", errConfig, rolloutPlanSchemaVersion)
	}
	if definition.Epoch == 0 {
		return nil, fmt.Errorf("%w: epoch must be greater than zero", errConfig)
	}
	validUntil, err := parseCanonicalTime(definition.ValidUntil, "valid_until")
	if err != nil {
		return nil, err
	}
	ackTimeout, err := parseBoundedDuration(definition.AckTimeout, "ack_timeout", minAckTimeout, maxAckTimeout)
	if err != nil {
		return nil, err
	}
	pollInterval, err := parseBoundedDuration(definition.PollInterval, "poll_interval", minPollInterval, maxPollInterval)
	if err != nil {
		return nil, err
	}
	if pollInterval > ackTimeout {
		return nil, fmt.Errorf("%w: poll_interval must not exceed ack_timeout", errConfig)
	}
	if len(definition.RequiredInstances) == 0 || len(definition.RequiredInstances) > maxRequiredInstances {
		return nil, fmt.Errorf("%w: required_instances must contain 1..%d members", errConfig, maxRequiredInstances)
	}
	baseDir := filepath.Dir(absolute)
	resolve := func(value, field string) (string, error) {
		if value == "" || value != strings.TrimSpace(value) {
			return "", fmt.Errorf("%w: %s must be a non-empty trimmed path", errConfig, field)
		}
		if !filepath.IsAbs(value) {
			value = filepath.Join(baseDir, value)
		}
		resolved, err := filepath.Abs(filepath.Clean(value))
		if err != nil {
			return "", fmt.Errorf("%w: %s path: %v", errConfig, field, err)
		}
		return resolved, nil
	}
	source, err := resolve(definition.RevocationSourceFile, "revocation_source_file")
	if err != nil {
		return nil, err
	}
	plan := &rolloutPlan{
		path:                 absolute,
		epoch:                definition.Epoch,
		validUntil:           validUntil,
		ackTimeout:           ackTimeout,
		pollInterval:         pollInterval,
		revocationSourceFile: source,
		members:              make([]rolloutMember, 0, len(definition.RequiredInstances)),
	}
	instanceSeen := make(map[string]struct{}, len(definition.RequiredInstances))
	ackSeen := make(map[string]string, len(definition.RequiredInstances))
	revToDist := make(map[string]string, len(definition.RequiredInstances))
	distToRev := make(map[string]string, len(definition.RequiredInstances))
	for index, raw := range definition.RequiredInstances {
		if err := validateInstanceID(raw.InstanceID); err != nil {
			return nil, fmt.Errorf("%w: required_instances[%d]: %v", errConfig, index, err)
		}
		if _, exists := instanceSeen[raw.InstanceID]; exists {
			return nil, fmt.Errorf("%w: duplicate instance_id %q", errConfig, raw.InstanceID)
		}
		instanceSeen[raw.InstanceID] = struct{}{}
		revocationFile, err := resolve(raw.RevocationFile, fmt.Sprintf("required_instances[%d].revocation_file", index))
		if err != nil {
			return nil, err
		}
		distributionFile, err := resolve(raw.DistributionFile, fmt.Sprintf("required_instances[%d].distribution_file", index))
		if err != nil {
			return nil, err
		}
		ackFile, err := resolve(raw.AckFile, fmt.Sprintf("required_instances[%d].ack_file", index))
		if err != nil {
			return nil, err
		}
		if revocationFile == distributionFile || revocationFile == ackFile || distributionFile == ackFile {
			return nil, fmt.Errorf("%w: required_instances[%d] revocation/distribution/ack paths must be distinct", errConfig, index)
		}
		if ackFile == absolute || ackFile == source || distributionFile == absolute || distributionFile == source || revocationFile == absolute {
			return nil, fmt.Errorf("%w: plan/source paths must not overlap distribution/ack targets", errConfig)
		}
		if previous, exists := ackSeen[ackFile]; exists {
			return nil, fmt.Errorf("%w: instances %q and %q share one ack_file", errConfig, previous, raw.InstanceID)
		}
		ackSeen[ackFile] = raw.InstanceID
		if dist, exists := revToDist[revocationFile]; exists && dist != distributionFile {
			return nil, fmt.Errorf("%w: shared revocation_file must use the same distribution_file", errConfig)
		}
		if rev, exists := distToRev[distributionFile]; exists && rev != revocationFile {
			return nil, fmt.Errorf("%w: shared distribution_file must use the same revocation_file", errConfig)
		}
		revToDist[revocationFile] = distributionFile
		distToRev[distributionFile] = revocationFile
		plan.members = append(plan.members, rolloutMember{
			instanceID:       raw.InstanceID,
			revocationFile:   revocationFile,
			distributionFile: distributionFile,
			ackFile:          ackFile,
		})
	}
	for _, member := range plan.members {
		for _, other := range plan.members {
			if member.ackFile == other.revocationFile || member.ackFile == other.distributionFile {
				return nil, fmt.Errorf("%w: ack_file for %q overlaps a rollout target", errConfig, member.instanceID)
			}
		}
	}
	sort.Slice(plan.members, func(i, j int) bool { return plan.members[i].instanceID < plan.members[j].instanceID })
	return plan, nil
}

func parseBoundedDuration(value, field string, minimum, maximum time.Duration) (time.Duration, error) {
	if value == "" || value != strings.TrimSpace(value) {
		return 0, fmt.Errorf("%w: %s must be a non-empty trimmed Go duration", errConfig, field)
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed < minimum || parsed > maximum {
		return 0, fmt.Errorf("%w: %s must be between %s and %s", errConfig, field, minimum, maximum)
	}
	return parsed, nil
}

func parseCanonicalTime(value, field string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil || value != parsed.UTC().Format(time.RFC3339) {
		return time.Time{}, fmt.Errorf("%w: %s must be canonical UTC RFC3339 seconds", errConfig, field)
	}
	return parsed.UTC(), nil
}

func validateInstanceID(value string) error {
	if value == "" || value != strings.TrimSpace(value) || len(value) > maxInstanceIDBytes {
		return fmt.Errorf("instance_id must be 1..%d trimmed bytes", maxInstanceIDBytes)
	}
	for _, b := range []byte(value) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' || b == '_' || b == '.' {
			continue
		}
		return fmt.Errorf("instance_id contains unsupported characters")
	}
	return nil
}

func readBoundedRegularFile(path string, maxBytes int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%q is not a regular file", path)
	}
	if info.Size() < 0 || info.Size() > maxBytes {
		return nil, fmt.Errorf("%q exceeds %d bytes", path, maxBytes)
	}
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%q exceeds %d bytes", path, maxBytes)
	}
	return data, nil
}

func decodeStrictJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return fmt.Errorf("trailing JSON data: %v", err)
	}
	return nil
}

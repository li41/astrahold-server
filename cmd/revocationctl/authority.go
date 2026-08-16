package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

func loadRevocationCandidate(path string) (*revocationCandidate, error) {
	data, err := readBoundedRegularFile(path, maxRevocationBytes)
	if err != nil {
		return nil, fmt.Errorf("%w: revocation source: %v", errConfig, err)
	}
	var definition revocationDefinition
	if err := decodeStrictJSON(data, &definition); err != nil {
		return nil, fmt.Errorf("%w: decode revocation source: %v", errConfig, err)
	}
	if definition.SchemaVersion != revocationSchemaVersion {
		return nil, fmt.Errorf("%w: revocation schema_version must be %d", errConfig, revocationSchemaVersion)
	}
	if definition.Revision == "" || definition.Revision != strings.TrimSpace(definition.Revision) || len(definition.Revision) > maxRevisionBytes {
		return nil, fmt.Errorf("%w: revocation revision must be 1..%d trimmed bytes", errConfig, maxRevisionBytes)
	}
	if len(definition.RevokedSPKISHA256) > maxRevokedEntries {
		return nil, fmt.Errorf("%w: revoked_spki_sha256 exceeds %d entries", errConfig, maxRevokedEntries)
	}
	unique := make(map[[sha256.Size]byte]struct{}, len(definition.RevokedSPKISHA256))
	for index, raw := range definition.RevokedSPKISHA256 {
		identifier, err := parseDigestHex(raw, fmt.Sprintf("revoked_spki_sha256[%d]", index))
		if err != nil {
			return nil, err
		}
		unique[identifier] = struct{}{}
	}
	identifiers := make([][sha256.Size]byte, 0, len(unique))
	for identifier := range unique {
		identifiers = append(identifiers, identifier)
	}
	sort.Slice(identifiers, func(i, j int) bool {
		return bytes.Compare(identifiers[i][:], identifiers[j][:]) < 0
	})
	hasher := sha256.New()
	_, _ = hasher.Write([]byte("astrahold/session-leaf-revocation-authority/v1\x00"))
	ids := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		_, _ = hasher.Write(identifier[:])
		ids = append(ids, hex.EncodeToString(identifier[:]))
	}
	var digest [sha256.Size]byte
	copy(digest[:], hasher.Sum(nil))
	return &revocationCandidate{revision: definition.Revision, ids: ids, digest: digest}, nil
}

func parseDigestHex(value, field string) ([sha256.Size]byte, error) {
	if len(value) != sha256.Size*2 || value != strings.TrimSpace(value) || strings.ToLower(value) != value {
		return [sha256.Size]byte{}, fmt.Errorf("%w: %s must be exactly 64 lowercase hex characters", errConfig, field)
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != sha256.Size {
		return [sha256.Size]byte{}, fmt.Errorf("%w: %s is invalid", errConfig, field)
	}
	var digest [sha256.Size]byte
	copy(digest[:], decoded)
	return digest, nil
}

func canonicalTargetBytes(plan *rolloutPlan, candidate *revocationCandidate) ([]byte, []byte, error) {
	revocationBytes, err := marshalCanonicalJSON(revocationDefinition{
		SchemaVersion:     revocationSchemaVersion,
		Revision:          candidate.revision,
		RevokedSPKISHA256: append([]string(nil), candidate.ids...),
	})
	if err != nil {
		return nil, nil, err
	}
	distributionBytes, err := marshalCanonicalJSON(distributionDefinition{
		SchemaVersion:             distributionSchemaVersion,
		Epoch:                     plan.epoch,
		RevocationAuthoritySHA256: hex.EncodeToString(candidate.digest[:]),
		ValidUntil:                plan.validUntil.Format(time.RFC3339),
	})
	if err != nil {
		return nil, nil, err
	}
	return revocationBytes, distributionBytes, nil
}

func marshalCanonicalJSON(value any) ([]byte, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("%w: encode target: %v", errConfig, err)
	}
	return append(data, '\n'), nil
}

func loadDistributionIfExists(path string) (*distributionSnapshot, error) {
	data, err := readBoundedRegularFile(path, maxDistributionBytes)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: existing distribution %q: %v", errConfig, path, err)
	}
	var definition distributionDefinition
	if err := decodeStrictJSON(data, &definition); err != nil {
		return nil, fmt.Errorf("%w: decode existing distribution %q: %v", errConfig, path, err)
	}
	if definition.SchemaVersion != distributionSchemaVersion || definition.Epoch == 0 {
		return nil, fmt.Errorf("%w: existing distribution %q has invalid schema or epoch", errConfig, path)
	}
	digest, err := parseDigestHex(definition.RevocationAuthoritySHA256, "revocation_authority_sha256")
	if err != nil {
		return nil, err
	}
	validUntil, err := parseCanonicalTime(definition.ValidUntil, "valid_until")
	if err != nil {
		return nil, err
	}
	return &distributionSnapshot{epoch: definition.Epoch, digest: digest, validUntil: validUntil}, nil
}

package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"sort"
	"strings"
	"time"
)

const (
	rolloutPlanSchemaVersion  uint16 = 1
	revocationSchemaVersion   uint16 = 1
	distributionSchemaVersion uint16 = 1
	ackSchemaVersion          uint16 = 1
	maxPlanBytes                     = 64 * 1024
	maxRevocationBytes               = 64 * 1024
	maxDistributionBytes             = 16 * 1024
	maxAckBytes                      = 16 * 1024
	maxRevokedEntries                = 256
	maxRequiredInstances             = 64
	maxRevisionBytes                 = 128
	maxInstanceIDBytes               = 64
	maxLease                         = 24 * time.Hour
	minAckTimeout                    = 100 * time.Millisecond
	maxAckTimeout                    = 30 * time.Minute
	minPollInterval                  = 10 * time.Millisecond
	maxPollInterval                  = 5 * time.Second
)

var errConfig = errors.New("revocationctl: invalid rollout config")

type rolloutPlanDefinition struct {
	SchemaVersion        uint16                    `json:"schema_version"`
	Epoch                uint64                    `json:"epoch"`
	ValidUntil           string                    `json:"valid_until"`
	AckTimeout           string                    `json:"ack_timeout"`
	PollInterval         string                    `json:"poll_interval"`
	RevocationSourceFile string                    `json:"revocation_source_file"`
	RequiredInstances    []rolloutMemberDefinition `json:"required_instances"`
}

type rolloutMemberDefinition struct {
	InstanceID       string `json:"instance_id"`
	RevocationFile   string `json:"revocation_file"`
	DistributionFile string `json:"distribution_file"`
	AckFile          string `json:"ack_file"`
}

type rolloutPlan struct {
	path                 string
	epoch                uint64
	validUntil           time.Time
	ackTimeout           time.Duration
	pollInterval         time.Duration
	revocationSourceFile string
	members              []rolloutMember
}

type rolloutMember struct {
	instanceID       string
	revocationFile   string
	distributionFile string
	ackFile          string
}

type revocationDefinition struct {
	SchemaVersion     uint16   `json:"schema_version"`
	Revision          string   `json:"revision"`
	RevokedSPKISHA256 []string `json:"revoked_spki_sha256"`
}

type revocationCandidate struct {
	revision string
	ids      []string
	digest   [sha256.Size]byte
}

type distributionDefinition struct {
	SchemaVersion             uint16 `json:"schema_version"`
	Epoch                     uint64 `json:"epoch"`
	RevocationAuthoritySHA256 string `json:"revocation_authority_sha256"`
	ValidUntil                string `json:"valid_until"`
}

type distributionSnapshot struct {
	epoch      uint64
	digest     [sha256.Size]byte
	validUntil time.Time
}

type ackDefinition struct {
	SchemaVersion             uint16 `json:"schema_version"`
	InstanceID                string `json:"instance_id"`
	Epoch                     uint64 `json:"epoch"`
	RevocationRevision        string `json:"revocation_revision"`
	RevocationAuthoritySHA256 string `json:"revocation_authority_sha256"`
	ValidUntil                string `json:"valid_until"`
	AcknowledgedAt            string `json:"acknowledged_at"`
}

type rolloutAckObservation struct {
	InstanceID        string `json:"instance_id"`
	FirstObservedAt   string `json:"first_observed_at"`
	ObservedElapsedMS int64  `json:"observed_elapsed_ms"`
}

type rolloutObservationEvidence struct {
	TimingSource string                  `json:"timing_source"`
	StartedAt    string                  `json:"started_at"`
	CompletedAt  string                  `json:"completed_at"`
	ElapsedMS    int64                   `json:"elapsed_ms"`
	Acks         []rolloutAckObservation `json:"acks,omitempty"`
}

type rolloutResult struct {
	SchemaVersion             uint16                      `json:"schema_version"`
	Status                    string                      `json:"status"`
	Epoch                     uint64                      `json:"epoch"`
	RevocationRevision        string                      `json:"revocation_revision"`
	RevocationAuthoritySHA256 string                      `json:"revocation_authority_sha256"`
	ValidUntil                string                      `json:"valid_until"`
	RequiredInstances         []string                    `json:"required_instances"`
	PublishedInstances        []string                    `json:"published_instances,omitempty"`
	FailedInstances           []string                    `json:"failed_instances,omitempty"`
	AcknowledgedInstances     []string                    `json:"acknowledged_instances,omitempty"`
	PendingInstances          []string                    `json:"pending_instances,omitempty"`
	Observation               *rolloutObservationEvidence `json:"observation,omitempty"`
	Reason                    string                      `json:"reason,omitempty"`
}

type targetPair struct {
	revocationFile   string
	distributionFile string
	instances        []string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, time.Now, time.Sleep))
}

func run(args []string, stdout, stderr io.Writer, now func() time.Time, sleep func(time.Duration)) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: revocationctl <publish|wait|rollout> -plan FILE")
		return 1
	}
	command := args[0]
	if command != "publish" && command != "wait" && command != "rollout" {
		fmt.Fprintf(stderr, "revocationctl: unsupported command %q\n", command)
		return 1
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	planPath := flags.String("plan", "", "strict schema-v1 F.26 rollout plan")
	if err := flags.Parse(args[1:]); err != nil {
		return 1
	}
	if flags.NArg() != 0 || strings.TrimSpace(*planPath) == "" {
		fmt.Fprintln(stderr, "revocationctl: -plan FILE is required and no positional arguments are accepted")
		return 1
	}
	plan, err := loadRolloutPlan(*planPath)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	candidate, err := loadRevocationCandidate(plan.revocationSourceFile)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	encode := func(result rolloutResult) {
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(false)
		_ = encoder.Encode(result)
	}
	switch command {
	case "publish":
		result, publishErr := publishRollout(plan, candidate, now().UTC(), writeAtomicFile)
		encode(result)
		if publishErr != nil {
			fmt.Fprintln(stderr, publishErr)
			return 1
		}
		return 0
	case "wait":
		result, waitErr := waitForAcks(plan, candidate, now, sleep)
		if waitErr != nil {
			fmt.Fprintln(stderr, waitErr)
			return 1
		}
		encode(result)
		if result.Status != "converged" {
			return 2
		}
		return 0
	case "rollout":
		result, publishErr := publishRollout(plan, candidate, now().UTC(), writeAtomicFile)
		if publishErr != nil {
			encode(result)
			fmt.Fprintln(stderr, publishErr)
			return 1
		}
		result, waitErr := waitForAcks(plan, candidate, now, sleep)
		if waitErr != nil {
			fmt.Fprintln(stderr, waitErr)
			return 1
		}
		encode(result)
		if result.Status != "converged" {
			return 2
		}
		return 0
	default:
		panic("unreachable")
	}
}

func publishRollout(plan *rolloutPlan, candidate *revocationCandidate, now time.Time, writer func(string, []byte, fs.FileMode) error) (rolloutResult, error) {
	result := baseResult(plan, candidate)
	if plan == nil || candidate == nil || writer == nil {
		result.Status = "rejected"
		return result, errConfig
	}
	now = now.UTC()
	if !now.Before(plan.validUntil) {
		result.Status = "rejected"
		return result, fmt.Errorf("%w: valid_until is already expired", errConfig)
	}
	if plan.validUntil.After(now.Add(maxLease)) {
		result.Status = "rejected"
		return result, fmt.Errorf("%w: valid_until exceeds the F.25 %s maximum lease", errConfig, maxLease)
	}
	if now.Add(plan.ackTimeout).After(plan.validUntil) {
		result.Status = "rejected"
		return result, fmt.Errorf("%w: ack_timeout extends beyond valid_until", errConfig)
	}
	pairs, err := preflightTargets(plan, candidate)
	if err != nil {
		result.Status = "rejected"
		return result, err
	}
	revocationBytes, distributionBytes, err := canonicalTargetBytes(plan, candidate)
	if err != nil {
		result.Status = "rejected"
		return result, err
	}
	for _, pair := range pairs {
		if err := writer(pair.revocationFile, revocationBytes, 0o600); err != nil {
			result.Status = "partial"
			result.FailedInstances = append([]string(nil), pair.instances...)
			return result, fmt.Errorf("%w: stage revocation for %s: %v", errConfig, strings.Join(pair.instances, ","), err)
		}
	}
	for _, pair := range pairs {
		if err := writer(pair.distributionFile, distributionBytes, 0o600); err != nil {
			result.Status = "partial"
			result.FailedInstances = append([]string(nil), pair.instances...)
			return result, fmt.Errorf("%w: publish distribution for %s: %v", errConfig, strings.Join(pair.instances, ","), err)
		}
		result.PublishedInstances = append(result.PublishedInstances, pair.instances...)
	}
	sort.Strings(result.PublishedInstances)
	result.Status = "published"
	return result, nil
}

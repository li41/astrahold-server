package siege

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/li41/astrahold-server/internal/characteridentity"
	"github.com/li41/astrahold-server/internal/gameplayworld"
	"github.com/li41/astrahold-server/internal/world"
)

const MatchConfigSchemaVersion uint16 = 3

const maxThroneCaptureDuration = time.Hour

var (
	ErrUnsupportedMatchConfigSchema = errors.New("siege: unsupported match config schema")
	ErrInvalidMatchConfig           = errors.New("siege: invalid match config")
)

type ThroneZoneConfig struct {
	Layer  world.LayerID          `json:"layer"`
	Bounds gameplayworld.BoundsXZ `json:"bounds"`
}

type ParticipantConfig struct {
	CharacterID string `json:"character_id"`
	Team        string `json:"team"`
}

type MatchConfig struct {
	SchemaVersion        uint16              `json:"schema_version"`
	Revision             string              `json:"revision"`
	MatchID              string              `json:"match_id"`
	AttackerID           string              `json:"attacker_id"`
	DefenderID           string              `json:"defender_id"`
	BreachGateID         string              `json:"breach_gate_id"`
	ThroneObjectiveID    string              `json:"throne_objective_id"`
	ThroneZone           ThroneZoneConfig    `json:"throne_zone"`
	ThroneCaptureSeconds float64             `json:"throne_capture_seconds"`
	Participants         []ParticipantConfig `json:"participants"`
}

type LoadedMatchConfig struct {
	Revision   string
	Definition MatchDefinition
}

func LoadMatchFile(path string) (LoadedMatchConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LoadedMatchConfig{}, err
	}
	return LoadMatch(bytes.NewReader(data))
}

func LoadMatch(r io.Reader) (LoadedMatchConfig, error) {
	decoder := json.NewDecoder(r)
	decoder.DisallowUnknownFields()
	var config MatchConfig
	if err := decoder.Decode(&config); err != nil {
		return LoadedMatchConfig{}, fmt.Errorf("%w: decode: %v", ErrInvalidMatchConfig, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return LoadedMatchConfig{}, fmt.Errorf("%w: trailing JSON value", ErrInvalidMatchConfig)
		}
		return LoadedMatchConfig{}, fmt.Errorf("%w: trailing data: %v", ErrInvalidMatchConfig, err)
	}
	if err := ValidateMatchConfig(config); err != nil {
		return LoadedMatchConfig{}, err
	}
	return LoadedMatchConfig{Revision: config.Revision, Definition: config.Definition()}, nil
}

func ValidateMatchConfig(config MatchConfig) error {
	if config.SchemaVersion != MatchConfigSchemaVersion {
		return fmt.Errorf("%w: got=%d want=%d", ErrUnsupportedMatchConfigSchema, config.SchemaVersion, MatchConfigSchemaVersion)
	}
	if config.Revision == "" {
		return fmt.Errorf("%w: revision", ErrInvalidMatchConfig)
	}
	if _, ok := throneCaptureDuration(config.ThroneCaptureSeconds); !ok {
		return fmt.Errorf("%w: throne_capture_seconds", ErrInvalidMatchConfig)
	}

	seen := make(map[characteridentity.ID]struct{}, len(config.Participants))
	for i, participant := range config.Participants {
		binding, err := characteridentity.NewTrusted(participant.CharacterID)
		if err != nil {
			return fmt.Errorf("%w: participants[%d].character_id", ErrInvalidMatchConfig, i)
		}
		if _, ok := configuredTeam(participant.Team); !ok {
			return fmt.Errorf("%w: participants[%d].team", ErrInvalidMatchConfig, i)
		}
		if _, duplicate := seen[binding.ID]; duplicate {
			return fmt.Errorf("%w: duplicate participant character_id %q", ErrInvalidMatchConfig, participant.CharacterID)
		}
		seen[binding.ID] = struct{}{}
	}
	if !config.Definition().Valid() {
		return fmt.Errorf("%w: match definition", ErrInvalidMatchConfig)
	}
	return nil
}

func (config MatchConfig) Definition() MatchDefinition {
	captureDuration, _ := throneCaptureDuration(config.ThroneCaptureSeconds)
	throne := ThroneObjectiveDefinition{
		ID: config.ThroneObjectiveID,
		Zone: ObjectiveZone{
			Layer:  config.ThroneZone.Layer,
			Bounds: config.ThroneZone.Bounds,
		},
		CaptureDuration: captureDuration,
	}
	participantTeams := make(map[characteridentity.ID]Team, len(config.Participants))
	for _, participant := range config.Participants {
		binding, err := characteridentity.NewTrusted(participant.CharacterID)
		team, ok := configuredTeam(participant.Team)
		if err == nil && ok {
			participantTeams[binding.ID] = team
		}
	}
	return MatchDefinition{
		ID:                config.MatchID,
		AttackerID:        config.AttackerID,
		DefenderID:        config.DefenderID,
		BreachGateID:      config.BreachGateID,
		ThroneObjectiveID: config.ThroneObjectiveID,
		Throne:            &throne,
		ParticipantTeams:  participantTeams,
	}
}

func (loaded LoadedMatchConfig) ResolveParticipant(binding characteridentity.Binding) (Team, bool) {
	if binding.Assurance != characteridentity.AssuranceTrusted || !binding.Valid() {
		return TeamUnknown, false
	}
	team, ok := loaded.Definition.ParticipantTeams[binding.ID]
	return team, ok
}

func (loaded LoadedMatchConfig) ParticipantCount() int { return len(loaded.Definition.ParticipantTeams) }

func ValidateMatchAgainstGates(definition MatchDefinition, gates []gameplayworld.Gate) error {
	if !definition.Valid() {
		return ErrInvalidMatchDefinition
	}
	for _, gate := range gates {
		if gate.ID == definition.BreachGateID {
			return nil
		}
	}
	return fmt.Errorf("%w: breach gate %q", ErrInvalidMatchDefinition, definition.BreachGateID)
}

func throneCaptureDuration(seconds float64) (time.Duration, bool) {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
		return 0, false
	}
	duration := time.Duration(seconds * float64(time.Second))
	return duration, duration > 0 && duration <= maxThroneCaptureDuration
}

func configuredTeam(value string) (Team, bool) {
	switch value {
	case "attacker":
		return TeamAttacker, true
	case "defender":
		return TeamDefender, true
	default:
		return TeamUnknown, false
	}
}

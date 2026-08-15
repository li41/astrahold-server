package siege

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/li41/astrahold-server/internal/gameplayworld"
)

const MatchConfigSchemaVersion uint16 = 1

var (
	ErrUnsupportedMatchConfigSchema = errors.New("siege: unsupported match config schema")
	ErrInvalidMatchConfig           = errors.New("siege: invalid match config")
)

type MatchConfig struct {
	SchemaVersion     uint16 `json:"schema_version"`
	Revision          string `json:"revision"`
	MatchID           string `json:"match_id"`
	AttackerID        string `json:"attacker_id"`
	DefenderID        string `json:"defender_id"`
	BreachGateID      string `json:"breach_gate_id"`
	ThroneObjectiveID string `json:"throne_objective_id"`
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
	if !config.Definition().Valid() {
		return fmt.Errorf("%w: match definition", ErrInvalidMatchConfig)
	}
	return nil
}

func (config MatchConfig) Definition() MatchDefinition {
	return MatchDefinition{
		ID:                config.MatchID,
		AttackerID:        config.AttackerID,
		DefenderID:        config.DefenderID,
		BreachGateID:      config.BreachGateID,
		ThroneObjectiveID: config.ThroneObjectiveID,
	}
}

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

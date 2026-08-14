// Package characteridentity defines the durable ownership identity carried by a player character.
package characteridentity

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
)

type ID string

type Assurance string

const (
	AssuranceEphemeral Assurance = "ephemeral"
	AssuranceTrusted   Assurance = "trusted"

	maxIDLength     = 128
	ephemeralPrefix = "ephemeral:"
)

var (
	ErrInvalidID        = errors.New("characteridentity: invalid character id")
	ErrInvalidAssurance = errors.New("characteridentity: invalid assurance")
)

type Binding struct {
	ID        ID
	Assurance Assurance
}

func NewTrusted(value string) (Binding, error) {
	if len(value) >= len(ephemeralPrefix) && value[:len(ephemeralPrefix)] == ephemeralPrefix {
		return Binding{}, ErrInvalidID
	}
	return New(value, AssuranceTrusted)
}

func NewEphemeral() (Binding, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return Binding{}, err
	}
	return New(ephemeralPrefix+hex.EncodeToString(raw[:]), AssuranceEphemeral)
}

func New(value string, assurance Assurance) (Binding, error) {
	binding := Binding{ID: ID(value), Assurance: assurance}
	if !binding.Valid() {
		if assurance != AssuranceEphemeral && assurance != AssuranceTrusted {
			return Binding{}, ErrInvalidAssurance
		}
		return Binding{}, ErrInvalidID
	}
	return binding, nil
}

func (b Binding) Valid() bool {
	if b.Assurance != AssuranceEphemeral && b.Assurance != AssuranceTrusted {
		return false
	}
	value := string(b.ID)
	if len(value) == 0 || len(value) > maxIDLength {
		return false
	}
	for i := 0; i < len(value); i++ {
		c := value[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '.' || c == '_' || c == ':' || c == '-' {
			continue
		}
		return false
	}
	if b.Assurance == AssuranceEphemeral {
		if len(value) != len(ephemeralPrefix)+32 || value[:len(ephemeralPrefix)] != ephemeralPrefix {
			return false
		}
		for i := len(ephemeralPrefix); i < len(value); i++ {
			c := value[i]
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				return false
			}
		}
		return true
	}
	return !(len(value) >= len(ephemeralPrefix) && value[:len(ephemeralPrefix)] == ephemeralPrefix)
}

package models

import (
	"github.com/strongo/validation"
)

// EnvironmentFile some file to be documented
type EnvironmentFile struct {
	ID string `json:"id"`
}

// Validate returns error if not valid
func (v EnvironmentFile) Validate() error {
	if v.ID == "" {
		return validation.NewErrRecordIsMissingRequiredField("id")
	}
	return nil
}

// Issues TODO: document what it is for
type Issues struct {
	Schema []string `json:"schema,omitempty"`
}

// StateByEnv states by env ID
type StateByEnv map[string]*EnvState

// Validate returns error if not valid
func (v StateByEnv) Validate() error {
	if v == nil {
		return nil
	}
	for _, state := range v {
		if err := state.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// EnvState hold state of env
type EnvState struct {
	Status      string            `json:"status"` // Possible values: exists, missing
	Differences []EnvDbDifference `json:"differences,omitempty"`
}

// Validate returns error if not valid
func (v EnvState) Validate() error {
	switch v.Status {
	case "":
		return validation.NewErrRecordIsMissingRequiredField("status")
	case "exists", "missing":
	default:
		return validation.NewErrBadRecordFieldValue("status", "unknown value: "+v.Status)
	}
	return nil
}

// EnvDbDifference hold diffs for a DB in specific environment
type EnvDbDifference struct {
	Property    string
	ActualValue string
}

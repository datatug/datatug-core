package execute

import (
	"errors"
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/strongo/validation"
	"time"
)

// Response holds execute response
type Response struct {
	Duration time.Duration      `json:"durationNanoseconds"`
	Commands []*CommandResponse `json:"commands"`
}

type CommandResponse struct {
	Items []CommandResponseItem `json:"items"`
}

type CommandResponseItem struct {
	Type  string      `json:"type"` // e.g. recordset
	Value interface{} `json:"value"`
}

// Request defines what needs to be executed
type Request struct {
	ID       string           `json:"id"`
	Project  string           `json:"project"`
	Commands []RequestCommand `json:"commands"`
}

// Validate checks if request is valid
func (v Request) Validate() error {
	if v.Project == "" {
		return validation.NewErrBadRequestFieldValue("project", "missing project")
	}
	for i, c := range v.Commands {
		if err := c.Validate(); err != nil {
			return fmt.Errorf("invalid command at %v: %w", i, err)
		}
	}
	return nil
}

// RequestCommand holds parameters for command to be executed
type RequestCommand struct {
	models.Credentials // holds username & password, if not provided trusted connection
	models.ServerReference
	Env        string             `json:"env"`
	DB         string             `json:"db"`
	Text       string             `json:"text"`
	Parameters []models.Parameter `json:"parameters"`
}

// Validate checks of command request is valid
func (v RequestCommand) Validate() error {
	if v.Env == "" {
		return validation.NewErrRequestIsMissingRequiredField("env")
	}
	if v.Text == "" {
		return validation.NewErrRequestIsMissingRequiredField("text")
	}
	if err := v.ServerReference.Validate(); err != nil {
		return err
	}
	if v.DB != "" {
		if v.ServerReference.Host != "" {
			return validation.NewBadRequestError(errors.New("both 'db' & 'host' were provided"))
		}
		if v.ServerReference.Driver != "" {
			return validation.NewBadRequestError(errors.New("both 'db' & 'driver' were provided"))
		}
		if v.ServerReference.Port != 0 {
			return validation.NewBadRequestError(errors.New("both 'db' & 'port' were provided"))
		}
	}
	return nil
}

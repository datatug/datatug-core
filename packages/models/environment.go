package models

import (
	"fmt"
	"github.com/strongo/validation"
	"strings"
)

// Environments is a slice of pointers to Environment
type Environments []*Environment

// Validate returns error if failed
func (v Environments) Validate() error {
	for i, env := range v {
		if err := env.Validate(); err != nil {
			return fmt.Errorf("validation failed for environment at index=%v, id=%v: %w", i, env.ID, err)
		}
	}
	return nil
}

// GetEnvByID returns Environment by ID
func (v Environments) GetEnvByID(id string) (environment *Environment) {
	for _, environment = range v {
		if environment.ID == id {
			return environment
		}
	}
	return nil
}

// Environment holds information about environment
type Environment struct {
	ProjectItem
	DbServers EnvDbServers `json:"dbServers"`
}

// Validate returns error if failed
func (v Environment) Validate() error {
	if err := v.ProjectItem.Validate(false); err != nil {
		return err
	}
	if err := v.DbServers.Validate(); err != nil {
		return err
	}
	return nil
}

// ProjEnvBrief hold env brief in project summary
type ProjEnvBrief struct {
	ProjectItem
	NumberOf ProjEnvNumbers `json:"numberOf"`
}

// ProjDbModelBrief hold env brief in project summary
type ProjDbModelBrief struct {
	ProjectItem
	NumberOf ProjDbModelNumbers `json:"numberOf"`
}

// ProjDbModelNumbers holds numbers for a dbmodel
type ProjDbModelNumbers struct {
	Schemas int `json:"schemas"`
	Tables  int `json:"tables"`
	Views   int `json:"views"`
}

// ProjEnvNumbers hold soem numbers for environment
type ProjEnvNumbers struct {
	DbServers int `json:"dbServer"`
	Databases int `json:"databases"`
}

// EnvDbServers is a slice of *EnvDbServer
type EnvDbServers []*EnvDbServer

// Validate returns error of failed
func (v EnvDbServers) Validate() error {
	if v == nil {
		return nil
	}
	for i, item := range v {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("failed validation of a DB server at index %v: %w", i, err)
		}
	}
	return nil
}

// GetByID returns *EnvDbServer by ID
func (v EnvDbServers) GetByID(id string) *EnvDbServer {
	for _, item := range v {
		if item.ID() == id {
			return item
		}
	}
	return nil
}

// EnvDbServer holds information about server in an environment
type EnvDbServer struct {
	DbServer
	Databases []string `json:"databases,omitempty"`
}

// Validate returns error if no valid
func (v EnvDbServer) Validate() error {
	if err := v.DbServer.Validate(); err != nil {
		return err
	}
	for i, db := range v.Databases {
		if strings.TrimSpace(db) == "" {
			return validation.NewErrRequestIsMissingRequiredField(fmt.Sprintf("databases[%v]", i))
		}
	}
	return nil
}

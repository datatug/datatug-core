package datatug

import (
	"fmt"
)

var _ IProjectItems[*Environment] = (Environments)(nil)

// Environments is a slice of pointers to Environment
type Environments ProjectItems[*Environment]

// Validate returns error if failed
func (v Environments) Validate() error {
	for i, env := range v {
		if err := env.Validate(); err != nil {
			return fmt.Errorf("validation failed for environment at index=%v, id=%v: %w", i, env.ID, err)
		}
	}
	return nil
}

func (v Environments) GetByID(id string) *Environment {
	return ProjectItems[*Environment](v).GetByID(id)
}

func (v Environments) IDs() []string {
	return ProjectItems[*Environment](v).IDs()
}

// Environment holds information about environment
type Environment struct {
	ProjectItem
	DbServers EnvDbServers `json:"dbServers"`
}

// Validate returns error if failed
func (v Environment) Validate() error {
	if err := v.ValidateWithOptions(false); err != nil {
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
	//NumberOf ProjEnvNumbers `json:"numberOf"` Lets not to have this for now as makes git conflicts resolution harder.
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

// ProjEnvNumbers hold some numbers for environment
type ProjEnvNumbers struct {
	DbServers int `json:"dbServer"`
	Databases int `json:"databases"`
}

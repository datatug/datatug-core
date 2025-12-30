package datatug

import "github.com/strongo/validation"

// EnvDb hold info about DB in specific environment
type EnvDb struct {
	ProjectItem
	DbModel string          `json:"dbModel"`
	Server  ServerReference `json:"server"`
}

// Validate returns error if not valid
func (v EnvDb) Validate() error {
	if err := v.ProjectItem.Validate(false); err != nil {
		return err
	}
	if err := v.Server.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("server", err.Error())
	}
	return nil
}

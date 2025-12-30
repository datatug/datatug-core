package datatug

import "github.com/strongo/validation"

// EnvironmentSummary holds environment summary
type EnvironmentSummary struct {
	ProjectItem
	Servers EnvDbServers `json:"dbServers,omitempty"`
	//Databases []EnvDb             `json:"databases,omitempty"`
}

// Validate returns error if not valid
func (v EnvironmentSummary) Validate() error {
	if err := v.ProjectItem.Validate(false); err != nil {
		return err
	}
	if err := v.Servers.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("servers", err.Error())
	}
	return nil
}

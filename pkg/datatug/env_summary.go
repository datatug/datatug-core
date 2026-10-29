package datatug

import "github.com/strongo/validation"

// EnvironmentSummary holds environment summary
type EnvironmentSummary struct {
	ProjectItem
	Servers            EnvDbServers `json:"dbServers,omitempty"`
	EditionConnections []string     `json:"editionConnections,omitempty"`
	//Databases []EnvDb             `json:"databases,omitempty"`
}

// Validate returns error if not valid
func (v EnvironmentSummary) Validate() error {
	if err := v.ValidateWithOptions(false); err != nil {
		return err
	}
	if err := v.Servers.Validate(); err != nil {
		return validation.NewErrBadRecordFieldValue("servers", err.Error())
	}
	if err := uniqueConnectionIDs(v.EditionConnections); err != nil {
		return validation.NewErrBadRecordFieldValue("editionConnections", err.Error())
	}
	return nil
}

// Summary copies only the safe connection identifiers, never opaque environment
// JSON or connection credentials. The copy cannot mutate the loaded environment.
func (v Environment) Summary() (EnvironmentSummary, error) {
	if err := uniqueConnectionIDs(v.EditionConnections); err != nil {
		return EnvironmentSummary{}, err
	}
	return EnvironmentSummary{ProjectItem: v.ProjectItem, Servers: v.DbServers, EditionConnections: append([]string(nil), v.EditionConnections...)}, nil
}

package datatug

import (
	"encoding/json"
	"strings"
)

type projectFileJSON ProjectFile

func (v *ProjectFile) UnmarshalJSON(body []byte) error {
	if err := validateProjectDataModelsJSON(body); err != nil {
		return err
	}
	if err := validateProjectEnvironmentOrderJSON(body); err != nil {
		return err
	}
	var typed projectFileJSON
	if err := json.Unmarshal(body, &typed); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	// The typed decode above has already established a JSON object (or null).
	_ = json.Unmarshal(body, &fields)
	for key := range fields {
		switch strings.ToLower(key) {
		case "id", "title", "folder", "tags", "userids", "access", "created", "repository", "environmentorder", "datamodels", "coordination", "databases":
			delete(fields, key)
		}
	}
	if len(fields) > 0 {
		typed.jsonMetadata = fields
	}
	*v = ProjectFile(typed)
	return nil
}
func (v ProjectFile) MarshalJSON() ([]byte, error) {
	body, err := json.Marshal(projectFileJSON(v))
	if err != nil || (v.Databases == nil && len(v.jsonMetadata) == 0 && (v.EnvironmentOrder == nil || len(v.EnvironmentOrder) > 0)) {
		return body, err
	}
	var fields map[string]json.RawMessage
	// body was just generated from a struct, so it is a valid JSON object.
	_ = json.Unmarshal(body, &fields)
	if v.Databases != nil {
		fields["databases"], _ = json.Marshal(v.Databases)
	}
	if v.EnvironmentOrder != nil {
		fields["environmentOrder"], _ = json.Marshal(v.EnvironmentOrder)
	}
	for key, value := range v.jsonMetadata {
		if _, exists := fields[key]; !exists {
			fields[key] = value
		}
	}
	return json.Marshal(fields)
}

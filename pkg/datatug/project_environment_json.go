package datatug

import (
	"encoding/json"
	"strings"

	"github.com/datatug/datatug-core/internal/jsonstrict"
)

// Validate only the ordering contract before encoding/json can replace invalid
// Unicode. Unrelated legacy metadata retains its existing decoding behavior.
func validateProjectEnvironmentOrderJSON(body []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return err
	}
	for key, value := range fields {
		if strings.EqualFold(key, "environmentOrder") {
			if err := jsonstrict.ValidateUnicode(value); err != nil {
				return err
			}
		}
	}
	return nil
}

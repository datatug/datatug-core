package dto

import "github.com/datatug/datatug-core/internal/jsonstrict"

// ValidateEnvironmentJSONUnicode refuses malformed UTF-8 and escaped unpaired surrogates.
func ValidateEnvironmentJSONUnicode(body []byte) error {
	return jsonstrict.ValidateUnicode(body)
}

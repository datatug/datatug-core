package models

import (
	"github.com/strongo/validation"
	"strings"
)

// ValidateName validates name
func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	if strings.Replace(strings.TrimSpace(name), " ", "", -1) != name {
		return validation.NewErrBadRecordFieldValue("name", "name can't contain whitespace characters")
	}
	return nil

}

// ValidateTitle validates title
func ValidateTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return validation.NewErrRecordIsMissingRequiredField("title")
	}
	if strings.TrimSpace(title) != title {
		return validation.NewErrBadRecordFieldValue("name", "title should not start or end with whitespace characters")
	}
	return nil

}

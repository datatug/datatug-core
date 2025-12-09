package models

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"

	"github.com/strongo/validation"
)

// Check defines a check
type Check struct {
	ID    string          `json:"id"` // This a random ID uniquely identifying a specific check instance
	Title string          `json:"title,omitempty"`
	Type  string          `json:"type"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// Validate returns error if not valid
func (v *Check) Validate() error {
	if v.ID == "" {
		return validation.NewErrRecordIsMissingRequiredField("id")
	}
	if v.Title == "" {
		return validation.NewErrRecordIsMissingRequiredField("title")
	}
	if v.Data == nil {
		return validation.NewErrRecordIsMissingRequiredField("data")
	}
	var validate = func() error {
		var check interface{ Validate() error }
		switch v.Type {
		case "options":
			check = new(OptionsValueCheck)
		case "regexp":
			check = new(RegexpValueCheck)
		case "sql":
			check = new(SQLCheck)
		}
		if check == nil {
			return fmt.Errorf("unknown check type: %v", v.Type)
		}
		if err := json.Unmarshal(v.Data, &check); err != nil {
			return err
		}
		return check.Validate()
	}
	if v.Type == "" {
		return validation.NewErrRecordIsMissingRequiredField("type")
	}
	if err := validate(); err != nil {
		return fmt.Errorf("check of type '%v' holds invalid data: %w", v.Type, err)
	}
	return nil
}

// Checks is a slice of *Check
type Checks []*Check

// Validate returns error if not valid
func (v Checks) Validate() error {
	for i, check := range v {
		if err := check.Validate(); err != nil {
			return fmt.Errorf("invalid check at index %v: %w", i, err)
		}
	}
	return nil
}

// SQLCheck holds and SQL that verifies data
type SQLCheck struct {
	Query string `json:"query"`
}

// Validate returns error if not valid
func (v SQLCheck) Validate() error {
	if v.Query == "" {
		return validation.NewErrRecordIsMissingRequiredField("query")
	}
	return nil
}

// RegexpValueCheck test value with regular expression
type RegexpValueCheck struct {
	Regexp string `json:"regexp"`
}

// Validate returns error if not valid
func (v RegexpValueCheck) Validate() error {
	if v.Regexp == "" {
		return validation.NewErrRecordIsMissingRequiredField("regexp")
	}
	if _, err := regexp.Compile(v.Regexp); err != nil {
		return validation.NewErrBadRecordFieldValue("regexp", "not valid regular expression")
	}
	return nil
}

// OptionsValueCheck test value is matching one of the available options
type OptionsValueCheck struct {
	Type    string        `json:"type"`
	Options []interface{} `json:"options"`
}

// Validate returns error if not valid
func (v OptionsValueCheck) Validate() error {
	if v.Type == "" {
		return validation.NewErrRecordIsMissingRequiredField("query")
	}
	if len(v.Options) == 0 {
		return validation.NewErrRecordIsMissingRequiredField("options")
	}
	for i, o := range v.Options {
		if t := reflect.TypeOf(o).String(); t != v.Type {
			return fmt.Errorf("option at index %v has invalid value type: expected %v, got %v", i, v.Type, t)
		}
	}
	return nil
}

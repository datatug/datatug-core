package models

import (
	"fmt"
	"github.com/strongo/validation"
	"regexp"
	"strconv"
)

// VarSetting record
type VarSetting struct {
	Type         string `json:"type" firestore:"type"`
	ValuePattern string `json:"valuePattern,omitempty" firestore:"valuePattern,omitempty"`
	Min          *int   `json:"min,omitempty" firestore:"min,omitempty"`
	Max          *int   `json:"max,omitempty" firestore:"max,omitempty"`
}

// Validate validates record
func (v VarSetting) Validate() error {
	if err := validateVarType(v.Type); err != nil {
		return err
	}
	if v.ValuePattern != "" {
		if _, err := regexp.Compile(v.ValuePattern); err != nil {
			return validation.NewErrBadRecordFieldValue("valuePattern", "not a valida regular expression")
		}
	}
	if v.Min != nil && v.Max != nil {
		if *v.Min > *v.Max {
			return validation.NewErrBadRecordFieldValue("max", "max is less then min")
		}
	}
	return nil
}

// VarInfo hold info about var type & value
type VarInfo struct {
	Type  string `json:"type" firestore:"type"`
	Value string `json:"value,omitempty" firestore:"value,omitempty"`
}

func validateVarType(t string) error {
	switch t {
	case "":
		return validation.NewErrRecordIsMissingRequiredField("type")
	case "str", "int": // known var types
	default:
		return validation.NewErrBadRecordFieldValue("type", fmt.Sprintf("unsupported var type: %v", t))
	}
	return nil
}

// Validate validates record
func (v VarInfo) Validate() error {
	if err := validateVarType(v.Type); err != nil {
		return err
	}
	switch v.Type {
	case "int":
		if v.Value != "" {
			if _, err := strconv.Atoi(v.Value); err != nil {
				return validation.NewErrBadRecordFieldValue("value", fmt.Sprintf("invalid value for int variable: %v", err))
			}
		}
	}
	return nil
}

// VarsByID type alias
type VarsByID = map[string]VarInfo

// Variables properties holder
type Variables struct {
	Vars map[string]VarInfo `json:"vars,omitempty" firestore:"vars,omitempty"`
}

// Validate validates record
func (v Variables) Validate() error {
	for name, variable := range v.Vars {
		if err := ValidateName(name); err != nil {
			return err
		}
		if err := variable.Validate(); err != nil {
			return err
		}
	}
	return nil
}

package models

import (
	"fmt"
	"github.com/strongo/validation"
	"strings"
)

// ProjectItem base class with ID and Title
type ProjectItem struct {
	ID    string   `json:"id,omitempty" firestore:"id,omitempty" yaml:"id"`
	Title string   `json:"title,omitempty" firestore:"title,omitempty" yaml:"title,omitempty"`
	Tags  []string `json:"tags,omitempty" yaml:",omitempty"` // consider moving to ProjectItem
}

// MaxTitleLength defines maximum length of a title = 100
const MaxTitleLength = 100

// MaxTagLength defines maximum length of a tag = 100
const MaxTagLength = 100

func validateStringField(name, value string, isRequired bool, maxLen int) error {
	if isRequired && strings.TrimSpace(value) == "" {
		return validation.NewErrRecordIsMissingRequiredField(name)
	}
	if maxLen > 0 {
		if l := len(value); l > maxLen {
			return validation.NewErrBadRecordFieldValue(name,
				fmt.Sprintf("exceeds max length (%v): %v", maxLen, l))
		}
	}
	return nil
}

// Validate return error if not valid
func (v ProjectItem) Validate(isTitleRequired bool) error {
	if v.ID == "" {
		return validation.NewErrRecordIsMissingRequiredField("id")
	}
	if err := validateStringField("title", v.Title, isTitleRequired, MaxTitleLength); err != nil {
		return err
	}
	tags := make([]string, 0, len(v.Tags))
	for i, tag := range v.Tags {
		tagFieldName := fmt.Sprintf("tags[%v]", i)
		if err := validateStringField(tagFieldName, tag, true, MaxTagLength); err != nil {
			return err
		}
		if strings.TrimSpace(tag) == "" {
			return validation.NewErrBadRecordFieldValue(tagFieldName, "empty value")
		}
		if strings.TrimSpace(tag) != tag {
			return validation.NewErrBadRecordFieldValue(tagFieldName, "tag should not have spaces at beginning or at the end")
		}
		for _, t := range tags {
			if t == tag {
				return validation.NewErrBadRecordFieldValue(tagFieldName, "duplicate tag value: "+tag)
			}
		}
	}
	return nil
}

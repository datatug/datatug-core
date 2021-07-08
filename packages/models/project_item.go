package models

import (
	"fmt"
	"github.com/strongo/validation"
	"strings"
)

const AutoID = "<auto/id>"

type ProjItemBrief struct {
	ID    string `json:"id,omitempty" firestore:"id,omitempty" yaml:"id,omitempty"`
	Title string `json:"title,omitempty" firestore:"title,omitempty" yaml:"title,omitempty"`
	ListOfTags
}

// Validate returns error if not valid
func (v ProjItemBrief) Validate(isTitleRequired bool) error {
	if v.ID == "" {
		return validation.NewErrRecordIsMissingRequiredField("id")
	}
	if err := validateStringField("title", v.Title, isTitleRequired, MaxTitleLength); err != nil {
		return err
	}
	if err := v.ListOfTags.Validate(); err != nil {
		return err
	}
	return nil
}

// ProjectItem base class with ID and Title
type ProjectItem struct {
	ProjItemBrief
	UserIDs []string `json:"userIds,omitempty" firestore:"userIds,omitempty"`
	Access  string   `json:"access,omitempty" firestore:"access,omitempty"` // e.g. "private", "protected", "public"
}

// Validate returns error if not valid
func (v ProjectItem) Validate(isTitleRequired bool) error {
	if err := v.ProjItemBrief.Validate(isTitleRequired); err != nil {
		return err
	}
	switch v.Access {
	case "", "private", "protected", "public":
	default:
		return validation.NewErrBadRecordFieldValue("access", "not empty and not equal one of next: private, protected, public")
	}
	for i, userID := range v.UserIDs {
		if strings.TrimSpace(userID) == "" {
			return validation.NewErrBadRecordFieldValue("userIds", fmt.Sprintf("empty at index %v", i))
		}
		for j, uid := range v.UserIDs {
			if uid == userID {
				return validation.NewErrBadRecordFieldValue("userIds", fmt.Sprintf("duplicate value at indexex %v and %v", i, j))
			}
		}
	}
	return nil
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

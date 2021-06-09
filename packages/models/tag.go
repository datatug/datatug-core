package models

import (
	"fmt"
	"github.com/strongo/validation"
)

// ListOfTags mixing
type ListOfTags struct {
	Tags []string `json:"tags,omitempty" firestore:"tags,omitempty"`
}

// Validate validates record
func (v ListOfTags) Validate() error {
	if len(v.Tags) > 0 {
		existing := make([]string, 0, len(v.Tags))
		for i, tag := range v.Tags {
			if tag == "" {
				return validation.NewErrBadRecordFieldValue("tags", fmt.Sprintf("empty tag at index %v", i))
			}
			for j, t := range existing {
				if t == tag {
					return validation.NewErrBadRecordFieldValue("tags", fmt.Sprintf("duplicate tag at indexes %v & %v: %v", j, i, tag))
				}
			}
			existing = append(existing, tag)
		}
	}
	return nil
}

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
			field := fmt.Sprintf("tags[%v]", i)
			if tag == "" {
				return validation.NewErrBadRecordFieldValue(field, fmt.Sprintf("empty tag at index %v", i))
			}
			if len(tag) > MaxTagLength {
				return validation.NewErrBadRecordFieldValue(field, fmt.Sprintf("too long tag (max %v, got %v)", len(tag), MaxTagLength))
			}
			for j, t := range existing {
				if t == tag {
					return validation.NewErrBadRecordFieldValue(field, fmt.Sprintf("duplicate tag at indexes %v & %v: %v", j, i, tag))
				}
			}
			existing = append(existing, tag)
		}
	}
	return nil
}

package models

import "fmt"

// UserDatatugInfo holds user info for DataTug project
type UserDatatugInfo struct {
	Projects []ProjectBrief `json:"projects,omitempty" firestore:"projects,omitempty"`
}

// Validate returns error if not valid
func (v UserDatatugInfo) Validate() error {
	if len(v.Projects) > 0 {
		ids := make([]string, 0, len(v.Projects))
		for i, p := range v.Projects {
			if err := p.Validate(true); err != nil {
				return fmt.Errorf("invalid project at index %v: %w", i, err)
			}
			for j, id := range ids {
				if id == p.ID {
					return fmt.Errorf("duplicate project ID at indexes %v & %v: %v", j, i, id)
				}
			}
			ids = append(ids, p.ID)
		}
	}
	return nil
}

// DatatugUser defines a user record with props related to Datatug
type DatatugUser struct {
	Datatug *UserDatatugInfo `json:"datatug,omitempty" firestore:"datatug,omitempty"`
}

// Validate returns error if not valid
func (v DatatugUser) Validate() error {
	if v.Datatug != nil {
		if err := v.Datatug.Validate(); err != nil {
			return fmt.Errorf("invalid datatug property: %w", err)
		}
	}
	return nil
}

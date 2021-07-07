package models

import (
	"fmt"
	"github.com/strongo/validation"
	"strings"
)

type FolderBrief struct {
	Name string `json:"name" firestore:"name"`
}

func (v FolderBrief) Validate() error {
	if strings.TrimSpace(v.Name) == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	return nil
}

// Folder keeps info about folder
type Folder struct {
	//Path    string            `json:"path" firestore:"path"`
	Name    string            `json:"name" firestore:"name"`
	Note    string            `json:"note,omitempty" firestore:"note,omitempty"`
	Folders []*FolderBrief    `json:"folders,omitempty" firestore:"folders,omitempty"`
	Queries []*QueryDefBrief  `json:"queries,omitempty" firestore:"queries,omitempty"`
	Boards  []*ProjBoardBrief `json:"boards,omitempty" firestore:"boards,omitempty"`
	// NumberOf keeps count of all successor objects in all sub-folders
	NumberOf *FolderCounts `json:"numberOf,omitempty" firestore:"numberOf,omitempty"`
}

type FolderCounts struct {
	Folders int `json:"folders,omitempty" firestore:"folders,omitempty"`
	Queries int `json:"queries,omitempty" firestore:"queries,omitempty"`
	Borders int `json:"borders,omitempty" firestore:"borders,omitempty"`
}

// Validate returns error if failed
func (v Folder) Validate() error {
	if strings.TrimSpace(v.Name) == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	//if strings.TrimSpace(v.Path) == "" {
	//	return validation.NewErrRecordIsMissingRequiredField("name")
	//}
	for i, brief := range v.Folders {
		if err := brief.Validate(); err != nil {
			return validation.NewErrBadRecordFieldValue(fmt.Sprintf("folders[%v]", i), err.Error())
		}
	}
	for i, brief := range v.Queries {
		if err := brief.Validate(); err != nil {
			return validation.NewErrBadRecordFieldValue(fmt.Sprintf("queries[%v]", i), err.Error())
		}
	}
	return nil
}

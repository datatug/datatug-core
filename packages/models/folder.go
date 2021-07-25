package models

import (
	"fmt"
	"github.com/strongo/validation"
	"strconv"
	"strings"
)

// Folder keeps info about folder
type Folder struct {
	Name    string                      `json:"name,omitempty" firestore:"name,omitempty"` // empty for root folder
	Note    string                      `json:"note,omitempty" firestore:"note,omitempty"`
	Folders map[string]*FolderItemBrief `json:"folders,omitempty" firestore:"folders,omitempty"`
	Boards  map[string]*ProjBoardBrief  `json:"boards,omitempty" firestore:"boards,omitempty"`
	Queries map[string]*QueryDefBrief   `json:"queries,omitempty" firestore:"queries,omitempty"`

	// NumberOf keeps count of all successor objects in all sub-folders
	NumberOf map[string]int `json:"numberOf,omitempty" firestore:"numberOf,omitempty"`
}

type FolderItemBrief struct {
	Title string `json:"title" firestore:"title"`
}

func (v FolderItemBrief) Validate() error {
	if strings.TrimSpace(v.Title) == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	if strings.TrimSpace(v.Title) != v.Title {
		return validation.NewErrBadRecordFieldValue("name", "can't start or end with spaces")
	}
	return nil
}

type FolderItem struct {
	ID   string `json:"id" firestore:"id"`
	Name string `json:"name" firestore:"name"`
}

func (v FolderItem) Validate() error {
	if strings.TrimSpace(v.ID) == "" {
		return validation.NewErrRecordIsMissingRequiredField("id")
	}
	if strings.TrimSpace(v.Name) == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	if strings.TrimSpace(v.ID) != v.ID {
		return validation.NewErrBadRecordFieldValue("id", "can't start or end with spaces")
	}
	if strings.TrimSpace(v.Name) != v.Name {
		return validation.NewErrBadRecordFieldValue("name", "can't start or end with spaces")
	}
	return nil
}

type FolderItemsByType = map[string][]*FolderItem

// Validate returns error if failed
func (v Folder) Validate() error {
	if strings.TrimSpace(v.Name) == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	if strings.TrimSpace(v.Name) == v.Name {
		return validation.NewErrBadRecordFieldValue("name", "folder name can't start or end with spaces")
	}

	validateMapOfItems := func(itemsType string, items map[string]*FolderItemBrief) error {
		names := make([]string, 0, len(items))
		for id, item := range items {
			if err := item.Validate(); err != nil {
				return validation.NewErrBadRecordFieldValue(fmt.Sprintf("%v[%v]", itemsType, id), err.Error())
			}
			for _, name := range names {
				if name == item.Title {
					return validation.NewErrBadRecordFieldValue(fmt.Sprintf("%v[%v]", itemsType, id), "duplicate name")
				}
			}
			names = append(names, item.Title)

		}
		return nil
	}
	if err := validateMapOfItems("folders", v.Folders); err != nil {
		return err
	}
	if err := validateBoardBriefsMappedByID(v.Boards); err != nil { // TODO: generic
		return validation.NewErrBadRecordFieldValue("boards", err.Error())
	}
	if err := validateQueryBriefsMappedByID(v.Queries); err != nil { // TODO: generic
		return validation.NewErrBadRecordFieldValue("queries", err.Error())
	}
	for k, n := range v.NumberOf {
		if n < 0 {
			return validation.NewErrBadRecordFieldValue("numberOf."+k, "has negative value: "+strconv.Itoa(n))
		}
		if n == 0 {
			delete(v.NumberOf, k)
		}
	}
	return nil
}

func validateItemMappedByID(mapID, itemID string, item validatable) error {
	return nil
}

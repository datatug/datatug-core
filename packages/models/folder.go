package models

import (
	"fmt"
	"github.com/strongo/validation"
	"strconv"
	"strings"
)

// Folder keeps info about folder
type Folder struct {
	Name    string            `json:"name,omitempty" firestore:"name,omitempty"` // empty for root folder
	Note    string            `json:"note,omitempty" firestore:"note,omitempty"`
	Items   FolderItemsByType `json:"items,omitempty" firestore:"items:omitempty"`
	Boards  map[string]string `json:"boards,omitempty" firestore:"boards,omitempty"`
	Queries map[string]string `json:"queries,omitempty" firestore:"queries:omitempty"`

	// NumberOf keeps count of all successor objects in all sub-folders
	NumberOf map[string]int `json:"numberOf,omitempty" firestore:"numberOf,omitempty"`
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
	var validateSliceOfItems = func(itemsType string, items []*FolderItem) error {
		names := make([]string, 0, len(items))
		ids := make([]string, 0, len(items))
		for i, item := range items {
			if err := item.Validate(); err != nil {
				return fmt.Errorf("invalid item of type %v at index %v: %w", itemsType, i, err)
			}
			for _, id := range ids {
				if id == item.ID {
					return validation.NewErrBadRecordFieldValue(fmt.Sprintf("items.%v[%v]", itemsType, i), "duplicate id")
				}
			}
			for _, name := range names {
				if name == item.Name {
					return validation.NewErrBadRecordFieldValue(fmt.Sprintf("items.%v[%v]", itemsType, i), "duplicate name")
				}
			}
			names = append(names, item.Name)
		}
		return nil
	}

	//validateMapOfItems := func(itemsType string, items map[string]string) error {
	//	//for id, name := range items {
	//	//
	//	//}
	//	return nil
	//}
	//if err := validateMapOfItems("boards", v.Boards); err != nil {
	//	return err
	//}
	//if err := validateMapOfItems("queries", v.Queries); err != nil {
	//	return err
	//}
	for itemsType, items := range v.Items {
		if !isKnownFolderItemType(itemsType) {
			return validation.NewErrBadRecordFieldValue("items", "unknown items type: "+itemsType)
		}
		if err := validateSliceOfItems(itemsType, items); err != nil {
			return err
		}
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

func isKnownFolderItemType(v string) bool {
	switch v {
	case "queries", "boards", "folders":
		return true
	default:
		return false
	}
}

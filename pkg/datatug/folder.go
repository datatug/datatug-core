package datatug

import (
	"strconv"
	"strings"

	"github.com/strongo/validation"
)

type Folders []*Folder

// Folder keeps info about folder
type Folder struct {
	Name string `json:"name,omitempty" firestore:"name,omitempty"` // empty for root folders
	Note string `json:"note,omitempty" firestore:"note,omitempty"`
	// NumberOf keeps count of all successor objects in all sub-folders
	NumberOf map[string]int `json:"numberOf,omitempty" firestore:"numberOf,omitempty"`
	//Folders map[string]*FolderItem `json:"folders,omitempty" firestore:"folders,omitempty"`
	//Boards  map[string]*FolderItem `json:"boards,omitempty" firestore:"boards,omitempty"`
	//Queries map[string]*FolderItem `json:"queries,omitempty" firestore:"queries,omitempty"`
}

func (f *Folder) GetID() string {
	return f.Name
}

func (f *Folder) SetID(id string) {
	f.Name = id
}

// FolderBrief holds brief about a folder item
type FolderBrief struct {
	Title string `json:"title" firestore:"title"`
}

// Validate returns error if not valid
func (v FolderBrief) Validate() error {
	if strings.TrimSpace(v.Title) == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	if strings.TrimSpace(v.Title) != v.Title {
		return validation.NewErrBadRecordFieldValue("name", "can't start or end with spaces")
	}
	return nil
}

// FolderItem holds info about a folder item
type FolderItem struct { // TODO: remove? Seems not to be used anywhere
	ID    string `json:"id" firestore:"id"`
	Title string `json:"title" firestore:"title"`
}

// Validate returns error if not valid
func (v FolderItem) Validate() error {
	if strings.TrimSpace(v.ID) == "" {
		return validation.NewErrRecordIsMissingRequiredField("id")
	}
	if strings.TrimSpace(v.Title) == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	if strings.TrimSpace(v.ID) != v.ID {
		return validation.NewErrBadRecordFieldValue("id", "can't start or end with spaces")
	}
	if strings.TrimSpace(v.Title) != v.Title {
		return validation.NewErrBadRecordFieldValue("name", "can't start or end with spaces")
	}
	return nil
}

// Validate returns error if failed
func (v *Folder) Validate() error {
	if strings.TrimSpace(v.Name) == "" {
		return validation.NewErrRecordIsMissingRequiredField("name")
	}
	if strings.TrimSpace(v.Name) != v.Name {
		return validation.NewErrBadRecordFieldValue("name", "folder name can't start or end with spaces")
	}

	//validateMapOfItems := func(itemsType string, items map[string]*FolderBrief) error {
	//	names := make([]string, 0, len(items))
	//	for id, item := range items {
	//		if err := item.Validate(); err != nil {
	//			return validation.NewErrBadRecordFieldValue(fmt.Sprintf("%v[%v]", itemsType, id), err.Error())
	//		}
	//		for _, name := range names {
	//			if name == item.Title {
	//				return validation.NewErrBadRecordFieldValue(fmt.Sprintf("%v[%v]", itemsType, id), "duplicate name")
	//			}
	//		}
	//		names = append(names, item.Title)
	//
	//	}
	//	return nil
	//}
	//if err := validateMapOfItems("folders", v.Folders); err != nil {
	//	return err
	//}
	//if err := validateBoardBriefsMappedByID(v.Boards); err != nil { // TODO: generic
	//	return validation.NewErrBadRecordFieldValue("boards", err.Error())
	//}
	//if err := validateQueryBriefsMappedByID(v.Queries); err != nil { // TODO: generic
	//	return validation.NewErrBadRecordFieldValue("queries", err.Error())
	//}
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

//func validateItemMappedByID(mapID, itemID string, item validatable) error {
//	if err := item.Validate(); err != nil {
//		return validation.NewErrBadRecordFieldValue(mapID+"["+itemID+"]", err.Error())
//	}
//	return nil
//}

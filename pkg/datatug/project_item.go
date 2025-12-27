package datatug

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/strongo/validation"
)

// AutoID defines a value that indicate system to use automatically generated ID
//const AutoID = "<auto/id>"

// RootSharedFolderName defines name for a root shared folder
const RootSharedFolderName = "~"
const RootUserFolderPrefix = "user:"

// FoldersPathSeparator defines a character to be used in folders path
const FoldersPathSeparator = `/`

// ProjItemBrief hold a brief about a project item
type ProjItemBrief struct {
	ID    string `json:"id,omitempty" firestore:"id,omitempty" yaml:"id,omitempty"`
	Title string `json:"title,omitempty" firestore:"title,omitempty" yaml:"title,omitempty"`
	// Document what is Folder? should it be moved somewhere?
	Folder string `json:"folder,omitempty" firestore:"folder,omitempty" yaml:"folder,omitempty"` // TODO: document purpose and usage
	ListOfTags
}

func (v *ProjItemBrief) GetID() string {
	return v.ID
}

func (v *ProjItemBrief) SetID(id string) {
	v.ID = id
}

// Validate returns error if not valid
func (v *ProjItemBrief) Validate(isTitleRequired bool) error {
	if v.ID == "" {
		return validation.NewErrRecordIsMissingRequiredField("id")
	}
	if err := validateStringField("title", v.Title, isTitleRequired, MaxTitleLength); err != nil {
		return err
	}
	if err := v.ListOfTags.Validate(); err != nil {
		return err
	}
	if v.Folder != "" {
		if err := ValidateFolderPath(v.Folder); err != nil {
			return err
		}
	}
	return nil
}

// ValidateFolderPath validates folder path
func ValidateFolderPath(folderPath string) error {
	if folderPath == "" {
		return validation.NewErrRecordIsMissingRequiredField("folder")
	}
	folders := strings.Split(folderPath, FoldersPathSeparator)
	rootFolderName := folders[0]
	if strings.HasPrefix(rootFolderName, RootUserFolderPrefix) {
		userID := rootFolderName[len(RootUserFolderPrefix):]
		if err := validateUserID(userID); err != nil {
			return validation.NewErrBadRecordFieldValue("folder", fmt.Sprintf("user's root folder references invalid user ID: %v", err))
		}
	} else if rootFolderName != RootSharedFolderName {
		return validation.NewErrBadRecordFieldValue("folder", fmt.Sprintf("should start with root folder '%v'", RootSharedFolderName))
	}
	for i, folder := range folders[1:] {
		name := strings.TrimSpace(folder)
		if name == "" {
			return validation.NewErrBadRecordFieldValue("folder", "invalid folder name at index "+strconv.Itoa(i))
		}
		if name != folder {
			return validation.NewErrBadRecordFieldValue("folder", "folder name at index starts or ends with spaces")
		}
		if name == RootSharedFolderName {
			return validation.NewErrBadRecordFieldValue("folder", "sub-folders can't be named as `~`")
		}
	}
	return nil
}

// ProjectItem base class with ID and Name
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
		if err := validateUserID(userID); err != nil {
			return validation.NewErrBadRecordFieldValue(fmt.Sprintf("userIDs[%v]", i), err.Error())
		}
		for j, uid := range v.UserIDs {
			if j != i && uid == userID {
				return validation.NewErrBadRecordFieldValue("userIds", fmt.Sprintf("duplicate value at indexex %v and %v", i, j))
			}
		}
	}
	return nil
}

func validateUserID(userID string) error {
	if strings.TrimSpace(userID) == "" {
		return fmt.Errorf("is empty")
	}
	return nil
}

// MaxTitleLength defines maximum length of a title = 100
const MaxTitleLength = 100

// MaxTagLength defines maximum length of a tag = 100
const MaxTagLength = 50

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

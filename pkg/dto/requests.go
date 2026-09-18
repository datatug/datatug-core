package dto

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/strongo/validation"
)

// GetServerDatabasesRequest input for /dbserver/databases API
type GetServerDatabasesRequest struct {
	Project     string `json:"proj"`
	Environment string `json:"env"`
	datatug.ServerRef
	Credentials *datatug.Credentials `json:"credentials"`
}

// Validate returns error if not valid
func (v GetServerDatabasesRequest) Validate() error {
	if strings.TrimSpace(v.Project) == "" {
		return validation.NewErrRequestIsMissingRequiredField("proj")
	}
	if strings.TrimSpace(v.Environment) == "" && v.Host == "" {
		return validation.NewErrRequestIsMissingRequiredField("env or host")
	}
	if v.Credentials != nil {
		if err := v.Credentials.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// CreateProjectRequest request
type CreateProjectRequest struct {
	StoreID string `json:"store"`

	// ID is the id of the project to create. It is supplied by the caller,
	// never derived from the title: it addresses the project for the rest of
	// its life — a key segment, and a directory name under a file-backed
	// store — so the caller, not the storage, owns it. See
	// validateProjectID for the rules it must satisfy.
	ID string `json:"id"`

	Title string `json:"title"`
}

type CreateProjectItemRequest struct {
	Title string `json:"title"`
}

func (v CreateProjectRequest) Validate() error {
	if strings.TrimSpace(v.StoreID) == "" {
		return validation.NewErrRequestIsMissingRequiredField("store")
	}
	if strings.TrimSpace(v.ID) == "" {
		return validation.NewErrRequestIsMissingRequiredField("id")
	}
	if err := validateProjectID(v.ID); err != nil {
		return err
	}
	if strings.TrimSpace(v.Title) == "" {
		return validation.NewErrRequestIsMissingRequiredField("title")
	}
	return nil
}

// maxProjectIDLength caps a project id well below the shortest filename
// limit a store's directory has to survive (255 bytes on ext4/APFS/NTFS),
// leaving room for the suffixes a project's own files add to it.
const maxProjectIDLength = 64

// validateProjectID applies DataTug's project-id rules. A project id is
// both a DALgo key segment and, under a file-backed store, a directory
// name, so it is restricted to what is unambiguous in both:
//
//   - 1 to maxProjectIDLength characters;
//   - lower-case ASCII letters, digits, "-" and "_" only. Upper case is
//     refused rather than folded, because a store cloned onto a
//     case-insensitive file system (macOS, Windows) would collapse two ids
//     that differ only in case into one directory;
//   - the first and last character must be a letter or a digit, so an id
//     never starts or ends with "-" or "_".
//
// The charset rules out every path separator, "." and "..", whitespace and
// control characters, so no id can escape or rename its own directory.
func validateProjectID(id string) error {
	if id == "" {
		return validation.NewErrRequestIsMissingRequiredField("id")
	}
	// Counted in characters, not bytes, so the message matches what a caller
	// typed. A multi-byte character is refused by the charset loop below in
	// any case, but not with a byte count it never asked about.
	if count := utf8.RuneCountInString(id); count > maxProjectIDLength {
		return validation.NewErrBadRequestFieldValue("id",
			fmt.Sprintf("must be at most %d characters, got %d", maxProjectIDLength, count))
	}
	for i, r := range id {
		isLetterOrDigit := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isLetterOrDigit {
			continue
		}
		if (r == '-' || r == '_') && i > 0 && i < len(id)-1 {
			continue
		}
		return validation.NewErrBadRequestFieldValue("id",
			fmt.Sprintf("invalid character %q at position %d: a project id is lower-case ASCII letters, digits, %q and %q, and starts and ends with a letter or a digit", r, i, "-", "_"))
	}
	return nil
}

type CreateQuery struct {
	ProjectRef
	Folder string                         `json:"folder"`
	Query  datatug.QueryDefWithFolderPath `json:"query"`
}

type UpdateQuery struct {
	ProjectItemRef
	Query datatug.QueryDefWithFolderPath `json:"query"`
}

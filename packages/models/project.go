package models

import (
	"fmt"
	"github.com/strongo/validation"
	"log"
	"strconv"
	"time"
)

// DatatugProject holds info about project
type DatatugProject struct {
	ID string `json:"id,omitempty" firestore:"id,omitempty"`
	//UUID          uuid.UUID           `json:"uuid"`
	Title         string              `json:"title,omitempty" firestore:"title,omitempty"`
	Created       *ProjectCreated     `json:"created,omitempty" firestore:"created,omitempty"`
	Access        string              `json:"access" firestore:"access,omitempty"` // e.g. "private", "protected", "public"
	Boards        Boards              `json:"boards,omitempty" firestore:"boards,omitempty"`
	Entities      Entities            `json:"entities,omitempty" firestore:"entities,omitempty"`
	Environments  Environments        `json:"environments,omitempty" firestore:"environments,omitempty"`
	DbModels      DbModels            `json:"dbModels,omitempty" firestore:"dbModels,omitempty"`
	DbServers     ProjDbServers       `json:"dbServers,omitempty" firestore:"dbServers,omitempty"`
	DbDifferences DatabaseDifferences `json:"dbDifferences,omitempty" firestore:"dbDifferences,omitempty"`
	Actions       Actions             `json:"actions,omitempty" firestore:"actions,omitempty"`
	Repository    *ProjectRepository  `json:"repository,omitempty" firestore:"repository,omitempty"`
}

// Validate returns error if not valid
func (v DatatugProject) Validate() error {
	switch v.Access {
	case "private", "protected", "public":
	case "":
		return validation.NewErrRecordIsMissingRequiredField("access")
	default:
		return validation.NewErrBadRecordFieldValue("access", "unknown value")
	}
	//if strings.TrimSpace(v.Title) == "" {
	//	return validation.NewErrRecordIsMissingRequiredField("title")
	//}
	if l := len(v.Title); l > 100 {
		return validation.NewErrBadRecordFieldValue("title", "too long title (max 100): "+strconv.Itoa(l))
	}
	log.Println("Validating environments...")
	if err := v.Environments.Validate(); err != nil {
		return fmt.Errorf("validation failed for project environments: %w", err)
	}
	log.Println("Validating entities...")
	if err := v.Entities.Validate(); err != nil {
		return fmt.Errorf("validation failed for project entities: %w", err)
	}
	log.Println("Validating DB models...")
	if err := v.DbModels.Validate(); err != nil {
		return fmt.Errorf("validation failed for project db models: %w", err)
	}
	log.Println("Validating boards...")
	if err := v.Boards.Validate(); err != nil {
		return fmt.Errorf("validation failed for project boards: %w", err)
	}
	log.Println("Validating DB servers...")
	if err := v.DbServers.Validate(); err != nil {
		return fmt.Errorf("validation failed for project db servers: %w", err)
	}
	log.Println("Validating actions...")
	if err := v.Actions.Validate(); err != nil {
		return fmt.Errorf("validation failed for project actions: %w", err)
	}
	return nil
}

//type EnvDbServers []*EnvDbServer

// ProjectBrief hold project brief info (e.g. for list)
type ProjectBrief struct {
	Access string `json:"access" firestore:"access"` // e.g. private, protected, public
	ProjectItem
	Repository *ProjectRepository `json:"repository,omitempty" firestore:"repository,omitempty"`
}

func (v *ProjectBrief) Validate() error {
	if err := v.ProjectItem.Validate(true); err != nil {
		return err
	}
	switch v.Access {
	case "":
		return validation.NewErrRecordIsMissingRequiredField("access")
	case "private", "protected", "public": // OK
		break
	default:
		return validation.NewErrBadRecordFieldValue("access", "unknown value: "+v.Access)
	}
	if v.Repository != nil {
		if err := v.Repository.Validate(); err != nil {
			return validation.NewErrBadRecordFieldValue("repository", err.Error())
		}
	}
	return nil
}

// ProjectSummary hold project summary
type ProjectSummary struct {
	ProjectFile
}

type ProjectRepository struct {
	Type      string `json:"type"` // e.g. "git"
	WebURL    string `json:"webURL"`
	ProjectID string `json:"projectId,omitempty"`
}

func (v *ProjectRepository) Validate() error {
	if v == nil {
		return nil
	}
	if v.Type == "" {
		return validation.NewErrRecordIsMissingRequiredField("type")
	}
	return nil
}

// ProjectFile defines what to store to project file
type ProjectFile struct {
	ProjectItem
	//UUID         uuid.UUID           `json:"uuid"`
	UserIDs      []string            `json:"userIds,omitempty" firestore:"userIds,omitempty"`
	Repository   *ProjectRepository  `json:"repository,omitempty" firestore:"repository,omitempty"`
	Created      *ProjectCreated     `json:"created,omitempty" firestore:"created,omitempty"`
	Access       string              `json:"access" firestore:"access"` // e.g. "private", "protected", "public"
	DbModels     []*ProjDbModelBrief `json:"dbModels,omitempty" firestore:"dbModels,omitempty"`
	Boards       []*ProjBoardBrief   `json:"boards,omitempty" firestore:"boards,omitempty"`
	Entities     []*ProjEntityBrief  `json:"entities,omitempty" firestore:"entities,omitempty"`
	Environments []*ProjEnvBrief     `json:"environments,omitempty" firestore:"environments,omitempty"`
}

// Validate returns error if not valid
func (v ProjectFile) Validate() error {
	// Do not check ID or title as they can be nil for project
	//if err := v.ProjectItem.Validate(); err != nil {
	//	return err
	//}
	if v.Created == nil {
		return validation.NewErrRecordIsMissingRequiredField("created")
	}
	//if v.Created.ByUsername == "" {
	//	return validation.NewErrRecordIsMissingRequiredField("created.byUsername")
	//}
	if v.Created.At.IsZero() {
		return validation.NewErrRecordIsMissingRequiredField("created.at")
	}
	switch v.Access {
	case "private", "protected", "public":
	default:
		return validation.NewErrBadRecordFieldValue("access", "expected 'private', 'protected' or 'public', got: "+v.Access)
	}
	for _, board := range v.Boards {
		if err := board.Validate(true); err != nil {
			return err
		}
	}
	for _, entity := range v.Entities {
		if err := entity.Validate(false); err != nil {
			return err
		}
	}
	for _, dbModel := range v.DbModels {
		if err := dbModel.Validate(false); err != nil {
			return err
		}
	}
	for _, env := range v.Environments {
		if err := env.Validate(false); err != nil {
			return err
		}
	}
	return nil
}

// ProjectCreated hold info about when & who created
type ProjectCreated struct {
	//ByName     string    `json:"byName,omitempty"`
	//ByUsername string    `json:"byUsername,omitempty"`
	At time.Time `json:"at" firestore:"at"`
}

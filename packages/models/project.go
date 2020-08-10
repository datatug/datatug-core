package models

import (
	"fmt"
	"github.com/strongo/validation"
	"strconv"
	"strings"
	"time"
)

// DataTugProject holds info about project
type DataTugProject struct {
	ID string `json:"id"`
	//UUID          uuid.UUID           `json:"uuid"`
	Title         string              `json:"title"`
	Created       *ProjectCreated     `json:"created,omitempty"`
	Access        string              `json:"access"` // e.g. "private", "protected", "public"
	Boards        Boards              `json:"boards,omitempty"`
	Entities      Entities            `json:"entities,omitempty"`
	Environments  Environments        `json:"environments,omitempty"`
	DbModels      DbModels            `json:"dbModels,omitempty"`
	DbServers     ProjDbServers       `json:"dbServers,omitempty"`
	DbDifferences DatabaseDifferences `json:"dbDifferences,omitempty"`
	Actions       Actions             `json:"actions,omitempty"`
}

// Validate returns error if not valid
func (v DataTugProject) Validate() error {
	switch v.Access {
	case "private", "protected", "public":
	case "":
		return validation.NewErrRecordIsMissingRequiredField("access")
	default:
		return validation.NewErrBadRecordFieldValue("access", "unknown value")
	}
	if strings.TrimSpace(v.Title) == "" {
		return validation.NewErrRecordIsMissingRequiredField("title")
	}
	if l := len(v.Title); l > 100 {
		return validation.NewErrBadRecordFieldValue("title", "too long title (max 100): "+strconv.Itoa(l))
	}
	if err := v.Environments.Validate(); err != nil {
		return fmt.Errorf("validation failed for project environments: %w", err)
	}
	if err := v.Entities.Validate(); err != nil {
		return fmt.Errorf("validation failed for project entities: %w", err)
	}
	if err := v.DbModels.Validate(); err != nil {
		return fmt.Errorf("validation failed for project db models: %w", err)
	}
	if err := v.Boards.Validate(); err != nil {
		return fmt.Errorf("validation failed for project boards: %w", err)
	}
	if err := v.DbServers.Validate(); err != nil {
		return fmt.Errorf("validation failed for project db servers: %w", err)
	}
	if err := v.Actions.Validate(); err != nil {
		return fmt.Errorf("validation failed for project actions: %w", err)
	}
	return nil
}

//type EnvDbServers []*EnvDbServer

// ProjectBrief hold project brief info (e.g. for list)
type ProjectBrief struct {
	ProjectEntity
}

// ProjectSummary hold project summary
type ProjectSummary struct {
	ProjectFile
}

// ProjectFile defines what to store to project file
type ProjectFile struct {
	ProjectEntity
	//UUID         uuid.UUID           `json:"uuid"`
	Created      *ProjectCreated     `json:"created,omitempty"`
	Access       string              `json:"access"` // e.g. "private", "protected", "public"
	DbModels     []*ProjDbModelBrief `json:"dbModels,omitempty"`
	Boards       []*ProjBoardBrief   `json:"boards,omitempty"`
	Entities     []*ProjEntityBrief  `json:"entities,omitempty"`
	Environments []*ProjEnvBrief     `json:"environments,omitempty"`
}

// Validate return error if not valid
func (v ProjectFile) Validate() error {
	// Do not check ID or title as they can be nil for project
	//if err := v.ProjectEntity.Validate(); err != nil {
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
	At time.Time `json:"at"`
}

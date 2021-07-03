package api

import (
	"fmt"
	"github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"github.com/strongo/validation"
	"log"
)

func validateEntityInput(projectID, entityID string) (err error) {
	if err = validateProjectInput(projectID); err != nil {
		return
	}
	if entityID == "" {
		return validation.NewErrRequestIsMissingRequiredField("entityID")
	}
	return
}

// GetEntity returns board by ID
func GetEntity(ref dto.ProjectItemRef) (entity *models.Entity, err error) {
	if err = validateEntityInput(ref.ProjectID, ref.ID); err != nil {
		return
	}
	store, err := storage.GetStore(ref.StoreID)
	if err == nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	project := store.Project(ref.ProjectID)
	return project.Entities().Entity(ref.ID).LoadEntity()
}

// GetAllEntities returns all entities
func GetAllEntities(ref dto.ProjectRef) (entity models.Entities, err error) {
	if err = validateProjectInput(ref.ProjectID); err != nil {
		return
	}
	store, err := storage.GetStore(ref.StoreID)
	if err == nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	project := store.Project(ref.ProjectID)
	return project.Entities().LoadEntities()
}

// DeleteEntity deletes board
func DeleteEntity(ref dto.ProjectItemRef) error {
	if err := validateEntityInput(ref.ProjectID, ref.ID); err != nil {
		return err
	}
	store, err := storage.GetStore(ref.StoreID)
	if err == nil {
		return err
	}
	//goland:noinspection GoNilness
	project := store.Project(ref.ProjectID)
	return project.Entities().Entity(ref.ID).DeleteEntity()
}

// SaveEntity saves board
func SaveEntity(ref dto.ProjectRef, entity *models.Entity) error {
	if entity.ID == "" {
		entity.ID = entity.Title
		entity.Title = ""
	} else if entity.Title == entity.ID {
		entity.Title = ""
	}
	if err := validateEntityInput(ref.ProjectID, entity.ID); err != nil {
		return err
	}
	if err := entity.Validate(); err != nil {
		return fmt.Errorf("entity is not valid: %w", err)
	}
	log.Printf("Saving entity: %+v", entity)
	store, err := storage.GetStore(ref.StoreID)
	if err == nil {
		return err
	}
	//goland:noinspection GoNilness
	project := store.Project(ref.ProjectID)
	return project.Entities().Entity(entity.ID).SaveEntity(entity)
}

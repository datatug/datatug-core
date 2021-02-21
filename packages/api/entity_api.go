package api

import (
	"fmt"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/store"
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
func GetEntity(projectID, entityID string) (entity models.Entity, err error) {
	if err = validateEntityInput(projectID, entityID); err != nil {
		return
	}
	return store.Current.LoadEntity(projectID, entityID)
}

// GetAllEntities returns all entities
func GetAllEntities(projectID string) (entity models.Entities, err error) {
	if err = validateProjectInput(projectID); err != nil {
		return
	}
	return store.Current.LoadEntities(projectID)
}

// DeleteEntity deletes board
func DeleteEntity(projectID, entityID string) (err error) {
	if err = validateEntityInput(projectID, entityID); err != nil {
		return
	}
	return store.Current.DeleteEntity(projectID, entityID)
}

// SaveEntity saves board
func SaveEntity(projectID string, entity *models.Entity) (err error) {
	if entity.ID == "" {
		entity.ID = entity.Title
		entity.Title = ""
	} else if entity.Title == entity.ID {
		entity.Title = ""
	}
	if err = validateEntityInput(projectID, entity.ID); err != nil {
		return
	}
	if err = entity.Validate(); err != nil {
		return fmt.Errorf("entity is not valid: %w", err)
	}
	log.Printf("Saving entity: %+v", entity)
	return store.Current.SaveEntity(projectID, entity)
}

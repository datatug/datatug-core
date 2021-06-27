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
func GetEntity(ref ProjectItemRef) (entity models.Entity, err error) {
	if err = validateEntityInput(ref.ProjectID, ref.ID); err != nil {
		return
	}
	var dal store.Interface
	dal, err = store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return
	}
	return dal.LoadEntity(ref.ProjectID, ref.ID)
}

// GetAllEntities returns all entities
func GetAllEntities(ref ProjectRef) (entity models.Entities, err error) {
	if err = validateProjectInput(ref.ProjectID); err != nil {
		return
	}
	var dal store.Interface
	dal, err = store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return
	}
	return dal.LoadEntities(ref.ProjectID)
}

// DeleteEntity deletes board
func DeleteEntity(ref ProjectItemRef) (err error) {
	if err = validateEntityInput(ref.ProjectID, ref.ID); err != nil {
		return
	}
	var dal store.Interface
	dal, err = store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return
	}
	return dal.DeleteEntity(ref.ProjectID, ref.ID)
}

// SaveEntity saves board
func SaveEntity(ref ProjectRef, entity *models.Entity) (err error) {
	if entity.ID == "" {
		entity.ID = entity.Title
		entity.Title = ""
	} else if entity.Title == entity.ID {
		entity.Title = ""
	}
	if err = validateEntityInput(ref.ProjectID, entity.ID); err != nil {
		return
	}
	if err = entity.Validate(); err != nil {
		return fmt.Errorf("entity is not valid: %w", err)
	}
	log.Printf("Saving entity: %+v", entity)
	var dal store.Interface
	dal, err = store.NewDatatugStore(ref.StoreID)
	if err != nil {
		return
	}
	return dal.SaveEntity(ref.ProjectID, entity)
}

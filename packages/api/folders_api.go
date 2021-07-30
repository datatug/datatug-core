package api

import (
	"context"
	"github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/models"
	"github.com/datatug/datatug/packages/storage"
	"github.com/strongo/validation"
)

// CreateFolder creates a new folder for queries
func CreateFolder(ctx context.Context, request dto.CreateFolder) (folder *models.Folder, err error) {
	if err := request.ProjectRef.Validate(); err != nil {
		return nil, err
	}
	store, err := storage.GetStore(ctx, request.StoreID)
	if err != nil {
		return nil, err
	}
	//goland:noinspection GoNilness
	project := store.Project(request.ProjectID)
	createFolderRequest := storage.CreateFolderRequest{
		Name: request.Name,
		Path: request.Path,
		Note: request.Name,
	}
	return project.Folders().CreateFolder(ctx, createFolderRequest)
}

// DeleteFolder deletes queries folder
func DeleteFolder(ctx context.Context, ref dto.ProjectItemRef) error {
	if ref.ProjectID == "" {
		return validation.NewErrRequestIsMissingRequiredField("projectID")
	}
	store, err := storage.GetStore(ctx, ref.StoreID)
	if err != nil {
		return err
	}
	//goland:noinspection GoNilness
	project := store.Project(ref.ProjectID)
	return project.Folders().DeleteFolder(ctx, ref.ID)
}

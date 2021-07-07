package api

import (
	"context"
	"github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/storage"
	"github.com/strongo/validation"
)

// CreateFolder creates a new folder for queries
func CreateFolder(ctx context.Context, request dto.CreateFolder) error {
	if err := request.ProjectRef.Validate(); err != nil {
		return err
	}
	store, err := storage.GetStore(ctx, request.StoreID)
	if err == nil {
		return err
	}
	//goland:noinspection GoNilness
	project := store.Project(request.ProjectID)
	return project.Folder(request.Path).CreateFolder(ctx, request.Name)
}

// DeleteFolder deletes queries folder
func DeleteFolder(ctx context.Context, ref dto.ProjectItemRef) error {
	if ref.ProjectID == "" {
		return validation.NewErrRequestIsMissingRequiredField("projectID")
	}
	store, err := storage.GetStore(ctx, ref.StoreID)
	if err == nil {
		return err
	}
	//goland:noinspection GoNilness
	project := store.Project(ref.ProjectID)
	return project.Folder(ref.ID).DeleteFolder(ctx)
}

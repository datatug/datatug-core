package storage

import (
	"context"
	"fmt"
	"github.com/datatug/datatug-core/pkg/models"
	"github.com/strongo/validation"
	"strings"
)

type CreateFolderRequest struct {
	Name string
	Path string
	Note string
}

func (v CreateFolderRequest) Validate() error {
	if strings.TrimSpace(v.Name) == "" {
		return validation.NewErrRequestIsMissingRequiredField("name")
	}
	if strings.TrimSpace(v.Path) == "" {
		return validation.NewErrRequestIsMissingRequiredField("path")
	}
	paths := strings.Split(v.Path, "/")
	for i, p := range paths {
		if strings.TrimSpace(p) == "" {
			return validation.NewErrBadRequestFieldValue("path",
				fmt.Sprintf("empty path segment at index %v", i))
		}
	}
	if len(v.Note) > 0 && strings.TrimSpace(v.Note) == "" {
		return validation.NewErrBadRequestFieldValue("path", "empty note")
	}
	return nil
}

type FoldersStore interface {
	CreateFolder(ctx context.Context, request CreateFolderRequest) (folder *models.Folder, err error)
	GetFolder(ctx context.Context, path string) (folder *models.Folder, err error)
	DeleteFolder(ctx context.Context, path string) (err error)
}

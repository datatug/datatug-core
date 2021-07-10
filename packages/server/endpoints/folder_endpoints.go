package endpoints

import (
	"context"
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/dto"
	"net/http"
)

// CreateFolder handles create query endpoint
func CreateFolder(w http.ResponseWriter, r *http.Request) {
	var ref dto.ProjectRef
	var request dto.CreateFolder
	saveFunc := func(ctx context.Context) (ResponseDTO, error) {
		return nil, api.CreateFolder(ctx, request)
	}
	createProjectItem(w, r, &ref, &request, saveFunc)
	return
}

// DeleteFolder handles delete query folder endpoint
var DeleteFolder = deleteProjItem(api.DeleteFolder)

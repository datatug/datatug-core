package endpoints

import (
	"context"
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/dto"
	"net/http"
)

// createFolder handles create query endpoint
func createFolder(w http.ResponseWriter, r *http.Request) {
	var ref dto.ProjectRef
	var request dto.CreateFolder
	saveFunc := func(ctx context.Context) (ResponseDTO, error) {
		return nil, api.CreateFolder(ctx, request)
	}
	createProjectItem(w, r, &ref, &request, saveFunc)
	return
}

// deleteFolder handles delete query folder endpoint
var deleteFolder = deleteProjItem(api.DeleteFolder)

package endpoints

import (
	"context"
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/dto"
	"net/http"
)

// CreateFolder handles create query endpoint
func CreateFolder(w http.ResponseWriter, r *http.Request) {
	var request dto.CreateFolder
	saveFunc := func(ctx context.Context, ref dto.ProjectItemRef) (interface{}, error) {
		return nil, api.CreateFolder(ctx, request)
	}
	saveItem(w, r, &request, saveFunc)
	return
}

// DeleteFolder handles delete query folder endpoint
var DeleteFolder = deleteProjItem(api.DeleteFolder)

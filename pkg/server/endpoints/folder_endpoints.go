package endpoints

import (
	"context"
	"github.com/datatug/datatug-core/pkg/api"
	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/sneat-co/sneat-go-core/apicore"
	"net/http"
)

// createFolder handles create query endpoint
func createFolder(w http.ResponseWriter, r *http.Request) {
	var ref dto.ProjectRef
	var request dto.CreateFolder
	saveFunc := func(ctx context.Context) (apicore.ResponseDTO, error) {
		return api.CreateFolder(ctx, request)
	}
	createProjectItem(w, r, &ref, &request, saveFunc)
}

// deleteFolder handles delete query folder endpoint
var deleteFolder = deleteProjItem(api.DeleteFolder)

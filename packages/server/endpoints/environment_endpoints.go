package endpoints

import (
	context "context"
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/dto"
	"net/http"
)

// getEnvironmentSummary returns summary about environment
func getEnvironmentSummary(w http.ResponseWriter, r *http.Request) {
	var ref dto.ProjectItemRef
	getProjectItem(w, r, &ref, func(ctx context.Context) (responseDTO ResponseDTO, err error) {
		return api.GetEnvironmentSummary(ctx, ref)
	})
}

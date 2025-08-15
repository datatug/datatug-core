package endpoints

import (
	context "context"
	"github.com/datatug/datatug-core/pkg/api"
	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/sneat-co/sneat-go-core/apicore"
	"net/http"
)

// getEnvironmentSummary returns summary about environment
func getEnvironmentSummary(w http.ResponseWriter, r *http.Request) {
	var ref dto.ProjectItemRef
	getProjectItem(w, r, &ref, func(ctx context.Context) (responseDTO apicore.ResponseDTO, err error) {
		return api.GetEnvironmentSummary(ctx, ref)
	})
}

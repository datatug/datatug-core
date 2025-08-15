package endpoints

import (
	"context"
	"github.com/datatug/datatug-core/pkg/api"
	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/datatug/datatug-core/pkg/models"
	"github.com/sneat-co/sneat-go-core/apicore"
	"github.com/strongo/random"
	"net/http"
)

// getBoard handles get board endpoint
func getBoard(w http.ResponseWriter, r *http.Request) {
	var ref dto.ProjectItemRef
	getProjectItem(w, r, &ref, func(ctx context.Context) (responseDTO apicore.ResponseDTO, err error) {
		return api.GetBoard(ctx, ref)
	})
}

// createBoard handles board creation endpoint
func createBoard(w http.ResponseWriter, r *http.Request) {
	var ref dto.ProjectRef
	var board models.Board
	board.ID = models.AutoID
	createProjectItem(w, r, &ref, &board, func(ctx context.Context) (apicore.ResponseDTO, error) {
		board.ID = random.ID(9)
		return api.CreateBoard(ctx, ref, board)
	})
}

// saveBoard handles save board endpoint
func saveBoard(w http.ResponseWriter, r *http.Request) {
	var ref dto.ProjectItemRef
	var board models.Board
	saveProjectItem(w, r, &ref, &board, func(ctx context.Context) (apicore.ResponseDTO, error) {
		return api.SaveBoard(ctx, ref.ProjectRef, board)
	})
}

// deleteBoard handles delete board endpoint
var deleteBoard = deleteProjItem(api.DeleteBoard)

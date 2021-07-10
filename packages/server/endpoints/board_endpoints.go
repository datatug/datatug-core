package endpoints

import (
	"context"
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/models"
	"github.com/strongo/random"
	"net/http"
)

// getBoard handles get board endpoint
func getBoard(w http.ResponseWriter, r *http.Request) {
	var ref dto.ProjectItemRef
	getProjectItem(w, r, &ref, func(ctx context.Context) (responseDTO ResponseDTO, err error) {
		return api.GetBoard(ctx, ref)
	})
}

// createBoard handles board creation endpoint
func createBoard(w http.ResponseWriter, r *http.Request) {
	var ref dto.ProjectRef
	var board models.Board
	createProjectItem(w, r, &ref, &board, func(ctx context.Context) (ResponseDTO, error) {
		board.ID = random.ID(9)
		return api.CreateBoard(ctx, ref, board)
	})
}

// saveBoard handles save board endpoint
func saveBoard(w http.ResponseWriter, r *http.Request) {
	var ref dto.ProjectItemRef
	var board models.Board
	saveProjectItem(w, r, &ref, &board, func(ctx context.Context) (ResponseDTO, error) {
		return api.SaveBoard(ctx, ref.ProjectRef, board)
	})
}

// deleteBoard handles delete board endpoint
var deleteBoard = deleteProjItem(api.DeleteBoard)

package endpoints

import (
	"context"
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/dto"
	"github.com/datatug/datatug/packages/models"
	"github.com/strongo/random"
	"net/http"
)

// GetBoard handles get board endpoint
func GetBoard(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	ref := newProjectItemRef(query, "")
	ctx, err := getContextFromRequest(r)
	if err != nil {
		handleError(err, w, r)
	}
	board, err := api.GetBoard(ctx, ref)
	returnJSON(w, r, http.StatusOK, err, board)
}

// CreateBoard handles board creation endpoint
func CreateBoard(w http.ResponseWriter, r *http.Request) {
	var ref dto.ProjectRef
	var board models.Board
	saveFunc := func(ctx context.Context) (ResponseDTO, error) {
		board.ID = random.ID(9)
		return api.CreateBoard(ctx, ref, board)
	}
	createProjectItem(w, r, &ref, &board, saveFunc)
}

// SaveBoard handles save board endpoint
func SaveBoard(w http.ResponseWriter, r *http.Request) {
	ref := newProjectItemRef(r.URL.Query(), "")
	var board models.Board
	saveBoard := func(ctx context.Context) (ResponseDTO, error) {
		return api.SaveBoard(ctx, ref.ProjectRef, board)
	}
	saveProjectItem(w, r, &ref, &board, saveBoard)
}

// DeleteBoard handles delete board endpoint
var DeleteBoard = deleteProjItem(api.DeleteBoard)

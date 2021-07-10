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
	ref := newProjectItemRef(query)
	ctx, err := GetContext(r)
	if err != nil {
		handleError(err, w, r)
	}
	board, err := api.GetBoard(ctx, ref)
	returnJSON(w, r, http.StatusOK, err, board)
}

// CreateBoard handles board creation endpoint
func CreateBoard(w http.ResponseWriter, r *http.Request) {
	var board models.Board
	saveBoard := func(ctx context.Context, projectIemRef dto.ProjectItemRef) (interface{}, error) {
		board.ID = random.ID(9)
		projectIemRef.ID = board.ID
		return api.SaveBoard(ctx, projectIemRef, board)
	}
	saveItem(w, r, &board, saveBoard)
}

// SaveBoard handles save board endpoint
func SaveBoard(w http.ResponseWriter, r *http.Request) {
	var board models.Board
	saveBoard := func(ctx context.Context, ref dto.ProjectItemRef) (interface{}, error) {
		return api.SaveBoard(ctx, ref, board)
	}
	saveItem(w, r, &board, saveBoard)
}

// DeleteBoard handles delete board endpoint
var DeleteBoard = deleteProjItem(api.DeleteBoard)

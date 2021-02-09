package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"github.com/datatug/datatug/packages/models"
	"github.com/strongo/random"
	"net/http"
)

// GetBoard handles get board endpoint
func GetBoard(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	projectID := query.Get(urlQueryParamProjectID)
	boardID := query.Get(urlQueryParamID)
	board, err := api.GetBoard(projectID, boardID)
	returnJSON(w, r, http.StatusOK, err, board)
}

// CreateBoard handles board creation endpoint
func CreateBoard(w http.ResponseWriter, r *http.Request) {
	var board models.Board
	saveBoard := func(projectID string) error {
		board.ID = random.ID(9)
		return api.SaveBoard(projectID, board)
	}
	saveItem(w, r, &board, saveBoard)
}

// SaveBoard handles save board endpoint
func SaveBoard(w http.ResponseWriter, r *http.Request) {
	var board models.Board
	saveBoard := func(projectID string) error {
		return api.SaveBoard(projectID, board)
	}
	saveItem(w, r, &board, saveBoard)
}

// DeleteBoard handles delete board endpoint
func DeleteBoard(w http.ResponseWriter, r *http.Request) {
	deleteItem(w, r, "board", api.DeleteBoard)
}

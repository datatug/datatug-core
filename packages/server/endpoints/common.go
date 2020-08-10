package endpoints

import (
	"encoding/json"
	"net/http"
)

func deleteItem(w http.ResponseWriter, request *http.Request, idParam string, del func(projectID string, id string) error) {
	query := request.URL.Query()
	projectID := query.Get("project")
	id := query.Get(idParam)
	err := del(projectID, id)
	ReturnJSON(w, request, http.StatusOK, err, true)
}

func saveItem(w http.ResponseWriter, r *http.Request, target interface{}, saveFunc func(projectID string) error) {
	projectID := r.URL.Query().Get("project")

	decoder := json.NewDecoder(r.Body)

	var err error
	if err = decoder.Decode(target); err != nil {
		handleError(err, w, r)
	}
	err = saveFunc(projectID)
	ReturnJSON(w, r, http.StatusOK, err, target)
}

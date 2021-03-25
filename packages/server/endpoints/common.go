package endpoints

import (
	"encoding/json"
	"net/http"
)

func deleteItem(w http.ResponseWriter, request *http.Request, idParam string, del func(projectID string, id string) error) {
	query := request.URL.Query()
	projectID := query.Get(urlQueryParamProjectID)
	id := query.Get(idParam)
	err := del(projectID, id)
	returnJSON(w, request, http.StatusOK, err, true)
}

func saveItem(
	w http.ResponseWriter, r *http.Request,
	target interface{},
	saveFunc func(projectID string) (result interface{}, err error),
) {
	projectID := r.URL.Query().Get(urlQueryParamProjectID)

	decoder := json.NewDecoder(r.Body)

	var err error
	if err = decoder.Decode(target); err != nil {
		handleError(err, w, r)
	}
	var result interface{}
	result, err = saveFunc(projectID)
	returnJSON(w, r, http.StatusOK, err, result)
}

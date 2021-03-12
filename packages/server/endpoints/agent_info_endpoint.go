package endpoints

import (
	"github.com/datatug/datatug/packages/api"
	"net/http"
)

// Version returns version of the agent
func AgentInfo(w http.ResponseWriter, r *http.Request) {
	result := api.GetAgentInfo()
	returnJSON(w, r, http.StatusOK, nil, result)
}

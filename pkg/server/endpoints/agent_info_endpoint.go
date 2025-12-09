package endpoints

import (
	"net/http"

	"github.com/datatug/datatug-core/pkg/api"
)

// AgentInfo returns version of the agent
func AgentInfo(w http.ResponseWriter, r *http.Request) {
	result := api.GetAgentInfo()
	returnJSON(w, r, http.StatusOK, nil, result)
}

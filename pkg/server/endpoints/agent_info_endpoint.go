package endpoints

import (
	"github.com/datatug/datatug-core/pkg/api"
	"net/http"
)

// AgentInfo returns version of the agent
func AgentInfo(w http.ResponseWriter, r *http.Request) {
	result := api.GetAgentInfo()
	returnJSON(w, r, http.StatusOK, nil, result)
}

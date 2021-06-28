package api

import "time"

// DataTugAgentVersion specifies agent version
const DataTugAgentVersion = "0.0.1"

// AgentInfo holds agent info
type AgentInfo struct {
	Version       string  `json:"version"`
	UptimeMinutes float64 `json:"uptimeMinutes"`
}

var started = time.Now()

// GetAgentInfo returns agent info
func GetAgentInfo() AgentInfo {
	return AgentInfo{
		Version:       DataTugAgentVersion,
		UptimeMinutes: time.Now().Sub(started).Minutes(),
	}
}

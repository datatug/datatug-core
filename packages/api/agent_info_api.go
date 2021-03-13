package api

import "time"

const DataTugAgentVersion = "0.0.1"

type AgentInfo struct {
	Version       string  `json:"version"`
	UptimeMinutes float64 `json:"uptimeMinutes"`
}

var started = time.Now()

func GetAgentInfo() AgentInfo {
	return AgentInfo{
		Version:       DataTugAgentVersion,
		UptimeMinutes: time.Now().Sub(started).Minutes(),
	}
}

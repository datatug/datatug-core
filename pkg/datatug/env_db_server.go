package datatug

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/strongo/slice"
	"github.com/strongo/validation"
)

// EnvDbServers is a slice of *EnvDbServer
type EnvDbServers []*EnvDbServer

// Validate returns error of failed
func (v EnvDbServers) Validate() error {
	if v == nil {
		return nil
	}
	ids := make(map[string]int, len(v))
	for i, item := range v {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("invalid env db server at index %v: %w", i, err)
		}
		// Two servers with one ID would be saved to one file, and one of them lost.
		id := item.GetID()
		if prev, taken := ids[id]; taken {
			return validation.NewErrBadRecordFieldValue("dbServers", fmt.Sprintf("duplicate server ID at indexes %v & %v: %v", prev, i, id))
		}
		ids[id] = i
	}
	return nil
}

// GetByServerRef returns *EnvDbServer by GetID
func (v EnvDbServers) GetByServerRef(serverRef ServerRef) *EnvDbServer {
	for _, item := range v {
		if item.Driver == serverRef.Driver && item.Host == serverRef.Host && item.Port == serverRef.Port {
			return item
		}
	}
	return nil
}

// EnvDbServer holds information about DB server in an environment
type EnvDbServer struct {
	ServerRef
	Catalogs []string `json:"catalogs,omitempty"`
}

// GetID is "host:port" for a server recorded by its host, and the driver for one recorded
// without a host (a server whose connection URL stays in an environment variable), so that two
// such servers of different drivers in one environment have different IDs.
func (v *EnvDbServer) GetID() string {
	if v.Host == "" {
		return v.Driver
	}
	return fmt.Sprintf("%s:%d", v.Host, v.Port)
}

// SetID is the reverse of GetID: an ID without a colon is a driver, any other is
// "host:port" (a port that is not a number is 0).
func (v *EnvDbServer) SetID(id string) {
	host, port, hasPort := strings.Cut(id, ":")
	if !hasPort {
		v.Driver, v.Host, v.Port = id, "", 0
		return
	}
	v.Host = host
	v.Port, _ = strconv.Atoi(port)
}

// Validate returns error if no valid
func (v *EnvDbServer) Validate() error {
	if err := v.ServerRef.Validate(); err != nil {
		return err
	}

	for i, catalogID := range v.Catalogs {
		if strings.TrimSpace(catalogID) == "" {
			return validation.NewErrRecordIsMissingRequiredField(fmt.Sprintf("catalogs[%v]", i))
		}
		if prevIndex := slice.Index(v.Catalogs[:i], catalogID); prevIndex >= 0 {
			return validation.NewErrBadRecordFieldValue("catalogs", fmt.Sprintf("duplicate value at indexes %v & %v: %v", prevIndex, i, catalogID))
		}
	}
	return nil
}

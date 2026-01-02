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
	for i, item := range v {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("invalid env db server at index %v: %w", i, err)
		}
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

func (v *EnvDbServer) GetID() string {
	return fmt.Sprintf("%s:%d", v.Host, v.Port)
}

func (v *EnvDbServer) SetID(id string) {
	vals := strings.Split(id, ":")
	v.Host = vals[0]
	v.Port, _ = strconv.Atoi(vals[1])
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

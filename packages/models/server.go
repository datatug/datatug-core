package models

import (
	"fmt"
	"github.com/strongo/validation"
	"strconv"
	"strings"
)

// DbServer hold info about DB server
type DbServer struct {
	Driver string `json:"driver"`
	Host   string `json:"host"`
	Port   int    `json:"port,omitempty"`
}

// FileName returns a name for a file (probably should be moved to a func in filestore package)
func (v DbServer) FileName() string {
	return v.name("@")
}

// Address returns a "host:port" string
func (v DbServer) Address() string {
	return v.name(":")
}

func (v DbServer) name(sep string) string {
	if v.Port > 0 {
		return v.Host + sep + strconv.Itoa(v.Port)
	}
	return v.Host
}

// NewDbServer creates DbServer
func NewDbServer(driver, hostWithOptionalPort, sep string) (dbServer DbServer, err error) {
	dbServer.Driver = driver
	i := strings.Index(hostWithOptionalPort, sep)
	if i < 0 {
		dbServer.Host = hostWithOptionalPort
		return
	}
	dbServer.Host = hostWithOptionalPort[:i]
	dbServer.Port, err = strconv.Atoi(hostWithOptionalPort[i+1:])
	return
}

// ID returns string key for the server
func (v DbServer) ID() string {
	if v.Port == 0 {
		return fmt.Sprintf("%v:%v", v.Driver, v.Host)
	}
	return fmt.Sprintf("%v:%v:%v", v.Driver, v.Host, v.Port)
}

// Validate returns error if not valid
func (v DbServer) Validate() error {
	switch v.Driver {
	case "":
		return validation.NewErrRecordIsMissingRequiredField("driver")
	case "sqlserver", "mysql", "oracle":
		//
	default:
		return validation.NewErrBadRecordFieldValue("driver", fmt.Sprintf("unexpected value: %v", v.Driver))
	}
	if v.Host == "" {
		return validation.NewErrRecordIsMissingRequiredField("host")
	}
	if v.Port < 0 {
		return validation.NewErrBadRecordFieldValue("port", "should be positive")
	}
	return nil
}

// ProjDbServer hold info about a project DB server - NOT sure if right way
type ProjDbServer struct {
	ProjectItem
	DbServer   DbServer   `json:"dbServer"`
	DbCatalogs DbCatalogs `json:"dbCatalogs"`
}

// Validate returns error if not valid
func (v ProjDbServer) Validate() error {
	if err := v.ProjectItem.Validate(false); err != nil {
		return err
	}
	if err := v.DbServer.Validate(); err != nil {
		return err
	}
	if err := v.DbCatalogs.Validate(); err != nil {
		return err
	}
	return nil
}

// ProjDbServers slice of ProjDbServer which holds DbServer and DbCatalogs
type ProjDbServers []*ProjDbServer

// Validate returns error if not valid
func (v ProjDbServers) Validate() error {
	for i, s := range v {
		if s == nil {
			return fmt.Errorf("nil at index=%v", i)
		}
		if err := s.Validate(); err != nil {
			return fmt.Errorf("failed validation for DB server at index=%v, id=%v: %w", i, s.ID, err)
		}
	}
	return nil
}

//func (v ProjDbServers) GetProjDbServer(driver, host string, port int) *ProjDbServer {
//	for _, item := range v {
//		if item.Host == host {
//			if port > 0 && item.Port == port || item.Driver == driver {
//				return item
//			}
//		}
//	}
//	return nil
//}

// ProjDbServerFile stores summary info about DbServer
type ProjDbServerFile struct {
}

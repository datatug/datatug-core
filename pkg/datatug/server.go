package datatug

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/strongo/validation"
)

// ServerReferences defines slice
type ServerReferences []ServerRef

// Validate returns error if not valid
func (v ServerReferences) Validate() error {
	for i, s := range v {
		if err := s.Validate(); err != nil {
			return fmt.Errorf("invalid server at index %v: %w", i, err)
		}
	}
	return nil
}

// ServerRef hold info about DB server
type ServerRef struct {
	Driver string `json:"driver"`
	Host   string `json:"host,omitempty"`
	Path   string `json:"path,omitempty"` // A path to a folder with database files - to be used by SQLite, for example.
	Port   int    `json:"port,omitempty"`
}

// FileName returns a name for a file (probably should be moved to a func in filestore package)
func (v ServerRef) FileName() string {
	return v.name("@")
}

// Address returns a "host:port" string
func (v ServerRef) Address() string {
	return v.name(":")
}

func (v ServerRef) name(sep string) string {
	if v.Port > 0 {
		return v.Host + sep + strconv.Itoa(v.Port)
	}
	return v.Host
}

// NewDbServer creates ServerRef
func NewDbServer(driver, hostWithOptionalPort, sep string) (dbServer ServerRef, err error) {
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

// GetID returns string key for the server
func (v ServerRef) GetID() string {
	if v.Port == 0 {
		return fmt.Sprintf("%v:%v", v.Driver, v.Host)
	}
	return fmt.Sprintf("%v:%v:%v", v.Driver, v.Host, v.Port)
}

// Validate returns error if not valid
func (v ServerRef) Validate() error {
	switch v.Driver {
	case "":
		return validation.NewErrRecordIsMissingRequiredField("driver")
	case "sqlite3":
		if v.Host != "" {
			return validation.NewErrBadRecordFieldValue("host", "cannot be used with sqlite3, got: "+v.Host)
		}
		if v.Port != 0 {
			return validation.NewErrBadRecordFieldValue("port", "cannot be used with sqlite3, got: "+strconv.Itoa(v.Port))
		}
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

// ProjDbServer holds info about a project DB server - NOT sure if right way
type ProjDbServer struct {
	ProjectItem
	Server   ServerRef  `json:"server"`
	Catalogs DbCatalogs `json:"catalogs"`
}

// Validate returns error if not valid
func (v ProjDbServer) Validate() error {
	if id := v.Server.GetID(); id != v.ID {
		return validation.NewErrBadRecordFieldValue("id", fmt.Sprintf("expected %s, got %s", id, v.ID))
	}
	if err := v.ValidateWithOptions(false); err != nil {
		return err
	}
	if err := v.Server.Validate(); err != nil {
		return err
	}
	if err := v.Catalogs.Validate(); err != nil {
		return err
	}
	return nil
}

var _ IProjectItems[*ProjDbDriver] = (ProjDbDrivers)(nil)

type ProjDbDrivers []*ProjDbDriver

func (p ProjDbDrivers) IDs() []string {
	return ProjectItems[*ProjDbDriver](p).IDs()
}

func (p ProjDbDrivers) GetByID(id string) *ProjDbDriver {
	return ProjectItems[*ProjDbDriver](p).GetByID(id)
}

func (p ProjDbDrivers) Validate() error {
	return ProjectItems[*ProjDbDriver](p).Validate()
}

type ProjDbDriver struct {
	ProjectItem
	Servers ProjDbServers `json:"servers"`
}

func (v ProjDbDriver) Validate() error {
	if err := v.ProjItemBrief.Validate(); err != nil {
		return err
	}
	return v.Servers.Validate()
}

// ProjDbServers slice of ProjDbServer which holds ServerRef and DbCatalogs
type ProjDbServers ProjectItems[*ProjDbServer]

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

// GetProjDbServer returns db servers
func (v ProjDbServers) GetProjDbServer(ref ServerRef) *ProjDbServer {
	for _, item := range v {
		if item.Server.Host == ref.Host && item.Server.Port == ref.Port && item.Server.Driver == ref.Driver {
			return item
		}
	}
	return nil
}

// ProjDbServerFile stores summary info about ServerRef
type ProjDbServerFile struct {
	ServerRef
	Catalogs []string `jsont:"catalogs,omitempty" firestore:"catalogs,omitempty"`
}

// Validate returns error if not valid
func (v ProjDbServerFile) Validate() error {
	if err := v.ServerRef.Validate(); err != nil {
		return err
	}
	for i, catalog := range v.Catalogs {
		if strings.TrimSpace(catalog) == "" {
			return validation.NewErrBadRecordFieldValue(fmt.Sprintf("catalogs[%v]", i), "empty catalog name")
		}
	}
	return nil
}

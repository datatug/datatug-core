package datatug

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/strongo/validation"
)

func NewProject(id string, newStore func(p *Project) ProjectStore) (p *Project) {
	p = new(Project)
	p.ID = id
	if newStore != nil {
		p.store = newStore(p)
	}
	return
}

// Project holds info about a project
type Project struct {
	ProjectItem
	store    ProjectStore
	Created  *ProjectCreated `json:"created,omitempty" firestore:"created,omitempty"`
	Boards   Boards          `json:"boards,omitempty" firestore:"boards,omitempty"`
	Queries  *QueriesFolder  `json:"queries,omitempty" firestore:"queries,omitempty"`
	Entities Entities        `json:"entities,omitempty" firestore:"entities,omitempty"`

	// Use GetEnvironments to get the latest
	Environments Environments `json:"environments,omitempty" firestore:"environments,omitempty"`

	DbModels DbModels `json:"dbModels,omitempty" firestore:"dbModels,omitempty"`

	DbDrivers ProjDbDrivers `json:"dbDrivers,omitempty" firestore:"dbDrivers,omitempty"`

	Actions    Actions            `json:"actions,omitempty" firestore:"actions,omitempty"`
	Repository *ProjectRepository `json:"repository,omitempty" firestore:"repository,omitempty"`
}

func (p *Project) GetEnvironments(ctx context.Context) (environments Environments, err error) {
	if p.Environments == nil {
		if p.Environments, err = p.store.LoadEnvironments(ctx); err != nil {
			return
		}
	}
	return p.Environments, nil
}

func (p *Project) GetDBs(ctx context.Context, o ...StoreOption) (dbs ProjDbDrivers, err error) {
	if p.DbDrivers == nil {
		p.DbDrivers, err = p.store.LoadProjDbDrivers(ctx, o...)
	}
	return p.DbDrivers, err
}

func (p *Project) GetProjDbServer(ctx context.Context, serverRef ServerRef) (server *ProjDbServer, err error) {
	var dbs ProjDbDrivers
	if dbs, err = p.GetDBs(ctx); err != nil {
		return
	}
	for _, db := range dbs {
		if db.ID == serverRef.Driver {
			return db.Servers.GetProjDbServer(serverRef), nil
		}
	}
	return nil, nil
}
func (p *Project) AddProjDbServer(ctx context.Context, dbServer *ProjDbServer) (err error) {
	var dbs ProjDbDrivers
	if dbs, err = p.GetDBs(ctx); err != nil {
		return
	}
	driverID := dbServer.Server.Driver
	db := dbs.GetByID(driverID)
	if db == nil {
		db = new(ProjDbDriver)
		db.ID = driverID
		p.DbDrivers = append(p.DbDrivers, db)
	}
	db.Servers = append(db.Servers, dbServer)
	p.DbDrivers = append(p.DbDrivers, db)
	return
}

// Validate returns error if not valid
func (p *Project) Validate() error {
	switch p.Access {
	case "private", "protected", "public":
	case "":
		return validation.NewErrRecordIsMissingRequiredField("access")
	default:
		return validation.NewErrBadRecordFieldValue("access", "unknown value")
	}
	//if strings.TrimSpace(p.Name) == "" {
	//	return validation.NewErrRecordIsMissingRequiredField("title")
	//}
	if l := len(p.Title); l > 100 {
		return validation.NewErrBadRecordFieldValue("title", "too long title (max 100): "+strconv.Itoa(l))
	}

	//log.Println("Validating environments...")
	if err := p.Environments.Validate(); err != nil {
		return fmt.Errorf("validation failed for project environments: %w", err)
	}

	//log.Println("Validating entities...")
	if err := p.Entities.Validate(); err != nil {
		return fmt.Errorf("validation failed for project entities: %w", err)
	}

	//log.Println("Validating DB models...")
	if err := p.DbModels.Validate(); err != nil {
		return fmt.Errorf("validation failed for project db models: %w", err)
	}
	//log.Println("Validating boards...")

	if err := p.Boards.Validate(); err != nil {
		return fmt.Errorf("validation failed for project boards: %w", err)
	}
	log.Println("Validating DB servers...")

	if err := p.DbDrivers.Validate(); err != nil {
		return fmt.Errorf("validation failed for project dbs: %w", err)
	}
	log.Println("Validating actions...")
	if err := p.Actions.Validate(); err != nil {
		return fmt.Errorf("validation failed for project actions: %w", err)
	}
	return nil
}

//type EnvDbServers []*EnvDbServer

// ProjectBrief hold project brief info (e.g. for list)
type ProjectBrief struct {
	Access string `json:"access" firestore:"access"` // e.g. private, protected, public
	ProjectItem
	Repository *ProjectRepository `json:"repository,omitempty" firestore:"repository,omitempty"`
}

// Validate returns error if not valid
func (v *ProjectBrief) Validate() error {
	if err := v.ValidateWithOptions(true); err != nil {
		return err
	}
	switch v.Access {
	case "":
		return validation.NewErrRecordIsMissingRequiredField("access")
	case "private", "protected", "public": // OK
		break
	default:
		return validation.NewErrBadRecordFieldValue("access", "unknown value: "+v.Access)
	}
	if v.Repository != nil {
		if err := v.Repository.Validate(); err != nil {
			return validation.NewErrBadRecordFieldValue("repository", err.Error())
		}
	}
	return nil
}

// ProjectSummary hold project summary - TODO: document why we need it in addition to ProjectFile
type ProjectSummary struct {
	ProjectFile
}

// ProjectRepository defines project repository
type ProjectRepository struct {
	Type      string `json:"type"` // e.g. "git"
	WebURL    string `json:"webURL"`
	ProjectID string `json:"projectId,omitempty"`
}

// Validate returns error if not valid
func (v *ProjectRepository) Validate() error {
	if v == nil {
		return nil
	}
	if v.Type == "" {
		return validation.NewErrRecordIsMissingRequiredField("type")
	}
	return nil
}

// ProjectFile defines what to storage to project file
type ProjectFile struct {
	Created *ProjectCreated `json:"created,omitempty" firestore:"created,omitempty"`
	ProjectItem
	Repository   *ProjectRepository  `json:"repository,omitempty" firestore:"repository,omitempty"`
	DbModels     []*ProjDbModelBrief `json:"dbModels,omitempty" firestore:"dbModels,omitempty"`
	Entities     []*ProjEntityBrief  `json:"entities,omitempty" firestore:"entities,omitempty"`
	Environments []*ProjEnvBrief     `json:"environments,omitempty" firestore:"environments,omitempty"`
}

// Validate returns error if not valid
func (v ProjectFile) Validate() error {
	// Do not check GetID or title as they can be nil for project
	//if err := v.ValidateWithOptions(); err != nil {
	//	return err
	//}
	if v.Created == nil {
		return validation.NewErrRecordIsMissingRequiredField("created")
	}
	//if v.Created.ByUsername == "" {
	//	return validation.NewErrRecordIsMissingRequiredField("created.byUsername")
	//}
	if v.Created.At.IsZero() {
		return validation.NewErrRecordIsMissingRequiredField("created.at")
	}
	switch v.Access {
	case "private", "protected", "public":
	default:
		return validation.NewErrBadRecordFieldValue("access", "expected 'private', 'protected' or 'public', got: "+v.Access)
	}
	for _, entity := range v.Entities {
		if err := entity.ValidateWithOptions(false); err != nil {
			return err
		}
	}
	for _, dbModel := range v.DbModels {
		if err := dbModel.ValidateWithOptions(false); err != nil {
			return err
		}
	}
	for _, env := range v.Environments {
		if err := env.ValidateWithOptions(false); err != nil {
			return err
		}
	}
	return nil
}

// ProjectCreated hold info about when and who created
type ProjectCreated struct {
	//ByName     string    `json:"byName,omitempty"`
	//ByUsername string    `json:"byUsername,omitempty"`
	At time.Time `json:"at" firestore:"at"`
}

package filestore

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/parallel"
	"github.com/datatug/datatug-core/pkg/storage"
)

// SaveProject saves project. It validates the whole project first - every
// query's content included - so a project that fails validation writes
// nothing. It is not atomic across the parts it then saves in parallel
// (the project file, entities, environments, DB models, boards and
// queries): when one part fails, the parts already saved stay written
// (review N5, pre-existing). Each query pair is still written as its own
// recoverable transaction.
func (s fsProjectStore) saveProjectUnlocked(ctx context.Context, project *datatug.Project) (err error) {
	//log.Println("Validating project for saving to: ", s.projectPath)
	if err = project.Validate(); err != nil {
		return fmt.Errorf("project validation failed: %w", err)
	}
	// Inspect existing recognized metadata while the project write lock is held,
	// before mkdir, README or any parallel project component can change state.
	if _, _, err = s.preflightProjectDataModels(project.DataModels); err != nil {
		return err
	}
	//log.Println("GetProjectStore is valid")
	if err = os.MkdirAll(s.projectPath, 0777); err != nil {
		return fmt.Errorf("failed to create datatug folder: %w", err)
	}
	if err = parallel.Run(
		func() (err error) {
			//log.Println("Saving project file...")
			if err = s.saveProjectFile(project); err != nil {
				return fmt.Errorf("failed to save project file: %w", err)
			}
			//log.Println("Saved project file.")
			return
		},
		func() (err error) {
			if len(project.Entities) > 0 {
				//log.Printf("Saving %v entities...\n", len(project.Entities))
				if err = s.SaveEntities(ctx, project.Entities); err != nil {
					return fmt.Errorf("failed to save entities: %w", err)
				}
				//log.Printf("Saved %v entities.\n", len(project.Entities))
			} else {
				log.Println("No entities to save.")
			}
			return nil
		},
		func() (err error) {
			if len(project.Environments) > 0 {
				log.Printf("Saving %v environments...\n", len(project.Environments))
				environmentStore := newFsEnvironmentsStore(s.projectPath)
				if err = environmentStore.SaveEnvironments(ctx, project.Environments); err != nil {
					return fmt.Errorf("failed to save environments: %w", err)
				}
				log.Printf("Saved %v environments.", len(project.Environments))
			} else {
				log.Println("No environments to save.")
			}
			return nil
		},
		func() (err error) {
			if len(project.DbModels) > 0 {
				log.Printf("Saving %v DB models...\n", len(project.DbModels))
				if err = s.SaveDbModels(ctx, project.DbModels); err != nil {
					return fmt.Errorf("failed to save DB models: %w", err)
				}
				log.Printf("Saved %v DB models.", len(project.DbModels))
			} else {
				log.Println("No DB models to save.")
			}
			return nil
		},
		func() (err error) {
			if len(project.Boards) > 0 {
				log.Printf("Saving %v boards...\n", len(project.Boards))
				if err = s.saveBoards(ctx, project.Boards); err != nil {
					return fmt.Errorf("failed to save boards: %w", err)
				}
				log.Printf("Saved %v boards.", len(project.Boards))
			} else {
				log.Println("No boards to save.")
			}
			return nil
		},
		func() (err error) {
			if project.Queries != nil {
				log.Println("Saving queries...")
				if err = s.saveQueriesTree(ctx, "", project.Queries); err != nil {
					return fmt.Errorf("failed to save queries: %w", err)
				}
				log.Println("Saved queries.")
			} else {
				log.Println("No queries to save.")
			}
			return nil
		},
		//func() (err error) {
		//	dbServersStore := newFsProjDbServersStore(s.projectPath)
		//	if err = dbServersStore.saveDbServers(ctx, project.DbServers, *project); err != nil {
		//		return fmt.Errorf("failed to save DB servers: %w", err)
		//	}
		//	return nil
		//},
	); err != nil {
		return err
	}
	return nil
}

func (s fsProjectStore) preflightProjectDataModels(incoming *datatug.ProjectDataModels) (datatug.ProjectFile, []byte, error) {
	var existing datatug.ProjectFile
	name := path.Join(s.projectPath, storage.ProjectSummaryFileName)
	if st, err := os.Lstat(name); err == nil && !st.Mode().IsRegular() {
		return existing, nil, fmt.Errorf("%w: project metadata is not a regular file", ErrEnvironmentRecoveryRequired)
	} else if err != nil && !os.IsNotExist(err) {
		return existing, nil, err
	}
	body, err := os.ReadFile(name)
	if err != nil && !os.IsNotExist(err) {
		return existing, nil, err
	}
	if len(body) > 0 {
		if err = json.Unmarshal(body, &existing); err != nil {
			return existing, nil, err
		}
	}
	candidate := datatug.ProjectFile{DataModels: incoming}
	if err = candidate.MergePreservedMetadata(existing); err != nil {
		return existing, nil, err
	}
	return existing, body, nil
}
func (s fsProjectStore) putProjectFile(projFile datatug.ProjectFile) error {
	existingFile, existing, err := s.preflightProjectDataModels(projFile.DataModels)
	if err != nil {
		return err
	}
	// Opaque members are merged below as RawMessages, preserving the
	// existing writer path and exact numeric tokens.
	if projFile.DataModels == nil {
		projFile.DataModels = existingFile.DataModels
	}
	if err := projFile.Validate(); err != nil {
		return fmt.Errorf("invalid project file: %w", err)
	}
	// Reuse the exact bytes validated by preflight rather than reopening the
	// summary between validation and merge. This retains the existing raw
	// opaque-member merge and avoids a second filesystem/JSON read.
	fields := map[string]json.RawMessage{}
	if len(existing) > 0 {
		_ = json.Unmarshal(existing, &fields)
	}
	body, err := json.Marshal(projFile)
	if err != nil {
		return err
	}
	var updated map[string]json.RawMessage
	// A successful ProjectFile marshal emits a JSON object; raw metadata was
	// validated by that marshal before it can enter this map.
	_ = json.Unmarshal(body, &updated)
	hasExtra := false
	for key := range fields {
		if _, ok := updated[key]; !ok {
			hasExtra = true
		}
	}
	if !hasExtra {
		return saveJSONFile(s.projectPath, s.projectPath, storage.ProjectSummaryFileName, projFile)
	}
	for key, value := range updated {
		fields[key] = value
	}
	// Both maps contain only RawMessages produced by successful JSON decoding.
	body, _ = json.Marshal(fields)
	return durableEnvironmentFile(path.Join(s.projectPath, storage.ProjectSummaryFileName), body, 0600)
}

//func projItemFileName(id, prefix string) string {
//	id = strings.ToLower(id)
//	if prefix == "" {
//		return fmt.Sprintf("%v.json", id)
//	}
//	return fmt.Sprintf("%v-%v.json", prefix, id)
//}

func (s fsProjectStore) saveProjectFile(project *datatug.Project) error {
	//var existingProject models.ProjectFile
	//if err := readJSONFile(projDirPath.Join(s.projectPath, DatatugFolder, ProjectSummaryFileName), false, &existingProject); err != nil {
	//	return err
	//}
	// The Title is persisted with the record: FsStore.GetProjects reads a
	// project's title back out of this very file (store.go:49), so dropping
	// it here wiped the title on every save - including a save that changed
	// nothing else - and left a brief that fails its own
	// datatug.ProjectBrief.Validate().
	projFile := datatug.ProjectFile{
		DataModels:       project.DataModels,
		EnvironmentOrder: project.EnvironmentOrder,
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: datatug.ProjItemBrief{
				ID:    project.ID,
				Title: project.Title,
			},
			Access: project.Access,
		},
		Repository: project.Repository,
		//UUID:    project.UUID,
		Created: project.Created,
	}
	//if existingProject.UUID == uuid.Nil {
	//	projFile.UUID = project.UUID
	//} else {
	//	projFile.UUID = existingProject.UUID
	//}
	//if existingProject.Access == "" {
	//	projFile.Access = project.Access
	//} else {
	//	projFile.Access = existingProject.Access
	//}
	//if existingProject.GetID == "" {
	//	projFile.GetID = project.GetID
	//} else {
	//	projFile.GetID = existingProject.GetID
	//}
	//for _, env := range project.Environments {
	//	envBrief := datatug.ProjEnvBrief{
	//		ProjectItem: env.ProjectItem,
	//		NumberOf: models.ProjEnvNumbers{
	//			DbServers: len(env.DbServers),
	//		},
	//	}
	//	for _, dbServer := range env.DbServers {
	//		envBrief.NumberOf.Catalogs += len(dbServer.Catalogs)
	//	}
	//	projFile.Environments = append(projFile.Environments, &envBrief)
	//}
	//for _, dbModel := range project.DbModels {
	//	brief := datatug.ProjDbModelBrief{
	//		ProjectItem: dbModel.ProjectItem,
	//		NumberOf: datatug.ProjDbModelNumbers{
	//			Schemas: len(dbModel.Schemas),
	//		},
	//	}
	//	for _, schema := range dbModel.Schemas {
	//		brief.NumberOf.Tables = len(schema.Tables)
	//		brief.NumberOf.Views = len(schema.Views)
	//	}
	//	projFile.DbModels = append(projFile.DbModels,
	//		&brief,
	//	)
	//}
	if err := s.writeProjectReadme(*project); err != nil {
		return fmt.Errorf("failed to write project doc file: %w", err)
	}
	if err := s.putProjectFile(projFile); err != nil {
		return fmt.Errorf("failed to save project file: %w", err)
	}
	return nil
}

//func (s fileSystemSaver) createStrFile() io.StringWriter {
//
//}
//
//func (s fileSystemSaver) getDatabasesReadme(project Project) (content bytes.Buffer, err error) {
//
//	_, err = w.WriteString("# DatabaseDiffs\n\n")
//	l, err := f.WriteString("Hello World")
//	if err != nil {
//		fmt.Println(err)
//		f.Close()
//		return
//	}
//	return err
//}
//
//func (s fileSystemSaver) writeDatabaseReadme(database *schemer.Database, dbDirPath string) (err error) {
//
//	return err
//}

//func saveToFile(tableDirPath, fileName string, data interface{ Validate() error }) func() error {
//	return func() (err error) {
//		if err = saveJSONFile(tableDirPath, fileName, data); err != nil {
//			return fmt.Errorf("failed to write json to file %v: %w", fileName, err)
//		}
//		return nil
//	}
//}
//
//type saveDbServerObjContext struct {
//	dirPath    string
//	catalog    string
//	plural     string
//	dbServer   datatug.ProjDbServer
//	repository *datatug.ProjectRepository
//}

func (s fsProjectStore) SaveProject(ctx context.Context, p *datatug.Project) error {
	if err := p.DataModels.Validate(); err != nil {
		return err
	}
	if err := p.Validate(); err != nil {
		return fmt.Errorf("project validation failed: %w", err)
	}
	if err := os.MkdirAll(s.projectPath, 0755); err != nil {
		return err
	}
	return withEnvironmentStoreLock(ctx, s.projectPath, func(ctx context.Context) error { return s.saveProjectUnlocked(ctx, p) })
}

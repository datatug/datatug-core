package filestore

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/parallel"
)

// SaveProject saves project
func (s fsProjectStore) SaveProject(ctx context.Context, project *datatug.Project) (err error) {
	log.Println("Validating project for saving to: ", s.projectPath)
	if err = project.Validate(); err != nil {
		return fmt.Errorf("project validation failed: %w", err)
	}
	log.Println("GetProjectStore is valid")
	if err = os.MkdirAll(path.Join(s.projectPath, DatatugFolder), 0777); err != nil {
		return fmt.Errorf("failed to create datatug folder: %w", err)
	}
	if err = parallel.Run(
		func() (err error) {
			log.Println("Saving project file...")
			if err = s.saveProjectFile(project); err != nil {
				return fmt.Errorf("failed to save project file: %w", err)
			}
			log.Println("Saved project file.")
			return
		},
		func() (err error) {
			if len(project.Entities) > 0 {
				log.Printf("Saving %v entities...\n", len(project.Entities))
				if err = s.saveEntities(ctx, project.Entities); err != nil {
					return fmt.Errorf("failed to save entities: %w", err)
				}
				log.Printf("Saved %v entities.\n", len(project.Entities))
			} else {
				log.Println("No entities to save.")
			}
			return nil
		},
		func() (err error) {
			if len(project.Environments) > 0 {
				log.Printf("Saving %v environments...\n", len(project.Environments))
				environmentStore := newFsEnvironmentsStore(s)
				if err = environmentStore.saveEnvironments(ctx, *project); err != nil {
					return fmt.Errorf("failed to save environments: %w", err)
				}
				log.Printf("Saved %v environments.", len(project.Environments))
			} else {
				log.Println("No environments to save.")
			}
			return nil
		},
		func() (err error) {
			log.Printf("Saving %v DB models...\n", len(project.DbModels))
			dbModelsStore := newFsDbModelsStore(s)
			if err = dbModelsStore.saveDbModels(project.DbModels); err != nil {
				return fmt.Errorf("failed to save DB models: %w", err)
			}
			log.Printf("Saved %v DB models.", len(project.DbModels))
			return nil
		},
		func() (err error) {
			if len(project.Boards) > 0 {
				log.Printf("Saving %v boards...\n", len(project.Boards))
				if err = newFsBoardsStore(s).saveBoards(ctx, project.Boards); err != nil {
					return fmt.Errorf("failed to save boards: %w", err)
				}
				log.Printf("Saved %v boards.", len(project.Boards))
			} else {
				log.Println("No boards to save.")
			}
			return nil
		},
		func() (err error) {
			dbServersStore := newFsDbServersStore(s)
			if err = dbServersStore.saveDbServers(ctx, project.DbServers, *project); err != nil {
				return fmt.Errorf("failed to save DB servers: %w", err)
			}
			return nil
		},
	); err != nil {
		return err
	}
	return nil
}

func (s fsProjectStore) putProjectFile(projFile datatug.ProjectFile) error {
	if err := projFile.Validate(); err != nil {
		return fmt.Errorf("invalid project file: %w", err)
	}
	return saveJSONFile(path.Join(s.projectPath, DatatugFolder), ProjectSummaryFileName, projFile)
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
	projFile := datatug.ProjectFile{
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: datatug.ProjItemBrief{
				ID: project.ID,
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
	//if existingProject.ID == "" {
	//	projFile.ID = project.ID
	//} else {
	//	projFile.ID = existingProject.ID
	//}
	for _, env := range project.Environments {
		envBrief := datatug.ProjEnvBrief{
			ProjectItem: env.ProjectItem,
			//NumberOf: models.ProjEnvNumbers{
			//	DbServers: len(env.DbServers),
			//},
		}
		//for _, dbServer := range env.DbServers {
		//	envBrief.NumberOf.Catalogs += len(dbServer.Catalogs)
		//}
		projFile.Environments = append(projFile.Environments, &envBrief)
	}
	for _, dbModel := range project.DbModels {
		brief := datatug.ProjDbModelBrief{
			ProjectItem: dbModel.ProjectItem,
			NumberOf: datatug.ProjDbModelNumbers{
				Schemas: len(dbModel.Schemas),
			},
		}
		for _, schema := range dbModel.Schemas {
			brief.NumberOf.Tables = len(schema.Tables)
			brief.NumberOf.Views = len(schema.Views)
		}
		projFile.DbModels = append(projFile.DbModels,
			&brief,
		)
	}
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

func saveToFile(tableDirPath, fileName string, data interface{ Validate() error }) func() error {
	return func() (err error) {
		if err = saveJSONFile(tableDirPath, fileName, data); err != nil {
			return fmt.Errorf("failed to write json to file %v: %w", fileName, err)
		}
		return nil
	}
}

type saveDbServerObjContext struct {
	dirPath    string
	catalog    string
	plural     string
	dbServer   datatug.ProjDbServer
	repository *datatug.ProjectRepository
}

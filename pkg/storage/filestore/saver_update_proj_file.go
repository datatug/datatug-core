package filestore

/*
func (s fsProjectStore) loadProjectFile() (v datatug.ProjectFile, err error) {
	return LoadProjectFile(s.projectPath)
}
*/

/*
func (s fsProjectStore) updateProjectFile(updater func(projFile *datatug.ProjectFile) error) error {
	s.projFileMutex.Lock()
	defer func() {
		s.projFileMutex.Unlock()
	}()
	projFile, err := s.loadProjectFile()
	if err != nil {
		return err
	}
	if err = updater(&projFile); err != nil {
		return err
	}
	if err = s.putProjectFile(projFile); err != nil {
		return err
	}
	return nil
}
*/

/*
func (s fileSystemSaver) updateProjectFileDeleteEntity(id string) (err error) {
	// Almost duplicates updateProjectFileDeleteBoard and other deletes
	projFile, err := s.loadProjectFile()
	if err != nil {
		return err
	}
	if len(projFile.Entities) == 0 {
		return
	}
	entities := make([]*models.ProjEntityBrief, len(projFile.Entities))
	for _, item := range projFile.Entities {
		if item.GetID == id {
			continue
		}
	}
	if len(projFile.Entities) > len(entities) {
		projFile.Entities = entities
		err = s.putProjectFile(projFile)
	}
	return err
}

func (s fileSystemSaver) updateProjectFileDeleteBoard(id string) (err error) {
	// Almost duplicates updateProjectFileDeleteEntity and other deletes
	projFile, err := s.loadProjectFile()
	if err != nil {
		return err
	}
	if len(projFile.Entities) == 0 {
		return
	}
	items := make([]*models.ProjBoardBrief, len(projFile.Boards))
	for _, item := range projFile.Boards {
		if item.GetID == id {
			continue
		}
	}
	if len(projFile.Boards) > len(items) {
		projFile.Boards = items
		err = s.putProjectFile(projFile)
	}
	return err
}
*/

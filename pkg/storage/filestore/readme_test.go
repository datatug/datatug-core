package filestore

import (
	"errors"
	"io"
	"os"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestReadme(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_test_readme")
	assert.NoError(t, err)
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	t.Run("saveReadme_Success", func(t *testing.T) {
		err := saveReadme(tmpDir, func(w io.Writer) error {
			_, err := w.Write([]byte("test readme"))
			return err
		})
		assert.NoError(t, err)
		assert.FileExists(t, path.Join(tmpDir, "README.md"))
	})

	t.Run("saveReadme_Error", func(t *testing.T) {
		err := saveReadme("/non-existent-path", func(w io.Writer) error {
			return nil
		})
		assert.Error(t, err)
	})

	t.Run("writeProjectReadme", func(t *testing.T) {
		projectID := "test_project"
		projectPath := path.Join(tmpDir, projectID)
		err := os.MkdirAll(path.Join(projectPath, DatatugFolder), 0755)
		assert.NoError(t, err)

		store := newFsProjectStore(projectID, projectPath)
		project := datatug.Project{
			ProjectItem: datatug.ProjectItem{
				ProjItemBrief: datatug.ProjItemBrief{
					ID: projectID,
				},
			},
		}
		err = store.writeProjectReadme(project)
		assert.NoError(t, err)
		assert.FileExists(t, path.Join(projectPath, DatatugFolder, "README.md"))
	})
}

func TestSaveReadme_SaverError(t *testing.T) {
	tmpDir, _ := os.MkdirTemp("", "datatug_test_readme_err")
	defer os.RemoveAll(tmpDir)

	err := saveReadme(tmpDir, func(w io.Writer) error {
		return errors.New("saver error")
	})
	assert.Error(t, err)
	assert.Equal(t, "saver error", err.Error())
}

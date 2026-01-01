package dtprojcreator

import (
	"context"
	"errors"
	"io"
	"path"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestCreator_WriteFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockStorage(ctrl)
	ctx := context.Background()
	projPath := "test-proj"
	c := creator{
		ctx:      ctx,
		s:        mockStorage,
		projPath: projPath,
	}

	fileName := "test.txt"
	content := []byte("test content")
	expectedPath := path.Join(projPath, fileName)

	mockStorage.EXPECT().
		WriteFile(ctx, expectedPath, gomock.Any()).
		DoAndReturn(func(ctx context.Context, filePath string, reader io.Reader) error {
			actualContent, err := io.ReadAll(reader)
			assert.NoError(t, err)
			assert.Equal(t, content, actualContent)
			return nil
		})

	err := c.writeFile(fileName, content)
	assert.NoError(t, err)
}

func TestCreator_AddProjectToRootRepoFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockStorage(ctrl)
	ctx := context.Background()
	projPath := "test-proj"
	c := creator{
		ctx:      ctx,
		s:        mockStorage,
		projPath: projPath,
	}

	expectedPath := path.Join(projPath, storage.RepoRootDataTugFileName)

	mockStorage.EXPECT().
		WriteFile(ctx, expectedPath, gomock.Any()).
		Return(nil)

	err := c.addProjectToRootRepoFile()
	assert.NoError(t, err)
}

func TestCreator_CreateProjectSummaryFile(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockStorage(ctrl)
	ctx := context.Background()
	projPath := "test-proj"
	p := &datatug.Project{
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: datatug.ProjItemBrief{
				ID: "test-id",
			},
		},
	}
	c := creator{
		ctx:      ctx,
		s:        mockStorage,
		projPath: projPath,
		p:        p,
	}

	expectedPath := path.Join(projPath, storage.ProjectSummaryFileName)

	mockStorage.EXPECT().
		WriteFile(ctx, expectedPath, gomock.Any()).
		Return(nil)

	err := c.createProjectSummaryFile()
	assert.NoError(t, err)
}

func TestCreator_AddProjectToRootRepoFile_Error(t *testing.T) {
	oldYamlMarshal := yamlMarshal
	defer func() { yamlMarshal = oldYamlMarshal }()
	yamlMarshal = func(v interface{}) ([]byte, error) {
		return nil, errors.New("marshal error")
	}

	c := creator{}
	err := c.addProjectToRootRepoFile()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "marshal error")
}

func TestCreator_CreateProjectSummaryFile_Error(t *testing.T) {
	oldYamlMarshal := yamlMarshal
	defer func() { yamlMarshal = oldYamlMarshal }()
	yamlMarshal = func(v interface{}) ([]byte, error) {
		return nil, errors.New("marshal error")
	}

	c := creator{p: &datatug.Project{}}
	err := c.createProjectSummaryFile()
	assert.Error(t, err)
	assert.Equal(t, "marshal error", err.Error())
}

func TestCreator_CreateProjectReadmeMD(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStorage := storage.NewMockStorage(ctrl)
	ctx := context.Background()
	projPath := "test-proj"
	c := creator{
		ctx:      ctx,
		s:        mockStorage,
		projPath: projPath,
	}

	expectedPath := path.Join(projPath, "README.md")

	mockStorage.EXPECT().
		WriteFile(ctx, expectedPath, gomock.Any()).
		DoAndReturn(func(ctx context.Context, filePath string, reader io.Reader) error {
			content, err := io.ReadAll(reader)
			assert.NoError(t, err)
			assert.Equal(t, []byte(ProjectReadmeContent), content)
			return nil
		})

	err := c.createProjectReadmeMD()
	assert.NoError(t, err)
}

func TestCreateProjectFiles(t *testing.T) {
	ctx := context.Background()
	projPath := "test-proj"
	p := &datatug.Project{
		ProjectItem: datatug.ProjectItem{
			ProjItemBrief: datatug.ProjItemBrief{
				ID: "test-id",
			},
		},
	}

	t.Run("success", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockStorage := storage.NewMockStorage(ctrl)

		mockStorage.EXPECT().WriteFile(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(nil)
		reportStatus := func(step string, status string) {}
		err := CreateProjectFiles(ctx, p, projPath, mockStorage, reportStatus)
		assert.NoError(t, err)
	})

	t.Run("error", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockStorage := storage.NewMockStorage(ctrl)

		mockStorage.EXPECT().WriteFile(ctx, gomock.Any(), gomock.Any()).AnyTimes().Return(errors.New("write error"))
		reportStatus := func(step string, status string) {}
		err := CreateProjectFiles(ctx, p, projPath, mockStorage, reportStatus)
		if assert.Error(t, err) {
			assert.Contains(t, err.Error(), "write error")
		}
	})
}

package datatug

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockProjectLoader struct {
	ProjectLoader
}

func (m mockProjectLoader) LoadEnvironments(_ context.Context) (Environments, error) {
	return Environments{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}}}}, nil
}

func (m mockProjectLoader) LoadDbServers(_ context.Context) (ProjDbServers, error) {
	return ProjDbServers{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "s1"}}}}, nil
}

func TestProject_GetEnvironments(t *testing.T) {
	p := NewProject("p1", &mockProjectLoader{})
	envs, err := p.GetEnvironments(context.Background())
	assert.NoError(t, err)
	assert.Len(t, envs, 1)
	assert.Equal(t, "e1", envs[0].ID)
}

func TestProject_GetDbServers(t *testing.T) {
	p := NewProject("p1", &mockProjectLoader{})
	servers, err := p.GetDbServers(context.Background())
	assert.NoError(t, err)
	assert.Len(t, servers, 1)
	assert.Equal(t, "s1", servers[0].ID)
}

func TestProject_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		p := Project{
			ProjectItem: ProjectItem{Access: "public", ProjItemBrief: ProjItemBrief{Title: "T"}},
		}
		assert.NoError(t, p.Validate())
	})
	t.Run("missing_access", func(t *testing.T) {
		p := Project{}
		assert.Error(t, p.Validate())
	})
	t.Run("unknown_access", func(t *testing.T) {
		p := Project{ProjectItem: ProjectItem{Access: "unknown"}}
		assert.Error(t, p.Validate())
	})
	t.Run("too_long_title", func(t *testing.T) {
		title := ""
		for i := 0; i < 101; i++ {
			title += "a"
		}
		p := Project{ProjectItem: ProjectItem{Access: "public", ProjItemBrief: ProjItemBrief{Title: title}}}
		assert.Error(t, p.Validate())
	})
	t.Run("invalid_environments", func(t *testing.T) {
		p := Project{
			ProjectItem:  ProjectItem{Access: "public"},
			Environments: Environments{{}},
		}
		assert.Error(t, p.Validate())
	})
	t.Run("invalid_entities", func(t *testing.T) {
		p := Project{
			ProjectItem: ProjectItem{Access: "public"},
			Entities:    Entities{{ProjEntityBrief: ProjEntityBrief{ProjItemBrief: ProjItemBrief{ID: ""}}}},
		}
		assert.Error(t, p.Validate())
	})
	t.Run("invalid_db_models", func(t *testing.T) {
		p := Project{
			ProjectItem: ProjectItem{Access: "public"},
			DbModels:    DbModels{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: ""}}}},
		}
		assert.Error(t, p.Validate())
	})
	t.Run("invalid_boards", func(t *testing.T) {
		p := Project{
			ProjectItem: ProjectItem{Access: "public"},
			Boards:      Boards{{ProjBoardBrief: ProjBoardBrief{ProjItemBrief: ProjItemBrief{ID: ""}}}},
		}
		assert.Error(t, p.Validate())
	})
	t.Run("invalid_db_servers", func(t *testing.T) {
		p := Project{
			ProjectItem: ProjectItem{Access: "public"},
			DbServers:   ProjDbServers{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: ""}}}},
		}
		assert.Error(t, p.Validate())
	})
	t.Run("invalid_actions", func(t *testing.T) {
		p := Project{
			ProjectItem: ProjectItem{Access: "public"},
			Actions:     Actions{{Type: ""}},
		}
		assert.Error(t, p.Validate())
	})
}

func TestProjectBrief_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := &ProjectBrief{Access: "public", ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "p1", Title: "T"}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("missing_access", func(t *testing.T) {
		v := &ProjectBrief{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "p1", Title: "T"}}}
		assert.Error(t, v.Validate())
	})
	t.Run("unknown_access", func(t *testing.T) {
		v := &ProjectBrief{Access: "unknown", ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "p1", Title: "T"}}}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_repository", func(t *testing.T) {
		v := &ProjectBrief{Access: "public", ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "p1", Title: "T"}}, Repository: &ProjectRepository{}}
		assert.Error(t, v.Validate())
	})
}

func TestProjectRepository_Validate(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		var v *ProjectRepository
		assert.NoError(t, v.Validate())
	})
	t.Run("valid", func(t *testing.T) {
		v := &ProjectRepository{Type: "git"}
		assert.NoError(t, v.Validate())
	})
	t.Run("missing_type", func(t *testing.T) {
		v := &ProjectRepository{}
		assert.Error(t, v.Validate())
	})
}

func TestProjectFile_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := ProjectFile{
			Created:     &ProjectCreated{At: time.Now()},
			ProjectItem: ProjectItem{Access: "public"},
		}
		assert.NoError(t, v.Validate())
	})
	t.Run("missing_created", func(t *testing.T) {
		v := ProjectFile{ProjectItem: ProjectItem{Access: "public"}}
		assert.Error(t, v.Validate())
	})
	t.Run("zero_created_at", func(t *testing.T) {
		v := ProjectFile{Created: &ProjectCreated{}, ProjectItem: ProjectItem{Access: "public"}}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_access", func(t *testing.T) {
		v := ProjectFile{Created: &ProjectCreated{At: time.Now()}, ProjectItem: ProjectItem{Access: "unknown"}}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_entity", func(t *testing.T) {
		v := ProjectFile{
			Created:     &ProjectCreated{At: time.Now()},
			ProjectItem: ProjectItem{Access: "public"},
			Entities:    []*ProjEntityBrief{{ProjItemBrief: ProjItemBrief{ID: ""}}},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_db_model", func(t *testing.T) {
		v := ProjectFile{
			Created:     &ProjectCreated{At: time.Now()},
			ProjectItem: ProjectItem{Access: "public"},
			DbModels:    []*ProjDbModelBrief{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: ""}}}},
		}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_env", func(t *testing.T) {
		v := ProjectFile{
			Created:      &ProjectCreated{At: time.Now()},
			ProjectItem:  ProjectItem{Access: "public"},
			Environments: []*ProjEnvBrief{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: ""}}}},
		}
		assert.Error(t, v.Validate())
	})
}

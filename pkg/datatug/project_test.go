package datatug

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockProjectLoader struct {
	ProjectStore
	errLoadEnvironments error
	envs                Environments
}

func (m mockProjectLoader) LoadEnvironments(_ context.Context, o ...StoreOption) (Environments, error) {
	_ = GetStoreOptions(o...)
	if m.envs != nil || m.errLoadEnvironments != nil {
		return m.envs, m.errLoadEnvironments
	}
	return Environments{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}}}}, nil
}

func (m mockProjectLoader) LoadProjDbServers(_ context.Context, o ...StoreOption) (ProjDbServers, error) {
	_ = GetStoreOptions(o...)
	return ProjDbServers{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "s1"}}}}, nil
}

func TestProject_GetEnvironments(t *testing.T) {
	ctx := context.Background()
	t.Run("success", func(t *testing.T) {
		p := NewProject("p1", func(p *Project) ProjectStore { return &mockProjectLoader{} })
		envs, err := p.GetEnvironments(ctx)
		assert.NoError(t, err)
		assert.Len(t, envs, 1)
		assert.Equal(t, "e1", envs[0].ID)
	})
	t.Run("nil_environments_success", func(t *testing.T) {
		envs := Environments{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "env1"}}}}
		p := Project{store: mockProjectLoader{envs: envs}}
		res, err := p.GetEnvironments(ctx)
		assert.NoError(t, err)
		assert.Equal(t, envs, res)
		assert.Equal(t, envs, p.Environments)
	})
	t.Run("nil_environments_error", func(t *testing.T) {
		p := Project{store: mockProjectLoader{errLoadEnvironments: errors.New("test error")}}
		_, err := p.GetEnvironments(ctx)
		assert.Error(t, err)
	})
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
			Entities:    Entities{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: ""}}}},
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
			Boards:      Boards{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: ""}}}},
		}
		assert.Error(t, p.Validate())
	})
	t.Run("invalid_db_servers", func(t *testing.T) {
		p := Project{
			ProjectItem: ProjectItem{Access: "public"},
		}
		p.DBs = ProjDbDrivers{
			{
				ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "sqlite"}},
				Servers:     ProjDbServers{{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: ""}}}},
			},
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

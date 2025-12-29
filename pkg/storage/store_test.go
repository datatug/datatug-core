package storage

import (
	"context"
	"testing"

	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/stretchr/testify/assert"
)

func TestNoOpStore(t *testing.T) {
	s := NewNoOpStore()
	ctx := context.Background()

	assert.Panics(t, func() {
		_, _ = s.CreateProject(ctx, dto.CreateProjectRequest{})
	})
	assert.Panics(t, func() {
		_, _ = s.GetProjects(ctx)
	})
	assert.Panics(t, func() {
		_ = s.GetProjectStore("p1")
	})
	assert.Panics(t, func() {
		_ = s.DeleteProject(ctx, "p1")
	})
}

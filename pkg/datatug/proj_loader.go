package datatug

import "context"

type ProjectLoader interface {
	LoadProject(ctx context.Context, projectID string) (project *Project, err error)
}

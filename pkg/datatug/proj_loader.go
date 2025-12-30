package datatug

import "context"

type ProjectsLoader interface {
	LoadProject(ctx context.Context, projectID string) (project *Project, err error)
}

type EnvironmentLoader interface {
	LoadDbServers(ctx context.Context, envID string) ([]*EnvDbServers, error)
}

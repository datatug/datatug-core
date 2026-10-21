package dalgostore

import (
	"context"
	"github.com/datatug/datatug-core/pkg/datatug"
)

func (s *ProjectStore) LoadEnvironment(ctx context.Context, id string, _ ...datatug.StoreOption) (*datatug.Environment, error) {
	v := new(datatug.Environment)
	if err := s.loadAsset(ctx, "environments", id, v); err != nil {
		return nil, err
	}
	return v, nil
}
func (s *ProjectStore) LoadEnvironmentSummary(ctx context.Context, id string) (*datatug.EnvironmentSummary, error) {
	v, err := s.LoadEnvironment(ctx, id)
	if err != nil {
		return nil, err
	}
	return &datatug.EnvironmentSummary{ProjectItem: v.ProjectItem, Servers: v.DbServers}, nil
}
func (s *ProjectStore) LoadEnvironments(ctx context.Context, _ ...datatug.StoreOption) (datatug.Environments, error) {
	values, err := s.listAssets(ctx, "environments", func() any { return new(datatug.Environment) })
	if err != nil {
		return nil, err
	}
	out := make(datatug.Environments, len(values))
	for i, v := range values {
		out[i] = v.(*datatug.Environment)
	}
	return out, nil
}
func (s *ProjectStore) SaveEnvironment(ctx context.Context, v *datatug.Environment) error {
	if v == nil {
		return ErrAssetInvalid
	}
	return s.saveAsset(ctx, "environments", v.ID, v)
}
func (s *ProjectStore) SaveEnvironments(ctx context.Context, values datatug.Environments) error {
	for _, v := range values {
		if err := s.SaveEnvironment(ctx, v); err != nil {
			return err
		}
	}
	return nil
}
func (s *ProjectStore) DeleteEnvironment(ctx context.Context, id string) error {
	return s.deleteAsset(ctx, "environments", id)
}

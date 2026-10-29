package dalgostore

import (
	"context"
	"github.com/datatug/datatug-core/pkg/datatug"
)

var summarizeStoredEnvironment = datatug.Environment.Summary

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
	summary, err := summarizeStoredEnvironment(*v)
	if err != nil {
		return nil, err
	}
	return &summary, nil
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
	saved := s.environmentOrder
	if s.itemRoot == nil {
		file, loadErr := s.LoadProjectFile(ctx)
		if loadErr != nil {
			return nil, loadErr
		}
		saved = file.EnvironmentOrder
	}
	ids := make([]string, len(out))
	byID := make(map[string]*datatug.Environment, len(out))
	for i, environment := range out {
		ids[i] = environment.ID
		byID[environment.ID] = environment
	}
	order, _, err := datatug.ResolveEnvironmentOrder(ids, saved)
	if err != nil {
		return nil, err
	}
	for i, id := range order {
		out[i] = byID[id]
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

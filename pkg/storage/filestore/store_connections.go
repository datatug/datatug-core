package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/datatug/datatug-core/internal/jsonstrict"
	"github.com/datatug/datatug-core/pkg/datatug"
)

const (
	maxConnectionCatalogs    = 32
	maxConnectionCatalogSize = 1 << 20
)

var (
	_ datatug.ProjectConnectionsReader      = (*fsProjectStore)(nil)
	_ datatug.EnvironmentConnectionResolver = (*fsProjectStore)(nil)
)

// LoadProjectConnections reads project declarations without using their remote
// URLs as source bindings. All returned fields are safe, validated metadata.
func (s fsProjectStore) LoadProjectConnections(ctx context.Context) (datatug.ProjectConnections, error) {
	var all datatug.ProjectConnections
	if err := ctx.Err(); err != nil {
		return all, err
	}
	dir := filepath.Join(s.projectPath, "connections")
	info, err := os.Lstat(dir)
	if err != nil {
		return all, err
	}
	if !info.IsDir() {
		return all, errors.New("connection catalog path is not a directory")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return all, err
	}
	seen := make(map[string]struct{})
	files := 0
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return datatug.ProjectConnections{}, err
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		files++
		if files > maxConnectionCatalogs {
			return datatug.ProjectConnections{}, errors.New("too many connection catalogs")
		}
		data, exists, err := readRegularFileCapped(filepath.Join(dir, name), maxConnectionCatalogSize)
		if err != nil {
			return datatug.ProjectConnections{}, fmt.Errorf("read connection catalog %q: %w", name, err)
		}
		if !exists {
			return datatug.ProjectConnections{}, fmt.Errorf("connection catalog %q disappeared", name)
		}
		var catalog datatug.ProjectConnections
		if err := jsonstrict.CheckNoDuplicateKeysFor(data, reflect.TypeOf(catalog)); err != nil {
			return datatug.ProjectConnections{}, fmt.Errorf("invalid connection catalog %q: %w", name, err)
		}
		if err := json.Unmarshal(data, &catalog); err != nil {
			return datatug.ProjectConnections{}, fmt.Errorf("invalid connection catalog %q: %w", name, err)
		}
		if err := catalog.Validate(); err != nil {
			return datatug.ProjectConnections{}, fmt.Errorf("invalid connection catalog %q: %w", name, err)
		}
		check := func(id string) error {
			if _, exists := seen[id]; exists {
				return &datatug.ConnectionResolutionError{Kind: datatug.ErrAmbiguousConnection, ConnectionID: id}
			}
			seen[id] = struct{}{}
			return nil
		}
		for _, connection := range catalog.Connections {
			if err := check(connection.ID); err != nil {
				return datatug.ProjectConnections{}, err
			}
		}
		for _, connection := range catalog.BigQueryEditions {
			if err := check(connection.ID); err != nil {
				return datatug.ProjectConnections{}, err
			}
		}
		for _, plan := range catalog.BigQueryPlans {
			if err := check(plan.ID); err != nil {
				return datatug.ProjectConnections{}, err
			}
		}
		all.Format = datatug.ConnectionCatalogFormat
		all.Connections = append(all.Connections, catalog.Connections...)
		all.BigQueryEditions = append(all.BigQueryEditions, catalog.BigQueryEditions...)
		all.BigQueryPlans = append(all.BigQueryPlans, catalog.BigQueryPlans...)
	}
	if files == 0 {
		return datatug.ProjectConnections{}, os.ErrNotExist
	}
	return all, nil
}

func (s fsProjectStore) ResolveEnvironmentConnection(ctx context.Context, environmentID, connectionID string) (datatug.EditionConnection, error) {
	var zero datatug.EditionConnection
	if err := ctx.Err(); err != nil {
		return zero, err
	}
	if !datatug.ValidConnectionID(environmentID) || !datatug.ValidConnectionID(connectionID) {
		return zero, errors.New("invalid environment or connection ID")
	}
	catalog, err := s.LoadProjectConnections(ctx)
	if errors.Is(err, os.ErrNotExist) {
		return zero, &datatug.ConnectionResolutionError{Kind: datatug.ErrConnectionNotFound, EnvironmentID: environmentID, ConnectionID: connectionID}
	}
	if err != nil {
		return zero, err
	}
	var selected *datatug.EditionConnection
	for i := range catalog.Connections {
		if catalog.Connections[i].ID == connectionID {
			selected = &catalog.Connections[i]
			break
		}
	}
	for i := range catalog.BigQueryEditions {
		if catalog.BigQueryEditions[i].ID == connectionID {
			selected = &catalog.BigQueryEditions[i]
			break
		}
	}
	var plan *datatug.ConnectionPlan
	for i := range catalog.BigQueryPlans {
		if catalog.BigQueryPlans[i].ID == connectionID {
			plan = &catalog.BigQueryPlans[i]
			break
		}
	}
	if selected == nil && plan == nil {
		return zero, &datatug.ConnectionResolutionError{Kind: datatug.ErrConnectionNotFound, EnvironmentID: environmentID, ConnectionID: connectionID}
	}
	if err := s.validateConnectionEnvironmentFiles(environmentID); err != nil {
		return zero, err
	}
	env, err := s.LoadEnvironment(ctx, environmentID)
	if err != nil {
		return zero, err
	}
	if err := env.Validate(); err != nil {
		return zero, fmt.Errorf("invalid environment: %w", err)
	}
	member := false
	for _, id := range env.EditionConnections {
		if id == connectionID {
			member = true
			break
		}
	}
	if !member {
		return zero, &datatug.ConnectionResolutionError{Kind: datatug.ErrConnectionNotInEnvironment, EnvironmentID: environmentID, ConnectionID: connectionID}
	}
	if selected != nil {
		connection := *selected
		if connection.Storage != "bigquery" && len(connection.Environments) == 0 {
			return zero, &datatug.ConnectionResolutionError{Kind: datatug.ErrConnectionNotInEnvironment, EnvironmentID: environmentID, ConnectionID: connectionID}
		}
		if len(connection.Environments) != 0 {
			declared := false
			for _, id := range connection.Environments {
				if id == environmentID {
					declared = true
					break
				}
			}
			if !declared {
				return zero, &datatug.ConnectionResolutionError{Kind: datatug.ErrConnectionNotInEnvironment, EnvironmentID: environmentID, ConnectionID: connectionID}
			}
		}
		if !connection.ReadyForBinding() {
			// A pending hosted endpoint may still have a separately configured,
			// explicit local/direct source. Preserve safe identity metadata for
			// that caller while keeping implicit opening a typed error.
			return connection, &datatug.ConnectionResolutionError{Kind: datatug.ErrConnectionNotReady, EnvironmentID: environmentID, ConnectionID: connectionID, Readiness: connection.Readiness}
		}
		return connection, nil
	}
	if plan != nil {
		return zero, &datatug.ConnectionResolutionError{Kind: datatug.ErrConnectionNotReady, EnvironmentID: environmentID, ConnectionID: connectionID, Readiness: plan.Readiness}
	}
	return zero, errors.New("connection resolver state is inconsistent")
}

// The older environment loader accepts conventional JSON, including duplicate
// keys. Resolution must reject that ambiguity before relying on membership.
func (s fsProjectStore) validateConnectionEnvironmentFiles(environmentID string) error {
	dir := filepath.Join(s.projectPath, "environments", environmentID)
	info, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("environment path is not a directory")
	}
	for _, name := range []string{environmentID + ".env.json", "environment-summary.json"} {
		data, exists, err := readRegularFileCapped(filepath.Join(dir, name), maxConnectionCatalogSize)
		if err != nil {
			return fmt.Errorf("read environment %q: %w", environmentID, err)
		}
		if exists {
			if err := jsonstrict.CheckNoDuplicateKeysFor(data, reflect.TypeOf(datatug.Environment{})); err != nil {
				return fmt.Errorf("ambiguous environment %q: %w", environmentID, err)
			}
		}
	}
	return nil
}

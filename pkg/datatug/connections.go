package datatug

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ConnectionCatalogFormat is the supported project connection manifest format.
// A project may keep several such manifests under connections/*.json.
const ConnectionCatalogFormat = "datatug-demo-connections/v1"

var (
	ErrConnectionNotFound         = errors.New("connection not found")
	ErrConnectionNotInEnvironment = errors.New("connection is not in environment")
	ErrConnectionNotReady         = errors.New("connection is not ready")
	ErrAmbiguousConnection        = errors.New("ambiguous connection catalog")
	ErrUnsupportedConnectionFile  = errors.New("unsupported connection catalog")
)

// ConnectionResolutionError carries only safe IDs and readiness metadata.
// Informational source URLs and any credentials are never included in errors.
type ConnectionResolutionError struct {
	Kind          error
	EnvironmentID string
	ConnectionID  string
	Readiness     string
}

func (e *ConnectionResolutionError) Error() string {
	location := e.ConnectionID
	if e.EnvironmentID != "" {
		location += " in " + e.EnvironmentID
	}
	if e.Readiness != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Kind, location, e.Readiness)
	}
	return fmt.Sprintf("%s: %s", e.Kind, location)
}

func (e *ConnectionResolutionError) Unwrap() error { return e.Kind }

// EditionConnection is a project-owned declaration, not a live DALgo source.
// Remote source/descriptor/manifest URLs in the on-disk v1 format are omitted
// intentionally: callers must supply an independently configured source binding.
type EditionConnection struct {
	ID              string   `json:"id"`
	Dataset         string   `json:"dataset"`
	Storage         string   `json:"storage"`
	Tags            []string `json:"tags,omitempty"`
	Environments    []string `json:"environments,omitempty"`
	Readiness       string   `json:"readiness"`
	Query           string   `json:"query,omitempty"`
	SourceProjectID string   `json:"sourceProjectId,omitempty"`
	DatasetID       string   `json:"datasetId,omitempty"`
	Location        string   `json:"location,omitempty"`
}

// ConnectionPlan reserves an ID for a proposed connection but cannot resolve
// to a usable source even if an environment mistakenly references it.
type ConnectionPlan struct {
	ID        string `json:"id"`
	Readiness string `json:"readiness"`
}

type ProjectConnections struct {
	Format           string              `json:"format"`
	Connections      []EditionConnection `json:"connections,omitempty"`
	BigQueryEditions []EditionConnection `json:"bigQueryEditions,omitempty"`
	BigQueryPlans    []ConnectionPlan    `json:"bigQueryPlans,omitempty"`
}

// ProjectConnectionsReader is an optional ProjectStore capability. The base
// ProjectStore interface and its existing implementations remain unchanged.
type ProjectConnectionsReader interface {
	LoadProjectConnections(context.Context) (ProjectConnections, error)
}

// EnvironmentConnectionResolver is an optional ProjectStore capability. The
// resolver checks both the environment's editionConnections list and the
// declaration's environments list when one is present. Pending declarations
// return safe identity metadata with ErrConnectionNotReady; callers may use
// an independently validated explicit source binding. It never opens a DB.
type EnvironmentConnectionResolver interface {
	ResolveEnvironmentConnection(ctx context.Context, environmentID, connectionID string) (EditionConnection, error)
}

var connectionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
var connectionTokenPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9._-]{0,127}$`)

func ValidConnectionID(id string) bool { return connectionIDPattern.MatchString(id) }

func validConnectionLabel(value string) bool {
	if value == "" || len(value) > 256 || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}

func uniqueConnectionIDs(ids []string) error {
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if !ValidConnectionID(id) {
			return fmt.Errorf("invalid connection ID")
		}
		if seen[id] {
			return &ConnectionResolutionError{Kind: ErrAmbiguousConnection, ConnectionID: id}
		}
		seen[id] = true
	}
	return nil
}

func (c EditionConnection) Validate() error {
	if !ValidConnectionID(c.ID) || !validConnectionLabel(c.Dataset) || !connectionTokenPattern.MatchString(c.Storage) || !connectionTokenPattern.MatchString(c.Readiness) {
		return errors.New("invalid connection declaration")
	}
	if c.Query != "" && !connectionTokenPattern.MatchString(c.Query) {
		return errors.New("invalid connection query capability")
	}
	if err := uniqueConnectionIDs(c.Environments); err != nil {
		return err
	}
	for _, tag := range c.Tags {
		if !connectionTokenPattern.MatchString(tag) {
			return errors.New("invalid connection tag")
		}
	}
	if c.SourceProjectID != "" || c.DatasetID != "" || c.Location != "" {
		if c.Storage != "bigquery" || !connectionTokenPattern.MatchString(c.SourceProjectID) || !connectionTokenPattern.MatchString(c.DatasetID) || !connectionTokenPattern.MatchString(c.Location) {
			return errors.New("invalid structured connection location")
		}
	}
	return nil
}

// ReadyForBinding says only whether the declaration is in a known actionable
// state. It does not say a local driver, credentials, or an execution project
// is configured; callers must establish those separately.
func (c EditionConnection) ReadyForBinding() bool {
	switch c.Readiness {
	case "public-api", "hosted-repository", "public-read-user-project-required":
		return true
	default:
		return false
	}
}

func (c ProjectConnections) Validate() error {
	if c.Format != ConnectionCatalogFormat {
		return ErrUnsupportedConnectionFile
	}
	seen := make(map[string]bool, len(c.Connections)+len(c.BigQueryEditions)+len(c.BigQueryPlans))
	checkID := func(id string) error {
		if !ValidConnectionID(id) {
			return errors.New("invalid connection ID")
		}
		if seen[id] {
			return &ConnectionResolutionError{Kind: ErrAmbiguousConnection, ConnectionID: id}
		}
		seen[id] = true
		return nil
	}
	for _, connection := range c.Connections {
		if err := checkID(connection.ID); err != nil {
			return err
		}
		if err := connection.Validate(); err != nil {
			return err
		}
	}
	for _, connection := range c.BigQueryEditions {
		if err := checkID(connection.ID); err != nil {
			return err
		}
		if err := connection.Validate(); err != nil {
			return err
		}
		if connection.Storage != "bigquery" || connection.SourceProjectID == "" || connection.DatasetID == "" || connection.Location == "" {
			return errors.New("invalid BigQuery edition location")
		}
	}
	for _, plan := range c.BigQueryPlans {
		if err := checkID(plan.ID); err != nil {
			return err
		}
		if !connectionTokenPattern.MatchString(plan.Readiness) {
			return errors.New("invalid connection plan")
		}
	}
	return nil
}

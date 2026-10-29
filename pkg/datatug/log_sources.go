package datatug

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/datatug/datatug-core/internal/jsonstrict"
)

// Log sources own a separate namespace in connections/log-sources.json. The
// contract is project-log-sources.md and Apps' parseLogSources; these descriptors
// name OVDB records and presentation roles, never EditionConnection IDs.
const ProjectLogSourcesFile = "log-sources.json"
const ProjectLogSourcesFormat = "datatug-log-sources/v1"

type ProjectLogSources struct {
	Format  string             `json:"format"`
	Sources []ProjectLogSource `json:"sources"`
}
type ProjectLogSource struct {
	ID          string                 `json:"id"`
	Title       string                 `json:"title"`
	Database    string                 `json:"database"`
	Synthetic   bool                   `json:"synthetic,omitempty"`
	Collections []ProjectLogCollection `json:"collections"`
	Fields      map[string]string      `json:"fields"`
	Identifiers []ProjectLogIdentifier `json:"identifiers,omitempty"`
	Window      ProjectLogWindow       `json:"window"`
	Records     []ProjectLogRecord     `json:"records,omitempty"`
}
type ProjectLogCollection struct {
	ID     string            `json:"id"`
	Title  string            `json:"title"`
	Fields map[string]string `json:"fields,omitempty"`
}
type ProjectLogIdentifier struct {
	Field string `json:"field"`
	Label string `json:"label"`
	Param string `json:"param"`
	Type  string `json:"type"`
}
type ProjectLogWindow struct {
	From string `json:"from"`
	To   string `json:"to"`
}
type ProjectLogRecord struct {
	Database   string `json:"database"`
	Collection string `json:"collection"`
	Field      string `json:"field"`
	Param      string `json:"param"`
}

var (
	logSourceID     = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	logCollectionID = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)
	logField        = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)
	logParam        = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
	logDatabaseURL  = regexp.MustCompile(`^https?://[^/@?#]+(?:/[^?#]*)?/v1/databases/[A-Za-z0-9_.-]+$`)
	logUTC          = regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?Z$`)
)

func validLogDatabase(value string) bool {
	if !logDatabaseURL.MatchString(value) {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return false
	}
	return parsed.Scheme == "https" || parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost" || parsed.Hostname() == "::1")
}
func validLogText(value string, limit int) bool {
	return strings.TrimSpace(value) != "" && len(utf16.Encode([]rune(value))) <= limit
}
func validLogFields(fields map[string]string, required bool) bool {
	for role, field := range fields {
		switch role {
		case "timestamp", "message", "severity", "service", "correlation", "sequence", "reference", "raw", "origin", "parseStatus", "parser", "structured":
		default:
			return false
		}
		if !logField.MatchString(field) {
			return false
		}
	}
	return !required || fields["timestamp"] != "" && fields["message"] != ""
}

// ParseProjectLogSourcesJSON validates the complete known contract. Unknown
// members fail closed rather than hiding a future connection-bearing extension.
// It grants no network access and never returns source values in catalog output.
func ParseProjectLogSourcesJSON(body []byte) (ProjectLogSources, error) {
	var file ProjectLogSources
	if len(body) > 1<<20 || jsonstrict.CheckNoDuplicateKeysFor(body, reflect.TypeOf(file)) != nil {
		return ProjectLogSources{}, ErrUnsupportedConnectionFile
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&file) != nil || decoder.Decode(new(any)) != io.EOF || file.Format != ProjectLogSourcesFormat || file.Sources == nil {
		return ProjectLogSources{}, ErrUnsupportedConnectionFile
	}
	seen := map[string]bool{}
	for _, source := range file.Sources {
		if !logSourceID.MatchString(source.ID) || seen[source.ID] || !validLogText(source.Title, 200) || !validLogDatabase(source.Database) || !validLogFields(source.Fields, true) || len(source.Collections) == 0 {
			return ProjectLogSources{}, ErrUnsupportedConnectionFile
		}
		seen[source.ID] = true
		collections := map[string]bool{}
		for _, collection := range source.Collections {
			if !logCollectionID.MatchString(collection.ID) || collections[collection.ID] || !validLogText(collection.Title, 200) || !validLogFields(collection.Fields, false) {
				return ProjectLogSources{}, ErrUnsupportedConnectionFile
			}
			collections[collection.ID] = true
		}
		params := map[string]bool{"source": true, "collection": true, "from": true, "to": true}
		for _, identifier := range source.Identifiers {
			if !logField.MatchString(identifier.Field) || !validLogText(identifier.Label, 80) || !logParam.MatchString(identifier.Param) || params[identifier.Param] || identifier.Type != "integer" && identifier.Type != "string" {
				return ProjectLogSources{}, ErrUnsupportedConnectionFile
			}
			params[identifier.Param] = true
		}
		from, errFrom := time.Parse(time.RFC3339Nano, source.Window.From)
		to, errTo := time.Parse(time.RFC3339Nano, source.Window.To)
		if !logUTC.MatchString(source.Window.From) || !logUTC.MatchString(source.Window.To) || errFrom != nil || errTo != nil || to.Before(from) {
			return ProjectLogSources{}, ErrUnsupportedConnectionFile
		}
		for _, record := range source.Records {
			if !validLogDatabase(record.Database) || !logCollectionID.MatchString(record.Collection) || !logField.MatchString(record.Field) || !params[record.Param] || record.Param == "source" || record.Param == "collection" || record.Param == "from" || record.Param == "to" {
				return ProjectLogSources{}, ErrUnsupportedConnectionFile
			}
		}
	}
	return file, nil
}

// ParseProjectConnectionDirectoryJSON recognizes the exact owning filename and
// fully validates its contract. false means a proved log-source document; every
// unknown format or misplaced log document is an error, never silently skipped.
func ParseProjectConnectionDirectoryJSON(name string, body []byte) (ProjectConnections, bool, error) {
	if name == ProjectLogSourcesFile {
		_, err := ParseProjectLogSourcesJSON(body)
		return ProjectConnections{}, false, err
	}
	catalog, err := ParseProjectConnectionsJSON(body)
	return catalog, true, err
}
func ParsePublicProjectConnectionDirectoryJSON(name string, body []byte) (ProjectConnections, bool, error) {
	if len(body) > MaxPublicConnectionCatalogBytes {
		return ProjectConnections{}, false, ErrUnsupportedConnectionFile
	}
	return ParseProjectConnectionDirectoryJSON(name, body)
}

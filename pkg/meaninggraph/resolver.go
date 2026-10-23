// Package meaninggraph resolves exact retained DataModel source revisions.
// It never resolves meanings, executes source code, opens a database or writes.
package meaninggraph

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
)

const RegistryRepository = "https://github.com/meaninggraph/registry"
const LegacyRegistryRepository = "https://github.com/modelspec-org/registry"
const TransportFixture = "local-fixture"
const TransportGitHub = "fixed-github-commit"

type Limits struct {
	MaxFiles, MaxModels         int
	MaxFileBytes, MaxTotalBytes int64
	Timeout                     time.Duration
}
type Config struct {
	RegistryCommit string
	CatalogSHA256  string // SHA-256 of exact catalog.json bytes, without sha256: prefix.
	FixtureRoot    string // Optional development transport; NOT proof of the Git commit.
	Limits         Limits
}
type Resolver struct {
	config Config
	client *http.Client
}
type ArchivedSource struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	MediaType string `json:"mediaType"`
	Bytes     []byte `json:"bytes"`
}
type DependencyPin struct {
	Alias      string `json:"alias"`
	RevisionID string `json:"revision_id"`
}
type ModelRevision struct {
	datatug.DataModelReference
	RevisionID   string           `json:"revisionId"`
	RegistryID   string           `json:"registryId"`
	Title        string           `json:"title"`
	Module       json.RawMessage  `json:"module"`
	Receipt      json.RawMessage  `json:"receipt"`
	Definition   json.RawMessage  `json:"definition"`
	SourceJSON   json.RawMessage  `json:"sourceJSON"`
	SourceHCL    []byte           `json:"sourceHCL"`
	Sources      []ArchivedSource `json:"sources"`
	Dependencies []DependencyPin  `json:"dependencies"`
}
type Resolution struct {
	Reference      datatug.DataModelReference `json:"reference"`
	Transport      string                     `json:"transport"`
	RegistryCommit string                     `json:"registryCommit"`
	CatalogSHA256  string                     `json:"catalogSHA256"`
	Root           ModelRevision              `json:"root"`
	Revisions      []ModelRevision            `json:"revisions"` // Exact dependency closure, root first; cycles visit once.
	ReadPaths      []string                   `json:"readPaths"`
	BytesRead      int64                      `json:"bytesRead"`
}

var sha40 = regexp.MustCompile(`^[0-9a-f]{40}$`)
var sha64 = regexp.MustCompile(`^[0-9a-f]{64}$`)
var registryID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)
var aliasName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
var repositoryPattern = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

func NewResolver(c Config) (*Resolver, error) {
	if !sha40.MatchString(c.RegistryCommit) || !sha64.MatchString(c.CatalogSHA256) {
		return nil, errors.New("explicit registry Git commit and raw catalog SHA-256 required")
	}
	if c.Limits.MaxFiles == 0 {
		c.Limits.MaxFiles = 96
	}
	if c.Limits.MaxModels == 0 {
		c.Limits.MaxModels = 64
	}
	if c.Limits.MaxFileBytes == 0 {
		c.Limits.MaxFileBytes = 5 << 20
	}
	if c.Limits.MaxTotalBytes == 0 {
		c.Limits.MaxTotalBytes = 5 << 20
	}
	if c.Limits.Timeout == 0 {
		c.Limits.Timeout = 15 * time.Second
	}
	if c.Limits.MaxFiles < 1 || c.Limits.MaxModels < 1 || c.Limits.MaxFileBytes < 1 || c.Limits.MaxTotalBytes < 1 || c.Limits.Timeout < 1 {
		return nil, errors.New("invalid resolver bounds")
	}
	if c.FixtureRoot != "" {
		if err := validateFixtureRoot(c.FixtureRoot); err != nil {
			return nil, err
		}
	}
	return &Resolver{config: c, client: &http.Client{Timeout: c.Limits.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func digest(body []byte) string { h := sha256.Sum256(body); return hex.EncodeToString(h[:]) }
func safePath(p string) error {
	if p == "" || len(p) > 2048 || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") {
		return errors.New("unsafe retained path")
	}
	for _, r := range p {
		if r < 32 || r == 127 {
			return errors.New("unsafe retained path")
		}
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." {
			return errors.New("unsafe retained path")
		}
	}
	return nil
}

type session struct {
	resolver *Resolver
	cache    map[string][]byte
	paths    []string
	bytes    int64
}

func (s *session) read(ctx context.Context, path, hash string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := safePath(path); err != nil {
		return nil, err
	}
	if hash != "" && !sha64.MatchString(hash) {
		return nil, errors.New("invalid retained file digest")
	}
	body, ok := s.cache[path]
	if !ok {
		c := s.resolver.config
		if len(s.paths) >= c.Limits.MaxFiles {
			return nil, errors.New("retained file-count bound exceeded")
		}
		remaining := min(c.Limits.MaxFileBytes, c.Limits.MaxTotalBytes-s.bytes)
		if remaining <= 0 {
			return nil, errors.New("retained byte bound exceeded")
		}
		var stream io.ReadCloser
		var err error
		if c.FixtureRoot != "" {
			stream, err = openFixtureFile(c.FixtureRoot, path)
		} else {
			req, e := http.NewRequestWithContext(ctx, http.MethodGet, "https://raw.githubusercontent.com/meaninggraph/registry/"+c.RegistryCommit+"/"+path, nil)
			if e != nil {
				return nil, e
			}
			req.Header.Set("Accept", "application/json, application/yaml, text/plain")
			response, e := s.resolver.client.Do(req)
			if e != nil {
				return nil, e
			}
			if response.StatusCode != http.StatusOK {
				_ = response.Body.Close()
				return nil, fmt.Errorf("retained GitHub file unavailable (HTTP %d)", response.StatusCode)
			}
			stream = response.Body
		}
		if err != nil {
			return nil, err
		}
		defer func() { _ = stream.Close() }()
		body, err = io.ReadAll(io.LimitReader(stream, remaining+1))
		if err != nil {
			return nil, err
		}
		if int64(len(body)) > remaining {
			return nil, errors.New("retained byte bound exceeded")
		}
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		s.bytes += int64(len(body))
		s.paths = append(s.paths, path)
		s.cache[path] = body
	}
	if hash != "" && digest(body) != hash {
		return nil, fmt.Errorf("retained digest mismatch: %s", path)
	}
	return body, nil
}

// Resolve selects solely by exact address+source commit, including historical
// index rows. Fixture mode verifies bytes and identity but makes no Git admission
// claim: its transport label remains local-fixture in every returned resolution.
func (r *Resolver) Resolve(ctx context.Context, reference datatug.DataModelReference) (*Resolution, error) {
	if err := reference.Validate(); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, r.config.Limits.Timeout)
	defer cancel()
	s := &session{resolver: r, cache: map[string][]byte{}}
	catalog, rows, err := s.catalog(ctx)
	if err != nil {
		return nil, err
	}
	id := revisionID("model", reference.NormalizedAddress(), reference.Commit)
	row, ok := rows[id]
	if !ok || row.Kind != "model" || row.Address != reference.NormalizedAddress() || row.Commit != reference.Commit {
		return nil, errors.New("exact retained revision missing; no latest fallback")
	}
	pending := []revisionRow{row}
	loaded := map[string]*loadedModel{}
	order := []string{}
	for i := 0; i < len(pending); i++ {
		current := pending[i]
		if _, ok = loaded[current.ID]; ok {
			continue
		}
		if len(loaded) >= r.config.Limits.MaxModels {
			return nil, errors.New("model closure bound exceeded")
		}
		model, e := s.load(ctx, current)
		if e != nil {
			return nil, e
		}
		loaded[current.ID] = model
		order = append(order, current.ID)
		for _, pin := range model.result.Dependencies {
			target, found := rows[pin.RevisionID]
			if !found || target.Kind != "model" {
				return nil, errors.New("missing exact model dependency; no latest fallback")
			}
			pending = append(pending, target)
			if len(pending) > r.config.Limits.MaxModels*r.config.Limits.MaxModels {
				return nil, errors.New("model dependency traversal bound exceeded")
			}
		}
	}
	for _, key := range order {
		if err = verifyClosure(loaded[key], loaded); err != nil {
			return nil, err
		}
	}
	root := loaded[id]
	for _, selection := range catalog.Models {
		if selected, present := loaded[selection.RevisionID]; present {
			if err = verifySelection(selection, selected); err != nil {
				return nil, err
			}
		}
	}
	mode := TransportGitHub
	if r.config.FixtureRoot != "" {
		mode = TransportFixture
	}
	result := &Resolution{Reference: reference, Transport: mode, RegistryCommit: r.config.RegistryCommit, CatalogSHA256: r.config.CatalogSHA256, Root: root.result, ReadPaths: append([]string(nil), s.paths...), BytesRead: s.bytes}
	for _, key := range order {
		result.Revisions = append(result.Revisions, loaded[key].result)
	}
	return result, nil
}

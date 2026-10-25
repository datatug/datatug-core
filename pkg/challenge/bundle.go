package challenge

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

// Opened is a manifest whose schema, references and fixture pins have all
// been verified. It holds its own fixture snapshots; later mutation of caller
// buffers cannot change what an attempt executes.
type Opened struct {
	Manifest         Manifest
	DefinitionDigest string

	fixtures map[string][]byte
}

// LoadManifest strictly decodes and validates manifest bytes. Unknown fields,
// duplicate keys, unsupported versions, range violations and broken internal
// references are refused before execution.
func LoadManifest(data []byte) (Manifest, error) {
	if len(data) == 0 || len(data) > MaxDefinitionBytes {
		return Manifest{}, refuse(CodeInvalidManifest, "manifest", "must be between 1 and %d bytes", MaxDefinitionBytes)
	}
	// Check the pinned versions before strict decoding: a future schema may add
	// fields this package cannot know, and it must still be reported as an
	// unsupported schema rather than a generic unknown-field failure.
	var header struct {
		SchemaVersion  *int    `json:"schemaVersion"`
		GradingVersion *string `json:"gradingVersion"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return Manifest{}, refuse(CodeInvalidManifest, "manifest", "cannot decode: %s", err)
	}
	if header.SchemaVersion == nil || header.GradingVersion == nil {
		return Manifest{}, refuse(CodeInvalidManifest, "manifest", "schemaVersion and gradingVersion are required")
	}
	if *header.SchemaVersion != SchemaVersion {
		return Manifest{}, refuse(CodeUnsupportedSchema, "schemaVersion", "supported version is %d", SchemaVersion)
	}
	if *header.GradingVersion != GradingVersion {
		return Manifest{}, refuse(CodeUnsupportedGradingVersion, "gradingVersion", "supported semantics are %q", GradingVersion)
	}
	var manifest Manifest
	if err := apicontract.DecodeStrict(data, &manifest); err != nil {
		return Manifest{}, refuse(CodeInvalidManifest, "manifest", "cannot decode: %s", err)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Open verifies the manifest and every pinned fixture byte before returning a
// definition. A refused open returns no Opened value, so no attempt or
// completion can be created from it.
func Open(manifestJSON []byte, fixtures map[string][]byte) (*Opened, error) {
	manifest, err := LoadManifest(manifestJSON)
	if err != nil {
		return nil, err
	}
	pinned := make(map[string]Fixture, len(manifest.FixtureSuite.Fixtures))
	for _, fixture := range manifest.FixtureSuite.Fixtures {
		pinned[fixture.ID] = fixture
	}
	for id := range fixtures {
		if _, ok := pinned[id]; !ok {
			return nil, refuse(CodeFixtureUnknown, "fixtures."+id, "no revision pin exists for these bytes")
		}
	}
	snapshots := make(map[string][]byte, len(pinned))
	for _, fixture := range manifest.FixtureSuite.Fixtures {
		data, ok := fixtures[fixture.ID]
		if !ok {
			return nil, refuse(CodeFixtureMissing, "fixtures."+fixture.ID, "pinned fixture bytes are unavailable")
		}
		if len(data) != fixture.Bytes {
			return nil, refuse(CodeFixtureBytes, "fixtures."+fixture.ID, "observed %d bytes, pinned %d", len(data), fixture.Bytes)
		}
		sum := sha256.Sum256(data)
		observed := hex.EncodeToString(sum[:])
		if subtle.ConstantTimeCompare([]byte(observed), []byte(fixture.SHA256)) != 1 {
			return nil, refuse(CodeFixtureHash, "fixtures."+fixture.ID, "SHA-256 differs from the revision pin")
		}
		snapshots[fixture.ID] = append([]byte(nil), data...)
	}

	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, refuse(CodeInvalidManifest, "manifest", "cannot canonicalize: %s", err)
	}
	digest := sha256.New()
	_, _ = digest.Write(encoded)
	ids := make([]string, 0, len(pinned))
	for id := range pinned {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write([]byte(id))
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write([]byte(pinned[id].SHA256))
	}
	return &Opened{
		Manifest:         manifest,
		DefinitionDigest: hex.EncodeToString(digest.Sum(nil)),
		fixtures:         snapshots,
	}, nil
}

// Fixture returns a defensive copy of one verified fixture snapshot.
func (o *Opened) Fixture(id string) ([]byte, bool) {
	data, ok := o.fixtures[id]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), data...), true
}

// FixtureIDs returns the pinned fixture IDs in deterministic order.
func (o *Opened) FixtureIDs() []string {
	ids := make([]string, 0, len(o.fixtures))
	for id := range o.fixtures {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// RequireEngine refuses a runtime whose exact engine differs from the
// manifest pin. Grading semantics and engine version travel together.
func (o *Opened) RequireEngine(engine Engine) error {
	if engine != o.Manifest.Engine {
		return refuse(CodeUnsupportedEngine, "engine", "runtime engine does not match the revision pin")
	}
	return nil
}

// HasFixture reports whether a fixture ID is part of the verified suite.
func (o *Opened) HasFixture(id string) bool {
	_, ok := o.fixtures[id]
	return ok
}

// FixtureBytesEqual is used by inspection receipts that need to prove two
// opens used identical pristine bytes.
func (o *Opened) FixtureBytesEqual(id string, other []byte) bool {
	data, ok := o.fixtures[id]
	return ok && bytes.Equal(data, other)
}

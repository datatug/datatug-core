package challenge

// SchemaVersion and GradingVersion are part of the wire contract. A manifest
// is graded only by the exact semantics it pins: changing a predicate without
// changing these values is a contract violation, and changing them requires a
// new revision.
const (
	SchemaVersion  = 1
	GradingVersion = "challenge-grading-v1"
)

// Limits are the approved execution limits. A manifest cannot relax them, and
// definition values are validated against them while the asset is opened.
const (
	MaxDefinitionBytes = 4 << 20
	MaxFixtureBytes    = 10 << 20
	MaxSQLBytes        = 64 << 10
	MaxResultRows      = 1000
	MaxResultColumns   = 40

	maxRevision        = 1_000_000
	maxChallengeIDLen  = 64
	maxFixtureBytes    = MaxFixtureBytes
	maxManifestTables  = 16
	maxManifestColumns = 40
	maxManifestParts   = 32
	maxManifestCases   = 16
	maxHints           = 20
	maxReferenceSols   = 32
	maxStructuredField = 16
	maxStructuredOpts  = 64
	maxQueryIDs        = 16
)

// Engine is the exact execution engine a revision is graded on. The fields are
// pinned in the manifest and must match the runtime that opens it.
type Engine struct {
	Package       string `json:"package"`
	Version       string `json:"version"`
	SQLiteVersion string `json:"sqliteVersion"`
}

// SupportedEngine is the browser challenge runtime pinned by the approved
// execution foundation. A revision declaring any other engine is refused
// rather than silently graded by different SQLite behaviour.
var SupportedEngine = Engine{
	Package:       "@sqlite.org/sqlite-wasm",
	Version:       "3.54.1-build1",
	SQLiteVersion: "3.54.1",
}

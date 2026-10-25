package challenge

import (
	"fmt"
	"reflect"

	"github.com/datatug/datatug-core/pkg/apicontract"
)

// TimingMode is the attempt timing policy pinned by a revision.
type TimingMode string

const (
	TimingOff      TimingMode = "off"
	TimingOptional TimingMode = "optional"
	TimingRequired TimingMode = "required"
)

// Answer kinds. Each part has exactly one kind and one matching answer body.
const (
	AnswerSQL             = "sql"
	AnswerStructured      = "structured"
	AnswerCheckDefinition = "check-definition"
)

// Field cardinalities for closed structured answers.
const (
	CardinalitySingle = "single"
	CardinalityMulti  = "multi"
)

// Difficulty values exposed by discovery.
const (
	DifficultyIntro        = "intro"
	DifficultyIntermediate = "intermediate"
	DifficultyAdvanced     = "advanced"
)

// Manifest is one immutable published challenge revision.
type Manifest struct {
	SchemaVersion      int                 `json:"schemaVersion"`
	GradingVersion     string              `json:"gradingVersion"`
	ChallengeID        string              `json:"challengeId"`
	Revision           int                 `json:"revision"`
	Title              string              `json:"title"`
	Objective          string              `json:"objective"`
	Difficulty         string              `json:"difficulty"`
	Dialect            string              `json:"dialect"`
	AllowedOperation   string              `json:"allowedOperation"`
	Timing             TimingMode          `json:"timing"`
	Engine             Engine              `json:"engine"`
	FixtureSuite       FixtureSuite        `json:"fixtureSuite"`
	VisibleSchema      VisibleSchema       `json:"visibleSchema"`
	Parts              []Part              `json:"parts"`
	Hints              []Hint              `json:"hints"`
	ReferenceSolutions []ReferenceSolution `json:"referenceSolutions"`
}

// FixtureSuite pins every byte the graded answer is executed against.
type FixtureSuite struct {
	ID       string    `json:"id"`
	Fixtures []Fixture `json:"fixtures"`
}

// Fixture is one immutable SHA-256/byte-length pin. Runs is the number of
// times the submitted SQL is applied (2 proves idempotency).
type Fixture struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
	Runs   int    `json:"runs"`
}

// VisibleSchema is the schema shown to the learner.
type VisibleSchema struct {
	Tables []VisibleTable `json:"tables"`
}

// VisibleTable is one table in the visible schema.
type VisibleTable struct {
	Name    string          `json:"name"`
	Columns []VisibleColumn `json:"columns"`
}

// VisibleColumn is one visible column, including the SQLite collation that
// participates in equality.
type VisibleColumn struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	PrimaryKey bool   `json:"primaryKey,omitempty"`
	NotNull    bool   `json:"notNull,omitempty"`
	Collation  string `json:"collation,omitempty"`
}

// Part is one named, required-by-default answer required to complete a
// challenge.
type Part struct {
	ID              string               `json:"id"`
	Title           string               `json:"title"`
	Required        bool                 `json:"required"`
	AnswerKind      string               `json:"answerKind"`
	SQL             *SQLPart             `json:"sql,omitempty"`
	Structured      *StructuredPart      `json:"structured,omitempty"`
	CheckDefinition *CheckDefinitionPart `json:"checkDefinition,omitempty"`
}

// SQLPart declares the fixture cases and their expected oracles.
type SQLPart struct {
	Cases []SQLCase `json:"cases"`
}

// SQLCase pins one fixture to one oracle.
type SQLCase struct {
	Fixture string `json:"fixture"`
	Oracle  Oracle `json:"oracle"`
}

// Hint is one progressive hint; order is the array order.
type Hint struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

// ReferenceSolution declares an assisted answer. Consuming UI must tag it as
// assistance; a reference answer never awards completion by itself.
type ReferenceSolution struct {
	PartID string          `json:"partId"`
	Answer ReferenceAnswer `json:"answer"`
}

// ReferenceAnswer is a closed union matching the part's answer kind.
type ReferenceAnswer struct {
	SQL             string                 `json:"sql,omitempty"`
	Structured      *StructuredAnswer      `json:"structured,omitempty"`
	CheckDefinition *CheckDefinitionAnswer `json:"checkDefinition,omitempty"`
}

// Validate checks every structural, range and internal reference rule. It is
// intentionally independent of fixture bytes; Open verifies the pins.
func (m Manifest) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return refuse(CodeUnsupportedSchema, "schemaVersion", "supported version is %d", SchemaVersion)
	}
	if m.GradingVersion != GradingVersion {
		return refuse(CodeUnsupportedGradingVersion, "gradingVersion", "supported semantics are %q", GradingVersion)
	}
	if !validID(m.ChallengeID) {
		return refuse(CodeInvalidManifest, "challengeId", "must be a lowercase slug of at most %d characters", maxChallengeIDLen)
	}
	if m.Revision < 1 || m.Revision > maxRevision {
		return refuse(CodeInvalidManifest, "revision", "must be between 1 and %d", maxRevision)
	}
	if !boundedText(m.Title, 1, 200) || len(m.Title) != len(trimSpace(m.Title)) {
		return refuse(CodeInvalidManifest, "title", "must be 1..200 characters without surrounding whitespace")
	}
	if !boundedText(m.Objective, 1, 4000) {
		return refuse(CodeInvalidManifest, "objective", "must be 1..4000 characters")
	}
	if !oneOf(m.Difficulty, DifficultyIntro, DifficultyIntermediate, DifficultyAdvanced) {
		return refuse(CodeInvalidManifest, "difficulty", "must be intro, intermediate or advanced")
	}
	if m.Dialect != "sqlite" {
		return refuse(CodeUnsupportedDialect, "dialect", "only sqlite is supported by the browser runtime")
	}
	if !oneOf(m.AllowedOperation, "read", "write") {
		return refuse(CodeInvalidManifest, "allowedOperation", "must be read or write")
	}
	if !oneOf(string(m.Timing), string(TimingOff), string(TimingOptional), string(TimingRequired)) {
		return refuse(CodeInvalidManifest, "timing", "must be off, optional or required")
	}
	if m.Engine != SupportedEngine {
		return refuse(CodeUnsupportedEngine, "engine", "revision is pinned to an unsupported engine")
	}
	if err := m.FixtureSuite.validate(); err != nil {
		return err
	}
	if err := m.VisibleSchema.validate(); err != nil {
		return err
	}
	if len(m.Parts) == 0 || len(m.Parts) > maxManifestParts {
		return refuse(CodeInvalidManifest, "parts", "must contain between 1 and %d parts", maxManifestParts)
	}
	fixtureIDs := map[string]bool{}
	for _, fixture := range m.FixtureSuite.Fixtures {
		fixtureIDs[fixture.ID] = true
	}
	partIDs := map[string]bool{}
	required := 0
	for _, part := range m.Parts {
		if err := part.validate(fixtureIDs); err != nil {
			return err
		}
		if partIDs[part.ID] {
			return refuse(CodeInvalidManifest, "parts", "duplicate part id %q", part.ID)
		}
		partIDs[part.ID] = true
		if part.Required {
			required++
		}
	}
	if required == 0 {
		return refuse(CodeInvalidManifest, "parts", "at least one part must be required")
	}
	if len(m.Hints) > maxHints {
		return refuse(CodeInvalidManifest, "hints", "at most %d hints are allowed", maxHints)
	}
	hintIDs := map[string]bool{}
	for _, hint := range m.Hints {
		if !validID(hint.ID) || hintIDs[hint.ID] || !boundedText(hint.Text, 1, 1000) {
			return refuse(CodeInvalidManifest, "hints", "hints must have unique ids and 1..1000 characters")
		}
		hintIDs[hint.ID] = true
	}
	if len(m.ReferenceSolutions) > maxReferenceSols {
		return refuse(CodeInvalidManifest, "referenceSolutions", "at most %d reference solutions are allowed", maxReferenceSols)
	}
	referenced := map[string]bool{}
	partsByID := make(map[string]Part, len(m.Parts))
	for _, part := range m.Parts {
		partsByID[part.ID] = part
	}
	for _, reference := range m.ReferenceSolutions {
		part, ok := partsByID[reference.PartID]
		if !ok || referenced[reference.PartID] {
			return refuse(CodeInvalidManifest, "referenceSolutions", "reference must name one existing part exactly once")
		}
		referenced[reference.PartID] = true
		if err := validateReferenceAnswer(part, reference.Answer); err != nil {
			return refuse(CodeInvalidManifest, "referenceSolutions."+reference.PartID, "%s", err)
		}
	}
	return nil
}

func (s FixtureSuite) validate() error {
	if !validID(s.ID) {
		return refuse(CodeInvalidManifest, "fixtureSuite.id", "must be a lowercase slug")
	}
	if len(s.Fixtures) == 0 || len(s.Fixtures) > maxManifestCases {
		return refuse(CodeInvalidManifest, "fixtureSuite.fixtures", "must contain between 1 and %d fixtures", maxManifestCases)
	}
	seen := map[string]bool{}
	for _, fixture := range s.Fixtures {
		if !validID(fixture.ID) || seen[fixture.ID] {
			return refuse(CodeInvalidManifest, "fixtureSuite.fixtures", "fixture ids must be valid and unique")
		}
		seen[fixture.ID] = true
		if !boundedText(fixture.Label, 1, 200) {
			return refuse(CodeInvalidManifest, "fixtureSuite.fixtures.label", "fixture labels must be 1..200 characters")
		}
		if !validFixtureHash(fixture.SHA256) {
			return refuse(CodeInvalidManifest, "fixtureSuite.fixtures.sha256", "must be 64 lowercase hexadecimal characters")
		}
		if fixture.Bytes < 1 || fixture.Bytes > maxFixtureBytes {
			return refuse(CodeInvalidManifest, "fixtureSuite.fixtures.bytes", "must be between 1 and %d bytes", maxFixtureBytes)
		}
		if fixture.Runs < 1 || fixture.Runs > 4 {
			return refuse(CodeInvalidManifest, "fixtureSuite.fixtures.runs", "must be between 1 and 4")
		}
	}
	return nil
}

func (s VisibleSchema) validate() error {
	if len(s.Tables) == 0 || len(s.Tables) > maxManifestTables {
		return refuse(CodeInvalidManifest, "visibleSchema.tables", "must contain between 1 and %d tables", maxManifestTables)
	}
	seenTables := map[string]bool{}
	for _, table := range s.Tables {
		if !validSQLName(table.Name) || seenTables[table.Name] {
			return refuse(CodeInvalidManifest, "visibleSchema.tables", "table names must be valid and unique")
		}
		seenTables[table.Name] = true
		if len(table.Columns) == 0 || len(table.Columns) > maxManifestColumns {
			return refuse(CodeInvalidManifest, "visibleSchema.tables.columns", "tables require 1..%d columns", maxManifestColumns)
		}
		seenColumns := map[string]bool{}
		for _, column := range table.Columns {
			if !validSQLName(column.Name) || seenColumns[column.Name] || !knownColumnType(column.Type) || !oneOf(column.Collation, "", "BINARY", "NOCASE", "RTRIM") {
				return refuse(CodeInvalidManifest, "visibleSchema.tables.columns", "invalid column declaration")
			}
			seenColumns[column.Name] = true
		}
	}
	return nil
}

func (p Part) Validate() error {
	fixtureIDs := map[string]bool{}
	if p.SQL != nil {
		for _, item := range p.SQL.Cases {
			fixtureIDs[item.Fixture] = true
		}
	}
	if p.CheckDefinition != nil {
		for _, item := range p.CheckDefinition.Cases {
			fixtureIDs[item.Fixture] = true
		}
	}
	return p.validate(fixtureIDs)
}

func (p Part) validate(fixtureIDs map[string]bool) error {
	if !validID(p.ID) || !boundedText(p.Title, 1, 200) {
		return refuse(CodeInvalidManifest, "parts", "parts require a valid id and a 1..200 character title")
	}
	if !oneOf(p.AnswerKind, AnswerSQL, AnswerStructured, AnswerCheckDefinition) {
		return refuse(CodeInvalidManifest, "parts."+p.ID+".answerKind", "must be sql, structured or check-definition")
	}
	answers := 0
	if p.SQL != nil {
		answers++
	}
	if p.Structured != nil {
		answers++
	}
	if p.CheckDefinition != nil {
		answers++
	}
	if answers != 1 {
		return refuse(CodeInvalidManifest, "parts."+p.ID, "exactly one answer body is required")
	}
	switch p.AnswerKind {
	case AnswerSQL:
		if p.SQL == nil || p.Structured != nil || p.CheckDefinition != nil {
			return refuse(CodeInvalidManifest, "parts."+p.ID, "answerKind sql requires only the sql body")
		}
		if err := p.SQL.validate(fixtureIDs); err != nil {
			return err
		}
	case AnswerStructured:
		if p.Structured == nil || p.SQL != nil || p.CheckDefinition != nil {
			return refuse(CodeInvalidManifest, "parts."+p.ID, "answerKind structured requires only the structured body")
		}
		if err := p.Structured.Validate(); err != nil {
			return refuse(CodeInvalidManifest, "parts."+p.ID+".structured", "%s", err)
		}
	case AnswerCheckDefinition:
		if p.CheckDefinition == nil || p.SQL != nil || p.Structured != nil {
			return refuse(CodeInvalidManifest, "parts."+p.ID, "answerKind check-definition requires only the checkDefinition body")
		}
		if err := p.CheckDefinition.validate(fixtureIDs); err != nil {
			return err
		}
	}
	return nil
}

func (s SQLPart) validate(fixtureIDs map[string]bool) error {
	if len(s.Cases) == 0 || len(s.Cases) > maxManifestCases {
		return refuse(CodeInvalidManifest, "sql.cases", "must contain between 1 and %d cases", maxManifestCases)
	}
	seen := map[string]bool{}
	for _, item := range s.Cases {
		if !fixtureIDs[item.Fixture] || seen[item.Fixture] {
			return refuse(CodeInvalidManifest, "sql.cases.fixture", "case fixtures must exist and appear once")
		}
		seen[item.Fixture] = true
		if err := item.Oracle.Validate(); err != nil {
			return refuse(CodeInvalidManifest, "sql.cases."+item.Fixture+".oracle", "%s", err)
		}
	}
	return nil
}

func validateReferenceAnswer(part Part, answer ReferenceAnswer) error {
	answers := 0
	if answer.SQL != "" {
		answers++
	}
	if answer.Structured != nil {
		answers++
	}
	if answer.CheckDefinition != nil {
		answers++
	}
	if answers != 1 {
		return fmt.Errorf("exactly one reference answer is required")
	}
	switch part.AnswerKind {
	case AnswerSQL:
		if answer.SQL == "" || answer.Structured != nil || answer.CheckDefinition != nil {
			return fmt.Errorf("reference answer must be SQL")
		}
		if len(answer.SQL) == 0 || len(answer.SQL) > MaxSQLBytes {
			return fmt.Errorf("reference SQL must be 1..%d bytes", MaxSQLBytes)
		}
	case AnswerStructured:
		if answer.Structured == nil || answer.SQL != "" || answer.CheckDefinition != nil {
			return fmt.Errorf("reference answer must be structured")
		}
		if !reflect.DeepEqual(answer.Structured.Selections, part.Structured.Expected.Selections) {
			return fmt.Errorf("reference structured answer must equal the declared expected selection")
		}
	case AnswerCheckDefinition:
		if answer.CheckDefinition == nil || answer.SQL != "" || answer.Structured != nil {
			return fmt.Errorf("reference answer must be a Check definition")
		}
		if err := validateCheckDefinitionAnswer(*part.CheckDefinition, *answer.CheckDefinition); err != nil {
			return err
		}
	}
	return nil
}

func knownColumnType(typ string) bool {
	return oneOf(typ,
		string(apicontract.ValueTypeString),
		string(apicontract.ValueTypeNumber),
		string(apicontract.ValueTypeInteger),
		string(apicontract.ValueTypeDecimal),
		string(apicontract.ValueTypeBoolean),
		string(apicontract.ValueTypeDate),
		string(apicontract.ValueTypeDatetime),
	)
}

func validFixtureHash(hash string) bool {
	if len(hash) != 64 {
		return false
	}
	for _, r := range hash {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func trimSpace(value string) string {
	start, end := 0, len(value)
	for start < end && (value[start] == ' ' || value[start] == '\t' || value[start] == '\n' || value[start] == '\r') {
		start++
	}
	for end > start && (value[end-1] == ' ' || value[end-1] == '\t' || value[end-1] == '\n' || value[end-1] == '\r') {
		end--
	}
	return value[start:end]
}

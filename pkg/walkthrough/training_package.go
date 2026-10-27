package walkthrough

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/datatug/datatug-core/pkg/challenge"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/detective"
	"github.com/datatug/datatug-core/pkg/investigation"
)

// PackageSchemaV1 is the only training package index schema this package
// admits.
const PackageSchemaV1 = "datatug-training-package/1"

// Package inventory bounds from the owner training-package format. They are
// separate from the declaration, narrative and challenge-owner bounds.
const (
	MaxPackageIndexBytes          = 1 << 20
	MaxPackageAssets              = 1000
	MaxPackageRecords             = 200
	MaxPackagePresentationTargets = 1000
	MaxCaseFindings               = 1000
	MaxCaseTimeline               = 1000
	MaxPackageTextBytes           = MaxNarrativeTextBytes
	MaxPackageBinaryBytes         = 64 << 20
	MaxTrainingTitleChars         = MaxTitleChars
	MaxTrainingDescriptionChars   = MaxTextChars
)

// MaxChallengeFixtureBytes is the unchanged execution-owner fixture bound,
// re-exported so callers of package admission see the exact value used.
const MaxChallengeFixtureBytes = challenge.MaxFixtureBytes

// ErrNewerTrainingPackage is returned by [TrainingPackageResult.Err] when the
// index names a later schema major. It is a recoverable newer-client boundary,
// not a malformed package, and no codec or native validation runs.
var ErrNewerTrainingPackage = errors.New("walkthrough: training package needs a newer DataTug")

var trainingPackageSchemaPattern = regexp.MustCompile(`^datatug-training-package/(0|[1-9][0-9]*)$`)

// trainingIntegerPattern is the canonical JSON integer spelling, including a
// negative sign. Revisions and byte lengths stay non-negative; only native SQL
// integer parameter defaults may be negative.
var trainingIntegerPattern = regexp.MustCompile(`^(0|-?[1-9][0-9]*)$`)

// TrainingPackageBinding is the trusted external owner context that is never
// duplicated inside package JSON: the resolving project, the exact content
// pin and the verified declaration, narrative and package digests, plus the
// admitted sidecar scenario identity. The package stage compares these values
// but cannot verify the declaration or narrative bytes themselves.
type TrainingPackageBinding struct {
	Project           ProjectIdentity
	ContentPin        ContentPin
	DeclarationSHA256 string
	NarrativeSHA256   string
	PackageSHA256     string
	ScenarioID        string
	ScenarioRevision  Integer
}

// TrainingPackageAssetBytes is one caller-supplied child file. The declared
// package index owns the pin; Path only identifies which declaration the bytes
// satisfy, so an index/byte disagreement is a verification refusal.
type TrainingPackageAssetBytes struct {
	Path  string
	Bytes []byte
}

// TrainingPackageInput is one pure package-stage admission request. No file is
// fetched and nothing executes.
type TrainingPackageInput struct {
	Reference  PackageReference
	IndexBytes []byte
	Assets     []TrainingPackageAssetBytes
	Binding    TrainingPackageBinding
}

// TrainingPackageResult is the outcome of package admission. Package is
// non-nil only when Admitted is true.
type TrainingPackageResult struct {
	Admitted    bool                     `json:"admitted"`
	NewerClient bool                     `json:"newerClient"`
	Reference   PackageReference         `json:"reference"`
	Package     *AdmittedTrainingPackage `json:"package,omitempty"`
	Issues      []Issue                  `json:"issues,omitempty"`
}

// Err returns nil for an admitted package, [ErrNewerTrainingPackage] for a
// later schema major, or an aggregate recoverable [Error].
func (r TrainingPackageResult) Err() error {
	if r.Admitted {
		return nil
	}
	if r.NewerClient {
		return ErrNewerTrainingPackage
	}
	return &Error{Issues: r.Issues}
}

// TrainingFictionalActor is a fictional presentation identity. It is not an
// actor UID, credential or notification address.
type TrainingFictionalActor struct {
	ID   string
	Name string
}

// TrainingSourceBinding binds one package query to its fixture-only source.
type TrainingSourceBinding struct {
	ID             string
	FixtureAssetID string
}

// TrainingRecordColumn is one declared evidence-records column.
type TrainingRecordColumn struct {
	Name string
	Type investigation.ValueType
}

// TrainingRecords is the typed evidence-records projection.
type TrainingRecords struct {
	Columns []TrainingRecordColumn
	Rows    [][]investigation.TypedValue
}

// TrainingLogEntry is one fictional evidence-log line.
type TrainingLogEntry struct {
	ID      string
	At      string
	ActorID string
	Kind    string
	Message string
}

// TrainingParameterDefault is one exact scalar SQL parameter default.
type TrainingParameterDefault struct {
	Kind   string
	Text   string
	Bool   bool
	Number string
	Float  float64
}

// TrainingParameter is one closed training SQL parameter record.
type TrainingParameter struct {
	ID         string
	Type       string
	Title      *string
	IsRequired *bool
	Default    *TrainingParameterDefault
}

// TrainingQueryMetadata is the closed query-metadata codec projection. It is
// not a hydrated native request.
type TrainingQueryMetadata struct {
	ID           string
	Title        string
	Type         string
	ConnectionID string
	Parameters   []TrainingParameter
}

// TrainingWatchRecommendation is an unsaved, display-only Watch proposal. It
// never creates, enrols, notifies or evaluates a production Watch.
type TrainingWatchRecommendation struct {
	Title              string
	CheckID            string
	CheckVersion       string
	Inputs             map[string]detective.InputValue
	IntervalSeconds    Integer
	Trigger            string
	InitialObservation string
	Mode               string
	CooldownSeconds    Integer
	Recovery           bool
}

// TrainingPackageAsset is the verified inventory entry and its codec
// projection. Exactly the fields named by Kind are populated: Text for
// evidence-text and query-body-sql, the matching pointer for the other JSON
// codecs, and Binary for challenge-fixture (whose raw bytes are deliberately
// not retained).
type TrainingPackageAsset struct {
	ID        string
	Kind      string
	MediaType string
	Reference AssetReference

	Text     string
	Records  *TrainingRecords
	Log      []TrainingLogEntry
	Metadata *TrainingQueryMetadata
	Check    *detective.Check
	Manifest *challenge.Manifest
	Watch    *TrainingWatchRecommendation
	Binary   bool
}

// TrainingFinding is one Case-local native Finding projection.
type TrainingFinding struct {
	ID              string
	Text            string
	Status          string
	EvidenceRefs    []string
	HasEvidenceRefs bool
}

// TrainingTimelineRef is the closed timeline reference.
type TrainingTimelineRef struct {
	Kind string
	ID   string
}

// TrainingTimelineEntry is one fictional display-only timeline record.
type TrainingTimelineEntry struct {
	ID      string
	At      string
	ActorID string
	Type    string
	Ref     TrainingTimelineRef
}

// TrainingCase is one package-local Case read projection.
type TrainingCase struct {
	ID          string
	Title       string
	Status      string
	Context     map[string]detective.InputValue
	IncidentIDs []string
	EvidenceIDs []string
	Findings    []TrainingFinding
	Timeline    []TrainingTimelineEntry
}

// TrainingIncident is one package-local incident read projection.
type TrainingIncident struct {
	ID          string
	Title       string
	Description string
	Status      string
	EvidenceIDs []string
}

// TrainingCheckRecord binds a package Check id to its definition asset.
type TrainingCheckRecord struct {
	ID                string
	DefinitionAssetID string
}

// TrainingQueryRecord binds the separately pinned query metadata and SQL body
// to one fixture-only source.
type TrainingQueryRecord struct {
	ID              string
	MetadataAssetID string
	BodyAssetID     string
	SourceID        string
	// Hydrated is the native query produced only after the separately pinned
	// metadata and body verified. It was validated but never executed.
	Hydrated datatug.QueryDef
}

// TrainingChallengeFixture binds one manifest fixture id to an asset.
type TrainingChallengeFixture struct {
	FixtureID string
	AssetID   string
}

// TrainingChallengeRecord binds one immutable challenge revision to its
// verified manifest and fixture suite.
type TrainingChallengeRecord struct {
	ID              string
	Revision        Integer
	ManifestAssetID string
	Fixtures        []TrainingChallengeFixture
}

// TrainingProposal is one unsaved training recommendation.
type TrainingProposal struct {
	ID           string
	Kind         string
	DefinitionID string
	EvidenceIDs  []string
	Rationale    string
}

// TrainingPresentationTarget is a manifest-to-view descriptor. It is never a
// selector, router command, component or focus authority.
type TrainingPresentationTarget struct {
	ID    string
	Kind  string
	RefID string
}

// TrainingAssetContribution is one unique canonical source+path contribution.
// Identical aliases share one contribution; the AssetIDs record every
// declaration that resolved to it.
type TrainingAssetContribution struct {
	Path     string   `json:"path"`
	SHA256   string   `json:"sha256"`
	Class    string   `json:"class"`
	Bytes    Integer  `json:"bytes"`
	AssetIDs []string `json:"assetIds"`
}

// AdmittedTrainingPackage is the defensive, read-only package-stage snapshot.
// It retains typed projections and pins only; caller byte slices are never
// aliased and raw fixture bytes are not retained.
type AdmittedTrainingPackage struct {
	Schema              string
	ID                  string
	Revision            Integer
	Reference           PackageReference
	Binding             TrainingPackageBinding
	Assets              []TrainingPackageAsset
	FictionalActors     []TrainingFictionalActor
	Sources             []TrainingSourceBinding
	Cases               []TrainingCase
	Incidents           []TrainingIncident
	Checks              []TrainingCheckRecord
	Queries             []TrainingQueryRecord
	Challenges          []TrainingChallengeRecord
	Proposals           []TrainingProposal
	PresentationTargets []TrainingPresentationTarget
	TextBytes           Integer
	BinaryBytes         Integer
	Contributions       []TrainingAssetContribution
}

var packageRootFields = map[string]bool{
	"schema": true, "id": true, "revision": true, "assets": true,
	"fictionalActors": true, "sources": true, "cases": true,
	"incidents": true, "checks": true, "queries": true, "challenges": true,
	"proposals": true, "presentationTargets": true,
}

var trainingAssetFields = map[string]bool{
	"id": true, "kind": true, "mediaType": true, "path": true, "sha256": true, "bytes": true,
}

// packageCodecMediaTypes is the exact owner codec/media-type registry. Kind,
// never mediaType, decides text versus binary accounting.
var packageCodecMediaTypes = map[string]string{
	"evidence-text":        "text/plain",
	"evidence-records":     "application/json",
	"evidence-log":         "application/x-ndjson",
	"query-metadata":       "application/json",
	"query-body-sql":       "application/sql",
	"check-definition":     "application/json",
	"challenge-manifest":   "application/json",
	"challenge-fixture":    "application/vnd.sqlite3",
	"watch-recommendation": "application/json",
}

// AdmitTrainingPackage performs pure package-stage admission. The caller
// supplies the exact pinned index bytes, the declared child bytes and a
// trusted external binding. The result is either a closed, native-valid,
// locally reference-closed defensive snapshot with exact text/binary
// contributions, or a precise recoverable refusal with no partial package.
//
// This stage proves neither reservation-before-read, concurrent-reader limits,
// streaming/deadline/cache/copy/transfer ledgers, live authority, pin or
// disposal revocation, nor production loading. Those remain transport and
// orchestration gates.
//
// Ownership: every admitted string and typed value is an independent copy; the
// caller's byte slices are not retained or aliased. Challenge fixture bytes are
// verified through the native Open path, whose retained snapshots are discarded
// before return, so no raw handle or executable capability is exposed.
func AdmitTrainingPackage(input TrainingPackageInput) TrainingPackageResult {
	result := TrainingPackageResult{Reference: input.Reference}
	if issues := validateTrainingPackageBinding(input); len(issues) > 0 {
		result.Issues = issues
		return result
	}
	data := input.IndexBytes
	if len(data) > MaxPackageIndexBytes {
		result.Issues = []Issue{{
			Code:    CodeTooLarge,
			Path:    "",
			Message: "package index exceeds 1 MiB",
		}}
		return result
	}
	if err := input.Reference.VerifyBytes(data); err != nil {
		result.Issues = issuesOfError(err)
		return result
	}
	if !utf8.Valid(data) {
		result.Issues = []Issue{{
			Code:    CodeInvalidUTF8,
			Path:    "",
			Message: "package index must be valid UTF-8",
		}}
		return result
	}
	root, issue := parseJSON(data)
	if issue != nil {
		result.Issues = []Issue{*issue}
		return result
	}
	if root.kind != jsonObject {
		result.Issues = []Issue{{
			Code:    CodeNotObject,
			Path:    "",
			Message: "package index root must be a JSON object",
		}}
		return result
	}
	schemaValue, present := root.member("schema")
	if !present {
		result.Issues = []Issue{{
			Code:    CodeMissingField,
			Path:    "/schema",
			Message: "required field is missing",
		}}
		return result
	}
	if schemaValue.kind != jsonString {
		result.Issues = []Issue{{
			Code:    CodeInvalidType,
			Path:    "/schema",
			Message: "schema must be a string",
		}}
		return result
	}
	if schemaValue.text != PackageSchemaV1 {
		if major, ok := packageSchemaMajor(schemaValue.text); ok && majorGreater(major, "1") {
			result.NewerClient = true
			return result
		}
		result.Issues = []Issue{{
			Code:    CodeInvalidValue,
			Path:    "/schema",
			Message: "schema must be \"" + PackageSchemaV1 + "\"",
		}}
		return result
	}

	c := newChecker(true)
	pkg := c.trainingPackageRoot(root, input.Reference)
	pkg.Binding = input.Binding
	if pkg.ID != input.Binding.ScenarioID {
		c.report(CodeMismatch, "/binding/scenarioId",
			"the package id must equal the admitted narrative scenarioId")
	}
	if pkg.Revision != input.Binding.ScenarioRevision {
		c.report(CodeMismatch, "/binding/scenarioRevision",
			"the package revision must equal the admitted narrative revision")
	}
	if len(c.issues) > 0 {
		result.Issues = c.issues
		return result
	}
	paths := c.packagePaths(pkg, input.Reference)
	if len(c.issues) > 0 {
		result.Issues = c.issues
		return result
	}
	if issues := c.packageBudgets(pkg, paths); len(issues) > 0 {
		result.Issues = issues
		return result
	}
	bytesByPath, issues := packageAssetBytes(pkg, paths, input.Assets)
	if len(issues) > 0 {
		result.Issues = issues
		return result
	}
	c.parsePackageCodecs(pkg, bytesByPath)
	if len(c.issues) > 0 {
		result.Issues = c.issues
		return result
	}
	c.closeTrainingPackage(pkg, bytesByPath)
	if len(c.issues) > 0 {
		result.Issues = c.issues
		return result
	}
	result.Admitted = true
	result.Package = pkg
	return result
}

// validateTrainingPackageBinding validates the trusted external context and
// the receiving package reference before any bytes are inspected.
func validateTrainingPackageBinding(input TrainingPackageInput) []Issue {
	c := newChecker(true)
	reference := input.Reference
	if !idPattern.MatchString(reference.ID) {
		c.report(CodeInvalidValue, "/reference/id",
			"package id must match ^[a-z0-9][a-z0-9-]{0,47}$")
	}
	if !positiveIntegerLexeme(string(reference.Revision)) {
		c.report(CodeInvalidValue, "/reference/revision",
			"package revision must be a positive canonical integer")
	}
	if !validAssetPath(reference.Path) {
		c.report(CodeInvalidPath, "/reference/path",
			"package path must be a safe project-relative path")
	}
	if !isLowerHex(reference.SHA256, 64) {
		c.report(CodeInvalidValue, "/reference/sha256",
			"package digest must be 64 lowercase hexadecimal characters")
	}
	if !positiveIntegerLexeme(string(reference.Bytes)) {
		c.report(CodeInvalidValue, "/reference/bytes",
			"package byte length must be a positive canonical integer")
	}
	binding := input.Binding
	if binding.Project.Value == nil {
		c.report(CodeMissingContext, "/binding/project",
			"the resolved declaring project identity is required")
	}
	if !binding.ContentPin.Valid() {
		c.report(CodePinMismatch, "/binding/contentPin",
			"the content pin must be one exact immutable pin")
	}
	if !isLowerHex(binding.DeclarationSHA256, 64) {
		c.report(CodeDigestMismatch, "/binding/declarationSha256",
			"the declaration digest must be 64 lowercase hexadecimal characters")
	}
	if !isLowerHex(binding.NarrativeSHA256, 64) {
		c.report(CodeDigestMismatch, "/binding/narrativeSha256",
			"the narrative digest must be 64 lowercase hexadecimal characters")
	}
	if !isLowerHex(binding.PackageSHA256, 64) {
		c.report(CodeDigestMismatch, "/binding/packageSha256",
			"the package digest must be 64 lowercase hexadecimal characters")
	} else if binding.PackageSHA256 != reference.SHA256 {
		c.report(CodeMismatch, "/binding/packageSha256",
			"the bound package digest does not equal the declaring package reference")
	}
	if !idPattern.MatchString(binding.ScenarioID) {
		c.report(CodeInvalidValue, "/binding/scenarioId",
			"the admitted scenario id must match ^[a-z0-9][a-z0-9-]{0,47}$")
	}
	if !positiveIntegerLexeme(string(binding.ScenarioRevision)) {
		c.report(CodeInvalidValue, "/binding/scenarioRevision",
			"the admitted scenario revision must be a positive canonical integer")
	}
	return c.issues
}

// packageSchemaMajor reads a canonical training-package schema major.
func packageSchemaMajor(schema string) (string, bool) {
	if !trainingPackageSchemaPattern.MatchString(schema) {
		return "", false
	}
	return schema[len("datatug-training-package/"):], true
}

// trainingPackageRoot parses the closed root object and every package-local
// record array. It does not parse asset bytes.
func (c *checker) trainingPackageRoot(root *jsonValue, reference PackageReference) *AdmittedTrainingPackage {
	c.rejectUnknown(root, "", packageRootFields)
	pkg := &AdmittedTrainingPackage{
		Schema:    PackageSchemaV1,
		ID:        c.requiredID(root, "", "id"),
		Revision:  c.positiveInteger(root, "", "revision", true),
		Reference: reference,
	}
	if pkg.ID != reference.ID {
		c.report(CodeMismatch, "/id", "package id must equal the declaring package reference id")
	}
	if pkg.Revision != reference.Revision {
		c.report(CodeMismatch, "/revision", "package revision must equal the declaring package reference revision")
	}

	pkg.Assets = c.trainingAssets(c.arrayField(root, "", "assets", true, 0, MaxPackageAssets))
	pkg.FictionalActors = c.trainingActors(c.arrayField(root, "", "fictionalActors", true, 0, 0))
	pkg.Sources = c.trainingSources(c.arrayField(root, "", "sources", true, 0, 0))
	pkg.Cases = c.trainingCases(c.arrayField(root, "", "cases", true, 0, MaxPackageRecords))
	pkg.Incidents = c.trainingIncidents(c.arrayField(root, "", "incidents", true, 0, MaxPackageRecords))
	pkg.Checks = c.trainingChecks(c.arrayField(root, "", "checks", true, 0, MaxPackageRecords))
	pkg.Queries = c.trainingQueries(c.arrayField(root, "", "queries", true, 0, MaxPackageRecords))
	pkg.Challenges = c.trainingChallenges(c.arrayField(root, "", "challenges", true, 0, MaxPackageRecords))
	pkg.Proposals = c.trainingProposals(c.arrayField(root, "", "proposals", true, 0, MaxPackageRecords))
	pkg.PresentationTargets = c.trainingPresentationTargets(c.arrayField(root, "", "presentationTargets", true, 0, MaxPackagePresentationTargets))

	c.uniqueTrainingIDs(assetIDs(pkg.Assets), "assets")
	c.uniqueTrainingIDs(actorIDs(pkg.FictionalActors), "fictionalActors")
	c.uniqueTrainingIDs(sourceIDs(pkg.Sources), "sources")
	c.uniqueTrainingIDs(caseIDs(pkg.Cases), "cases")
	c.uniqueTrainingIDs(incidentIDs(pkg.Incidents), "incidents")
	c.uniqueTrainingIDs(checkIDs(pkg.Checks), "checks")
	c.uniqueTrainingIDs(queryIDs(pkg.Queries), "queries")
	c.uniqueTrainingIDs(challengeIDs(pkg.Challenges), "challenges")
	c.uniqueTrainingIDs(proposalIDs(pkg.Proposals), "proposals")
	c.uniqueTrainingIDs(targetIDs(pkg.PresentationTargets), "presentationTargets")
	return pkg
}

func (c *checker) uniqueTrainingIDs(ids []string, array string) {
	seen := make(map[string]bool, len(ids))
	for index, id := range ids {
		if seen[id] {
			c.report(CodeDuplicateID, pointerIndex("/"+array, index), "id %q is repeated", id)
			continue
		}
		seen[id] = true
	}
}

func (c *checker) trainingAssets(values []*jsonValue) []TrainingPackageAsset {
	assets := make([]TrainingPackageAsset, 0, len(values))
	for index, value := range values {
		assets = append(assets, c.trainingAsset(value, pointerIndex("/assets", index)))
	}
	return assets
}

func (c *checker) trainingAsset(value *jsonValue, path string) TrainingPackageAsset {
	if !c.requireObject(value, path) {
		return TrainingPackageAsset{}
	}
	c.rejectUnknown(value, path, trainingAssetFields)
	asset := TrainingPackageAsset{
		ID:        c.requiredID(value, path, "id"),
		Kind:      c.textValue(value, path, "kind", true),
		MediaType: c.textValue(value, path, "mediaType", true),
		Reference: AssetReference{
			Path:   c.assetPath(value, path, "path", true),
			SHA256: c.lowerDigest(value, path, "sha256", true),
			Bytes:  c.positiveInteger(value, path, "bytes", true),
		},
	}
	requiredMediaType, known := packageCodecMediaTypes[asset.Kind]
	if !known {
		c.report(CodeInvalidValue, pointerChild(path, "kind"),
			"unknown or unreviewed codec kind")
		return asset
	}
	if asset.MediaType != requiredMediaType {
		c.report(CodeInvalidValue, pointerChild(path, "mediaType"),
			"mediaType must equal the codec's required media type %q", requiredMediaType)
	}
	return asset
}

func (c *checker) trainingActors(values []*jsonValue) []TrainingFictionalActor {
	actors := make([]TrainingFictionalActor, 0, len(values))
	for index, value := range values {
		path := pointerIndex("/fictionalActors", index)
		if !c.requireObject(value, path) {
			continue
		}
		c.rejectUnknown(value, path, map[string]bool{"id": true, "name": true})
		actors = append(actors, TrainingFictionalActor{
			ID:   c.requiredID(value, path, "id"),
			Name: c.textLimited(value, path, "name", MaxTrainingTitleChars, true),
		})
	}
	return actors
}

func (c *checker) trainingSources(values []*jsonValue) []TrainingSourceBinding {
	sources := make([]TrainingSourceBinding, 0, len(values))
	for index, value := range values {
		path := pointerIndex("/sources", index)
		if !c.requireObject(value, path) {
			continue
		}
		c.rejectUnknown(value, path, map[string]bool{"id": true, "fixtureAssetId": true})
		sources = append(sources, TrainingSourceBinding{
			ID:             c.requiredID(value, path, "id"),
			FixtureAssetID: c.requiredID(value, path, "fixtureAssetId"),
		})
	}
	return sources
}

func (c *checker) trainingCases(values []*jsonValue) []TrainingCase {
	cases := make([]TrainingCase, 0, len(values))
	for index, value := range values {
		path := pointerIndex("/cases", index)
		if !c.requireObject(value, path) {
			continue
		}
		myCase := TrainingCase{
			ID:          c.requiredID(value, path, "id"),
			Title:       c.textLimited(value, path, "title", MaxTrainingTitleChars, true),
			Status:      c.textValue(value, path, "status", true),
			Context:     c.trainingInputValues(value, path, "context"),
			IncidentIDs: c.trainingIDArray(value, path, "incidentIds", true),
			EvidenceIDs: c.trainingIDArray(value, path, "evidenceIds", true),
			Findings:    c.trainingFindings(value, path),
			Timeline:    c.trainingTimeline(value, path),
		}
		c.rejectUnknown(value, path, map[string]bool{
			"id": true, "title": true, "status": true, "context": true,
			"incidentIds": true, "evidenceIds": true, "findings": true, "timeline": true,
		})
		if !oneOfTraining(myCase.Status, "active", "closed", "archived") {
			c.report(CodeInvalidValue, pointerChild(path, "status"),
				"status must be active, closed or archived")
		}
		c.uniqueCaseLocalIDs(findingIDsOf(myCase.Findings), path, "findings")
		c.uniqueCaseLocalIDs(timelineIDsOf(myCase.Timeline), path, "timeline")
		c.uniqueReferenceIDs(myCase.IncidentIDs, path, "incidentIds")
		c.uniqueReferenceIDs(myCase.EvidenceIDs, path, "evidenceIds")
		cases = append(cases, myCase)
	}
	return cases
}

func (c *checker) uniqueCaseLocalIDs(ids []string, path, name string) {
	seen := make(map[string]bool, len(ids))
	for index, id := range ids {
		if seen[id] {
			c.report(CodeDuplicateID, pointerIndex(pointerChild(path, name), index),
				"id %q is repeated in this Case", id)
		}
		seen[id] = true
	}
}

// uniqueReferenceIDs reports one duplicate per repeated reference id.
func (c *checker) uniqueReferenceIDs(ids []string, path, name string) {
	seen := make(map[string]bool, len(ids))
	for index, id := range ids {
		if seen[id] {
			c.report(CodeDuplicateID, pointerIndex(pointerChild(path, name), index),
				"reference %q is repeated", id)
		}
		seen[id] = true
	}
}

func findingIDsOf(findings []TrainingFinding) []string {
	ids := make([]string, len(findings))
	for index := range findings {
		ids[index] = findings[index].ID
	}
	return ids
}

func timelineIDsOf(entries []TrainingTimelineEntry) []string {
	ids := make([]string, len(entries))
	for index := range entries {
		ids[index] = entries[index].ID
	}
	return ids
}

func (c *checker) trainingFindings(object *jsonValue, path string) []TrainingFinding {
	values := c.arrayField(object, path, "findings", true, 0, MaxCaseFindings)
	findings := make([]TrainingFinding, 0, len(values))
	for index, value := range values {
		findingPath := pointerIndex(pointerChild(path, "findings"), index)
		if !c.requireObject(value, findingPath) {
			continue
		}
		c.rejectUnknown(value, findingPath, map[string]bool{
			"id": true, "text": true, "status": true, "evidenceRefs": true,
		})
		finding := TrainingFinding{
			ID:     c.requiredID(value, findingPath, "id"),
			Text:   c.textValue(value, findingPath, "text", true),
			Status: c.textValue(value, findingPath, "status", true),
		}
		if refs, present := c.optionalArray(value, findingPath, "evidenceRefs"); present {
			finding.HasEvidenceRefs = true
			finding.EvidenceRefs = make([]string, 0, len(refs))
			for refIndex, ref := range refs {
				finding.EvidenceRefs = append(finding.EvidenceRefs,
					c.idValue(ref, pointerIndex(pointerChild(findingPath, "evidenceRefs"), refIndex)))
			}
		}
		findings = append(findings, finding)
	}
	return findings
}

func (c *checker) trainingTimeline(object *jsonValue, path string) []TrainingTimelineEntry {
	values := c.arrayField(object, path, "timeline", true, 0, MaxCaseTimeline)
	entries := make([]TrainingTimelineEntry, 0, len(values))
	for index, value := range values {
		entryPath := pointerIndex(pointerChild(path, "timeline"), index)
		if !c.requireObject(value, entryPath) {
			continue
		}
		c.rejectUnknown(value, entryPath, map[string]bool{
			"id": true, "at": true, "actorId": true, "type": true, "ref": true,
		})
		entry := TrainingTimelineEntry{
			ID:      c.requiredID(value, entryPath, "id"),
			At:      c.textValue(value, entryPath, "at", true),
			ActorID: c.requiredID(value, entryPath, "actorId"),
			Type:    c.textValue(value, entryPath, "type", true),
		}
		entry.Ref = c.trainingTimelineRef(value, entryPath)
		if !oneOfTraining(entry.Type, "evidence-revealed", "finding-recorded", "check-observed") {
			c.report(CodeInvalidValue, pointerChild(entryPath, "type"),
				"type must be evidence-revealed, finding-recorded or check-observed")
		}
		if !validFictionalTimestamp(entry.At) {
			c.report(CodeInvalidValue, pointerChild(entryPath, "at"),
				"at must be a fictional RFC3339 UTC timestamp")
		}
		entries = append(entries, entry)
	}
	return entries
}

func (c *checker) trainingTimelineRef(object *jsonValue, path string) TrainingTimelineRef {
	refPath := pointerChild(path, "ref")
	value, present := object.member("ref")
	if !present {
		c.report(CodeMissingField, refPath, "required field is missing")
		return TrainingTimelineRef{}
	}
	if !c.requireObject(value, refPath) {
		return TrainingTimelineRef{}
	}
	c.rejectUnknown(value, refPath, map[string]bool{"kind": true, "id": true})
	ref := TrainingTimelineRef{
		Kind: c.textValue(value, refPath, "kind", true),
		ID:   c.requiredID(value, refPath, "id"),
	}
	if !oneOfTraining(ref.Kind, "evidence", "finding", "check") {
		c.report(CodeInvalidValue, pointerChild(refPath, "kind"),
			"kind must be evidence, finding or check")
	}
	return ref
}

func (c *checker) trainingIncidents(values []*jsonValue) []TrainingIncident {
	incidents := make([]TrainingIncident, 0, len(values))
	for index, value := range values {
		path := pointerIndex("/incidents", index)
		if !c.requireObject(value, path) {
			continue
		}
		c.rejectUnknown(value, path, map[string]bool{
			"id": true, "title": true, "description": true, "status": true, "evidenceIds": true,
		})
		incident := TrainingIncident{
			ID:          c.requiredID(value, path, "id"),
			Title:       c.textLimited(value, path, "title", MaxTrainingTitleChars, true),
			Description: c.textLimited(value, path, "description", MaxTrainingDescriptionChars, true),
			Status:      c.textValue(value, path, "status", true),
			EvidenceIDs: c.trainingIDArray(value, path, "evidenceIds", true),
		}
		if !oneOfTraining(incident.Status,
			"open", "investigating", "mitigating", "recovering", "resolved", "watching", "closed") {
			c.report(CodeInvalidValue, pointerChild(path, "status"),
				"status must be one of the native incident statuses")
		}
		c.uniqueReferenceIDs(incident.EvidenceIDs, path, "evidenceIds")
		incidents = append(incidents, incident)
	}
	return incidents
}

func (c *checker) trainingChecks(values []*jsonValue) []TrainingCheckRecord {
	checks := make([]TrainingCheckRecord, 0, len(values))
	for index, value := range values {
		path := pointerIndex("/checks", index)
		if !c.requireObject(value, path) {
			continue
		}
		c.rejectUnknown(value, path, map[string]bool{"id": true, "definitionAssetId": true})
		checks = append(checks, TrainingCheckRecord{
			ID:                c.textValue(value, path, "id", true),
			DefinitionAssetID: c.requiredID(value, path, "definitionAssetId"),
		})
	}
	return checks
}

func (c *checker) trainingQueries(values []*jsonValue) []TrainingQueryRecord {
	queries := make([]TrainingQueryRecord, 0, len(values))
	for index, value := range values {
		path := pointerIndex("/queries", index)
		if !c.requireObject(value, path) {
			continue
		}
		c.rejectUnknown(value, path, map[string]bool{
			"id": true, "metadataAssetId": true, "bodyAssetId": true, "sourceId": true,
		})
		query := TrainingQueryRecord{
			ID:              c.textValue(value, path, "id", true),
			MetadataAssetID: c.requiredID(value, path, "metadataAssetId"),
			BodyAssetID:     c.requiredID(value, path, "bodyAssetId"),
			SourceID:        c.requiredID(value, path, "sourceId"),
		}
		if !detective.ValidQueryRef(query.ID) {
			c.report(CodeInvalidValue, pointerChild(path, "id"),
				"id must be an owner-valid query identifier, including folder qualification")
		}
		queries = append(queries, query)
	}
	return queries
}

func (c *checker) trainingChallenges(values []*jsonValue) []TrainingChallengeRecord {
	challenges := make([]TrainingChallengeRecord, 0, len(values))
	for index, value := range values {
		path := pointerIndex("/challenges", index)
		if !c.requireObject(value, path) {
			continue
		}
		c.rejectUnknown(value, path, map[string]bool{
			"id": true, "revision": true, "manifestAssetId": true, "fixtures": true,
		})
		challengeRecord := TrainingChallengeRecord{
			ID:              c.textValue(value, path, "id", true),
			Revision:        c.boundedPositiveInteger(value, path, "revision", 1_000_000),
			ManifestAssetID: c.requiredID(value, path, "manifestAssetId"),
		}
		if !challengeIDPattern.MatchString(challengeRecord.ID) {
			c.report(CodeInvalidValue, pointerChild(path, "id"),
				"id must be a challenge-execution lowercase slug")
		}
		challengeRecord.Fixtures = c.trainingChallengeFixtures(value, path)
		c.uniqueFixtureIDs(challengeRecord.Fixtures, path)
		challenges = append(challenges, challengeRecord)
	}
	return challenges
}

func (c *checker) uniqueFixtureIDs(fixtures []TrainingChallengeFixture, path string) {
	seen := make(map[string]bool, len(fixtures))
	for index, fixture := range fixtures {
		if seen[fixture.FixtureID] {
			c.report(CodeDuplicateID, pointerIndex(pointerChild(path, "fixtures"), index),
				"fixture id %q is repeated", fixture.FixtureID)
		}
		seen[fixture.FixtureID] = true
	}
}

func (c *checker) trainingChallengeFixtures(object *jsonValue, path string) []TrainingChallengeFixture {
	values := c.arrayField(object, path, "fixtures", true, 0, 0)
	fixtures := make([]TrainingChallengeFixture, 0, len(values))
	for index, value := range values {
		fixturePath := pointerIndex(pointerChild(path, "fixtures"), index)
		if !c.requireObject(value, fixturePath) {
			continue
		}
		c.rejectUnknown(value, fixturePath, map[string]bool{"fixtureId": true, "assetId": true})
		fixtures = append(fixtures, TrainingChallengeFixture{
			FixtureID: c.textValue(value, fixturePath, "fixtureId", true),
			AssetID:   c.requiredID(value, fixturePath, "assetId"),
		})
	}
	return fixtures
}

func (c *checker) trainingProposals(values []*jsonValue) []TrainingProposal {
	proposals := make([]TrainingProposal, 0, len(values))
	for index, value := range values {
		path := pointerIndex("/proposals", index)
		if !c.requireObject(value, path) {
			continue
		}
		c.rejectUnknown(value, path, map[string]bool{
			"id": true, "kind": true, "definitionId": true, "evidenceIds": true, "rationale": true,
		})
		proposal := TrainingProposal{
			ID:           c.requiredID(value, path, "id"),
			Kind:         c.textValue(value, path, "kind", true),
			DefinitionID: c.textValue(value, path, "definitionId", true),
			EvidenceIDs:  c.trainingIDArray(value, path, "evidenceIds", true),
			Rationale:    c.textLimited(value, path, "rationale", MaxTrainingDescriptionChars, true),
		}
		if !oneOfTraining(proposal.Kind, "query", "check", "watch") {
			c.report(CodeInvalidValue, pointerChild(path, "kind"),
				"kind must be query, check or watch")
		}
		c.uniqueReferenceIDs(proposal.EvidenceIDs, path, "evidenceIds")
		proposals = append(proposals, proposal)
	}
	return proposals
}

func (c *checker) trainingPresentationTargets(values []*jsonValue) []TrainingPresentationTarget {
	targets := make([]TrainingPresentationTarget, 0, len(values))
	for index, value := range values {
		path := pointerIndex("/presentationTargets", index)
		if !c.requireObject(value, path) {
			continue
		}
		c.rejectUnknown(value, path, map[string]bool{"id": true, "kind": true, "refId": true})
		target := TrainingPresentationTarget{
			ID:    c.requiredID(value, path, "id"),
			Kind:  c.textValue(value, path, "kind", true),
			RefID: c.textValue(value, path, "refId", true),
		}
		if !oneOfTraining(target.Kind, "case", "incident", "evidence", "check", "proposal") {
			c.report(CodeInvalidValue, pointerChild(path, "kind"),
				"kind must be case, incident, evidence, check or proposal")
		}
		targets = append(targets, target)
	}
	return targets
}

// trainingInputValues parses the closed native InputValue map used by a Case
// context.
func (c *checker) trainingInputValues(object *jsonValue, path, name string) map[string]detective.InputValue {
	mapPath := pointerChild(path, name)
	value, present := object.member(name)
	if !present {
		c.report(CodeMissingField, mapPath, "required field is missing")
		return nil
	}
	if !c.requireObject(value, mapPath) {
		return nil
	}
	values := make(map[string]detective.InputValue, len(value.object))
	for _, member := range value.object {
		if !detective.ValidParameterName(member.key) {
			c.report(CodeInvalidValue, pointerChild(mapPath, member.key),
				"context name must be a valid native parameter name")
			continue
		}
		values[member.key] = c.trainingInputValue(member.value, pointerChild(mapPath, member.key))
	}
	return values
}

// idArray parses a required array of walkthrough IDs.
func (c *checker) trainingIDArray(object *jsonValue, path, name string, required bool) []string {
	return c.idArray(object, path, name, required, 0, 0)
}

// textValue reads a required or optional string, returning "" after reporting
// the matching issue. It exists so closed-object record literals stay legible.
func (c *checker) textValue(object *jsonValue, path, name string, required bool) string {
	text, _ := c.text(object, path, name, required)
	return text
}

// optionalTextField distinguishes an absent optional string from a present one.
func (c *checker) optionalTextField(object *jsonValue, path, name string) (*string, bool) {
	value, present := object.member(name)
	if !present {
		return nil, true
	}
	if value.kind != jsonString {
		c.report(CodeInvalidType, pointerChild(path, name), "must be a string")
		return nil, false
	}
	text := value.text
	return &text, true
}

// canonicalInt64Field reads a canonical non-negative integer that must fit a
// native int64 field.
func (c *checker) canonicalInt64Field(object *jsonValue, path, name string) (int64, bool) {
	value, present := object.member(name)
	if !present {
		c.report(CodeMissingField, pointerChild(path, name), "required field is missing")
		return 0, false
	}
	if value.kind != jsonNumber || !canonicalIntegerPattern.MatchString(value.number) {
		c.report(CodeInvalidValue, pointerChild(path, name),
			"must be a canonical non-negative integer")
		return 0, false
	}
	parsed, err := strconv.ParseInt(value.number, 10, 64)
	if err != nil {
		c.report(CodeInvalidValue, pointerChild(path, name), "must fit a native 64-bit integer")
		return 0, false
	}
	return parsed, true
}

// boundedPositiveInteger reads a positive canonical integer no greater than
// max, keeping the exact lexeme.
func (c *checker) boundedPositiveInteger(object *jsonValue, path, name string, max int64) Integer {
	value := c.positiveInteger(object, path, name, true)
	if value == "" {
		return ""
	}
	parsed, ok := integerAsInt(string(value), max)
	if !ok || parsed == 0 {
		c.report(CodeInvalidValue, pointerChild(path, name),
			"must be between 1 and %d", max)
		return ""
	}
	return value
}

// nonNegativeInteger keeps a canonical non-negative integer lexeme.
func (c *checker) nonNegativeInteger(object *jsonValue, path, name string) (Integer, bool) {
	value, present := object.member(name)
	if !present {
		c.report(CodeMissingField, pointerChild(path, name), "required field is missing")
		return "", false
	}
	if value.kind != jsonNumber || !canonicalIntegerPattern.MatchString(value.number) {
		c.report(CodeInvalidValue, pointerChild(path, name),
			"must be a canonical non-negative integer")
		return "", false
	}
	return Integer(value.number), true
}

// nonNegativeIntegerField reads an optional non-negative integer, keeping the
// exact lexeme and the absent/present distinction.
// (No package field currently needs it; the Watch cooldown is required.)

func oneOfTraining(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

// validFictionalTimestamp requires one RFC3339 UTC instant.
func validFictionalTimestamp(text string) bool {
	if !strings.HasSuffix(text, "Z") {
		return false
	}
	_, err := parseRFC3339(text)
	return err == nil
}

func assetIDs(assets []TrainingPackageAsset) []string {
	ids := make([]string, len(assets))
	for index := range assets {
		ids[index] = assets[index].ID
	}
	return ids
}

func actorIDs(actors []TrainingFictionalActor) []string {
	ids := make([]string, len(actors))
	for index := range actors {
		ids[index] = actors[index].ID
	}
	return ids
}

func sourceIDs(sources []TrainingSourceBinding) []string {
	ids := make([]string, len(sources))
	for index := range sources {
		ids[index] = sources[index].ID
	}
	return ids
}

func caseIDs(cases []TrainingCase) []string {
	ids := make([]string, len(cases))
	for index := range cases {
		ids[index] = cases[index].ID
	}
	return ids
}

func incidentIDs(incidents []TrainingIncident) []string {
	ids := make([]string, len(incidents))
	for index := range incidents {
		ids[index] = incidents[index].ID
	}
	return ids
}

func checkIDs(checks []TrainingCheckRecord) []string {
	ids := make([]string, len(checks))
	for index := range checks {
		ids[index] = checks[index].ID
	}
	return ids
}

func queryIDs(queries []TrainingQueryRecord) []string {
	ids := make([]string, len(queries))
	for index := range queries {
		ids[index] = queries[index].ID
	}
	return ids
}

func challengeIDs(challenges []TrainingChallengeRecord) []string {
	ids := make([]string, len(challenges))
	for index := range challenges {
		ids[index] = challenges[index].ID
	}
	return ids
}

func proposalIDs(proposals []TrainingProposal) []string {
	ids := make([]string, len(proposals))
	for index := range proposals {
		ids[index] = proposals[index].ID
	}
	return ids
}

func targetIDs(targets []TrainingPresentationTarget) []string {
	ids := make([]string, len(targets))
	for index := range targets {
		ids[index] = targets[index].ID
	}
	return ids
}

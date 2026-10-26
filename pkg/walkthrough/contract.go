package walkthrough

import "sort"

// SchemaV1 is the ordinary schema this package admits through
// [ParseDeclaration]. Any other major is reported as needing a newer DataTug
// rather than parsed.
const SchemaV1 = "datatug-walkthrough/1"

// SchemaV2 is the opt-in training extension admitted only through
// [ParseTrainingDeclaration]. The ordinary entry point keeps reporting it as a
// newer-client declaration.
const SchemaV2 = "datatug-walkthrough/2"

// MaxAttemptIDLen is the owner limit for the external checkpoint envelope's
// opaque execution-owned attempt id: 1..128 printable ASCII characters.
const MaxAttemptIDLen = 128

// CompletionWholeChallenge is the only checkpoint completion kind this version
// admits. Part-level, multiple or alternative mappings require a separately
// reviewed owning contract.
const CompletionWholeChallenge = "whole-challenge"

// Ordinary /1 bounds from the normative owner declaration table.
const (
	MaxDeclarationBytes = 64 << 10
	MaxChapters         = 20
	MaxStepsPerChapter  = 30
	MaxSteps            = 200
	MaxTitleChars       = 120
	MaxTextChars        = 400
)

// Support is the admission status of a target, condition or step.
type Support string

const (
	// SupportSupported is a fully validated /1 variant.
	SupportSupported Support = "supported"
	// SupportNeedsNewer is an unknown target/condition kind retained only so
	// the rest of the walkthrough stays usable. Its opaque body is not exposed
	// or executed.
	SupportNeedsNewer Support = "needs-newer"
)

// Integer is a canonical, non-negative decimal integer lexeme. Keeping the
// lexeme rather than a machine integer preserves versions and row thresholds
// exactly in both Go and TypeScript without inventing a representability cap.
type Integer string

func (i Integer) String() string { return string(i) }

// ScalarKind is the closed JSON scalar kind of a literal value.
type ScalarKind string

const (
	ScalarNull    ScalarKind = "null"
	ScalarBoolean ScalarKind = "boolean"
	ScalarNumber  ScalarKind = "number"
	ScalarString  ScalarKind = "string"
)

// Scalar is one JSON scalar. Number retains the exact input lexeme.
type Scalar struct {
	Kind   ScalarKind
	Bool   bool
	Number string
	Text   string
}

// Value is either a literal scalar or a captured reference with its mandatory
// literal default.
type Value struct {
	Captured string
	Literal  *Scalar
	Default  *Scalar
}

// IsCaptured reports whether the value references an earlier capture.
func (v Value) IsCaptured() bool { return v.Captured != "" }

// NamedValue is one entry of a parameters, key or filter object. Entries are
// kept sorted by name so normalized projections are deterministic.
type NamedValue struct {
	Name  string
	Value Value
}

// TableRecordTarget is a table of a project source, optionally with a record
// open.
type TableRecordTarget struct {
	Source string
	Table  string
	Key    []NamedValue
}

// QueryTarget is a saved query with optional parameter values. It never runs
// the query.
type QueryTarget struct {
	QueryID    string
	Parameters []NamedValue
}

// BoardTarget is a saved board; Tutorial adds the board-action tutorial mode.
type BoardTarget struct {
	BoardID    string
	Parameters []NamedValue
	Tutorial   bool
}

// CardTarget is a saved card with optional parameter values.
type CardTarget struct {
	CardID     string
	Parameters []NamedValue
}

// RelatedTarget is a record with its Related panel open.
type RelatedTarget struct {
	Source  string
	Table   string
	Key     []NamedValue
	Meaning string
}

// LogSourceTarget is the generic log reader grid.
type LogSourceTarget struct {
	Source string
	Stream string
	Filter []NamedValue
}

// PublicDataProviderTarget is a provider's OVDB Directory entry.
type PublicDataProviderTarget struct {
	Provider string
}

// IncidentTarget is an incident in the resolved project, or its incident list
// when Incident is absent or null.
type IncidentTarget struct {
	Incident *Value
}

// CheckTarget is a Check and its runs.
type CheckTarget struct {
	CheckID string
}

// ChallengeTarget is the exact published challenge revision this step targets.
// The id/revision follow the challenge-execution owner contract, not the
// shorter walkthrough ID grammar.
type ChallengeTarget struct {
	ChallengeID string
	Revision    Integer
}

// TrainingCaseTarget is existing generic Case presentation through a training
// adapter. TrainingPackage must equal the declaration's top-level package
// reference field for field.
type TrainingCaseTarget struct {
	ManifestCaseID  string
	TrainingPackage PackageReference
}

// Target is one closed-registry target variant. Exactly the pointer matching
// Kind is populated for a supported kind; an unknown kind has Supported false
// and no variant data.
type Target struct {
	Kind               string
	Supported          bool
	TableRecord        *TableRecordTarget
	Query              *QueryTarget
	Board              *BoardTarget
	Card               *CardTarget
	Related            *RelatedTarget
	LogSource          *LogSourceTarget
	PublicDataProvider *PublicDataProviderTarget
	Incident           *IncidentTarget
	Check              *CheckTarget
	None               *struct{}
	Challenge          *ChallengeTarget
	TrainingCase       *TrainingCaseTarget
}

// ParameterValueCondition observes a query, board or card parameter.
type ParameterValueCondition struct {
	Name   string
	Equals *Scalar
}

// RecordSelectedCondition observes a selected record or query result row.
type RecordSelectedCondition struct {
	Field  string
	Equals *Scalar
}

// QueryRunCondition observes a completed saved-query run.
type QueryRunCondition struct {
	MinRows *Integer
}

// PanelOpenCondition observes a panel belonging to the target asset.
type PanelOpenCondition struct {
	Panel string
}

// FilterAppliedCondition observes a log or table filter.
type FilterAppliedCondition struct {
	Field  string
	Equals *Scalar
}

// CheckRunCondition observes a Check completion with an optional outcome.
type CheckRunCondition struct {
	Outcome string
}

// AssetExistsCondition observes that an asset is readable in the resolved
// project.
type AssetExistsCondition struct {
	Asset string
	ID    string
}

// BoardTutorialCondition observes board tutorial progress.
type BoardTutorialCondition struct {
	UntilCard string
}

// CheckpointMapping is the walkthrough-owned whole-challenge checkpoint
// mapping. Completion is always "whole-challenge"; part-level or
// narrative-defined mappings are not representable.
type CheckpointMapping struct {
	ID         string
	Completion string
}

// DialogueFinishedCondition is presentation-step completion of the pinned Play
// segment. It never awards a challenge pass.
type DialogueFinishedCondition struct{}

// ChallengePassedCondition observes whole-attempt completion of the step's exact
// challenge target. It carries exactly one walkthrough-owned checkpoint mapping.
type ChallengePassedCondition struct {
	Checkpoints []CheckpointMapping
}

// Condition is one closed-registry completion condition. Exactly the pointer
// matching Kind is populated for a supported kind; an unknown kind has
// Supported false and no variant data.
type Condition struct {
	Kind             string
	Supported        bool
	ParameterValue   *ParameterValueCondition
	RecordSelected   *RecordSelectedCondition
	QueryRun         *QueryRunCondition
	PanelOpen        *PanelOpenCondition
	FilterApplied    *FilterAppliedCondition
	CheckRun         *CheckRunCondition
	AssetExists      *AssetExistsCondition
	BoardTutorial    *BoardTutorialCondition
	DialogueFinished *DialogueFinishedCondition
	ChallengePassed  *ChallengePassedCondition
}

// Exit is the walkthrough finish (or a chapter's next exit). Exactly one
// variant is set.
type Exit struct {
	Kind        string
	Chapter     string
	Walkthrough string
	Target      *Target
}

// Step is one ordered walkthrough step. Kind and SegmentID exist only in the
// /2 training profile; ordinary /1 steps leave Kind empty.
type Step struct {
	ID        string
	Kind      string
	SegmentID string
	Title     string
	Why       string
	Hint      string
	Modes     []string
	Target    Target
	Done      Condition
	Capture   string
	Needs     string
	Support   Support
}

// Chapter is one ordered chapter. EntryStepID is the first Explore step whose
// target is not none (an unknown target kind still declares the entry).
type Chapter struct {
	ID             string
	FormerIDs      []string
	Title          string
	Why            string
	Requires       string
	RequiresReason string
	ContinuesFrom  string
	Readiness      *Condition
	Steps          []Step
	EntryStepID    string
}

// Declaration is a fully validated ordinary /1 walkthrough. Revision is the
// lowercase SHA-256 of the exact input bytes.
type Declaration struct {
	Schema   string
	ID       string
	Version  Integer
	Title    string
	Summary  string
	Chapters []Chapter
	Finish   Exit
	Revision string
}

// TrainingDeclaration is a structurally valid datatug-walkthrough/2
// declaration. It is not a resolved admission: package, Case, challenge and
// source-context authority are established by [ResolveTraining].
type TrainingDeclaration struct {
	Schema          string
	ID              string
	Version         Integer
	Title           string
	Summary         string
	TrainingPackage PackageReference
	Narrative       AssetReference
	Chapters        []Chapter
	Finish          Exit
	Revision        string
}

// DeclarationPath returns the exact project-relative location of a
// walkthrough declaration.
func DeclarationPath(id string) string {
	return "walkthroughs/" + id + "/walkthrough.json"
}

var knownTargetKinds = map[string]bool{
	"table-record":         true,
	"query":                true,
	"board":                true,
	"card":                 true,
	"related":              true,
	"log-source":           true,
	"public-data-provider": true,
	"incident":             true,
	"check":                true,
	"none":                 true,
}

var knownConditionKinds = map[string]bool{
	"route-reached":   true,
	"parameter-value": true,
	"record-selected": true,
	"query-run":       true,
	"panel-open":      true,
	"filter-applied":  true,
	"check-run":       true,
	"incident-open":   true,
	"asset-exists":    true,
	"board-tutorial":  true,
	"acknowledged":    true,
}

// KnownTargetKind reports whether kind is in the ordinary /1 target registry.
func KnownTargetKind(kind string) bool { return knownTargetKinds[kind] }

// KnownConditionKind reports whether kind is in the ordinary /1 condition
// registry.
func KnownConditionKind(kind string) bool { return knownConditionKinds[kind] }

// Projection renders the stable, language-neutral normalized view used by the
// shared conformance corpus. It exposes defaults, references, support status
// and the exact digest-related identity, but no presentation prose.
func (d *Declaration) Projection() map[string]any {
	chapters := make([]any, len(d.Chapters))
	for index := range d.Chapters {
		chapters[index] = projectChapter(d.Chapters[index])
	}
	return map[string]any{
		"id":       d.ID,
		"version":  string(d.Version),
		"summary":  d.Summary,
		"chapters": chapters,
		"finish":   projectExit(d.Finish),
	}
}

// Projection renders the stable, language-neutral normalized view of a /2
// declaration used by the shared conformance corpus. It extends the ordinary
// projection with the package and narrative references, the step kind, the
// dialogue segment id and the checkpoint mapping.
func (d *TrainingDeclaration) Projection() map[string]any {
	chapters := make([]any, len(d.Chapters))
	for index := range d.Chapters {
		chapters[index] = projectTrainingChapter(d.Chapters[index])
	}
	return map[string]any{
		"id":              d.ID,
		"version":         string(d.Version),
		"summary":         d.Summary,
		"trainingPackage": projectPackageReference(d.TrainingPackage),
		"narrative":       projectAssetReference(d.Narrative),
		"chapters":        chapters,
		"finish":          projectExit(d.Finish),
	}
}

func projectAssetReference(ref AssetReference) map[string]any {
	return map[string]any{
		"path":   ref.Path,
		"sha256": ref.SHA256,
		"bytes":  string(ref.Bytes),
	}
}

func projectPackageReference(ref PackageReference) map[string]any {
	return map[string]any{
		"id":       ref.ID,
		"revision": string(ref.Revision),
		"path":     ref.Path,
		"sha256":   ref.SHA256,
		"bytes":    string(ref.Bytes),
	}
}

func projectChapter(chapter Chapter) map[string]any {
	return projectChapterWith(chapter, projectStep)
}

func projectTrainingChapter(chapter Chapter) map[string]any {
	return projectChapterWith(chapter, projectTrainingStep)
}

func projectChapterWith(chapter Chapter, projectStep func(Step) map[string]any) map[string]any {
	former := append([]string{}, chapter.FormerIDs...)
	steps := make([]any, len(chapter.Steps))
	for index := range chapter.Steps {
		steps[index] = projectStep(chapter.Steps[index])
	}
	var readiness any
	if chapter.Readiness != nil {
		readiness = projectCondition(*chapter.Readiness)
	}
	return map[string]any{
		"id":             chapter.ID,
		"formerIds":      former,
		"requires":       chapter.Requires,
		"requiresReason": chapter.RequiresReason,
		"continuesFrom":  chapter.ContinuesFrom,
		"readiness":      readiness,
		"entryStepId":    chapter.EntryStepID,
		"steps":          steps,
	}
}

func projectStep(step Step) map[string]any {
	modes := append([]string{}, step.Modes...)
	return map[string]any{
		"id":      step.ID,
		"modes":   modes,
		"support": string(step.Support),
		"capture": step.Capture,
		"needs":   step.Needs,
		"target":  projectTarget(step.Target),
		"done":    projectCondition(step.Done),
	}
}

func projectTrainingStep(step Step) map[string]any {
	projected := projectStep(step)
	projected["kind"] = step.Kind
	projected["segmentId"] = step.SegmentID
	return projected
}

func projectTarget(target Target) map[string]any {
	projected := map[string]any{
		"kind":    target.Kind,
		"support": string(supportOf(target.Supported)),
	}
	switch {
	case target.TableRecord != nil:
		projected["source"] = target.TableRecord.Source
		projected["table"] = target.TableRecord.Table
		projected["key"] = projectNamedValues(target.TableRecord.Key)
	case target.Query != nil:
		projected["queryId"] = target.Query.QueryID
		projected["parameters"] = projectNamedValues(target.Query.Parameters)
	case target.Board != nil:
		projected["boardId"] = target.Board.BoardID
		projected["parameters"] = projectNamedValues(target.Board.Parameters)
		projected["tutorial"] = target.Board.Tutorial
	case target.Card != nil:
		projected["cardId"] = target.Card.CardID
		projected["parameters"] = projectNamedValues(target.Card.Parameters)
	case target.Related != nil:
		projected["source"] = target.Related.Source
		projected["table"] = target.Related.Table
		projected["key"] = projectNamedValues(target.Related.Key)
		projected["meaning"] = target.Related.Meaning
	case target.LogSource != nil:
		projected["source"] = target.LogSource.Source
		projected["stream"] = target.LogSource.Stream
		projected["filter"] = projectNamedValues(target.LogSource.Filter)
	case target.PublicDataProvider != nil:
		projected["provider"] = target.PublicDataProvider.Provider
	case target.Incident != nil:
		if target.Incident.Incident == nil {
			projected["incident"] = nil
		} else {
			projected["incident"] = projectValue(*target.Incident.Incident)
		}
	case target.Check != nil:
		projected["checkId"] = target.Check.CheckID
	case target.Challenge != nil:
		projected["challengeId"] = target.Challenge.ChallengeID
		projected["revision"] = string(target.Challenge.Revision)
	case target.TrainingCase != nil:
		projected["manifestCaseId"] = target.TrainingCase.ManifestCaseID
		projected["trainingPackage"] = projectPackageReference(target.TrainingCase.TrainingPackage)
	}
	return projected
}

func projectCondition(condition Condition) map[string]any {
	projected := map[string]any{
		"kind":    condition.Kind,
		"support": string(supportOf(condition.Supported)),
	}
	switch {
	case condition.ParameterValue != nil:
		projected["name"] = condition.ParameterValue.Name
		projected["equals"] = projectScalarPointer(condition.ParameterValue.Equals)
	case condition.RecordSelected != nil:
		projected["field"] = condition.RecordSelected.Field
		projected["equals"] = projectScalarPointer(condition.RecordSelected.Equals)
	case condition.QueryRun != nil:
		if condition.QueryRun.MinRows == nil {
			projected["minRows"] = nil
		} else {
			projected["minRows"] = string(*condition.QueryRun.MinRows)
		}
	case condition.PanelOpen != nil:
		projected["panel"] = condition.PanelOpen.Panel
	case condition.FilterApplied != nil:
		projected["field"] = condition.FilterApplied.Field
		projected["equals"] = projectScalarPointer(condition.FilterApplied.Equals)
	case condition.CheckRun != nil:
		if condition.CheckRun.Outcome == "" {
			projected["outcome"] = nil
		} else {
			projected["outcome"] = condition.CheckRun.Outcome
		}
	case condition.AssetExists != nil:
		projected["asset"] = condition.AssetExists.Asset
		projected["id"] = condition.AssetExists.ID
	case condition.BoardTutorial != nil:
		projected["untilCard"] = condition.BoardTutorial.UntilCard
	case condition.ChallengePassed != nil:
		checkpoints := make([]any, len(condition.ChallengePassed.Checkpoints))
		for index, checkpoint := range condition.ChallengePassed.Checkpoints {
			checkpoints[index] = map[string]any{
				"id":         checkpoint.ID,
				"completion": checkpoint.Completion,
			}
		}
		projected["checkpoints"] = checkpoints
	}
	return projected
}

func projectExit(exit Exit) map[string]any {
	switch exit.Kind {
	case "chapter":
		return map[string]any{"kind": "chapter", "chapter": exit.Chapter}
	case "walkthrough":
		return map[string]any{"kind": "walkthrough", "walkthrough": exit.Walkthrough}
	case "target":
		return map[string]any{"kind": "target", "target": projectTarget(*exit.Target)}
	default:
		return map[string]any{"kind": "index"}
	}
}

func projectNamedValues(values []NamedValue) []any {
	sorted := append([]NamedValue{}, values...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	projected := make([]any, len(sorted))
	for index := range sorted {
		projected[index] = map[string]any{
			"name":  sorted[index].Name,
			"value": projectValue(sorted[index].Value),
		}
	}
	return projected
}

func projectValue(value Value) map[string]any {
	if value.IsCaptured() {
		return map[string]any{
			"kind":    "captured",
			"capture": value.Captured,
			"default": projectScalarPointer(value.Default),
		}
	}
	return map[string]any{"kind": "literal", "value": projectScalarPointer(value.Literal)}
}

func projectScalarPointer(scalar *Scalar) any {
	if scalar == nil {
		return nil
	}
	switch scalar.Kind {
	case ScalarNull:
		return map[string]any{"kind": "null"}
	case ScalarBoolean:
		return map[string]any{"kind": "boolean", "value": scalar.Bool}
	case ScalarNumber:
		return map[string]any{"kind": "number", "lexeme": scalar.Number}
	case ScalarString:
		return map[string]any{"kind": "string", "value": scalar.Text}
	default:
		return nil
	}
}

func supportOf(supported bool) Support {
	if supported {
		return SupportSupported
	}
	return SupportNeedsNewer
}

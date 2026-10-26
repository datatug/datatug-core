package walkthrough

import (
	"strings"
)

// StoryRegistryEntry is the strict structural shape of one public story
// registry entry. Shape admission only: canonical immutable URL, public scope
// and chapter lookup are owner-established by [AdmitStoryChapter].
type StoryRegistryEntry struct {
	StoryID           string
	Revision          Integer
	ProjectURL        string
	WalkthroughID     string
	ContentPin        ContentPin
	DeclarationSHA256 string
}

// StoryRegistryResult is the outcome of the strict shape parse.
type StoryRegistryResult struct {
	Entry  *StoryRegistryEntry
	Issues []Issue
}

// Err returns nil for a parsed entry or the aggregate recoverable refusal.
func (r StoryRegistryResult) Err() error {
	if r.Entry != nil {
		return nil
	}
	return &Error{Issues: r.Issues}
}

// DeclaredWalkthrough is the admitted walkthrough identity a registry entry
// must resolve to: exact ID, exact revision and ordered declared chapters.
type DeclaredWalkthrough struct {
	ID       string
	Revision string
	Chapters []string
}

// RouteDescriptor is the resolved public alias target. It names the declared
// project, walkthrough and chapter and retains the pins; it deliberately does
// not invent another walkthrough URL or router.
type RouteDescriptor struct {
	StoryID           string
	Revision          Integer
	ProjectURL        string
	WalkthroughID     string
	ChapterID         string
	ContentPin        ContentPin
	DeclarationSHA256 string
}

// RegistryAdmission is the owner-context admission outcome. Route is non-nil
// only when the alias may resolve.
type RegistryAdmission struct {
	Route  *RouteDescriptor
	Issues []Issue
}

// Err returns nil for an admitted alias or the aggregate recoverable refusal.
func (r RegistryAdmission) Err() error {
	if r.Route != nil {
		return nil
	}
	return &Error{Issues: r.Issues}
}

var storyRegistryFields = map[string]bool{
	"storyId": true, "revision": true, "projectUrl": true,
	"walkthroughId": true, "contentPin": true, "declarationSha256": true,
}

// ParseStoryRegistryEntry parses exactly one closed registry entry shape.
func ParseStoryRegistryEntry(data []byte) StoryRegistryResult {
	_, root, issue := decodeDeclaration(data)
	if issue != nil {
		return StoryRegistryResult{Issues: []Issue{*issue}}
	}
	checker := newChecker(true)
	checker.rejectUnknown(root, "", storyRegistryFields)

	entry := &StoryRegistryEntry{
		StoryID:       checker.requiredID(root, "", "storyId"),
		Revision:      checker.positiveInteger(root, "", "revision", true),
		WalkthroughID: checker.requiredID(root, "", "walkthroughId"),
	}
	if url, ok := checker.text(root, "", "projectUrl", true); ok {
		if url == "" {
			checker.report(CodeInvalidValue, "/projectUrl", "must not be empty")
		}
		entry.ProjectURL = url
	}
	if value, ok := root.member("contentPin"); ok {
		entry.ContentPin = checker.contentPin(value, "/contentPin")
	} else {
		checker.report(CodeMissingField, "/contentPin", "required field is missing")
	}
	entry.DeclarationSHA256 = checker.hexDigest(root, "", "declarationSha256", true)

	if len(checker.issues) > 0 {
		return StoryRegistryResult{Issues: checker.issues}
	}
	return StoryRegistryResult{Entry: entry}
}

// Projection is the stable normalized view used by the shared corpus.
func (e StoryRegistryEntry) Projection() map[string]any {
	return map[string]any{
		"storyId":           e.StoryID,
		"revision":          string(e.Revision),
		"projectUrl":        e.ProjectURL,
		"walkthroughId":     e.WalkthroughID,
		"contentPin":        projectContentPin(e.ContentPin),
		"declarationSha256": e.DeclarationSHA256,
	}
}

// AdmitStoryChapter admits one public alias for one declared chapter. It
// requires an owner-established canonical immutable project address, an
// owner-established public scope, exact pin agreement and a declared chapter.
// A private or unknown scope, a branch/HEAD/default address, a historical pin
// mismatch or an undeclared chapter refuses; nothing falls back to latest.
func AdmitStoryChapter(
	entry StoryRegistryEntry,
	chapterID string,
	declared DeclaredWalkthrough,
	authority ProjectAuthority,
) RegistryAdmission {
	if authority == nil {
		return registryRefusal(CodeMissingContext, "", "a trusted project authority is required")
	}
	if !containsString(declared.Chapters, chapterID) {
		return registryRefusal(CodeInvalidReference, "",
			"the chapter is not declared by the walkthrough")
	}
	if declared.ID != entry.WalkthroughID {
		return registryRefusal(CodeMismatch, "",
			"the registry walkthrough id does not equal the admitted walkthrough id")
	}
	if strings.ToLower(declared.Revision) != entry.DeclarationSHA256 {
		return registryRefusal(CodeDigestMismatch, "",
			"the admitted declaration digest does not equal the registry declarationSha256")
	}
	if !entry.ContentPin.Valid() {
		return registryRefusal(CodePinMismatch, "",
			"the registry content pin is not one exact immutable pin")
	}
	// A digest source pins the exact declaration bytes: a shape-valid entry
	// whose digest pin and declaration digest disagree is not an admitted alias.
	if entry.ContentPin.Kind == PinDigest && entry.ContentPin.SHA256 != entry.DeclarationSHA256 {
		return registryRefusal(CodePinMismatch, "",
			"a digest content pin must equal the declaration digest")
	}
	binding, err := authority.BindProject(entry.ProjectURL)
	if err != nil {
		return registryRefusal(CodeNotAdmitted, "", err.Error())
	}
	if !binding.Public {
		return registryRefusal(CodeIneligible, "",
			"the project scope is not established as publicly readable")
	}
	if binding.CanonicalURL != entry.ProjectURL {
		return registryRefusal(CodeMismatch, "",
			"the registry projectUrl is not the owner's canonical address")
	}
	switch entry.ContentPin.Kind {
	case PinGit:
		if binding.Commit == "" {
			return registryRefusal(CodePinMismatch, "",
				"the project reference is not pinned to one immutable commit")
		}
		if binding.Commit != entry.ContentPin.Commit {
			return registryRefusal(CodePinMismatch, "",
				"the project commit does not agree with the registry content pin")
		}
	case PinDigest:
		if binding.Commit != "" || !binding.DigestAddressed {
			return registryRefusal(CodePinMismatch, "",
				"the project source is not the digest-addressed source of the content pin")
		}
	}
	return RegistryAdmission{Route: &RouteDescriptor{
		StoryID:           entry.StoryID,
		Revision:          entry.Revision,
		ProjectURL:        binding.CanonicalURL,
		WalkthroughID:     entry.WalkthroughID,
		ChapterID:         chapterID,
		ContentPin:        entry.ContentPin,
		DeclarationSHA256: entry.DeclarationSHA256,
	}}
}

// ---------------------------------------------------------------------------
// External checkpoint envelope.
// ---------------------------------------------------------------------------

// EnvelopeDraft is the strictly parsed shape of one external checkpoint
// envelope, before the owner resolves the opaque project reference. No SQL,
// rows, evidence, transcript, token or any other extra field is representable.
type EnvelopeDraft struct {
	ProjectRaw            []byte
	AuthorityGeneration   Integer
	ContentPin            ContentPin
	DeclarationSHA256     string
	NarrativeSHA256       string
	TrainingPackageSHA256 string
	WalkthroughID         string
	ChapterID             string
	StepID                string
	ChallengeID           string
	ChallengeRevision     Integer
	CheckpointID          string
	AttemptID             string
	SubmissionGeneration  Integer
	Outcome               string
	Complete              bool
	Assisted              bool
}

// CheckpointEnvelope is an admitted envelope: the owner-resolved project
// identity plus every verified field.
type CheckpointEnvelope struct {
	Project ProjectIdentity
	EnvelopeDraft
}

// EnvelopeResult is the outcome of the strict envelope shape parse.
type EnvelopeResult struct {
	Draft  *EnvelopeDraft
	Issues []Issue
}

// Err returns nil for a parsed draft or the aggregate recoverable refusal.
func (r EnvelopeResult) Err() error {
	if r.Draft != nil {
		return nil
	}
	return &Error{Issues: r.Issues}
}

// EnvelopeAdmission is the outcome of owner resolution.
type EnvelopeAdmission struct {
	Envelope *CheckpointEnvelope
	Issues   []Issue
}

// Err returns nil for an admitted envelope or the aggregate recoverable refusal.
func (r EnvelopeAdmission) Err() error {
	if r.Envelope != nil {
		return nil
	}
	return &Error{Issues: r.Issues}
}

var envelopeFields = map[string]bool{
	"project": true, "authorityGeneration": true, "contentPin": true,
	"declarationSha256": true, "narrativeSha256": true,
	"trainingPackageSha256": true, "walkthroughId": true, "chapterId": true,
	"stepId": true, "challengeId": true, "challengeRevision": true,
	"checkpointId": true, "attemptId": true, "submissionGeneration": true,
	"outcome": true, "complete": true, "assisted": true,
}

// Outcomes is the closed checkpoint-result outcome set. Assistance is
// orthogonal and never a replacement outcome.
var Outcomes = []string{"passed", "failed", "cancelled", "timed-out", "error"}

// ParseCheckpointEnvelope parses exactly one closed envelope shape. An
// inconsistent outcome/complete summary refuses here: the adapter must never
// hand an inconsistent summary to matching.
func ParseCheckpointEnvelope(data []byte) EnvelopeResult {
	_, root, issue := decodeDeclaration(data)
	if issue != nil {
		return EnvelopeResult{Issues: []Issue{*issue}}
	}
	checker := newChecker(true)
	checker.rejectUnknown(root, "", envelopeFields)

	draft := &EnvelopeDraft{}
	if project, present := root.member("project"); present {
		if project.kind == jsonNull {
			checker.report(CodeInvalidType, "/project", "must not be null")
		} else {
			// Copy the exact input span: the caller's buffer must not be able to
			// change the draft (or anything later admitted from it) afterwards.
			draft.ProjectRaw = append([]byte(nil), project.raw...)
		}
	} else {
		checker.report(CodeMissingField, "/project", "required field is missing")
	}
	draft.AuthorityGeneration = checker.positiveInteger(root, "", "authorityGeneration", true)
	if value, ok := root.member("contentPin"); ok {
		draft.ContentPin = checker.contentPin(value, "/contentPin")
	} else {
		checker.report(CodeMissingField, "/contentPin", "required field is missing")
	}
	draft.DeclarationSHA256 = checker.hexDigest(root, "", "declarationSha256", true)
	draft.NarrativeSHA256 = checker.hexDigest(root, "", "narrativeSha256", true)
	draft.TrainingPackageSHA256 = checker.hexDigest(root, "", "trainingPackageSha256", true)
	draft.WalkthroughID = checker.requiredID(root, "", "walkthroughId")
	draft.ChapterID = checker.requiredID(root, "", "chapterId")
	draft.StepID = checker.requiredID(root, "", "stepId")
	draft.ChallengeID = checker.challengeID(root, "")
	draft.ChallengeRevision = checker.challengeRevisionNamed(root, "", "challengeRevision")
	draft.CheckpointID = checker.requiredID(root, "", "checkpointId")
	draft.AttemptID = checker.attemptID(root, "")
	draft.SubmissionGeneration = checker.positiveInteger(root, "", "submissionGeneration", true)
	draft.Outcome = checker.enumField(root, "", "outcome", true, Outcomes...)
	complete, completeOK := checker.requiredBool(root, "", "complete")
	assisted, assistedOK := checker.requiredBool(root, "", "assisted")
	draft.Complete = complete
	draft.Assisted = assisted
	if completeOK && draft.Outcome != "" && (draft.Outcome == "passed") != complete {
		checker.report(CodeInconsistentSummary, "/complete",
			"outcome passed requires complete true and every other outcome requires complete false")
	}

	if len(checker.issues) > 0 || !assistedOK {
		return EnvelopeResult{Issues: checker.issues}
	}
	return EnvelopeResult{Draft: draft}
}

// Projection is the stable normalized view used by the shared corpus.
func (d EnvelopeDraft) Projection() map[string]any {
	return map[string]any{
		"projectPresent":        len(d.ProjectRaw) > 0,
		"authorityGeneration":   string(d.AuthorityGeneration),
		"contentPin":            projectContentPin(d.ContentPin),
		"declarationSha256":     d.DeclarationSHA256,
		"narrativeSha256":       d.NarrativeSHA256,
		"trainingPackageSha256": d.TrainingPackageSHA256,
		"walkthroughId":         d.WalkthroughID,
		"chapterId":             d.ChapterID,
		"stepId":                d.StepID,
		"challengeId":           d.ChallengeID,
		"challengeRevision":     string(d.ChallengeRevision),
		"checkpointId":          d.CheckpointID,
		"attemptId":             d.AttemptID,
		"submissionGeneration":  string(d.SubmissionGeneration),
		"outcome":               d.Outcome,
		"complete":              d.Complete,
		"assisted":              d.Assisted,
	}
}

// AdmitCheckpointEnvelope resolves the envelope's opaque project reference
// through the trusted owner authority. A missing or refusing adapter admits
// nothing.
func AdmitCheckpointEnvelope(draft EnvelopeDraft, authority ProjectAuthority) EnvelopeAdmission {
	if authority == nil {
		return envelopeRefusal(CodeMissingContext, "", "a trusted project authority is required")
	}
	// A digest source's content pin is the exact declaration digest, so the two
	// fields must agree before this shape can become an admitted envelope.
	if draft.ContentPin.Kind == PinDigest && draft.ContentPin.SHA256 != draft.DeclarationSHA256 {
		return envelopeRefusal(CodePinMismatch, "/contentPin",
			"a digest content pin must equal the declaration digest")
	}
	// The adapter receives its own copy of the project span and can never mutate
	// the draft or the admitted envelope.
	identity, err := authority.AdmitProject(append([]byte(nil), draft.ProjectRaw...))
	if err != nil {
		return envelopeRefusal(CodeNotAdmitted, "/project", err.Error())
	}
	if identity.Value == nil {
		return envelopeRefusal(CodeNotAdmitted, "/project",
			"the project authority did not return an identity")
	}
	// ProjectIdentity.Value is opaque and snapshotting it is the trusted owner
	// adapter's responsibility; every contract-owned field is cloned here.
	return EnvelopeAdmission{Envelope: &CheckpointEnvelope{
		Project:       identity,
		EnvelopeDraft: cloneEnvelopeDraft(draft),
	}}
}

// cloneEnvelopeDraft returns an independent snapshot of a parsed envelope
// draft. The project span is the only caller-owned buffer.
func cloneEnvelopeDraft(source EnvelopeDraft) EnvelopeDraft {
	snapshot := source
	snapshot.ProjectRaw = append([]byte(nil), source.ProjectRaw...)
	return snapshot
}

// MatchVerdict is a pure matching decision. Consuming, retaining for revisit,
// pausing/scheduling and authority lifecycle remain runtime owner work.
type MatchVerdict string

const (
	// MatchConsume means the envelope satisfies the active wait and may be
	// consumed once by the runtime.
	MatchConsume MatchVerdict = "consume"
	// MatchRevisit means a retained execution completion satisfies the same
	// wait without a new envelope, resubmission or replayed external effect.
	MatchRevisit MatchVerdict = "revisit"
	// MatchNoMatch means wrong, stale, duplicate, non-passing or unvalidated
	// input: do nothing and keep waiting.
	MatchNoMatch MatchVerdict = "no-match"
)

// Stable matching reasons. They are decisions, not refusal issues.
const (
	ReasonMatched             = "matched"
	ReasonMissingContext      = "missing_context"
	ReasonWaitInvalid         = "wait_invalid"
	ReasonBindingUnvalidated  = "binding_not_validated"
	ReasonProjectMismatch     = "project_mismatch"
	ReasonAuthorityStale      = "authority_stale"
	ReasonPinMismatch         = "pin_mismatch"
	ReasonDeclarationMismatch = "declaration_mismatch"
	ReasonNarrativeMismatch   = "narrative_mismatch"
	ReasonPackageMismatch     = "package_mismatch"
	ReasonWalkthroughMismatch = "walkthrough_mismatch"
	ReasonChapterMismatch     = "chapter_mismatch"
	ReasonStepMismatch        = "step_mismatch"
	ReasonChallengeMismatch   = "challenge_mismatch"
	ReasonRevisionMismatch    = "revision_mismatch"
	ReasonCheckpointMismatch  = "checkpoint_mismatch"
	ReasonAttemptMismatch     = "attempt_mismatch"
	ReasonSubmissionStale     = "submission_stale"
	ReasonOutcomeNotPassed    = "outcome_not_passed"
	ReasonAttemptIncomplete   = "attempt_incomplete"
)

// MatchDecision is the result of one pure match. Reason is stable and
// machine-readable; human wording is the runtime's.
type MatchDecision struct {
	Verdict MatchVerdict
	Reason  string
}

func noMatch(reason string) MatchDecision {
	return MatchDecision{Verdict: MatchNoMatch, Reason: reason}
}

// DialogueChallengeBinding is the a3 narrative validator's verdict that the
// pinned dialogue segment bound to this chapter/step waits for this exact
// challenge step. This slice cannot validate a sidecar, so an unvalidated
// binding never matches.
type DialogueChallengeBinding struct {
	Validated       bool
	SegmentID       string
	ChapterID       string
	DialogueStepID  string
	ChallengeStepID string
}

// ActiveWait is the explicit runtime wait a result must match field for field.
type ActiveWait struct {
	Project               ProjectIdentity
	AuthorityGeneration   Integer
	ContentPin            ContentPin
	DeclarationSHA256     string
	NarrativeSHA256       string
	TrainingPackageSHA256 string
	WalkthroughID         string
	ChapterID             string
	ChallengeStepID       string
	ChallengeID           string
	ChallengeRevision     Integer
	CheckpointID          string
	AttemptID             string
	SubmissionGeneration  Integer
	Dialogue              DialogueChallengeBinding
}

// ValidateActiveWait reports every structural problem that would make a wait
// unable to consume or revisit a result.
func ValidateActiveWait(wait ActiveWait) []Issue {
	issues := make([]Issue, 0)
	report := func(code Code, path, message string) {
		issues = append(issues, Issue{Code: code, Path: path, Message: message})
	}
	if wait.Project.Value == nil {
		report(CodeMissingContext, "", "the active wait has no project identity")
	}
	if !isPositiveInteger(wait.AuthorityGeneration) {
		report(CodeInvalidValue, "", "authorityGeneration must be a positive canonical integer")
	}
	if !wait.ContentPin.Valid() {
		report(CodePinMismatch, "", "the active wait content pin is not one exact immutable pin")
	}
	if wait.ContentPin.Kind == PinDigest && wait.ContentPin.SHA256 != wait.DeclarationSHA256 {
		report(CodePinMismatch, "", "a digest content pin must equal the declaration digest")
	}
	for _, digest := range []string{
		wait.DeclarationSHA256, wait.NarrativeSHA256, wait.TrainingPackageSHA256,
	} {
		if !isLowerHex(digest, 64) {
			report(CodeDigestMismatch, "", "active wait digests must be lowercase 64-hex")
			break
		}
	}
	for _, identity := range []string{
		wait.WalkthroughID, wait.ChapterID, wait.ChallengeStepID, wait.CheckpointID,
	} {
		if !idPattern.MatchString(identity) {
			report(CodeInvalidValue, "", "active wait identifiers must be walkthrough IDs")
			break
		}
	}
	if !challengeIDPattern.MatchString(wait.ChallengeID) {
		report(CodeInvalidValue, "", "active wait challengeId must be a challenge slug")
	}
	if !isPositiveInteger(wait.ChallengeRevision) || !integerAtMost(string(wait.ChallengeRevision), "1000000") {
		report(CodeInvalidValue, "", "active wait challengeRevision must be between 1 and 1000000")
	}
	if !validAttemptID(wait.AttemptID) {
		report(CodeInvalidValue, "", "active wait attemptId must be 1..128 printable ASCII characters")
	}
	if !isPositiveInteger(wait.SubmissionGeneration) {
		report(CodeInvalidValue, "", "submissionGeneration must be a positive canonical integer")
	}
	if !wait.Dialogue.Validated {
		report(CodeMissingContext, "", "the active wait dialogue-to-challenge binding is not validated")
	} else {
		if wait.Dialogue.ChapterID != wait.ChapterID {
			report(CodeInvalidReference, "", "the dialogue binding belongs to another chapter")
		}
		if wait.Dialogue.ChallengeStepID != wait.ChallengeStepID {
			report(CodeInvalidReference, "", "the dialogue binding names another challenge step")
		}
		for _, identity := range []string{
			wait.Dialogue.SegmentID, wait.Dialogue.DialogueStepID,
		} {
			if !idPattern.MatchString(identity) {
				report(CodeInvalidValue, "", "the dialogue binding identifiers must be walkthrough IDs")
				break
			}
		}
	}
	return issues
}

// MatchCheckpointEnvelope compares every envelope field with the explicit
// active wait. Only a validated, fully consistent, fully matching pass may be
// consumed. The validation envelope type has no reference/Test namespace
// field, so such a payload is an unknown-field refusal rather than a match.
func MatchCheckpointEnvelope(
	wait ActiveWait,
	envelope CheckpointEnvelope,
	authority ProjectAuthority,
) MatchDecision {
	if len(ValidateActiveWait(wait)) > 0 {
		return noMatch(ReasonWaitInvalid)
	}
	if authority == nil {
		return noMatch(ReasonMissingContext)
	}
	if !authority.EqualProject(wait.Project, envelope.Project) {
		return noMatch(ReasonProjectMismatch)
	}
	if wait.AuthorityGeneration != envelope.AuthorityGeneration {
		return noMatch(ReasonAuthorityStale)
	}
	if !wait.ContentPin.Equal(envelope.ContentPin) {
		return noMatch(ReasonPinMismatch)
	}
	if wait.DeclarationSHA256 != envelope.DeclarationSHA256 {
		return noMatch(ReasonDeclarationMismatch)
	}
	if wait.NarrativeSHA256 != envelope.NarrativeSHA256 {
		return noMatch(ReasonNarrativeMismatch)
	}
	if wait.TrainingPackageSHA256 != envelope.TrainingPackageSHA256 {
		return noMatch(ReasonPackageMismatch)
	}
	if wait.WalkthroughID != envelope.WalkthroughID {
		return noMatch(ReasonWalkthroughMismatch)
	}
	if wait.ChapterID != envelope.ChapterID {
		return noMatch(ReasonChapterMismatch)
	}
	if wait.ChallengeStepID != envelope.StepID {
		return noMatch(ReasonStepMismatch)
	}
	if wait.ChallengeID != envelope.ChallengeID {
		return noMatch(ReasonChallengeMismatch)
	}
	if wait.ChallengeRevision != envelope.ChallengeRevision {
		return noMatch(ReasonRevisionMismatch)
	}
	if wait.CheckpointID != envelope.CheckpointID {
		return noMatch(ReasonCheckpointMismatch)
	}
	if wait.AttemptID != envelope.AttemptID {
		return noMatch(ReasonAttemptMismatch)
	}
	if wait.SubmissionGeneration != envelope.SubmissionGeneration {
		return noMatch(ReasonSubmissionStale)
	}
	if envelope.Outcome != "passed" {
		return noMatch(ReasonOutcomeNotPassed)
	}
	if !envelope.Complete {
		return noMatch(ReasonAttemptIncomplete)
	}
	return MatchDecision{Verdict: MatchConsume, Reason: ReasonMatched}
}

// CompletionRecord is one runtime-retained execution completion. Revisit
// never creates an envelope or replays an external effect.
type CompletionRecord struct {
	Project               ProjectIdentity
	AuthorityGeneration   Integer
	ContentPin            ContentPin
	DeclarationSHA256     string
	NarrativeSHA256       string
	TrainingPackageSHA256 string
	WalkthroughID         string
	ChapterID             string
	ChallengeStepID       string
	ChallengeID           string
	ChallengeRevision     Integer
	CheckpointID          string
	Outcome               string
	Complete              bool
}

// MatchRetainedCompletion decides whether a stored completion may satisfy a
// revisit with identical pins, checkpoint and live authority.
func MatchRetainedCompletion(
	record CompletionRecord,
	wait ActiveWait,
	authority ProjectAuthority,
) MatchDecision {
	if len(ValidateActiveWait(wait)) > 0 {
		return noMatch(ReasonWaitInvalid)
	}
	if authority == nil {
		return noMatch(ReasonMissingContext)
	}
	if !authority.EqualProject(wait.Project, record.Project) {
		return noMatch(ReasonProjectMismatch)
	}
	if wait.AuthorityGeneration != record.AuthorityGeneration {
		return noMatch(ReasonAuthorityStale)
	}
	if !wait.ContentPin.Equal(record.ContentPin) {
		return noMatch(ReasonPinMismatch)
	}
	if wait.DeclarationSHA256 != record.DeclarationSHA256 {
		return noMatch(ReasonDeclarationMismatch)
	}
	if wait.NarrativeSHA256 != record.NarrativeSHA256 {
		return noMatch(ReasonNarrativeMismatch)
	}
	if wait.TrainingPackageSHA256 != record.TrainingPackageSHA256 {
		return noMatch(ReasonPackageMismatch)
	}
	if wait.WalkthroughID != record.WalkthroughID {
		return noMatch(ReasonWalkthroughMismatch)
	}
	if wait.ChapterID != record.ChapterID {
		return noMatch(ReasonChapterMismatch)
	}
	if wait.ChallengeStepID != record.ChallengeStepID {
		return noMatch(ReasonStepMismatch)
	}
	if wait.ChallengeID != record.ChallengeID {
		return noMatch(ReasonChallengeMismatch)
	}
	if wait.ChallengeRevision != record.ChallengeRevision {
		return noMatch(ReasonRevisionMismatch)
	}
	if wait.CheckpointID != record.CheckpointID {
		return noMatch(ReasonCheckpointMismatch)
	}
	if record.Outcome != "passed" {
		return noMatch(ReasonOutcomeNotPassed)
	}
	if !record.Complete {
		return noMatch(ReasonAttemptIncomplete)
	}
	return MatchDecision{Verdict: MatchRevisit, Reason: ReasonMatched}
}

func (c *checker) attemptID(object *jsonValue, path string) string {
	text, ok := c.text(object, path, "attemptId", true)
	if !ok {
		return ""
	}
	if !validAttemptID(text) {
		c.report(CodeInvalidValue, pointerChild(path, "attemptId"),
			"must be 1..128 printable ASCII characters")
		return ""
	}
	return text
}

func validAttemptID(value string) bool {
	if len(value) == 0 || len(value) > MaxAttemptIDLen {
		return false
	}
	for index := 0; index < len(value); index++ {
		if value[index] < 0x20 || value[index] > 0x7e {
			return false
		}
	}
	return true
}

func isPositiveInteger(value Integer) bool {
	return value != "" && canonicalIntegerPattern.MatchString(string(value)) && value != "0"
}

func projectContentPin(pin ContentPin) map[string]any {
	switch pin.Kind {
	case PinGit:
		return map[string]any{"kind": PinGit, "commit": pin.Commit}
	case PinDigest:
		return map[string]any{"kind": PinDigest, "sha256": pin.SHA256}
	default:
		return map[string]any{"kind": pin.Kind}
	}
}

func registryRefusal(code Code, path, message string) RegistryAdmission {
	return RegistryAdmission{Issues: []Issue{{Code: code, Path: path, Message: message}}}
}

func envelopeRefusal(code Code, path, message string) EnvelopeAdmission {
	return EnvelopeAdmission{Issues: []Issue{{Code: code, Path: path, Message: message}}}
}

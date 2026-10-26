package walkthrough

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
)

// AssetReference pins one immutable UTF-8 JSON asset of the declaring project:
// a safe project-root-relative path, the lowercase SHA-256 of its exact bytes
// and its positive byte length.
type AssetReference struct {
	Path   string
	SHA256 string
	Bytes  Integer
}

// PackageReference pins the immutable fictional training package. It carries
// the same three asset fields plus the package's own ID and revision.
type PackageReference struct {
	ID       string
	Revision Integer
	Path     string
	SHA256   string
	Bytes    Integer
}

// Asset returns the package's asset-reference view.
func (p PackageReference) Asset() AssetReference {
	return AssetReference{Path: p.Path, SHA256: p.SHA256, Bytes: p.Bytes}
}

// Equal reports field-for-field equality of two asset references.
func (r AssetReference) Equal(other AssetReference) bool {
	return r.Path == other.Path && r.SHA256 == other.SHA256 && r.Bytes == other.Bytes
}

// Equal reports field-for-field equality of two package references.
func (p PackageReference) Equal(other PackageReference) bool {
	return p.ID == other.ID && p.Revision == other.Revision && p.Asset().Equal(other.Asset())
}

// VerifyBytes checks supplied bytes against this reference before any parse:
// the byte length must match exactly and the SHA-256 must equal the pinned
// digest. A mismatch is a recoverable refusal with no partially admitted asset.
func (r AssetReference) VerifyBytes(data []byte) error {
	if Integer(strconv.Itoa(len(data))) != r.Bytes {
		return &Error{Issues: []Issue{{
			Code:    CodeByteCountMismatch,
			Path:    "",
			Message: "asset " + r.Path + " byte length does not match its pin",
		}}}
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != r.SHA256 {
		return &Error{Issues: []Issue{{
			Code:    CodeDigestMismatch,
			Path:    "",
			Message: "asset " + r.Path + " SHA-256 does not match its pin",
		}}}
	}
	return nil
}

// VerifyBytes checks supplied package bytes against the package reference.
func (p PackageReference) VerifyBytes(data []byte) error { return p.Asset().VerifyBytes(data) }

// ContentPin is the resolved source pin. Exactly one variant is populated:
// KindGit pins a 40-hex commit (normalized lowercase); KindDigest pins the
// lowercase SHA-256 of the exact declaration bytes.
type ContentPin struct {
	Kind   string
	Commit string
	SHA256 string
}

const (
	// PinGit is an immutable Git commit pin.
	PinGit = "git"
	// PinDigest is an immutable content-digest pin for cloud/local sources.
	PinDigest = "digest"
)

// Valid reports whether the pin is one exact, immutable variant.
func (p ContentPin) Valid() bool {
	switch p.Kind {
	case PinGit:
		return isHex(p.Commit, 40) && p.SHA256 == ""
	case PinDigest:
		return p.Commit == "" && isLowerHex(p.SHA256, 64)
	default:
		return false
	}
}

// Equal compares two pins after their parsers normalized hex spelling.
func (p ContentPin) Equal(other ContentPin) bool {
	if p.Kind != other.Kind {
		return false
	}
	switch p.Kind {
	case PinGit:
		return p.Commit == other.Commit
	case PinDigest:
		return p.SHA256 == other.SHA256
	default:
		return false
	}
}

// SourceContext is the resolved declaring-project context. It is deliberately
// not a declaration field: the caller's owner adapter establishes the project
// identity, the exact content pin and the verified declaration digest.
type SourceContext struct {
	Project           ProjectIdentity
	ContentPin        ContentPin
	DeclarationSHA256 string
}

// ProjectIdentity is an opaque snapshot owned by the injected
// [ProjectAuthority]. This package never interprets Value, never builds a
// canonical project serialization and never compares identities itself.
type ProjectIdentity struct {
	Value any
}

// ProjectBinding is the owner's admission verdict for one project URL or
// reference. Only the owner adapter may populate it; this package only reads
// the verdicts the training contracts need.
type ProjectBinding struct {
	Identity ProjectIdentity
	// CanonicalURL is the owner's one canonical address of the project.
	CanonicalURL string
	// Public is the owner-established public-scope verdict. A URL spelling never
	// implies it.
	Public bool
	// Commit is the exact lowercase 40-hex commit the reference pins, or "" when
	// the reference does not name an immutable Git commit (a branch, HEAD, the
	// default branch or a non-Git source).
	Commit string
	// DigestAddressed reports whether the source serves exact bytes by content
	// digest (cloud/local), which is the only valid content pin for it.
	DigestAddressed bool
}

// ProjectAuthority is the trusted opaque project-identity adapter port. The
// production Apps adapter reuses the owning project-context normalization; a
// test double only proves wiring, never authority.
type ProjectAuthority interface {
	// BindProject admits a canonical project URL or reference spelling and
	// reports the owner's verdicts.
	BindProject(raw string) (ProjectBinding, error)
	// AdmitProject admits an opaque project-reference JSON value, as carried by
	// an external checkpoint envelope, and returns only its snapshot.
	AdmitProject(raw []byte) (ProjectIdentity, error)
	// EqualProject is the owner's full identity equality; this package never
	// compares opaque identities by itself.
	EqualProject(a, b ProjectIdentity) bool
}

// PackageInventory is the owner-established inventory of one admitted package.
type PackageInventory struct {
	Ref    PackageReference
	Assets []AssetReference
	Cases  []string
}

// ChallengeBinding is one owner-validated immutable challenge revision.
type ChallengeBinding struct {
	ChallengeID string
	Revision    Integer
}

// TrainingOwnerContext is the trusted validated-owner lookup port. It never
// reads files, loads a package document this package would have to invent, or
// falls back to an ordinary (non-training) service.
type TrainingOwnerContext interface {
	// AdmitPackage returns the exact admitted package and its declared asset and
	// Case inventory.
	AdmitPackage(ref PackageReference) (PackageInventory, error)
	// AdmitAsset admits one exact project-relative asset reference.
	AdmitAsset(ref AssetReference) error
	// AdmitChallenge resolves the exact challenge manifest through the challenge
	// owner. A manifest that has not passed the owning validator is not admitted.
	AdmitChallenge(challengeID string, revision Integer) (ChallengeBinding, error)
}

// AdmittedChallengeStep is one structurally valid challenge step whose target
// and checkpoint the owner context has admitted.
type AdmittedChallengeStep struct {
	ChapterID    string
	StepID       string
	ChallengeID  string
	Revision     Integer
	CheckpointID string
}

// AdmittedTraining is the resolved, immutable training identity: no file has
// been read, but every pin, package, Case, challenge and checkpoint identity is
// bound together and admitted by its owner.
type AdmittedTraining struct {
	Declaration           *TrainingDeclaration
	Project               ProjectIdentity
	ContentPin            ContentPin
	DeclarationSHA256     string
	Narrative             AssetReference
	NarrativeSHA256       string
	Package               PackageReference
	TrainingPackageSHA256 string
	WalkthroughID         string
	Chapters              []string
	ChallengeSteps        []AdmittedChallengeStep
	Cases                 []string
}

// Resolution is the outcome of owner-context admission. Admitted is true only
// when Bundle is non-nil; a partially resolved bundle is never exposed.
type Resolution struct {
	Admitted bool
	Bundle   *AdmittedTraining
	Issues   []Issue
}

// Err returns nil for an admitted bundle or the aggregate recoverable refusal.
func (r Resolution) Err() error {
	if r.Admitted {
		return nil
	}
	return &Error{Issues: r.Issues}
}

// ResolveTraining binds a structurally valid /2 declaration to the resolved
// declaring-project context and to its owner-admitted package, asset, Case and
// challenge identities. Missing authority or context refuses; nothing is read.
func ResolveTraining(
	declaration *TrainingDeclaration,
	source SourceContext,
	authority ProjectAuthority,
	owners TrainingOwnerContext,
) Resolution {
	if declaration == nil {
		return unresolved(CodeMissingContext, "", "a training declaration is required")
	}
	if authority == nil {
		return unresolved(CodeMissingContext, "", "a trusted project authority is required")
	}
	if owners == nil {
		return unresolved(CodeMissingContext, "", "a trusted training owner context is required")
	}
	if source.Project.Value == nil {
		return unresolved(CodeMissingContext, "", "the resolved declaring project identity is required")
	}
	if !source.ContentPin.Valid() {
		return unresolved(CodePinMismatch, "", "the resolved content pin is not one exact immutable pin")
	}
	if !isHex(source.DeclarationSHA256, 64) {
		return unresolved(CodeDigestMismatch, "", "the resolved declaration digest is not a 64-hex digest")
	}
	if strings.ToLower(source.DeclarationSHA256) != declaration.Revision {
		return unresolved(CodeDigestMismatch, "",
			"the resolved declaration digest does not match the declaration bytes")
	}
	// A digest source pins the exact declaration bytes, so its content digest
	// must be the parsed declaration digest itself rather than a second,
	// independently supplied value.
	if source.ContentPin.Kind == PinDigest && source.ContentPin.SHA256 != declaration.Revision {
		return unresolved(CodePinMismatch, "",
			"a digest content pin must equal the exact declaration digest")
	}

	// Take an independent snapshot before asking any owner to admit anything:
	// the parsed declaration stays caller-owned and mutable, while the admitted
	// bundle must never borrow it.
	snapshot := cloneTrainingDeclaration(declaration)

	inventory, err := owners.AdmitPackage(snapshot.TrainingPackage)
	if err != nil {
		return unresolved(CodeNotAdmitted, "/trainingPackage", err.Error())
	}
	if !inventory.Ref.Equal(snapshot.TrainingPackage) {
		return unresolved(CodeMismatch, "/trainingPackage",
			"the admitted package does not equal the declared package reference")
	}
	if err := owners.AdmitAsset(snapshot.Narrative); err != nil {
		return unresolved(CodeNotAdmitted, "/narrative", err.Error())
	}

	bundle := &AdmittedTraining{
		Declaration:           snapshot,
		Project:               source.Project,
		ContentPin:            source.ContentPin,
		DeclarationSHA256:     snapshot.Revision,
		Narrative:             snapshot.Narrative,
		NarrativeSHA256:       snapshot.Narrative.SHA256,
		Package:               snapshot.TrainingPackage,
		TrainingPackageSHA256: snapshot.TrainingPackage.SHA256,
		WalkthroughID:         snapshot.ID,
	}
	for _, chapter := range snapshot.Chapters {
		bundle.Chapters = append(bundle.Chapters, chapter.ID)
	}
	issues := make([]Issue, 0)
	for chapterIndex, chapter := range snapshot.Chapters {
		for stepIndex, step := range chapter.Steps {
			path := pointerIndex(pointerChild(pointerIndex("/chapters", chapterIndex), "steps"), stepIndex)
			if target := step.Target.TrainingCase; target != nil {
				casePath := pointerChild(path, "target")
				if !containsString(inventory.Cases, target.ManifestCaseID) {
					issues = append(issues, Issue{
						Code:    CodeNotAdmitted,
						Path:    pointerChild(casePath, "manifestCaseId"),
						Message: "the training package does not declare Case " + target.ManifestCaseID,
					})
					continue
				}
				if !containsString(bundle.Cases, target.ManifestCaseID) {
					bundle.Cases = append(bundle.Cases, target.ManifestCaseID)
				}
			}
			target := step.Target.Challenge
			if target == nil {
				continue
			}
			binding, err := owners.AdmitChallenge(target.ChallengeID, target.Revision)
			if err != nil {
				issues = append(issues, Issue{
					Code:    CodeNotAdmitted,
					Path:    pointerChild(pointerChild(path, "target"), "challengeId"),
					Message: err.Error(),
				})
				continue
			}
			if binding.ChallengeID != target.ChallengeID || binding.Revision != target.Revision {
				issues = append(issues, Issue{
					Code:    CodeMismatch,
					Path:    pointerChild(pointerChild(path, "target"), "challengeId"),
					Message: "the admitted challenge does not equal the declared challenge target",
				})
				continue
			}
			admitted := AdmittedChallengeStep{
				ChapterID:   chapter.ID,
				StepID:      step.ID,
				ChallengeID: target.ChallengeID,
				Revision:    target.Revision,
			}
			if step.Done.ChallengePassed != nil && len(step.Done.ChallengePassed.Checkpoints) == 1 {
				admitted.CheckpointID = step.Done.ChallengePassed.Checkpoints[0].ID
			}
			bundle.ChallengeSteps = append(bundle.ChallengeSteps, admitted)
		}
	}
	if len(issues) > 0 {
		return Resolution{Issues: issues}
	}
	return Resolution{Admitted: true, Bundle: bundle}
}

// WalkthroughIdentity is the declared walkthrough identity used by registry
// admission: the ID, the exact revision and the ordered chapter IDs.
func (d *TrainingDeclaration) WalkthroughIdentity() DeclaredWalkthrough {
	chapters := make([]string, len(d.Chapters))
	for index := range d.Chapters {
		chapters[index] = d.Chapters[index].ID
	}
	return DeclaredWalkthrough{ID: d.ID, Revision: d.Revision, Chapters: chapters}
}

func unresolved(code Code, path, message string) Resolution {
	return Resolution{Issues: []Issue{{Code: code, Path: path, Message: message}}}
}

// ---------------------------------------------------------------------------
// Strict structural parsers for the source artifacts.
// ---------------------------------------------------------------------------

var assetReferenceFields = map[string]bool{"path": true, "sha256": true, "bytes": true}

var packageReferenceFields = map[string]bool{
	"id": true, "revision": true, "path": true, "sha256": true, "bytes": true,
}

var contentPinGitFields = map[string]bool{"kind": true, "commit": true}
var contentPinDigestFields = map[string]bool{"kind": true, "sha256": true}

// assetReference parses exactly {path, sha256, bytes}.
func (c *checker) assetReference(value *jsonValue, path string) AssetReference {
	if !c.requireObject(value, path) {
		return AssetReference{}
	}
	c.rejectUnknown(value, path, assetReferenceFields)
	return AssetReference{
		Path:   c.assetPath(value, path, "path", true),
		SHA256: c.lowerDigest(value, path, "sha256", true),
		Bytes:  c.positiveInteger(value, path, "bytes", true),
	}
}

// packageReference parses exactly {id, revision, path, sha256, bytes}.
func (c *checker) packageReference(value *jsonValue, path string) PackageReference {
	if !c.requireObject(value, path) {
		return PackageReference{}
	}
	c.rejectUnknown(value, path, packageReferenceFields)
	return PackageReference{
		ID:       c.requiredID(value, path, "id"),
		Revision: c.positiveInteger(value, path, "revision", true),
		Path:     c.assetPath(value, path, "path", true),
		SHA256:   c.lowerDigest(value, path, "sha256", true),
		Bytes:    c.positiveInteger(value, path, "bytes", true),
	}
}

// contentPin parses exactly one {kind: git, commit} or {kind: digest, sha256}
// variant. A Git commit is accepted in either hex case and normalized
// lowercase; a content digest must already be lowercase as the owner specifies.
func (c *checker) contentPin(value *jsonValue, path string) ContentPin {
	if !c.requireObject(value, path) {
		return ContentPin{}
	}
	kindValue, present := value.member("kind")
	if !present {
		c.report(CodeMissingField, pointerChild(path, "kind"), "required field is missing")
		return ContentPin{}
	}
	if kindValue.kind != jsonString {
		c.report(CodeInvalidType, pointerChild(path, "kind"), "must be a string")
		return ContentPin{}
	}
	switch kindValue.text {
	case PinGit:
		c.rejectUnknown(value, path, contentPinGitFields)
		commit, ok := c.text(value, path, "commit", true)
		if !ok {
			return ContentPin{Kind: PinGit}
		}
		if !isHex(commit, 40) {
			c.report(CodeInvalidValue, pointerChild(path, "commit"),
				"must be a 40-character hexadecimal commit")
			return ContentPin{Kind: PinGit}
		}
		return ContentPin{Kind: PinGit, Commit: strings.ToLower(commit)}
	case PinDigest:
		c.rejectUnknown(value, path, contentPinDigestFields)
		digest := c.lowerDigest(value, path, "sha256", true)
		return ContentPin{Kind: PinDigest, SHA256: digest}
	default:
		c.report(CodeInvalidValue, pointerChild(path, "kind"), "kind must be git or digest")
		return ContentPin{}
	}
}

// assetPath validates the owner's safe project-relative path rules.
func (c *checker) assetPath(object *jsonValue, path, name string, required bool) string {
	text, ok := c.text(object, path, name, required)
	if !ok {
		return ""
	}
	if !validAssetPath(text) {
		c.report(CodeInvalidPath, pointerChild(path, name),
			"must be a safe project-relative path without empty, . or .. segments, backslash, scheme, query or fragment")
		return ""
	}
	return text
}

// lowerDigest reads a lowercase 64-hex digest.
func (c *checker) lowerDigest(object *jsonValue, path, name string, required bool) string {
	text, ok := c.text(object, path, name, required)
	if !ok {
		return ""
	}
	if !isLowerHex(text, 64) {
		c.report(CodeInvalidValue, pointerChild(path, name),
			"must be 64 lowercase hexadecimal characters")
		return ""
	}
	return text
}

// hexDigest reads a 64-hex digest in either case and normalizes it lowercase.
// It exists for the owner-verified digests the registry and envelope repeat.
func (c *checker) hexDigest(object *jsonValue, path, name string, required bool) string {
	text, ok := c.text(object, path, name, required)
	if !ok {
		return ""
	}
	if !isHex(text, 64) {
		c.report(CodeInvalidValue, pointerChild(path, name),
			"must be 64 hexadecimal characters")
		return ""
	}
	return strings.ToLower(text)
}

// positiveInteger reads a canonical positive decimal integer lexeme. It keeps
// the lexeme and applies no machine-integer cap, so 2^53, the uint64 range and
// larger values all survive exactly.
func (c *checker) positiveInteger(object *jsonValue, path, name string, required bool) Integer {
	value, present := object.member(name)
	if !present {
		if required {
			c.report(CodeMissingField, pointerChild(path, name), "required field is missing")
		}
		return ""
	}
	if value.kind != jsonNumber {
		c.report(CodeInvalidType, pointerChild(path, name), "must be a number")
		return ""
	}
	if !canonicalIntegerPattern.MatchString(value.number) || value.number == "0" {
		c.report(CodeInvalidValue, pointerChild(path, name), "must be a positive canonical integer")
		return ""
	}
	return Integer(value.number)
}

func validAssetPath(path string) bool {
	if path == "" || strings.HasPrefix(path, "/") || strings.ContainsAny(path, "\\?#") || hasURLScheme(path) {
		return false
	}
	for _, char := range path {
		if char < 0x20 || char == 0x7f {
			return false
		}
	}
	for _, segment := range strings.Split(path, "/") {
		if segment == "" || segment == "." || segment == ".." || segment != strings.TrimSpace(segment) {
			return false
		}
	}
	return true
}

// hasURLScheme rejects a leading RFC 3986 scheme (`https:`), which is the only
// way a URL scheme can appear in a project-root-relative path. A colon inside a
// later segment is an ordinary file-name character.
func hasURLScheme(path string) bool {
	colon := strings.IndexByte(path, ':')
	if colon <= 0 {
		return false
	}
	first := path[0]
	if (first < 'a' || first > 'z') && (first < 'A' || first > 'Z') {
		return false
	}
	for index := 1; index < colon; index++ {
		char := path[index]
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '+' || char == '-' || char == '.' {
			continue
		}
		return false
	}
	return true
}

func isLowerHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func isHex(value string, length int) bool {
	if len(value) != length {
		return false
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') && (char < 'A' || char > 'F') {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------------------
// Defensive snapshots of contract-owned admission inputs.
// ---------------------------------------------------------------------------

// cloneTrainingDeclaration returns an independent deep snapshot of a
// structurally valid /2 declaration. A parsed declaration stays caller-owned
// and mutable; an admitted bundle must never change when the caller edits it.
//
// ProjectIdentity is deliberately not part of this snapshot: its Value is
// opaque to this package and snapshotting it remains the trusted owner
// adapter's responsibility.
func cloneTrainingDeclaration(source *TrainingDeclaration) *TrainingDeclaration {
	if source == nil {
		return nil
	}
	snapshot := *source
	snapshot.Chapters = make([]Chapter, len(source.Chapters))
	for index := range source.Chapters {
		snapshot.Chapters[index] = cloneChapter(source.Chapters[index])
	}
	snapshot.Finish = cloneExit(source.Finish)
	return &snapshot
}

func cloneChapter(source Chapter) Chapter {
	snapshot := source
	snapshot.FormerIDs = cloneStrings(source.FormerIDs)
	if source.Readiness != nil {
		readiness := cloneCondition(*source.Readiness)
		snapshot.Readiness = &readiness
	}
	snapshot.Steps = make([]Step, len(source.Steps))
	for index := range source.Steps {
		snapshot.Steps[index] = cloneStep(source.Steps[index])
	}
	return snapshot
}

func cloneStep(source Step) Step {
	snapshot := source
	snapshot.Modes = cloneStrings(source.Modes)
	snapshot.Target = cloneTarget(source.Target)
	snapshot.Done = cloneCondition(source.Done)
	return snapshot
}

func cloneTarget(source Target) Target {
	snapshot := source
	if source.TableRecord != nil {
		value := *source.TableRecord
		value.Key = cloneNamedValues(source.TableRecord.Key)
		snapshot.TableRecord = &value
	}
	if source.Query != nil {
		value := *source.Query
		value.Parameters = cloneNamedValues(source.Query.Parameters)
		snapshot.Query = &value
	}
	if source.Board != nil {
		value := *source.Board
		value.Parameters = cloneNamedValues(source.Board.Parameters)
		snapshot.Board = &value
	}
	if source.Card != nil {
		value := *source.Card
		value.Parameters = cloneNamedValues(source.Card.Parameters)
		snapshot.Card = &value
	}
	if source.Related != nil {
		value := *source.Related
		value.Key = cloneNamedValues(source.Related.Key)
		snapshot.Related = &value
	}
	if source.LogSource != nil {
		value := *source.LogSource
		value.Filter = cloneNamedValues(source.LogSource.Filter)
		snapshot.LogSource = &value
	}
	if source.PublicDataProvider != nil {
		value := *source.PublicDataProvider
		snapshot.PublicDataProvider = &value
	}
	if source.Incident != nil {
		value := *source.Incident
		if source.Incident.Incident != nil {
			incident := cloneValue(*source.Incident.Incident)
			value.Incident = &incident
		}
		snapshot.Incident = &value
	}
	if source.Check != nil {
		value := *source.Check
		snapshot.Check = &value
	}
	if source.None != nil {
		snapshot.None = &struct{}{}
	}
	if source.Challenge != nil {
		value := *source.Challenge
		snapshot.Challenge = &value
	}
	if source.TrainingCase != nil {
		value := *source.TrainingCase
		snapshot.TrainingCase = &value
	}
	return snapshot
}

func cloneCondition(source Condition) Condition {
	snapshot := source
	if source.ParameterValue != nil {
		value := *source.ParameterValue
		value.Equals = cloneScalar(source.ParameterValue.Equals)
		snapshot.ParameterValue = &value
	}
	if source.RecordSelected != nil {
		value := *source.RecordSelected
		value.Equals = cloneScalar(source.RecordSelected.Equals)
		snapshot.RecordSelected = &value
	}
	if source.QueryRun != nil {
		value := *source.QueryRun
		if source.QueryRun.MinRows != nil {
			minRows := *source.QueryRun.MinRows
			value.MinRows = &minRows
		}
		snapshot.QueryRun = &value
	}
	if source.PanelOpen != nil {
		value := *source.PanelOpen
		snapshot.PanelOpen = &value
	}
	if source.FilterApplied != nil {
		value := *source.FilterApplied
		value.Equals = cloneScalar(source.FilterApplied.Equals)
		snapshot.FilterApplied = &value
	}
	if source.CheckRun != nil {
		value := *source.CheckRun
		snapshot.CheckRun = &value
	}
	if source.AssetExists != nil {
		value := *source.AssetExists
		snapshot.AssetExists = &value
	}
	if source.BoardTutorial != nil {
		value := *source.BoardTutorial
		snapshot.BoardTutorial = &value
	}
	if source.DialogueFinished != nil {
		snapshot.DialogueFinished = &DialogueFinishedCondition{}
	}
	if source.ChallengePassed != nil {
		value := *source.ChallengePassed
		value.Checkpoints = cloneCheckpoints(source.ChallengePassed.Checkpoints)
		snapshot.ChallengePassed = &value
	}
	return snapshot
}

func cloneExit(source Exit) Exit {
	snapshot := source
	if source.Target != nil {
		target := cloneTarget(*source.Target)
		snapshot.Target = &target
	}
	return snapshot
}

func cloneValue(source Value) Value {
	snapshot := source
	snapshot.Literal = cloneScalar(source.Literal)
	snapshot.Default = cloneScalar(source.Default)
	return snapshot
}

func cloneScalar(source *Scalar) *Scalar {
	if source == nil {
		return nil
	}
	snapshot := *source
	return &snapshot
}

func cloneNamedValues(source []NamedValue) []NamedValue {
	if source == nil {
		return nil
	}
	snapshot := make([]NamedValue, len(source))
	for index := range source {
		snapshot[index] = NamedValue{
			Name:  source[index].Name,
			Value: cloneValue(source[index].Value),
		}
	}
	return snapshot
}

func cloneCheckpoints(source []CheckpointMapping) []CheckpointMapping {
	if source == nil {
		return nil
	}
	snapshot := make([]CheckpointMapping, len(source))
	copy(snapshot, source)
	return snapshot
}

func cloneStrings(source []string) []string {
	if source == nil {
		return nil
	}
	snapshot := make([]string, len(source))
	copy(snapshot, source)
	return snapshot
}

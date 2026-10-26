package walkthrough

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"unicode/utf8"
)

var (
	idPattern               = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,47}$`)
	newerSchemaPattern      = regexp.MustCompile(`^datatug-walkthrough/(0|[2-9]|[1-9][0-9]+)$`)
	canonicalIntegerPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)$`)
)

var (
	topLevelFields = map[string]bool{
		"schema": true, "id": true, "version": true, "title": true,
		"summary": true, "chapters": true, "finish": true,
	}
	chapterFields = map[string]bool{
		"id": true, "formerIds": true, "title": true, "why": true,
		"requires": true, "requiresReason": true, "continuesFrom": true,
		"readiness": true, "steps": true,
	}
	stepFields = map[string]bool{
		"id": true, "title": true, "why": true, "hint": true, "modes": true,
		"target": true, "done": true, "capture": true, "needs": true,
	}
)

// ErrNewerClient is returned by [Result.Err] when the declaration bytes name a
// schema this client cannot parse. It is a recoverable newer-client boundary,
// not a malformed declaration.
var ErrNewerClient = errors.New("walkthrough: declaration needs a newer DataTug")

// Result is the outcome of one declaration parse. Definition is non-nil only
// when Accepted is true. Digest is always the SHA-256 of the exact bytes
// supplied.
type Result struct {
	Accepted    bool         `json:"accepted"`
	NewerClient bool         `json:"newerClient"`
	Digest      string       `json:"digest"`
	Definition  *Declaration `json:"definition,omitempty"`
	Issues      []Issue      `json:"issues,omitempty"`
}

// Err returns nil for an admitted declaration, [ErrNewerClient] for a newer
// schema major, or an aggregate recoverable [Error].
func (r Result) Err() error {
	if r.Accepted {
		return nil
	}
	if r.NewerClient {
		return ErrNewerClient
	}
	return &Error{Issues: r.Issues}
}

// ParseDeclaration validates one ordinary /1 declaration. directoryID is the
// `<id>` in `walkthroughs/<id>/walkthrough.json`; it must equal the declaration
// ID. The input slice is never retained or mutated.
func ParseDeclaration(directoryID string, data []byte) Result {
	digest := sha256.Sum256(data)
	result := Result{Digest: hex.EncodeToString(digest[:])}
	if len(data) > MaxDeclarationBytes {
		result.Issues = []Issue{{
			Code:    CodeTooLarge,
			Path:    "",
			Message: "declaration exceeds 64 KiB",
		}}
		return result
	}
	if !utf8.Valid(data) {
		result.Issues = []Issue{{
			Code:    CodeInvalidUTF8,
			Path:    "",
			Message: "declaration must be valid UTF-8",
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
			Message: "declaration root must be a JSON object",
		}}
		return result
	}
	schemaValue, ok := root.member("schema")
	if !ok {
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
	if schemaValue.text != SchemaV1 {
		if newerSchemaPattern.MatchString(schemaValue.text) {
			result.NewerClient = true
			return result
		}
		result.Issues = []Issue{{
			Code:    CodeInvalidValue,
			Path:    "/schema",
			Message: "schema must be \"datatug-walkthrough/1\"",
		}}
		return result
	}

	checker := &checker{
		chapterIdentities: make(map[string]string),
		stepIdentities:    make(map[string]string),
	}
	declaration := checker.declaration(root, directoryID)
	result.Issues = checker.issues
	if len(checker.issues) == 0 {
		declaration.Revision = result.Digest
		result.Accepted = true
		result.Definition = declaration
	}
	return result
}

type captureDecl struct {
	name    string
	ordinal int
	path    string
}

type captureRef struct {
	name    string
	ordinal int
	path    string
}

type checker struct {
	issues            []Issue
	nextStepOrdinal   int
	chapterIdentities map[string]string
	stepIdentities    map[string]string
	captures          []captureDecl
	captureRefs       []captureRef
	chapterPaths      []string
}

func (c *checker) declaration(root *jsonValue, directoryID string) *Declaration {
	c.rejectUnknown(root, "", topLevelFields)
	declaration := &Declaration{Schema: SchemaV1}

	declaration.ID = c.requiredID(root, "", "id")
	if declaration.ID != "" && declaration.ID != directoryID {
		c.report(CodeInvalidValue, "/id",
			"declaration id %q must equal directory id %q", declaration.ID, directoryID)
	}
	declaration.Version = c.versionField(root, "")
	declaration.Title = c.textLimited(root, "", "title", MaxTitleChars, true)
	declaration.Summary = c.textLimited(root, "", "summary", 0, false)

	chapterValues := c.arrayField(root, "", "chapters", true, 1, MaxChapters)
	totalSteps := 0
	for index, chapterValue := range chapterValues {
		path := pointerIndex("/chapters", index)
		c.chapterPaths = append(c.chapterPaths, path)
		chapter := c.chapter(chapterValue, path)
		totalSteps += len(chapter.Steps)
		declaration.Chapters = append(declaration.Chapters, chapter)
	}
	if totalSteps > MaxSteps {
		c.report(CodeTooMany, "/chapters",
			"walkthrough declares %d steps; the maximum is %d", totalSteps, MaxSteps)
	}

	if finishValue, ok := root.member("finish"); ok {
		declaration.Finish = c.exit(finishValue, "/finish")
	} else {
		declaration.Finish = Exit{Kind: "index"}
	}

	c.checkChapterReferences(declaration)
	c.checkCaptures()
	return declaration
}

func (c *checker) chapter(value *jsonValue, path string) Chapter {
	chapter := Chapter{Requires: "anonymous"}
	if !c.requireObject(value, path) {
		return chapter
	}
	c.rejectUnknown(value, path, chapterFields)

	chapter.ID = c.requiredID(value, path, "id")
	c.recordChapterIdentity(chapter.ID, pointerChild(path, "id"))

	if formerValues, ok := c.optionalArray(value, path, "formerIds"); ok {
		for index, formerValue := range formerValues {
			formerPath := pointerIndex(pointerChild(path, "formerIds"), index)
			former := c.idValue(formerValue, formerPath)
			if former != "" {
				chapter.FormerIDs = append(chapter.FormerIDs, former)
				c.recordChapterIdentity(former, formerPath)
			}
		}
	}
	chapter.Title = c.textLimited(value, path, "title", MaxTitleChars, true)
	chapter.Why = c.textLimited(value, path, "why", MaxTextChars, true)

	if requiresValue, present := value.member("requires"); present {
		if requiresValue.kind != jsonString {
			c.report(CodeInvalidType, pointerChild(path, "requires"), "must be a string")
		} else {
			switch requiresValue.text {
			case "anonymous", "signed-in", "training-copy":
				chapter.Requires = requiresValue.text
			default:
				c.report(CodeInvalidValue, pointerChild(path, "requires"),
					"requires must be anonymous, signed-in or training-copy")
			}
		}
	}
	chapter.RequiresReason = c.textLimited(value, path, "requiresReason", MaxTextChars, false)
	if chapter.Requires != "anonymous" {
		reasonValue, present := value.member("requiresReason")
		if !present {
			c.report(CodeMissingField, pointerChild(path, "requiresReason"),
				"requiresReason is required for a gated chapter")
		} else if reasonValue.kind == jsonString && reasonValue.text == "" {
			c.report(CodeInvalidValue, pointerChild(path, "requiresReason"),
				"requiresReason must not be empty")
		}
	}

	chapter.ContinuesFrom = c.optionalID(value, path, "continuesFrom")

	readinessPresent := false
	if readinessValue, ok := value.member("readiness"); ok {
		readinessPresent = true
		condition := c.condition(readinessValue, pointerChild(path, "readiness"),
			c.nextStepOrdinal)
		if !condition.Supported || condition.Kind != "asset-exists" {
			c.report(CodeInvalidValue, pointerChild(pointerChild(path, "readiness"), "kind"),
				"readiness must be an asset-exists condition")
		}
		chapter.Readiness = &condition
	}

	stepValues := c.arrayField(value, path, "steps", true, 1, MaxStepsPerChapter)
	for index, stepValue := range stepValues {
		stepPath := pointerIndex(pointerChild(path, "steps"), index)
		chapter.Steps = append(chapter.Steps, c.step(stepValue, stepPath, readinessPresent))
	}
	entryFound := false
	if len(chapter.Steps) > 0 {
		for index := range chapter.Steps {
			if !containsString(chapter.Steps[index].Modes, "explore") {
				continue
			}
			entryFound = true
			chapter.EntryStepID = chapter.Steps[index].ID
			if chapter.Steps[index].Target.Supported && chapter.Steps[index].Target.Kind == "none" {
				c.report(CodeInvalidEntryTarget,
					pointerIndex(pointerChild(path, "steps"), index)+"/target",
					"the first Explore entry target must not be none")
			}
			break
		}
		if !entryFound {
			c.report(CodeMissingExploreEntry, pointerChild(path, "steps"),
				"chapter has no step available in explore mode")
		}
	}
	return chapter
}

func (c *checker) step(value *jsonValue, path string, chapterHasReadiness bool) Step {
	step := Step{Support: SupportSupported, Modes: []string{"guided", "explore"}}
	ordinal := c.nextStepOrdinal
	c.nextStepOrdinal++
	if !c.requireObject(value, path) {
		return step
	}
	c.rejectUnknown(value, path, stepFields)

	step.ID = c.requiredID(value, path, "id")
	if step.ID != "" {
		if existing, ok := c.stepIdentities[step.ID]; ok {
			c.report(CodeDuplicateID, pointerChild(path, "id"),
				"step id %q is already declared at %s", step.ID, existing)
		} else {
			c.stepIdentities[step.ID] = pointerChild(path, "id")
		}
	}
	step.Title = c.textLimited(value, path, "title", MaxTitleChars, true)
	step.Why = c.textLimited(value, path, "why", MaxTextChars, true)
	step.Hint = c.textLimited(value, path, "hint", MaxTextChars, false)

	if modesValue, ok := value.member("modes"); ok {
		if modesValue.kind != jsonArray {
			c.report(CodeInvalidType, pointerChild(path, "modes"), "modes must be an array")
		} else {
			modes := make([]string, 0, len(modesValue.array))
			for index, modeValue := range modesValue.array {
				modePath := pointerIndex(pointerChild(path, "modes"), index)
				if modeValue.kind != jsonString {
					c.report(CodeInvalidType, modePath, "mode must be a string")
					continue
				}
				if modeValue.text != "guided" && modeValue.text != "explore" {
					c.report(CodeInvalidValue, modePath, "mode must be guided or explore")
					continue
				}
				modes = append(modes, modeValue.text)
			}
			step.Modes = modes
		}
	}

	targetValue, targetPresent := value.member("target")
	if !targetPresent {
		c.report(CodeMissingField, pointerChild(path, "target"), "required field is missing")
	} else {
		step.Target = c.target(targetValue, pointerChild(path, "target"), ordinal)
	}
	doneValue, donePresent := value.member("done")
	if !donePresent {
		c.report(CodeMissingField, pointerChild(path, "done"), "required field is missing")
	} else {
		step.Done = c.condition(doneValue, pointerChild(path, "done"), ordinal)
	}

	if !step.Target.Supported || !step.Done.Supported {
		step.Support = SupportNeedsNewer
	}

	step.Capture = c.optionalID(value, path, "capture")
	if step.Capture != "" {
		if step.Done.Supported && !captureAllowed(step.Done.Kind) {
			c.report(CodeCaptureNotAllowed, pointerChild(path, "capture"),
				"capture is allowed only on parameter-value, record-selected and incident-open")
		}
		c.captures = append(c.captures, captureDecl{
			name:    step.Capture,
			ordinal: ordinal,
			path:    pointerChild(path, "capture"),
		})
	}

	if needsValue, present := value.member("needs"); present {
		if needsValue.kind != jsonString {
			c.report(CodeInvalidType, pointerChild(path, "needs"), "must be a string")
		} else if needsValue.text != "readiness" {
			c.report(CodeInvalidValue, pointerChild(path, "needs"),
				"needs must be readiness")
		} else {
			step.Needs = needsValue.text
			if !chapterHasReadiness {
				c.report(CodeInvalidReference, pointerChild(path, "needs"),
					"needs readiness requires the chapter to declare readiness")
			}
		}
	}
	return step
}

func (c *checker) checkChapterReferences(declaration *Declaration) {
	index := make(map[string]int, len(declaration.Chapters))
	for position, chapter := range declaration.Chapters {
		if chapter.ID != "" {
			if _, exists := index[chapter.ID]; !exists {
				index[chapter.ID] = position
			}
		}
	}
	for position, chapter := range declaration.Chapters {
		path := c.chapterPaths[position]
		if chapter.ContinuesFrom != "" {
			target, exists := index[chapter.ContinuesFrom]
			if !exists || target >= position {
				c.report(CodeInvalidReference, pointerChild(path, "continuesFrom"),
					"continuesFrom must name an earlier chapter")
			}
		}
		if chapter.Readiness != nil && chapter.ContinuesFrom == "" {
			c.report(CodeMissingField, pointerChild(path, "continuesFrom"),
				"readiness requires continuesFrom")
		}
	}
	if declaration.Finish.Kind == "chapter" {
		if _, exists := index[declaration.Finish.Chapter]; !exists {
			c.report(CodeInvalidReference, "/finish/chapter",
				"finish chapter reference does not resolve")
		}
	}
}

func (c *checker) checkCaptures() {
	seen := make(map[string]captureDecl, len(c.captures))
	first := make(map[string]captureDecl, len(c.captures))
	for _, declaration := range c.captures {
		if existing, ok := seen[declaration.name]; ok {
			c.report(CodeDuplicateID, declaration.path,
				"capture %q is already declared at %s", declaration.name, existing.path)
			continue
		}
		seen[declaration.name] = declaration
		first[declaration.name] = declaration
	}
	for _, reference := range c.captureRefs {
		producer, ok := first[reference.name]
		if !ok {
			c.report(CodeUnknownCapture, reference.path,
				"captured value %q has no producer", reference.name)
			continue
		}
		if producer.ordinal >= reference.ordinal {
			c.report(CodeCaptureNotEarlier, reference.path,
				"captured value %q is produced at %s or later", reference.name, producer.path)
		}
	}
}

func captureAllowed(kind string) bool {
	return kind == "parameter-value" || kind == "record-selected" || kind == "incident-open"
}

func (c *checker) recordChapterIdentity(name, path string) {
	if name == "" {
		return
	}
	if existing, ok := c.chapterIdentities[name]; ok {
		c.report(CodeDuplicateID, path,
			"chapter identity %q is already declared at %s", name, existing)
		return
	}
	c.chapterIdentities[name] = path
}

func (c *checker) requireObject(value *jsonValue, path string) bool {
	if value.kind == jsonObject {
		return true
	}
	c.report(CodeInvalidType, path, "must be a JSON object")
	return false
}

func (c *checker) rejectUnknown(object *jsonValue, path string, allowed map[string]bool) {
	for _, member := range object.object {
		if !allowed[member.key] {
			c.report(CodeUnknownField, pointerChild(path, member.key),
				"unknown field %q", member.key)
		}
	}
}

func (c *checker) text(object *jsonValue, path, name string, required bool) (string, bool) {
	value, present := object.member(name)
	if !present {
		if required {
			c.report(CodeMissingField, pointerChild(path, name), "required field is missing")
			return "", false
		}
		return "", true
	}
	if value.kind != jsonString {
		c.report(CodeInvalidType, pointerChild(path, name), "must be a string")
		return "", false
	}
	return value.text, true
}

func (c *checker) textLimited(object *jsonValue, path, name string, maxChars int, required bool) string {
	text, ok := c.text(object, path, name, required)
	if !ok {
		return ""
	}
	if maxChars > 0 && utf8.RuneCountInString(text) > maxChars {
		c.report(CodeTooLong, pointerChild(path, name),
			"must be at most %d characters", maxChars)
	}
	return text
}

func (c *checker) requiredID(object *jsonValue, path, name string) string {
	text, ok := c.text(object, path, name, true)
	if !ok {
		return ""
	}
	return c.checkID(text, pointerChild(path, name))
}

func (c *checker) optionalID(object *jsonValue, path, name string) string {
	value, present := object.member(name)
	if !present {
		return ""
	}
	if value.kind != jsonString {
		c.report(CodeInvalidType, pointerChild(path, name), "must be a string")
		return ""
	}
	return c.checkID(value.text, pointerChild(path, name))
}

func (c *checker) idValue(value *jsonValue, path string) string {
	if value.kind != jsonString {
		c.report(CodeInvalidType, path, "must be a string ID")
		return ""
	}
	return c.checkID(value.text, path)
}

func (c *checker) checkID(value, path string) string {
	if !idPattern.MatchString(value) {
		c.report(CodeInvalidValue, path,
			"must match ^[a-z0-9][a-z0-9-]{0,47}$")
		return ""
	}
	return value
}

func (c *checker) versionField(object *jsonValue, path string) Integer {
	value, present := object.member("version")
	if !present {
		c.report(CodeMissingField, pointerChild(path, "version"), "required field is missing")
		return ""
	}
	if value.kind != jsonNumber {
		c.report(CodeInvalidType, pointerChild(path, "version"), "must be a number")
		return ""
	}
	fieldPath := pointerChild(path, "version")
	if !canonicalIntegerPattern.MatchString(value.number) {
		c.report(CodeInvalidValue, fieldPath, "must be a canonical integer")
		return ""
	}
	if value.number == "0" {
		c.report(CodeInvalidValue, fieldPath, "must be at least 1")
		return ""
	}
	return Integer(value.number)
}

// boolField reads an optional boolean. Callers that need presence first check
// the member themselves (the exit index variant), so an absent field is always
// the optional case here.
func (c *checker) boolField(object *jsonValue, path, name string) (bool, bool) {
	value, present := object.member(name)
	if !present {
		return false, true
	}
	if value.kind != jsonBoolean {
		c.report(CodeInvalidType, pointerChild(path, name), "must be a boolean")
		return false, false
	}
	return value.bool, true
}

func (c *checker) arrayField(object *jsonValue, path, name string, required bool, min, max int) []*jsonValue {
	value, present := object.member(name)
	if !present {
		if required {
			c.report(CodeMissingField, pointerChild(path, name), "required field is missing")
		}
		return nil
	}
	if value.kind != jsonArray {
		c.report(CodeInvalidType, pointerChild(path, name), "must be an array")
		return nil
	}
	if len(value.array) < min {
		c.report(CodeTooMany, pointerChild(path, name), "must contain at least %d items", min)
	}
	if max > 0 && len(value.array) > max {
		c.report(CodeTooMany, pointerChild(path, name), "must contain at most %d items", max)
	}
	return value.array
}

func (c *checker) optionalArray(object *jsonValue, path, name string) ([]*jsonValue, bool) {
	value, present := object.member(name)
	if !present {
		return nil, false
	}
	if value.kind != jsonArray {
		c.report(CodeInvalidType, pointerChild(path, name), "must be an array")
		return nil, false
	}
	return value.array, true
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

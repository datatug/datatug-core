package walkthrough

import "sort"

var targetFields = map[string]map[string]bool{
	"table-record":         {"kind": true, "source": true, "table": true, "key": true},
	"query":                {"kind": true, "queryId": true, "parameters": true},
	"board":                {"kind": true, "boardId": true, "parameters": true, "tutorial": true},
	"card":                 {"kind": true, "cardId": true, "parameters": true},
	"related":              {"kind": true, "source": true, "table": true, "key": true, "meaning": true},
	"log-source":           {"kind": true, "source": true, "stream": true, "filter": true},
	"public-data-provider": {"kind": true, "provider": true},
	"incident":             {"kind": true, "incident": true},
	"check":                {"kind": true, "checkId": true},
	"none":                 {"kind": true},
}

var trainingTargetFields = map[string]map[string]bool{
	"challenge":     {"kind": true, "challengeId": true, "revision": true},
	"training-case": {"kind": true, "manifestCaseId": true, "trainingPackage": true},
}

var conditionFields = map[string]map[string]bool{
	"route-reached":   {"kind": true},
	"parameter-value": {"kind": true, "name": true, "equals": true},
	"record-selected": {"kind": true, "field": true, "equals": true},
	"query-run":       {"kind": true, "minRows": true},
	"panel-open":      {"kind": true, "panel": true},
	"filter-applied":  {"kind": true, "field": true, "equals": true},
	"check-run":       {"kind": true, "outcome": true},
	"incident-open":   {"kind": true},
	"asset-exists":    {"kind": true, "asset": true, "id": true},
	"board-tutorial":  {"kind": true, "untilCard": true},
	"acknowledged":    {"kind": true},
}

var trainingConditionFields = map[string]map[string]bool{
	"dialogue-finished": {"kind": true},
	"challenge-passed":  {"kind": true, "checkpoints": true},
}

var checkpointFields = map[string]bool{"id": true, "completion": true}

// knownTargetKind is the closed registry for the active profile. The /2-only
// kinds are not admitted into ordinary /1, where they remain unknown step-local
// kinds.
func (c *checker) knownTargetKind(kind string) bool {
	if knownTargetKinds[kind] {
		return true
	}
	if !c.training {
		return false
	}
	_, ok := trainingTargetFields[kind]
	return ok
}

// knownConditionKind is the closed registry for the active profile.
func (c *checker) knownConditionKind(kind string) bool {
	if knownConditionKinds[kind] {
		return true
	}
	if !c.training {
		return false
	}
	_, ok := trainingConditionFields[kind]
	return ok
}

func (c *checker) targetFieldSet(kind string) map[string]bool {
	if fields, ok := targetFields[kind]; ok {
		return fields
	}
	return trainingTargetFields[kind]
}

func (c *checker) conditionFieldSet(kind string) map[string]bool {
	if fields, ok := conditionFields[kind]; ok {
		return fields
	}
	return trainingConditionFields[kind]
}

func (c *checker) target(value *jsonValue, path string, ordinal int) Target {
	if !c.requireObject(value, path) {
		return Target{}
	}
	kindValue, present := value.member("kind")
	if !present {
		c.report(CodeMissingField, pointerChild(path, "kind"), "required field is missing")
		return Target{}
	}
	if kindValue.kind != jsonString {
		c.report(CodeInvalidType, pointerChild(path, "kind"), "kind must be a string")
		return Target{}
	}
	kind := kindValue.text
	if !c.knownTargetKind(kind) {
		return Target{Kind: kind, Supported: false}
	}
	target := Target{Kind: kind, Supported: true}
	c.rejectUnknown(value, path, c.targetFieldSet(kind))
	switch kind {
	case "table-record":
		target.TableRecord = &TableRecordTarget{
			Source: c.assetString(value, path, "source", true),
			Table:  c.assetString(value, path, "table", true),
			Key:    c.namedValues(value, path, "key", false, ordinal, false),
		}
	case "query":
		target.Query = &QueryTarget{
			QueryID:    c.assetString(value, path, "queryId", true),
			Parameters: c.namedValues(value, path, "parameters", false, ordinal, false),
		}
	case "board":
		tutorial, _ := c.boolField(value, path, "tutorial")
		target.Board = &BoardTarget{
			BoardID:    c.assetString(value, path, "boardId", true),
			Parameters: c.namedValues(value, path, "parameters", false, ordinal, false),
			Tutorial:   tutorial,
		}
	case "card":
		target.Card = &CardTarget{
			CardID:     c.assetString(value, path, "cardId", true),
			Parameters: c.namedValues(value, path, "parameters", false, ordinal, false),
		}
	case "related":
		target.Related = &RelatedTarget{
			Source:  c.assetString(value, path, "source", true),
			Table:   c.assetString(value, path, "table", true),
			Key:     c.namedValues(value, path, "key", true, ordinal, false),
			Meaning: c.optionalText(value, path, "meaning"),
		}
	case "log-source":
		target.LogSource = &LogSourceTarget{
			Source: c.assetString(value, path, "source", true),
			Stream: c.optionalText(value, path, "stream"),
			Filter: c.namedValues(value, path, "filter", false, ordinal, false),
		}
	case "public-data-provider":
		target.PublicDataProvider = &PublicDataProviderTarget{
			Provider: c.assetString(value, path, "provider", true),
		}
	case "incident":
		incident := &IncidentTarget{}
		if incidentValue, ok := value.member("incident"); ok {
			parsed := c.value(incidentValue, pointerChild(path, "incident"), ordinal, true)
			incident.Incident = &parsed
		}
		target.Incident = incident
	case "check":
		target.Check = &CheckTarget{CheckID: c.assetString(value, path, "checkId", true)}
	case "none":
		target.None = &struct{}{}
	case "challenge":
		target.Challenge = &ChallengeTarget{
			ChallengeID: c.challengeID(value, path),
			Revision:    c.challengeRevision(value, path),
		}
	case "training-case":
		parsed := &TrainingCaseTarget{
			ManifestCaseID:  c.requiredID(value, path, "manifestCaseId"),
			TrainingPackage: c.packageReferenceField(value, path, "trainingPackage"),
		}
		if c.trainingPackage.Path != "" && parsed.TrainingPackage.Path != "" &&
			!parsed.TrainingPackage.Equal(c.trainingPackage) {
			c.report(CodeMismatch, pointerChild(pointerChild(path, "trainingPackage"), "id"),
				"training-case trainingPackage must equal the declaration trainingPackage field for field")
		}
		target.TrainingCase = parsed
	}
	return target
}

func (c *checker) condition(value *jsonValue, path string, ordinal int) Condition {
	if !c.requireObject(value, path) {
		return Condition{}
	}
	kindValue, present := value.member("kind")
	if !present {
		c.report(CodeMissingField, pointerChild(path, "kind"), "required field is missing")
		return Condition{}
	}
	if kindValue.kind != jsonString {
		c.report(CodeInvalidType, pointerChild(path, "kind"), "kind must be a string")
		return Condition{}
	}
	kind := kindValue.text
	if !c.knownConditionKind(kind) {
		return Condition{Kind: kind, Supported: false}
	}
	condition := Condition{Kind: kind, Supported: true}
	c.rejectUnknown(value, path, c.conditionFieldSet(kind))
	switch kind {
	case "parameter-value":
		condition.ParameterValue = &ParameterValueCondition{
			Name:   c.assetString(value, path, "name", true),
			Equals: c.optionalLiteral(value, path, "equals"),
		}
	case "record-selected":
		condition.RecordSelected = &RecordSelectedCondition{
			Field:  c.optionalText(value, path, "field"),
			Equals: c.optionalLiteral(value, path, "equals"),
		}
	case "query-run":
		condition.QueryRun = &QueryRunCondition{MinRows: c.optionalInteger(value, path, "minRows")}
	case "panel-open":
		condition.PanelOpen = &PanelOpenCondition{Panel: c.enumField(value, path, "panel", true,
			"related", "record-details", "query-inspection", "log-entry")}
	case "filter-applied":
		condition.FilterApplied = &FilterAppliedCondition{
			Field:  c.assetString(value, path, "field", true),
			Equals: c.optionalLiteral(value, path, "equals"),
		}
	case "check-run":
		condition.CheckRun = &CheckRunCondition{Outcome: c.enumField(value, path, "outcome", false,
			"pass", "fail")}
	case "asset-exists":
		condition.AssetExists = &AssetExistsCondition{
			Asset: c.enumField(value, path, "asset", true,
				"query", "board", "card", "check", "meaning-mapping", "incident"),
			ID: c.assetString(value, path, "id", true),
		}
	case "board-tutorial":
		condition.BoardTutorial = &BoardTutorialCondition{UntilCard: c.optionalText(value, path, "untilCard")}
	case "dialogue-finished":
		condition.DialogueFinished = &DialogueFinishedCondition{}
	case "challenge-passed":
		condition.ChallengePassed = &ChallengePassedCondition{Checkpoints: c.checkpointMappings(value, path)}
	}
	return condition
}

// checkpointMappings parses exactly one closed {id, completion: whole-challenge}
// mapping. More than one, none, unknown fields, part-level kinds and unknown
// completion kinds all refuse before rendering.
func (c *checker) checkpointMappings(object *jsonValue, path string) []CheckpointMapping {
	values := c.arrayField(object, path, "checkpoints", true, 1, 1)
	mappings := make([]CheckpointMapping, 0, len(values))
	for index, value := range values {
		mappingPath := pointerIndex(pointerChild(path, "checkpoints"), index)
		if !c.requireObject(value, mappingPath) {
			continue
		}
		c.rejectUnknown(value, mappingPath, checkpointFields)
		mappings = append(mappings, CheckpointMapping{
			ID:         c.requiredID(value, mappingPath, "id"),
			Completion: c.enumField(value, mappingPath, "completion", true, CompletionWholeChallenge),
		})
	}
	return mappings
}

// challengeID validates the challenge-execution owner's lowercase slug
// contract, which is not the shorter walkthrough ID grammar.
func (c *checker) challengeID(object *jsonValue, path string) string {
	text, ok := c.text(object, path, "challengeId", true)
	if !ok {
		return ""
	}
	if !challengeIDPattern.MatchString(text) {
		c.report(CodeInvalidValue, pointerChild(path, "challengeId"),
			"must match ^[a-z][a-z0-9-]{0,63}$")
		return ""
	}
	return text
}

// challengeRevision validates the challenge-execution owner's revision range
// without converting the lexeme to a machine integer.
func (c *checker) challengeRevision(object *jsonValue, path string) Integer {
	return c.challengeRevisionNamed(object, path, "revision")
}

func (c *checker) challengeRevisionNamed(object *jsonValue, path, name string) Integer {
	revision := c.positiveInteger(object, path, name, true)
	if revision == "" {
		return ""
	}
	if !integerAtMost(string(revision), "1000000") {
		c.report(CodeInvalidValue, pointerChild(path, name),
			"must be between 1 and 1000000")
		return ""
	}
	return revision
}

// integerAtMost compares a canonical positive decimal lexeme with a bound
// without a numeric conversion.
func integerAtMost(value, bound string) bool {
	if len(value) != len(bound) {
		return len(value) < len(bound)
	}
	return value <= bound
}

// packageReferenceField parses one required package-reference member.
func (c *checker) packageReferenceField(object *jsonValue, path, name string) PackageReference {
	value, present := object.member(name)
	if !present {
		c.report(CodeMissingField, pointerChild(path, name), "required field is missing")
		return PackageReference{}
	}
	return c.packageReference(value, pointerChild(path, name))
}

func (c *checker) exit(value *jsonValue, path string) Exit {
	if !c.requireObject(value, path) {
		return Exit{Kind: "index"}
	}
	c.rejectUnknown(value, path, map[string]bool{
		"chapter": true, "walkthrough": true, "target": true, "index": true,
	})
	count := 0
	for _, name := range []string{"chapter", "walkthrough", "target", "index"} {
		if _, ok := value.member(name); ok {
			count++
		}
	}
	if count != 1 {
		c.report(CodeInvalidValue, path, "exactly one exit variant is required")
	}
	if _, ok := value.member("chapter"); ok {
		return Exit{Kind: "chapter", Chapter: c.requiredID(value, path, "chapter")}
	}
	if _, ok := value.member("walkthrough"); ok {
		return Exit{Kind: "walkthrough", Walkthrough: c.requiredID(value, path, "walkthrough")}
	}
	if targetValue, ok := value.member("target"); ok {
		target := c.target(targetValue, pointerChild(path, "target"), c.nextStepOrdinal)
		return Exit{Kind: "target", Target: &target}
	}
	if _, ok := value.member("index"); ok {
		index, valid := c.boolField(value, path, "index")
		if valid && !index {
			c.report(CodeInvalidValue, pointerChild(path, "index"), "index must be true")
		}
		return Exit{Kind: "index"}
	}
	return Exit{Kind: "index"}
}

func (c *checker) assetString(object *jsonValue, path, name string, required bool) string {
	text, ok := c.text(object, path, name, required)
	if !ok {
		return ""
	}
	if text == "" {
		c.report(CodeInvalidValue, pointerChild(path, name), "must not be empty")
	}
	return text
}

func (c *checker) optionalText(object *jsonValue, path, name string) string {
	text, _ := c.text(object, path, name, false)
	return text
}

func (c *checker) enumField(object *jsonValue, path, name string, required bool, allowed ...string) string {
	value, present := object.member(name)
	if !present {
		if required {
			c.report(CodeMissingField, pointerChild(path, name), "required field is missing")
		}
		return ""
	}
	if value.kind != jsonString {
		c.report(CodeInvalidType, pointerChild(path, name), "must be a string")
		return ""
	}
	for _, candidate := range allowed {
		if value.text == candidate {
			return value.text
		}
	}
	c.report(CodeInvalidValue, pointerChild(path, name),
		"must be one of %v", allowed)
	return ""
}

func (c *checker) optionalInteger(object *jsonValue, path, name string) *Integer {
	value, present := object.member(name)
	if !present {
		return nil
	}
	if value.kind != jsonNumber {
		c.report(CodeInvalidType, pointerChild(path, name), "must be a number")
		return nil
	}
	if !canonicalIntegerPattern.MatchString(value.number) {
		c.report(CodeInvalidValue, pointerChild(path, name), "must be a canonical non-negative integer")
		return nil
	}
	integer := Integer(value.number)
	return &integer
}

func (c *checker) optionalLiteral(object *jsonValue, path, name string) *Scalar {
	value, present := object.member(name)
	if !present {
		return nil
	}
	return c.scalarValue(value, pointerChild(path, name), true)
}

func (c *checker) scalarValue(value *jsonValue, path string, allowNull bool) *Scalar {
	switch value.kind {
	case jsonNull:
		if !allowNull {
			c.report(CodeInvalidValue, path, "null is not allowed here")
			return nil
		}
		return &Scalar{Kind: ScalarNull}
	case jsonBoolean:
		return &Scalar{Kind: ScalarBoolean, Bool: value.bool}
	case jsonNumber:
		return &Scalar{Kind: ScalarNumber, Number: value.number}
	case jsonString:
		return &Scalar{Kind: ScalarString, Text: value.text}
	default:
		c.report(CodeInvalidType, path, "must be a JSON scalar")
		return nil
	}
}

func (c *checker) value(value *jsonValue, path string, ordinal int, allowNullDefault bool) Value {
	switch value.kind {
	case jsonNull, jsonBoolean, jsonNumber, jsonString:
		return Value{Literal: c.scalarValue(value, path, true)}
	case jsonObject:
		// fall through to the exact captured/default object
	default:
		c.report(CodeInvalidType, path, "must be a JSON scalar or a {captured, default} object")
		return Value{}
	}
	c.rejectUnknown(value, path, map[string]bool{"captured": true, "default": true})
	capturedValue, capturedPresent := value.member("captured")
	defaultValue, defaultPresent := value.member("default")
	if !capturedPresent && !defaultPresent {
		c.report(CodeInvalidType, path, "must be a JSON scalar or a {captured, default} object")
		return Value{}
	}
	result := Value{}
	if !capturedPresent {
		c.report(CodeMissingField, pointerChild(path, "captured"), "required field is missing")
		return result
	}
	if capturedValue.kind != jsonString {
		c.report(CodeInvalidType, pointerChild(path, "captured"), "must be a string")
		return result
	}
	result.Captured = c.checkID(capturedValue.text, pointerChild(path, "captured"))
	if result.Captured != "" {
		c.captureRefs = append(c.captureRefs, captureRef{
			name:    result.Captured,
			ordinal: ordinal,
			path:    pointerChild(path, "captured"),
		})
	}
	if !defaultPresent {
		c.report(CodeMissingDefault, pointerChild(path, "default"),
			"a captured value requires a literal default")
		return result
	}
	result.Default = c.scalarValue(defaultValue, pointerChild(path, "default"), allowNullDefault)
	return result
}

func (c *checker) namedValues(object *jsonValue, path, name string, required bool, ordinal int, allowNullDefault bool) []NamedValue {
	value, present := object.member(name)
	if !present {
		if required {
			c.report(CodeMissingField, pointerChild(path, name), "required field is missing")
		}
		return nil
	}
	if value.kind != jsonObject {
		c.report(CodeInvalidType, pointerChild(path, name), "must be a JSON object")
		return nil
	}
	values := make([]NamedValue, 0, len(value.object))
	for _, member := range value.object {
		values = append(values, NamedValue{
			Name:  member.key,
			Value: c.value(member.value, pointerChild(pointerChild(path, name), member.key), ordinal, allowNullDefault),
		})
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Name < values[j].Name })
	return values
}

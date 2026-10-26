package walkthrough

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/challenge"
	"github.com/datatug/datatug-core/pkg/detective"
	"github.com/datatug/datatug-core/pkg/investigation"
)

var (
	evidenceRecordsFields = map[string]bool{"columns": true, "rows": true}
	recordColumnFields    = map[string]bool{"name": true, "type": true}
	queryMetadataFields   = map[string]bool{
		"id": true, "title": true, "type": true, "connectionId": true, "parameters": true,
	}
	trainingParameterFields = map[string]bool{
		"id": true, "type": true, "title": true, "isRequired": true, "defaultValue": true,
	}
	evidenceLogFields = map[string]bool{
		"id": true, "at": true, "actorId": true, "kind": true, "message": true,
	}
	checkDefinitionFields = map[string]bool{
		"id": true, "title": true, "description": true, "version": true,
		"inputs": true, "steps": true, "updatedAt": true, "updatedBy": true, "createdAt": true,
	}
	checkInputFields = map[string]bool{
		"name": true, "type": true, "cardinality": true, "required": true,
	}
	checkStepFields = map[string]bool{
		"id": true, "kind": true, "queryId": true, "checkId": true,
		"bindings": true, "validators": true, "onUnavailable": true,
	}
	checkBindingFields = map[string]bool{
		"input": true, "fromInput": true, "part": true, "fromColumn": true,
	}
	checkValidatorFields = map[string]bool{
		"id": true, "validatorId": true, "scope": true, "kind": true, "severity": true,
		"field": true, "otherField": true, "expected": true, "checkId": true,
		"bindings": true, "onUnavailable": true,
	}
	watchRecommendationFields = map[string]bool{
		"title": true, "checkId": true, "checkVersion": true, "inputs": true,
		"schedule": true, "trigger": true, "initialObservation": true, "mode": true,
		"notificationPolicy": true,
	}
	watchScheduleFields    = map[string]bool{"intervalSeconds": true}
	watchNotificationField = map[string]bool{"cooldownSeconds": true, "recovery": true}
	typedValueFields       = map[string]bool{"type": true, "value": true}
	inputValueSetFields    = map[string]bool{"type": true, "values": true}
	inputValueRangeFields  = map[string]bool{"type": true, "start": true, "end": true}

	// The challenge manifest is owned by the execution package. These maps
	// mirror its actual wire DTO exactly so the package stage can enforce the
	// owner's exact-case/unknown/null rules before native semantic validation.
	manifestRootFields = map[string]bool{
		"schemaVersion": true, "gradingVersion": true, "challengeId": true,
		"revision": true, "title": true, "objective": true, "difficulty": true,
		"dialect": true, "allowedOperation": true, "timing": true, "engine": true,
		"fixtureSuite": true, "visibleSchema": true, "parts": true, "hints": true,
		"referenceSolutions": true,
	}
	manifestEngineFields = map[string]bool{
		"package": true, "version": true, "sqliteVersion": true,
	}
	manifestFixtureSuiteFields = map[string]bool{"id": true, "fixtures": true}
	manifestFixtureFields      = map[string]bool{
		"id": true, "label": true, "sha256": true, "bytes": true, "runs": true,
	}
	manifestVisibleSchemaFields = map[string]bool{"tables": true}
	manifestVisibleTableFields  = map[string]bool{"name": true, "columns": true}
	manifestVisibleColumnFields = map[string]bool{
		"name": true, "type": true, "primaryKey": true, "notNull": true, "collation": true,
	}
	manifestPartFields = map[string]bool{
		"id": true, "title": true, "required": true, "answerKind": true,
		"sql": true, "structured": true, "checkDefinition": true,
	}
	manifestSQLPartFields      = map[string]bool{"cases": true}
	manifestSQLCaseFields      = map[string]bool{"fixture": true, "oracle": true}
	manifestResultOracleFields = map[string]bool{
		"kind": true, "columns": true, "rows": true, "ordering": true,
		"requireColumnNames": true, "numericTolerance": true,
	}
	manifestFinalStateFields = map[string]bool{"kind": true, "tables": true, "invariants": true}
	manifestTableFields      = map[string]bool{"name": true, "columns": true, "rows": true}
	manifestInvariantFields  = map[string]bool{
		"id": true, "kind": true, "table": true, "groupBy": true, "column": true,
		"description": true,
	}
	manifestResultColumnFields    = map[string]bool{"name": true, "type": true}
	manifestStructuredFields      = map[string]bool{"fields": true, "expected": true}
	manifestStructuredFieldFields = map[string]bool{
		"id": true, "label": true, "cardinality": true, "required": true,
		"minSelections": true, "maxSelections": true, "options": true,
	}
	manifestStructuredOptionFields = map[string]bool{"id": true, "label": true}
	manifestStructuredAnswerFields = map[string]bool{"selections": true}
	manifestCheckDefinitionFields  = map[string]bool{
		"queryIds": true, "expectation": true, "cases": true,
	}
	manifestCheckCaseFields         = map[string]bool{"fixture": true, "expectedOutcome": true}
	manifestHintFields              = map[string]bool{"id": true, "text": true}
	manifestReferenceSolutionFields = map[string]bool{"partId": true, "answer": true}
	manifestReferenceAnswerFields   = map[string]bool{
		"sql": true, "structured": true, "checkDefinition": true,
	}
	manifestCheckDefinitionAnswerFields = map[string]bool{"queryId": true, "expectation": true}
)

// trainingScalarTypes is the closed evidence-records column discriminator set.
var trainingScalarTypes = map[investigation.ValueType]bool{
	investigation.ValueTypeString:   true,
	investigation.ValueTypeNumber:   true,
	investigation.ValueTypeInteger:  true,
	investigation.ValueTypeDecimal:  true,
	investigation.ValueTypeBoolean:  true,
	investigation.ValueTypeDate:     true,
	investigation.ValueTypeDatetime: true,
	investigation.ValueTypeNull:     true,
}

// parseRFC3339 parses one RFC3339 instant. Callers add the UTC requirement
// where the owner field is a fictional scenario time.
func parseRFC3339(text string) (time.Time, error) { return time.Parse(time.RFC3339, text) }

// childJSON parses one closed child object after UTF-8 validation. The
// returned pointer tree is owned by this call only.
func (c *checker) childJSON(data []byte, path string) (*jsonValue, bool) {
	if !utf8.Valid(data) {
		c.report(CodeInvalidUTF8, path, "asset bytes must be valid UTF-8")
		return nil, false
	}
	root, issue := parseJSON(data)
	if issue != nil {
		issue.Path = pointerSuffix(path, issue.Path)
		c.issues = append(c.issues, *issue)
		return nil, false
	}
	if root.kind != jsonObject {
		c.report(CodeNotObject, path, "asset must be a JSON object")
		return nil, false
	}
	return root, true
}

// pointerSuffix joins a child-document pointer to the package asset path.
func pointerSuffix(base, suffix string) string {
	if suffix == "" {
		return base
	}
	return base + suffix
}

// parsePackageCodecs parses every verified child asset by its owner codec.
// It runs after byte verification and text/binary accounting, so a codec never
// sees unverified or over-budget bytes.
func (c *checker) parsePackageCodecs(pkg *AdmittedTrainingPackage, bytesByPath map[string][]byte) {
	actors := make(map[string]bool, len(pkg.FictionalActors))
	for _, actor := range pkg.FictionalActors {
		actors[actor.ID] = true
	}
	for index := range pkg.Assets {
		asset := &pkg.Assets[index]
		path := pointerIndex("/assets", index)
		data := bytesByPath[asset.Reference.Path]
		switch asset.Kind {
		case "evidence-text":
			if !utf8.Valid(data) {
				c.report(CodeInvalidUTF8, path, "evidence text must be valid UTF-8")
				continue
			}
			asset.Text = string(data)
		case "evidence-records":
			asset.Records = c.parseEvidenceRecords(data, path)
		case "evidence-log":
			asset.Log = c.parseEvidenceLog(data, path, actors)
		case "query-metadata":
			asset.Metadata = c.parseQueryMetadata(data, path)
		case "query-body-sql":
			if !utf8.Valid(data) {
				c.report(CodeInvalidUTF8, path, "SQL body must be valid UTF-8")
				continue
			}
			asset.Text = string(data)
		case "check-definition":
			asset.Check = c.parseCheckDefinition(data, path)
			if asset.Check != nil {
				if err := detective.ValidateCheck(*asset.Check); err != nil {
					c.report(CodeInvalidValue, path, "native Check validation refused the definition")
				}
			}
		case "challenge-manifest":
			root, ok := c.childJSON(data, path)
			if !ok {
				continue
			}
			c.validateChallengeManifestShape(root, path)
			if len(c.issues) > 0 {
				continue
			}
			manifest, err := challenge.LoadManifest(data)
			if err != nil {
				c.reportChallengeError(path, err)
				continue
			}
			asset.Manifest = &manifest
		case "challenge-fixture":
			asset.Binary = true
		case "watch-recommendation":
			asset.Watch = c.parseWatchRecommendation(data, path)
		}
	}
}

func (c *checker) parseEvidenceRecords(data []byte, path string) *TrainingRecords {
	root, ok := c.childJSON(data, path)
	if !ok {
		return nil
	}
	c.rejectUnknown(root, path, evidenceRecordsFields)
	records := &TrainingRecords{}
	columns := c.arrayField(root, path, "columns", true, 1, 100)
	names := make(map[string]bool, len(columns))
	typeOK := make([]bool, 0, len(columns))
	for index, value := range columns {
		columnPath := pointerIndex(pointerChild(path, "columns"), index)
		if !c.requireObject(value, columnPath) {
			typeOK = append(typeOK, false)
			continue
		}
		c.rejectUnknown(value, columnPath, recordColumnFields)
		name := c.textValue(value, columnPath, "name", true)
		if name == "" {
			c.report(CodeInvalidValue, pointerChild(columnPath, "name"), "must be nonempty")
		} else if names[name] {
			c.report(CodeDuplicateID, pointerChild(columnPath, "name"), "column name %q is repeated", name)
		}
		names[name] = true
		kind := investigation.ValueType(c.textValue(value, columnPath, "type", true))
		known := trainingScalarTypes[kind]
		if !known {
			c.report(CodeInvalidValue, pointerChild(columnPath, "type"),
				"must be a native scalar TypedValue discriminator")
		}
		typeOK = append(typeOK, known)
		records.Columns = append(records.Columns, TrainingRecordColumn{Name: name, Type: kind})
	}
	rows := c.arrayField(root, path, "rows", true, 0, 1000)
	records.Rows = make([][]investigation.TypedValue, 0, len(rows))
	for rowIndex, row := range rows {
		rowPath := pointerIndex(pointerChild(path, "rows"), rowIndex)
		if row.kind != jsonArray {
			c.report(CodeInvalidType, rowPath, "must be an array")
			continue
		}
		if len(row.array) != len(records.Columns) {
			c.report(CodeInvalidValue, rowPath,
				"row must have exactly the declared column count")
			continue
		}
		cells := make([]investigation.TypedValue, 0, len(row.array))
		for cellIndex, cell := range row.array {
			cellPath := pointerIndex(rowPath, cellIndex)
			before := len(c.issues)
			typed := c.typedValue(cell, cellPath)
			column := records.Columns[cellIndex]
			if len(c.issues) == before && typeOK[cellIndex] &&
				typed.Type != investigation.ValueTypeNull && typed.Type != column.Type {
				c.report(CodeInvalidValue, cellPath,
					"value type must equal its column's declared type or be null")
			}
			cells = append(cells, typed)
		}
		records.Rows = append(records.Rows, cells)
	}
	return records
}

func (c *checker) parseEvidenceLog(data []byte, path string, actors map[string]bool) []TrainingLogEntry {
	if !utf8.Valid(data) {
		c.report(CodeInvalidUTF8, path, "evidence log must be valid UTF-8")
		return nil
	}
	entries := make([]TrainingLogEntry, 0)
	ids := make(map[string]bool)
	lines := strings.Split(string(data), "\n")
	for lineIndex, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if line == "" {
			continue
		}
		linePath := pointerIndex(path, lineIndex)
		root, issue := parseJSON([]byte(line))
		if issue != nil {
			issue.Path = pointerSuffix(linePath, issue.Path)
			c.issues = append(c.issues, *issue)
			continue
		}
		if root.kind != jsonObject {
			c.report(CodeNotObject, linePath, "log line must be a JSON object")
			continue
		}
		c.rejectUnknown(root, linePath, evidenceLogFields)
		entry := TrainingLogEntry{
			ID:      c.textValue(root, linePath, "id", true),
			At:      c.textValue(root, linePath, "at", true),
			ActorID: c.requiredID(root, linePath, "actorId"),
			Kind:    c.textValue(root, linePath, "kind", true),
			Message: c.textValue(root, linePath, "message", true),
		}
		if entry.ID == "" {
			c.report(CodeInvalidValue, pointerChild(linePath, "id"), "must be nonempty")
		} else if ids[entry.ID] {
			c.report(CodeDuplicateID, pointerChild(linePath, "id"), "log entry id %q is repeated", entry.ID)
		}
		ids[entry.ID] = true
		if !validFictionalTimestamp(entry.At) {
			c.report(CodeInvalidValue, pointerChild(linePath, "at"),
				"at must be a fictional RFC3339 UTC timestamp")
		}
		if !actors[entry.ActorID] {
			c.report(CodeInvalidReference, pointerChild(linePath, "actorId"),
				"actorId must resolve a declared fictional actor")
		}
		if entry.Kind == "" {
			c.report(CodeInvalidValue, pointerChild(linePath, "kind"), "must be nonempty")
		}
		if entry.Message == "" {
			c.report(CodeInvalidValue, pointerChild(linePath, "message"), "must be nonempty")
		}
		entries = append(entries, entry)
	}
	return entries
}

func (c *checker) parseQueryMetadata(data []byte, path string) *TrainingQueryMetadata {
	root, ok := c.childJSON(data, path)
	if !ok {
		return nil
	}
	c.rejectUnknown(root, path, queryMetadataFields)
	metadata := &TrainingQueryMetadata{
		ID:           c.textValue(root, path, "id", true),
		Title:        c.textLimited(root, path, "title", MaxTrainingTitleChars, true),
		Type:         c.textValue(root, path, "type", true),
		ConnectionID: c.textValue(root, path, "connectionId", true),
	}
	if metadata.ID == "" {
		c.report(CodeInvalidValue, pointerChild(path, "id"), "must be nonempty")
	}
	if metadata.Type != "SQL" {
		c.report(CodeInvalidValue, pointerChild(path, "type"), "type must be \"SQL\"")
	}
	if metadata.ConnectionID == "" {
		c.report(CodeInvalidValue, pointerChild(path, "connectionId"), "must be nonempty")
	}
	parameters := c.arrayField(root, path, "parameters", true, 0, 0)
	parameterIDs := make(map[string]bool, len(parameters))
	for index, value := range parameters {
		parameterPath := pointerIndex(pointerChild(path, "parameters"), index)
		parameter := c.trainingParameter(value, parameterPath)
		if parameter.ID != "" {
			if parameterIDs[parameter.ID] {
				c.report(CodeDuplicateID, pointerChild(parameterPath, "id"),
					"parameter id %q is repeated", parameter.ID)
			}
			parameterIDs[parameter.ID] = true
		}
		metadata.Parameters = append(metadata.Parameters, parameter)
	}
	return metadata
}

func (c *checker) trainingParameter(value *jsonValue, path string) TrainingParameter {
	if !c.requireObject(value, path) {
		return TrainingParameter{}
	}
	// The allowlist is checked on the raw shape before native validation, so a
	// forbidden key refuses even when it is false, null or empty.
	c.rejectUnknown(value, path, trainingParameterFields)
	parameter := TrainingParameter{
		ID:   c.textValue(value, path, "id", true),
		Type: c.textValue(value, path, "type", true),
	}
	if parameter.ID == "" {
		c.report(CodeInvalidValue, pointerChild(path, "id"), "must be nonempty")
	}
	if !oneOfTraining(parameter.Type, "string", "integer", "number", "boolean") {
		c.report(CodeInvalidValue, pointerChild(path, "type"),
			"type must be string, integer, number or boolean")
	}
	parameter.Title, _ = c.optionalTextField(value, path, "title")
	if parameter.Title != nil && utf8.RuneCountInString(*parameter.Title) > MaxTrainingTitleChars {
		c.report(CodeTooLong, pointerChild(path, "title"),
			"must be at most %d characters", MaxTrainingTitleChars)
	}
	if required, present := c.optionalBoolField(value, path, "isRequired"); present {
		parameter.IsRequired = required
	}
	parameter.Default = c.trainingParameterDefault(value, path, parameter.Type)
	return parameter
}

// optionalBoolField returns the pointer only when the field is present.
func (c *checker) optionalBoolField(object *jsonValue, path, name string) (*bool, bool) {
	value, present := object.member(name)
	if !present {
		return nil, true
	}
	if value.kind != jsonBoolean {
		c.report(CodeInvalidType, pointerChild(path, name), "must be a boolean")
		return nil, false
	}
	boolean := value.bool
	return &boolean, true
}

// trainingParameterDefault applies the exact scalar/default rules and keeps
// omission distinct from explicit null.
func (c *checker) trainingParameterDefault(object *jsonValue, path, parameterType string) *TrainingParameterDefault {
	value, present := object.member("defaultValue")
	if !present {
		return nil
	}
	defaultPath := pointerChild(path, "defaultValue")
	typ := parameterType
	if !oneOfTraining(typ, "string", "integer", "number", "boolean") {
		// The declared type was already refused; do not invent a second
		// interpretation of the default.
		return nil
	}
	switch typ {
	case "string":
		if value.kind != jsonString {
			c.report(CodeInvalidType, defaultPath, "must be a JSON string")
			return nil
		}
		return &TrainingParameterDefault{Kind: typ, Text: value.text}
	case "boolean":
		if value.kind != jsonBoolean {
			c.report(CodeInvalidType, defaultPath, "must be a JSON boolean")
			return nil
		}
		return &TrainingParameterDefault{Kind: typ, Bool: value.bool}
	case "integer":
		if value.kind != jsonNumber || !trainingIntegerPattern.MatchString(value.number) {
			c.report(CodeInvalidType, defaultPath, "must be an exact JSON integer")
			return nil
		}
		parsed, err := strconv.ParseInt(value.number, 10, 64)
		if err != nil || parsed < -9007199254740991 || parsed > 9007199254740991 {
			c.report(CodeInvalidValue, defaultPath,
				"integer default must be within the native safe range")
			return nil
		}
		return &TrainingParameterDefault{Kind: typ, Number: value.number}
	default:
		if value.kind != jsonNumber {
			c.report(CodeInvalidType, defaultPath, "must be a JSON number")
			return nil
		}
		parsed, err := strconv.ParseFloat(value.number, 64)
		if err != nil {
			c.report(CodeInvalidValue, defaultPath, "number default must be a finite binary64")
			return nil
		}
		return &TrainingParameterDefault{Kind: typ, Number: value.number, Float: parsed}
	}
}

func (c *checker) parseCheckDefinition(data []byte, path string) *detective.Check {
	root, ok := c.childJSON(data, path)
	if !ok {
		return nil
	}
	c.rejectUnknown(root, path, checkDefinitionFields)
	check := &detective.Check{
		ID:        c.textValue(root, path, "id", true),
		Title:     c.textValue(root, path, "title", true),
		UpdatedBy: c.textValue(root, path, "updatedBy", true),
	}
	if description, _ := c.optionalTextField(root, path, "description"); description != nil {
		check.Description = *description
	}
	check.Version, _ = c.canonicalInt64Field(root, path, "version")
	if createdAt, _ := c.optionalTextField(root, path, "createdAt"); createdAt != nil {
		check.CreatedAt = *createdAt
	}
	check.UpdatedAt = c.requiredTimestamp(root, path, "updatedAt")
	check.Inputs = c.parseCheckInputs(root, path)
	check.Steps = c.parseCheckSteps(root, path)
	return check
}

func (c *checker) requiredTimestamp(object *jsonValue, path, name string) time.Time {
	text, ok := c.text(object, path, name, true)
	if !ok {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		c.report(CodeInvalidValue, pointerChild(path, name),
			"must be an RFC3339 timestamp")
		return time.Time{}
	}
	return parsed
}

func (c *checker) parseCheckInputs(root *jsonValue, path string) []detective.Input {
	values := c.arrayField(root, path, "inputs", true, 0, 0)
	inputs := make([]detective.Input, 0, len(values))
	for index, value := range values {
		inputPath := pointerIndex(pointerChild(path, "inputs"), index)
		if !c.requireObject(value, inputPath) {
			continue
		}
		c.rejectUnknown(value, inputPath, checkInputFields)
		input := detective.Input{
			Name:        c.textValue(value, inputPath, "name", true),
			Type:        investigation.ValueType(c.textValue(value, inputPath, "type", true)),
			Cardinality: c.textValue(value, inputPath, "cardinality", true),
		}
		if required, present := c.optionalBoolField(value, inputPath, "required"); present && required != nil {
			input.Required = *required
		}
		inputs = append(inputs, input)
	}
	return inputs
}

func (c *checker) parseCheckSteps(root *jsonValue, path string) []detective.Step {
	values := c.arrayField(root, path, "steps", true, 0, 0)
	steps := make([]detective.Step, 0, len(values))
	for index, value := range values {
		stepPath := pointerIndex(pointerChild(path, "steps"), index)
		if !c.requireObject(value, stepPath) {
			continue
		}
		c.rejectUnknown(value, stepPath, checkStepFields)
		step := detective.Step{
			ID:   c.textValue(value, stepPath, "id", true),
			Kind: c.textValue(value, stepPath, "kind", true),
		}
		if queryID, _ := c.optionalTextField(value, stepPath, "queryId"); queryID != nil {
			step.QueryID = *queryID
		}
		if checkID, _ := c.optionalTextField(value, stepPath, "checkId"); checkID != nil {
			step.CheckID = *checkID
		}
		if policy, _ := c.optionalTextField(value, stepPath, "onUnavailable"); policy != nil {
			step.OnUnavailable = *policy
		}
		step.Bindings = c.parseCheckBindings(value, stepPath)
		step.Validators = c.parseCheckValidators(value, stepPath)
		steps = append(steps, step)
	}
	return steps
}

func (c *checker) parseCheckBindings(object *jsonValue, path string) []detective.Binding {
	values, present := c.optionalArray(object, path, "bindings")
	if !present {
		return nil
	}
	bindings := make([]detective.Binding, 0, len(values))
	for index, value := range values {
		bindingPath := pointerIndex(pointerChild(path, "bindings"), index)
		if !c.requireObject(value, bindingPath) {
			continue
		}
		c.rejectUnknown(value, bindingPath, checkBindingFields)
		binding := detective.Binding{Input: c.textValue(value, bindingPath, "input", true)}
		if fromInput, _ := c.optionalTextField(value, bindingPath, "fromInput"); fromInput != nil {
			binding.FromInput = *fromInput
		}
		if part, _ := c.optionalTextField(value, bindingPath, "part"); part != nil {
			binding.Part = *part
		}
		if fromColumn, _ := c.optionalTextField(value, bindingPath, "fromColumn"); fromColumn != nil {
			binding.FromColumn = *fromColumn
		}
		bindings = append(bindings, binding)
	}
	return bindings
}

func (c *checker) parseCheckValidators(object *jsonValue, path string) []detective.Validator {
	values, present := c.optionalArray(object, path, "validators")
	if !present {
		return nil
	}
	validators := make([]detective.Validator, 0, len(values))
	for index, value := range values {
		validatorPath := pointerIndex(pointerChild(path, "validators"), index)
		if !c.requireObject(value, validatorPath) {
			continue
		}
		c.rejectUnknown(value, validatorPath, checkValidatorFields)
		validator := detective.Validator{
			ID:   c.textValue(value, validatorPath, "id", true),
			Kind: c.textValue(value, validatorPath, "kind", false),
		}
		for _, field := range []struct {
			name   string
			target *string
		}{
			{"validatorId", &validator.ValidatorID},
			{"scope", &validator.Scope},
			{"severity", &validator.Severity},
			{"field", &validator.Field},
			{"otherField", &validator.OtherField},
			{"checkId", &validator.CheckID},
			{"onUnavailable", &validator.OnUnavailable},
		} {
			if text, _ := c.optionalTextField(value, validatorPath, field.name); text != nil {
				*field.target = *text
			}
		}
		validator.Bindings = c.parseCheckBindings(value, validatorPath)
		if expectedValue, present := value.member("expected"); present {
			expected := c.typedValue(expectedValue, pointerChild(validatorPath, "expected"))
			validator.Expected = &expected
		}
		validators = append(validators, validator)
	}
	return validators
}

func (c *checker) parseWatchRecommendation(data []byte, path string) *TrainingWatchRecommendation {
	root, ok := c.childJSON(data, path)
	if !ok {
		return nil
	}
	c.rejectUnknown(root, path, watchRecommendationFields)
	watch := &TrainingWatchRecommendation{
		Title:              c.textLimited(root, path, "title", MaxTrainingTitleChars, true),
		CheckID:            c.textValue(root, path, "checkId", true),
		CheckVersion:       c.textValue(root, path, "checkVersion", true),
		Trigger:            c.textValue(root, path, "trigger", true),
		InitialObservation: c.textValue(root, path, "initialObservation", true),
		Mode:               c.textValue(root, path, "mode", true),
	}
	if watch.CheckVersion == "" {
		c.report(CodeInvalidValue, pointerChild(path, "checkVersion"), "must be nonempty")
	}
	if !oneOfTraining(watch.Trigger, "becomes_true", "becomes_false", "changes") {
		c.report(CodeInvalidValue, pointerChild(path, "trigger"),
			"trigger must be becomes_true, becomes_false or changes")
	}
	if !oneOfTraining(watch.InitialObservation, "notify", "baseline") {
		c.report(CodeInvalidValue, pointerChild(path, "initialObservation"),
			"initialObservation must be notify or baseline")
	}
	if !oneOfTraining(watch.Mode, "one_shot", "continuous") {
		c.report(CodeInvalidValue, pointerChild(path, "mode"),
			"mode must be one_shot or continuous")
	}
	watch.Inputs = c.trainingWatchInputs(root, path)
	watch.IntervalSeconds, _ = c.positiveIntegerField(root, path, "schedule", "intervalSeconds")
	watch.CooldownSeconds, watch.Recovery = c.trainingNotificationPolicy(root, path)
	return watch
}

// positiveIntegerField reads one required positive integer nested in one
// closed subobject.
func (c *checker) positiveIntegerField(object *jsonValue, path, subobject, name string) (Integer, bool) {
	subPath := pointerChild(path, subobject)
	value, present := object.member(subobject)
	if !present {
		c.report(CodeMissingField, subPath, "required field is missing")
		return "", false
	}
	if !c.requireObject(value, subPath) {
		return "", false
	}
	switch subobject {
	case "schedule":
		c.rejectUnknown(value, subPath, watchScheduleFields)
	}
	integer := c.positiveInteger(value, subPath, name, true)
	return integer, integer != ""
}

func (c *checker) trainingNotificationPolicy(root *jsonValue, path string) (Integer, bool) {
	policyPath := pointerChild(path, "notificationPolicy")
	value, present := root.member("notificationPolicy")
	if !present {
		c.report(CodeMissingField, policyPath, "required field is missing")
		return "", false
	}
	if !c.requireObject(value, policyPath) {
		return "", false
	}
	c.rejectUnknown(value, policyPath, watchNotificationField)
	cooldown, ok := c.nonNegativeInteger(value, policyPath, "cooldownSeconds")
	recovery, recoveryOK := c.requiredBool(value, policyPath, "recovery")
	return cooldown, ok && recoveryOK && recovery
}

func (c *checker) trainingWatchInputs(root *jsonValue, path string) map[string]detective.InputValue {
	inputsPath := pointerChild(path, "inputs")
	value, present := root.member("inputs")
	if !present {
		c.report(CodeMissingField, inputsPath, "required field is missing")
		return nil
	}
	if !c.requireObject(value, inputsPath) {
		return nil
	}
	inputs := make(map[string]detective.InputValue, len(value.object))
	for _, member := range value.object {
		if !detective.ValidParameterName(member.key) {
			c.report(CodeInvalidValue, pointerChild(inputsPath, member.key),
				"input name must be a valid native parameter name")
			continue
		}
		inputs[member.key] = c.trainingInputValue(member.value, pointerChild(inputsPath, member.key))
	}
	return inputs
}

// trainingInputValue parses one closed native InputValue wire value.
func (c *checker) trainingInputValue(value *jsonValue, path string) detective.InputValue {
	if !c.requireObject(value, path) {
		return detective.InputValue{}
	}
	before := len(c.issues)
	typeValue, present := value.member("type")
	if !present {
		c.report(CodeMissingField, pointerChild(path, "type"), "required field is missing")
		return detective.InputValue{}
	}
	if typeValue.kind != jsonString {
		c.report(CodeInvalidType, pointerChild(path, "type"), "must be a string")
		return detective.InputValue{}
	}
	var input detective.InputValue
	switch typeValue.text {
	case "set":
		c.rejectUnknown(value, path, inputValueSetFields)
		set := &apicontract.TypedValueSet{}
		for index, item := range c.arrayField(value, path, "values", true, 0, 0) {
			set.Values = append(set.Values, c.typedValue(item, pointerIndex(pointerChild(path, "values"), index)))
		}
		input = detective.InputValue{Set: set}
	case "range":
		c.rejectUnknown(value, path, inputValueRangeFields)
		r := &detective.ValueRange{Type: "range"}
		r.Start = c.requiredTypedValue(value, path, "start")
		r.End = c.requiredTypedValue(value, path, "end")
		input = detective.InputValue{Range: r}
	default:
		c.rejectUnknown(value, path, typedValueFields)
		scalar := c.typedValue(value, path)
		input = detective.InputValue{Scalar: &scalar}
	}
	if err := input.Validate(); err != nil && len(c.issues) == before {
		c.report(CodeInvalidValue, path, "native InputValue validation refused the value")
	}
	return input
}

func (c *checker) requiredTypedValue(object *jsonValue, path, name string) investigation.TypedValue {
	value, present := object.member(name)
	if !present {
		c.report(CodeMissingField, pointerChild(path, name), "required field is missing")
		return investigation.TypedValue{}
	}
	return c.typedValue(value, pointerChild(path, name))
}

// typedValue parses one closed native TypedValue and applies its own Validate.
func (c *checker) typedValue(value *jsonValue, path string) investigation.TypedValue {
	if !c.requireObject(value, path) {
		return investigation.TypedValue{}
	}
	c.rejectUnknown(value, path, typedValueFields)
	typeValue, present := value.member("type")
	if !present {
		c.report(CodeMissingField, pointerChild(path, "type"), "required field is missing")
		return investigation.TypedValue{}
	}
	if typeValue.kind != jsonString {
		c.report(CodeInvalidType, pointerChild(path, "type"), "must be a string")
		return investigation.TypedValue{}
	}
	valueMember, present := value.member("value")
	if !present {
		c.report(CodeMissingField, pointerChild(path, "value"), "required field is missing")
		return investigation.TypedValue{}
	}
	var typed investigation.TypedValue
	switch investigation.ValueType(typeValue.text) {
	case investigation.ValueTypeString, investigation.ValueTypeInteger,
		investigation.ValueTypeDecimal, investigation.ValueTypeDate, investigation.ValueTypeDatetime:
		if valueMember.kind != jsonString {
			c.report(CodeInvalidType, pointerChild(path, "value"), "must be a JSON string")
			return investigation.TypedValue{}
		}
		typed = investigation.TypedValue{Type: investigation.ValueType(typeValue.text), Str: valueMember.text}
	case investigation.ValueTypeNumber:
		if valueMember.kind != jsonNumber {
			c.report(CodeInvalidType, pointerChild(path, "value"), "must be a JSON number")
			return investigation.TypedValue{}
		}
		parsed, err := strconv.ParseFloat(valueMember.number, 64)
		if err != nil {
			c.report(CodeInvalidValue, pointerChild(path, "value"), "must be a finite number")
			return investigation.TypedValue{}
		}
		typed = investigation.TypedValue{Type: investigation.ValueTypeNumber, Num: parsed}
	case investigation.ValueTypeBoolean:
		if valueMember.kind != jsonBoolean {
			c.report(CodeInvalidType, pointerChild(path, "value"), "must be a JSON boolean")
			return investigation.TypedValue{}
		}
		typed = investigation.TypedValue{Type: investigation.ValueTypeBoolean, Bool: valueMember.bool}
	case investigation.ValueTypeNull:
		if valueMember.kind != jsonNull {
			c.report(CodeInvalidType, pointerChild(path, "value"), "must be JSON null")
			return investigation.TypedValue{}
		}
		typed = investigation.TypedValue{Type: investigation.ValueTypeNull}
	default:
		c.report(CodeInvalidValue, pointerChild(path, "type"), "unknown TypedValue type")
		return investigation.TypedValue{}
	}
	if err := typed.Validate(); err != nil {
		c.report(CodeInvalidValue, pointerChild(path, "value"), "native TypedValue validation refused the value")
		return investigation.TypedValue{}
	}
	return typed
}

// validateChallengeManifestShape applies the package-wide exact-case,
// duplicate, unknown-field, null and JSON-type rules to the whole native
// challenge manifest before challenge.LoadManifest runs. Native validation
// still owns every semantic, range and fixture-pin rule.
func (c *checker) validateChallengeManifestShape(root *jsonValue, path string) {
	c.rejectUnknown(root, path, manifestRootFields)
	c.manifestStringField(root, path, "gradingVersion")
	c.manifestStringField(root, path, "challengeId")
	c.manifestStringField(root, path, "title")
	c.manifestStringField(root, path, "objective")
	c.manifestStringField(root, path, "difficulty")
	c.manifestStringField(root, path, "dialect")
	c.manifestStringField(root, path, "allowedOperation")
	c.manifestStringField(root, path, "timing")
	c.manifestIntegerField(root, path, "schemaVersion")
	c.manifestIntegerField(root, path, "revision")
	c.manifestEngine(root, path)
	c.manifestFixtureSuite(root, path)
	c.manifestVisibleSchema(root, path)
	c.manifestParts(root, path)
	c.manifestHints(root, path)
	c.manifestReferenceSolutions(root, path)
}

func (c *checker) manifestStringField(object *jsonValue, path, name string) {
	value, present := object.member(name)
	if !present {
		return
	}
	if value.kind != jsonString {
		c.report(CodeInvalidType, pointerChild(path, name), "must be a string")
	}
}

func (c *checker) manifestBoolField(object *jsonValue, path, name string) {
	value, present := object.member(name)
	if !present {
		return
	}
	if value.kind != jsonBoolean {
		c.report(CodeInvalidType, pointerChild(path, name), "must be a boolean")
	}
}

func (c *checker) manifestIntegerField(object *jsonValue, path, name string) {
	value, present := object.member(name)
	if !present {
		return
	}
	if value.kind != jsonNumber || !trainingIntegerPattern.MatchString(value.number) {
		c.report(CodeInvalidValue, pointerChild(path, name),
			"must be a canonical JSON integer")
	}
}

func (c *checker) manifestArray(object *jsonValue, path, name string) ([]*jsonValue, bool) {
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

func (c *checker) manifestObjectField(object *jsonValue, path, name string) (*jsonValue, bool) {
	value, present := object.member(name)
	if !present {
		return nil, false
	}
	if value.kind != jsonObject {
		c.report(CodeInvalidType, pointerChild(path, name), "must be a JSON object")
		return nil, false
	}
	return value, true
}

func (c *checker) manifestEngine(root *jsonValue, path string) {
	object, present := c.manifestObjectField(root, path, "engine")
	if !present {
		return
	}
	enginePath := pointerChild(path, "engine")
	c.rejectUnknown(object, enginePath, manifestEngineFields)
	c.manifestStringField(object, enginePath, "package")
	c.manifestStringField(object, enginePath, "version")
	c.manifestStringField(object, enginePath, "sqliteVersion")
}

func (c *checker) manifestFixtureSuite(root *jsonValue, path string) {
	object, present := c.manifestObjectField(root, path, "fixtureSuite")
	if !present {
		return
	}
	suitePath := pointerChild(path, "fixtureSuite")
	c.rejectUnknown(object, suitePath, manifestFixtureSuiteFields)
	c.manifestStringField(object, suitePath, "id")
	fixtures, ok := c.manifestArray(object, suitePath, "fixtures")
	if !ok {
		return
	}
	for index, value := range fixtures {
		fixturePath := pointerIndex(pointerChild(suitePath, "fixtures"), index)
		if !c.requireObject(value, fixturePath) {
			continue
		}
		c.rejectUnknown(value, fixturePath, manifestFixtureFields)
		c.manifestStringField(value, fixturePath, "id")
		c.manifestStringField(value, fixturePath, "label")
		c.manifestStringField(value, fixturePath, "sha256")
		c.manifestIntegerField(value, fixturePath, "bytes")
		c.manifestIntegerField(value, fixturePath, "runs")
	}
}

func (c *checker) manifestVisibleSchema(root *jsonValue, path string) {
	object, present := c.manifestObjectField(root, path, "visibleSchema")
	if !present {
		return
	}
	schemaPath := pointerChild(path, "visibleSchema")
	c.rejectUnknown(object, schemaPath, manifestVisibleSchemaFields)
	tables, ok := c.manifestArray(object, schemaPath, "tables")
	if !ok {
		return
	}
	for tableIndex, table := range tables {
		tablePath := pointerIndex(pointerChild(schemaPath, "tables"), tableIndex)
		if !c.requireObject(table, tablePath) {
			continue
		}
		c.rejectUnknown(table, tablePath, manifestVisibleTableFields)
		c.manifestStringField(table, tablePath, "name")
		columns, ok := c.manifestArray(table, tablePath, "columns")
		if !ok {
			continue
		}
		for columnIndex, column := range columns {
			columnPath := pointerIndex(pointerChild(tablePath, "columns"), columnIndex)
			if !c.requireObject(column, columnPath) {
				continue
			}
			c.rejectUnknown(column, columnPath, manifestVisibleColumnFields)
			c.manifestStringField(column, columnPath, "name")
			c.manifestStringField(column, columnPath, "type")
			c.manifestBoolField(column, columnPath, "primaryKey")
			c.manifestBoolField(column, columnPath, "notNull")
			c.manifestStringField(column, columnPath, "collation")
		}
	}
}

func (c *checker) manifestParts(root *jsonValue, path string) {
	parts, ok := c.manifestArray(root, path, "parts")
	if !ok {
		return
	}
	for index, value := range parts {
		partPath := pointerIndex(pointerChild(path, "parts"), index)
		if !c.requireObject(value, partPath) {
			continue
		}
		c.rejectUnknown(value, partPath, manifestPartFields)
		c.manifestStringField(value, partPath, "id")
		c.manifestStringField(value, partPath, "title")
		c.manifestBoolField(value, partPath, "required")
		c.manifestStringField(value, partPath, "answerKind")
		if sqlPart, present := c.manifestObjectField(value, partPath, "sql"); present {
			c.manifestSQLPart(sqlPart, pointerChild(partPath, "sql"))
		}
		if structured, present := c.manifestObjectField(value, partPath, "structured"); present {
			c.manifestStructuredPart(structured, pointerChild(partPath, "structured"))
		}
		if checkDefinition, present := c.manifestObjectField(value, partPath, "checkDefinition"); present {
			c.manifestCheckDefinition(checkDefinition, pointerChild(partPath, "checkDefinition"))
		}
	}
}

func (c *checker) manifestSQLPart(object *jsonValue, path string) {
	c.rejectUnknown(object, path, manifestSQLPartFields)
	cases, ok := c.manifestArray(object, path, "cases")
	if !ok {
		return
	}
	for index, value := range cases {
		casePath := pointerIndex(pointerChild(path, "cases"), index)
		if !c.requireObject(value, casePath) {
			continue
		}
		c.rejectUnknown(value, casePath, manifestSQLCaseFields)
		c.manifestStringField(value, casePath, "fixture")
		oracle, present := c.manifestObjectField(value, casePath, "oracle")
		if present {
			c.manifestOracle(oracle, pointerChild(casePath, "oracle"))
		}
	}
}

func (c *checker) manifestOracle(object *jsonValue, path string) {
	kindValue, present := object.member("kind")
	if !present {
		c.report(CodeMissingField, pointerChild(path, "kind"), "required field is missing")
		return
	}
	if kindValue.kind != jsonString {
		c.report(CodeInvalidType, pointerChild(path, "kind"), "must be a string")
		return
	}
	switch kindValue.text {
	case challenge.OracleResult:
		c.rejectUnknown(object, path, manifestResultOracleFields)
		c.manifestColumnsRows(object, path)
		c.manifestStringField(object, path, "ordering")
		c.manifestBoolField(object, path, "requireColumnNames")
		c.manifestStringField(object, path, "numericTolerance")
	case challenge.OracleFinalState:
		c.rejectUnknown(object, path, manifestFinalStateFields)
		c.manifestFinalState(object, path)
	default:
		c.report(CodeInvalidValue, pointerChild(path, "kind"),
			"oracle kind must be %q or %q", challenge.OracleResult, challenge.OracleFinalState)
	}
}

func (c *checker) manifestColumnsRows(object *jsonValue, path string) {
	columns, ok := c.manifestArray(object, path, "columns")
	if ok {
		for index, value := range columns {
			columnPath := pointerIndex(pointerChild(path, "columns"), index)
			if !c.requireObject(value, columnPath) {
				continue
			}
			c.rejectUnknown(value, columnPath, manifestResultColumnFields)
			c.manifestStringField(value, columnPath, "name")
			c.manifestStringField(value, columnPath, "type")
		}
	}
	rows, ok := c.manifestArray(object, path, "rows")
	if !ok {
		return
	}
	for rowIndex, row := range rows {
		rowPath := pointerIndex(pointerChild(path, "rows"), rowIndex)
		if row.kind != jsonArray {
			c.report(CodeInvalidType, rowPath, "must be an array")
			continue
		}
		for cellIndex, cell := range row.array {
			c.typedValue(cell, pointerIndex(rowPath, cellIndex))
		}
	}
}

func (c *checker) manifestFinalState(object *jsonValue, path string) {
	tables, ok := c.manifestArray(object, path, "tables")
	if ok {
		for index, value := range tables {
			tablePath := pointerIndex(pointerChild(path, "tables"), index)
			if !c.requireObject(value, tablePath) {
				continue
			}
			c.rejectUnknown(value, tablePath, manifestTableFields)
			c.manifestStringField(value, tablePath, "name")
			c.manifestColumnsRows(value, tablePath)
		}
	}
	invariants, ok := c.manifestArray(object, path, "invariants")
	if !ok {
		return
	}
	for index, value := range invariants {
		invariantPath := pointerIndex(pointerChild(path, "invariants"), index)
		if !c.requireObject(value, invariantPath) {
			continue
		}
		c.rejectUnknown(value, invariantPath, manifestInvariantFields)
		c.manifestStringField(value, invariantPath, "id")
		c.manifestStringField(value, invariantPath, "kind")
		c.manifestStringField(value, invariantPath, "table")
		c.manifestStringField(value, invariantPath, "column")
		c.manifestStringField(value, invariantPath, "description")
		groupBy, ok := c.manifestArray(value, invariantPath, "groupBy")
		if !ok {
			continue
		}
		for groupIndex, group := range groupBy {
			if group.kind != jsonString {
				c.report(CodeInvalidType,
					pointerIndex(pointerChild(invariantPath, "groupBy"), groupIndex),
					"must be a string")
			}
		}
	}
}

func (c *checker) manifestStructuredPart(object *jsonValue, path string) {
	c.rejectUnknown(object, path, manifestStructuredFields)
	fields, ok := c.manifestArray(object, path, "fields")
	if ok {
		for index, value := range fields {
			fieldPath := pointerIndex(pointerChild(path, "fields"), index)
			if !c.requireObject(value, fieldPath) {
				continue
			}
			c.rejectUnknown(value, fieldPath, manifestStructuredFieldFields)
			c.manifestStringField(value, fieldPath, "id")
			c.manifestStringField(value, fieldPath, "label")
			c.manifestStringField(value, fieldPath, "cardinality")
			c.manifestBoolField(value, fieldPath, "required")
			c.manifestIntegerField(value, fieldPath, "minSelections")
			c.manifestIntegerField(value, fieldPath, "maxSelections")
			options, ok := c.manifestArray(value, fieldPath, "options")
			if !ok {
				continue
			}
			for optionIndex, option := range options {
				optionPath := pointerIndex(pointerChild(fieldPath, "options"), optionIndex)
				if !c.requireObject(option, optionPath) {
					continue
				}
				c.rejectUnknown(option, optionPath, manifestStructuredOptionFields)
				c.manifestStringField(option, optionPath, "id")
				c.manifestStringField(option, optionPath, "label")
			}
		}
	}
	expected, present := c.manifestObjectField(object, path, "expected")
	if present {
		c.manifestStructuredAnswer(expected, pointerChild(path, "expected"))
	}
}

func (c *checker) manifestStructuredAnswer(object *jsonValue, path string) {
	c.rejectUnknown(object, path, manifestStructuredAnswerFields)
	selections, present := object.member("selections")
	if !present {
		return
	}
	if selections.kind != jsonObject {
		c.report(CodeInvalidType, pointerChild(path, "selections"), "must be a JSON object")
		return
	}
	for _, member := range selections.object {
		selectionPath := pointerChild(pointerChild(path, "selections"), member.key)
		if member.value.kind != jsonArray {
			c.report(CodeInvalidType, selectionPath, "must be an array of strings")
			continue
		}
		for index, item := range member.value.array {
			if item.kind != jsonString {
				c.report(CodeInvalidType, pointerIndex(selectionPath, index), "must be a string")
			}
		}
	}
}

func (c *checker) manifestCheckDefinition(object *jsonValue, path string) {
	c.rejectUnknown(object, path, manifestCheckDefinitionFields)
	queryIDs, ok := c.manifestArray(object, path, "queryIds")
	if ok {
		for index, queryID := range queryIDs {
			if queryID.kind != jsonString {
				c.report(CodeInvalidType, pointerIndex(pointerChild(path, "queryIds"), index), "must be a string")
			}
		}
	}
	if expectation, present := c.manifestObjectField(object, path, "expectation"); present {
		c.manifestValidator(expectation, pointerChild(path, "expectation"))
	}
	cases, ok := c.manifestArray(object, path, "cases")
	if !ok {
		return
	}
	for index, value := range cases {
		casePath := pointerIndex(pointerChild(path, "cases"), index)
		if !c.requireObject(value, casePath) {
			continue
		}
		c.rejectUnknown(value, casePath, manifestCheckCaseFields)
		c.manifestStringField(value, casePath, "fixture")
		c.manifestStringField(value, casePath, "expectedOutcome")
	}
}

func (c *checker) manifestHints(root *jsonValue, path string) {
	hints, ok := c.manifestArray(root, path, "hints")
	if !ok {
		return
	}
	for index, value := range hints {
		hintPath := pointerIndex(pointerChild(path, "hints"), index)
		if !c.requireObject(value, hintPath) {
			continue
		}
		c.rejectUnknown(value, hintPath, manifestHintFields)
		c.manifestStringField(value, hintPath, "id")
		c.manifestStringField(value, hintPath, "text")
	}
}

func (c *checker) manifestReferenceSolutions(root *jsonValue, path string) {
	solutions, ok := c.manifestArray(root, path, "referenceSolutions")
	if !ok {
		return
	}
	for index, value := range solutions {
		solutionPath := pointerIndex(pointerChild(path, "referenceSolutions"), index)
		if !c.requireObject(value, solutionPath) {
			continue
		}
		c.rejectUnknown(value, solutionPath, manifestReferenceSolutionFields)
		c.manifestStringField(value, solutionPath, "partId")
		answer, present := c.manifestObjectField(value, solutionPath, "answer")
		if present {
			c.manifestReferenceAnswer(answer, pointerChild(solutionPath, "answer"))
		}
	}
}

func (c *checker) manifestReferenceAnswer(object *jsonValue, path string) {
	c.rejectUnknown(object, path, manifestReferenceAnswerFields)
	c.manifestStringField(object, path, "sql")
	if structured, present := c.manifestObjectField(object, path, "structured"); present {
		c.manifestStructuredAnswer(structured, pointerChild(path, "structured"))
	}
	if checkDefinition, present := c.manifestObjectField(object, path, "checkDefinition"); present {
		checkPath := pointerChild(path, "checkDefinition")
		c.rejectUnknown(checkDefinition, checkPath, manifestCheckDefinitionAnswerFields)
		c.manifestStringField(checkDefinition, checkPath, "queryId")
		if expectation, present := c.manifestObjectField(checkDefinition, checkPath, "expectation"); present {
			c.manifestValidator(expectation, pointerChild(checkPath, "expectation"))
		}
	}
}

// manifestValidator shape-checks one native detective.Validator wire object.
// Callers pass an already type-checked object.
func (c *checker) manifestValidator(value *jsonValue, path string) {
	c.rejectUnknown(value, path, checkValidatorFields)
	for _, name := range []string{
		"id", "validatorId", "scope", "kind", "severity", "field", "otherField",
		"checkId", "onUnavailable",
	} {
		c.manifestStringField(value, path, name)
	}
	if expected, present := value.member("expected"); present {
		c.typedValue(expected, pointerChild(path, "expected"))
	}
	c.manifestBindings(value, path)
}

func (c *checker) manifestBindings(value *jsonValue, path string) {
	bindings, ok := c.manifestArray(value, path, "bindings")
	if !ok {
		return
	}
	for index, binding := range bindings {
		bindingPath := pointerIndex(pointerChild(path, "bindings"), index)
		if !c.requireObject(binding, bindingPath) {
			continue
		}
		c.rejectUnknown(binding, bindingPath, checkBindingFields)
		for _, name := range []string{"input", "fromInput", "part", "fromColumn"} {
			c.manifestStringField(binding, bindingPath, name)
		}
	}
}

// reportChallengeError maps a native challenge refusal onto this package's
// stable issue codes without reimplementing any native rule.
func (c *checker) reportChallengeError(path string, err error) {
	code := CodeInvalidValue
	switch challenge.ErrorCode(err) {
	case challenge.CodeFixtureMissing, challenge.CodeFixtureUnknown:
		code = CodeInvalidReference
	case challenge.CodeFixtureBytes, challenge.CodeFixtureHash:
		code = CodeMismatch
	}
	c.report(code, path, "native challenge validation refused the manifest")
}

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
	if !knownTargetKinds[kind] {
		return Target{Kind: kind, Supported: false}
	}
	target := Target{Kind: kind, Supported: true}
	c.rejectUnknown(value, path, targetFields[kind])
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
		tutorial, _ := c.boolField(value, path, "tutorial", false)
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
	if !knownConditionKinds[kind] {
		return Condition{Kind: kind, Supported: false}
	}
	condition := Condition{Kind: kind, Supported: true}
	c.rejectUnknown(value, path, conditionFields[kind])
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
	}
	return condition
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
		index, valid := c.boolField(value, path, "index", true)
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

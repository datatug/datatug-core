package detectiveai

import "encoding/json"

// The provider sees the same discriminated, nested shape that Core decodes.
// These closed schemas guide generation; Core still validates IDs, graph
// references, typed values, provenance, authority and byte limits itself.
var proposalToolSchema = buildProposalToolSchema()

type toolSchema map[string]any

func objectSchema(properties toolSchema, required ...string) toolSchema {
	return toolSchema{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

func enumSchema(values ...string) toolSchema {
	return toolSchema{"type": "string", "enum": values}
}

func textSchema(max int) toolSchema {
	return toolSchema{"type": "string", "minLength": 1, "maxLength": max}
}

func arraySchema(item any, min, max int) toolSchema {
	return toolSchema{"type": "array", "items": item, "minItems": min, "maxItems": max}
}

func refSchema(name string) toolSchema {
	return toolSchema{"$ref": "#/$defs/" + name}
}

func typedScalarSchema(kinds []string, valueType string) toolSchema {
	return objectSchema(toolSchema{"type": enumSchema(kinds...), "value": toolSchema{"type": valueType}}, "type", "value")
}

func validatorSchema(kind, scope string, extraRequired ...string) toolSchema {
	properties := toolSchema{
		"id": textSchema(128), "kind": enumSchema(kind), "scope": enumSchema(scope),
		"severity": enumSchema("critical", "error", "warning", "info"),
		"field":    textSchema(256), "otherField": textSchema(256),
		"expected": refSchema("scalar"), "checkId": textSchema(128),
		"bindings":      arraySchema(refSchema("binding"), 0, 64),
		"onUnavailable": enumSchema("block", "fail", "warn", "skip"),
	}
	required := append([]string{"id", "kind", "scope", "severity"}, extraRequired...)
	return objectSchema(properties, required...)
}

func stepSchema(kind string) toolSchema {
	properties := toolSchema{
		"id": textSchema(128), "kind": enumSchema(kind),
		"onUnavailable": enumSchema("block", "fail", "warn", "skip"),
	}
	if kind == "query" {
		properties["queryId"] = textSchema(512)
		properties["validators"] = arraySchema(refSchema("validator"), 0, 32)
		return objectSchema(properties, "id", "kind", "queryId")
	}
	properties["checkId"] = textSchema(128)
	properties["bindings"] = arraySchema(refSchema("binding"), 0, 64)
	return objectSchema(properties, "id", "kind", "checkId")
}

func proposalBranch(kind, payload, definition string) toolSchema {
	properties := toolSchema{
		"kind": enumSchema(kind), "title": textSchema(256),
		"sources":     arraySchema(refSchema("provenance"), 1, 32),
		"ambiguities": arraySchema(textSchema(256), 0, 16),
		payload:       refSchema(definition),
	}
	return objectSchema(properties, "kind", "title", "sources", payload)
}

func buildProposalToolSchema() json.RawMessage {
	scalar := toolSchema{"oneOf": []any{
		typedScalarSchema([]string{"string", "integer", "decimal", "date", "datetime"}, "string"),
		typedScalarSchema([]string{"number"}, "number"),
		typedScalarSchema([]string{"boolean"}, "boolean"),
		typedScalarSchema([]string{"null"}, "null"),
	}}
	ordered := toolSchema{"oneOf": []any{
		typedScalarSchema([]string{"integer", "decimal", "date", "datetime"}, "string"),
		typedScalarSchema([]string{"number"}, "number"),
	}}
	inputValue := toolSchema{"oneOf": []any{
		refSchema("scalar"),
		objectSchema(toolSchema{"type": enumSchema("set"), "values": arraySchema(refSchema("nonNullScalar"), 1, 500)}, "type", "values"),
		objectSchema(toolSchema{"type": enumSchema("range"), "start": refSchema("orderedScalar"), "end": refSchema("orderedScalar")}, "type", "start", "end"),
	}}
	provenance := objectSchema(toolSchema{
		"sourceId": textSchema(128), "start": toolSchema{"type": "integer", "minimum": 0},
		"end":        toolSchema{"type": "integer", "minimum": 1},
		"quote":      toolSchema{"type": "string", "maxLength": 512},
		"confidence": toolSchema{"type": "number", "minimum": 0, "maximum": 1},
		"ambiguous":  toolSchema{"type": "boolean"},
	}, "sourceId", "start", "end", "confidence")
	context := toolSchema{"type": "object", "additionalProperties": refSchema("extractedValue"), "minProperties": 1, "maxProperties": 64}
	input := objectSchema(toolSchema{
		"name": textSchema(128), "type": enumSchema("string", "number", "integer", "decimal", "boolean", "date", "datetime"),
		"cardinality": enumSchema("scalar", "set", "range"), "required": toolSchema{"type": "boolean"},
	}, "name", "type", "cardinality")
	binding := objectSchema(toolSchema{
		"input": textSchema(128), "fromInput": textSchema(128),
		"fromColumn": textSchema(256), "part": enumSchema("start", "end"),
	}, "input")
	validator := toolSchema{"oneOf": []any{
		objectSchema(toolSchema{"id": textSchema(128), "validatorId": textSchema(128)}, "id", "validatorId"),
		validatorSchema("non-null", "row", "field"),
		validatorSchema("equals-fields", "row", "field", "otherField"),
		validatorSchema("row-count", "set", "expected"),
		validatorSchema("unique", "set", "field"),
		validatorSchema("sum-equals", "set", "field", "otherField"),
		validatorSchema("check", "row", "checkId"),
	}}
	definitions := toolSchema{
		"scalar": scalar,
		"nonNullScalar": toolSchema{"oneOf": []any{
			typedScalarSchema([]string{"string", "integer", "decimal", "date", "datetime"}, "string"),
			typedScalarSchema([]string{"number"}, "number"),
			typedScalarSchema([]string{"boolean"}, "boolean"),
		}},
		"orderedScalar": ordered, "inputValue": inputValue,
		"provenance":     provenance,
		"extractedValue": objectSchema(toolSchema{"value": refSchema("inputValue"), "provenance": arraySchema(refSchema("provenance"), 1, 32)}, "value", "provenance"),
		"context":        context, "input": input, "binding": binding, "validator": validator,
		"query": objectSchema(toolSchema{
			"id": textSchema(512), "title": textSchema(256), "type": enumSchema("SQL", "TugQL", "DTQL", "StructuredSQL", "HTTP"),
			"text": textSchema(maxText), "connectionId": textSchema(128),
			"inputs": arraySchema(refSchema("input"), 0, 64), "expectedRevision": toolSchema{"type": "string", "maxLength": 256},
		}, "id", "title", "type", "text"),
		"publishedValidator": objectSchema(toolSchema{
			"id": textSchema(128), "title": textSchema(256), "version": toolSchema{"type": "integer"}, "validator": refSchema("validator"),
		}, "id", "title", "validator"),
		"step": toolSchema{"oneOf": []any{stepSchema("query"), stepSchema("check")}},
		"check": objectSchema(toolSchema{
			"id": textSchema(128), "title": textSchema(256), "description": toolSchema{"type": "string", "maxLength": 2048},
			"version": toolSchema{"type": "integer"}, "inputs": arraySchema(refSchema("input"), 0, 64),
			"steps": arraySchema(refSchema("step"), 1, 32), "updatedAt": toolSchema{"type": "string"}, "updatedBy": toolSchema{"type": "string"},
		}, "id", "title", "steps"),
		"scenarioStep": objectSchema(toolSchema{
			"id": textSchema(128), "kind": enumSchema("query", "check"), "ref": textSchema(512),
		}, "id", "kind", "ref"),
		"scenario": objectSchema(toolSchema{
			"id": textSchema(128), "title": textSchema(256), "description": toolSchema{"type": "string", "maxLength": 2048},
			"version": toolSchema{"type": "integer"}, "inputs": arraySchema(refSchema("input"), 0, 64),
			"steps": arraySchema(refSchema("scenarioStep"), 1, 32),
		}, "id", "title", "steps"),
		"finding": objectSchema(toolSchema{
			"id": textSchema(128), "text": textSchema(2048), "status": enumSchema("hypothesis", "confirmed"),
			"evidenceRefs": arraySchema(textSchema(128), 0, 20),
		}, "id", "text", "status"),
		"case": objectSchema(toolSchema{
			"caseId": textSchema(128), "expectedVersion": toolSchema{"type": "integer", "minimum": 1},
			"title":    toolSchema{"type": "string", "maxLength": 256},
			"context":  toolSchema{"type": "object", "additionalProperties": refSchema("inputValue"), "maxProperties": 64},
			"findings": arraySchema(refSchema("finding"), 0, 100),
			"status":   enumSchema("active", "closed"), "closureReason": toolSchema{"type": "string", "maxLength": 2048},
		}, "caseId", "expectedVersion"),
		"run": objectSchema(toolSchema{
			"checkId": textSchema(128), "caseId": textSchema(128), "environment": textSchema(128),
			"inputs": toolSchema{"type": "object", "additionalProperties": refSchema("inputValue"), "maxProperties": 64},
		}, "checkId", "environment", "inputs"),
		"explanation": objectSchema(toolSchema{
			"runId": textSchema(128), "summary": textSchema(2048), "basis": textSchema(2048),
		}, "runId", "summary", "basis"),
	}
	branches := []any{
		proposalBranch(KindContext, "context", "context"), proposalBranch(KindQuery, "query", "query"),
		proposalBranch(KindCheck, "check", "check"), proposalBranch(KindValidator, "validator", "publishedValidator"),
		proposalBranch(KindScenario, "scenario", "scenario"), proposalBranch(KindCase, "case", "case"),
		proposalBranch(KindRun, "run", "run"), proposalBranch(KindExplain, "explanation", "explanation"),
	}
	// The tree contains only static JSON-safe primitives, maps and slices.
	encoded, _ := json.Marshal(toolSchema{"oneOf": branches, "$defs": definitions})
	return encoded
}

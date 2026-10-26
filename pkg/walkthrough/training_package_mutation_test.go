package walkthrough

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/detective"
	"github.com/datatug/datatug-core/pkg/investigation"
	"github.com/stretchr/testify/require"
)

// These tests drive the branches an independently authored valid corpus cannot
// reach without becoming unreadable: every closed-object type error, every
// optional-field absence, and the defensive projection of a hand-built
// snapshot. They do not replace the corpus; they complement it.

func corpusVectorByID(t *testing.T, id string) trainingPackageVector {
	t.Helper()
	corpus := loadTrainingPackageCorpus(t)
	for _, vector := range corpus.Vectors {
		if vector.ID == id {
			return vector
		}
	}
	require.FailNow(t, "corpus vector not found", id)
	return trainingPackageVector{}
}

func corpusInput(t *testing.T, id string) TrainingPackageInput {
	t.Helper()
	return trainingPackageInputFor(t, corpusVectorByID(t, id))
}

func repin(input *TrainingPackageInput, indexBytes []byte) {
	input.IndexBytes = indexBytes
	input.Reference.SHA256 = sha256Hex(indexBytes)
	input.Reference.Bytes = Integer(itoa(int64(len(indexBytes))))
	input.Binding.PackageSHA256 = input.Reference.SHA256
}

func indexObject(t *testing.T, input *TrainingPackageInput) map[string]any {
	t.Helper()
	var index map[string]any
	require.NoError(t, json.Unmarshal(input.IndexBytes, &index))
	return index
}

func writeIndex(t *testing.T, input *TrainingPackageInput, index map[string]any) {
	t.Helper()
	raw, err := json.Marshal(index)
	require.NoError(t, err)
	repin(input, raw)
}

func mutateIndex(t *testing.T, input *TrainingPackageInput, mutate func(map[string]any)) {
	t.Helper()
	index := indexObject(t, input)
	mutate(index)
	writeIndex(t, input, index)
}

func assetBody(input *TrainingPackageInput, path string) []byte {
	for _, asset := range input.Assets {
		if asset.Path == path {
			return asset.Bytes
		}
	}
	return nil
}

func assetPathByID(t *testing.T, index map[string]any, assetID string) string {
	t.Helper()
	for _, raw := range index["assets"].([]any) {
		entry := raw.(map[string]any)
		if entry["id"] == assetID {
			return entry["path"].(string)
		}
	}
	require.FailNow(t, "asset not found", assetID)
	return ""
}

// mutateAsset rewrites one declared child file and its index pin.
func mutateAsset(t *testing.T, input *TrainingPackageInput, assetID string, mutate func([]byte) []byte) {
	t.Helper()
	index := indexObject(t, input)
	path := assetPathByID(t, index, assetID)
	body := mutate(append([]byte{}, assetBody(input, path)...))
	for index := range input.Assets {
		if input.Assets[index].Path == path {
			input.Assets[index].Bytes = body
		}
	}
	for _, raw := range index["assets"].([]any) {
		entry := raw.(map[string]any)
		if entry["id"] == assetID {
			entry["sha256"] = sha256Hex(body)
			entry["bytes"] = float64(len(body))
		}
	}
	writeIndex(t, input, index)
}

func mutateAssetJSON(t *testing.T, input *TrainingPackageInput, assetID string, mutate func(map[string]any)) {
	t.Helper()
	mutateAsset(t, input, assetID, func(body []byte) []byte {
		var object map[string]any
		require.NoError(t, json.Unmarshal(body, &object))
		mutate(object)
		raw, err := json.Marshal(object)
		require.NoError(t, err)
		return raw
	})
}

func TestTrainingPackageIndexShapeMutations(t *testing.T) {
	cases := []struct {
		name   string
		base   string
		mutate func(t *testing.T, input *TrainingPackageInput)
	}{
		{"missing-schema", "minimal", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) { delete(index, "schema") })
		}},
		{"schema-not-string", "minimal", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) { index["schema"] = 1 })
		}},
		{"index-id-mismatch", "minimal", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) { index["id"] = "other-case" })
		}},
		{"index-revision-mismatch", "minimal", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) { index["revision"] = 2 })
		}},
		{"asset-not-object", "minimal", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) { index["assets"] = []any{"x"} })
		}},
		{"actor-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) { index["fictionalActors"] = []any{"x"} })
		}},
		{"source-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) { index["sources"] = []any{"x"} })
		}},
		{"case-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) { index["cases"] = []any{"x"} })
		}},
		{"finding-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				cases := index["cases"].([]any)
				cases[0].(map[string]any)["findings"] = []any{"x"}
			})
		}},
		{"timeline-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				cases := index["cases"].([]any)
				cases[0].(map[string]any)["timeline"] = []any{"x"}
			})
		}},
		{"timeline-ref-missing", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				cases := index["cases"].([]any)
				timeline := cases[0].(map[string]any)["timeline"].([]any)
				delete(timeline[0].(map[string]any), "ref")
			})
		}},
		{"timeline-ref-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				cases := index["cases"].([]any)
				timeline := cases[0].(map[string]any)["timeline"].([]any)
				timeline[0].(map[string]any)["ref"] = 1
			})
		}},
		{"timeline-ref-kind", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				cases := index["cases"].([]any)
				timeline := cases[0].(map[string]any)["timeline"].([]any)
				timeline[0].(map[string]any)["ref"].(map[string]any)["kind"] = "run"
			})
		}},
		{"incident-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) { index["incidents"] = []any{"x"} })
		}},
		{"check-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) { index["checks"] = []any{"x"} })
		}},
		{"query-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) { index["queries"] = []any{"x"} })
		}},
		{"query-id-invalid", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				index["queries"].([]any)[0].(map[string]any)["id"] = "/bad"
			})
		}},
		{"challenge-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) { index["challenges"] = []any{"x"} })
		}},
		{"challenge-id-invalid", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				index["challenges"].([]any)[0].(map[string]any)["id"] = "Bad_ID"
			})
		}},
		{"challenge-revision-zero", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				index["challenges"].([]any)[0].(map[string]any)["revision"] = 0
			})
		}},
		{"challenge-fixture-duplicate", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				challenge := index["challenges"].([]any)[0].(map[string]any)
				fixtures := challenge["fixtures"].([]any)
				challenge["fixtures"] = append(fixtures, fixtures[0])
			})
		}},
		{"challenge-fixture-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				challenge := index["challenges"].([]any)[0].(map[string]any)
				challenge["fixtures"] = []any{"x"}
			})
		}},
		{"proposal-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) { index["proposals"] = []any{"x"} })
		}},
		{"proposal-kind", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				index["proposals"].([]any)[0].(map[string]any)["kind"] = "report"
			})
		}},
		{"target-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) { index["presentationTargets"] = []any{"x"} })
		}},
		{"target-kind", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				index["presentationTargets"].([]any)[0].(map[string]any)["kind"] = "board"
			})
		}},
		{"context-missing", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				delete(index["cases"].([]any)[0].(map[string]any), "context")
			})
		}},
		{"context-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				index["cases"].([]any)[0].(map[string]any)["context"] = []any{}
			})
		}},
		{"check-description-not-string", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-check", func(check map[string]any) {
				check["description"] = 3
			})
		}},
		{"check-version-missing", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-check", func(check map[string]any) {
				delete(check, "version")
			})
		}},
		{"check-version-noncanonical", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-check", func(check map[string]any) {
				check["version"] = 1.5
			})
		}},
		{"check-version-overflow", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAsset(t, input, "retry-check", func(body []byte) []byte {
				return []byte(replaceOnce(string(body), `"version": 1`, `"version": 9223372036854775808`))
			})
		}},
		{"check-updated-at-missing", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-check", func(check map[string]any) {
				delete(check, "updatedAt")
			})
		}},
		{"check-updated-at-invalid", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-check", func(check map[string]any) {
				check["updatedAt"] = "yesterday"
			})
		}},
		{"check-input-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-check", func(check map[string]any) {
				check["inputs"] = []any{"x"}
			})
		}},
		{"check-step-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-check", func(check map[string]any) {
				check["steps"] = []any{"x"}
			})
		}},
		{"check-bindings", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-check", func(check map[string]any) {
				step := check["steps"].([]any)[0].(map[string]any)
				step["bindings"] = []any{
					map[string]any{"input": "status", "fromInput": "status", "part": "start", "fromColumn": "column"},
					"x",
				}
			})
		}},
		{"check-validator-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-check", func(check map[string]any) {
				step := check["steps"].([]any)[0].(map[string]any)
				step["validators"] = []any{"x"}
			})
		}},
		{"check-validator-expected", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-check", func(check map[string]any) {
				step := check["steps"].([]any)[0].(map[string]any)
				step["validators"] = []any{map[string]any{
					"id": "status-count", "scope": "set", "kind": "row-count",
					"severity": "error", "expected": map[string]any{"type": "uuid", "value": "x"},
				}}
			})
		}},
		{"check-native-invalid", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-check", func(check map[string]any) {
				check["steps"] = []any{}
			})
		}},
		{"parameter-is-required-not-bool", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-metadata", func(metadata map[string]any) {
				parameters := metadata["parameters"].([]any)
				parameters[0].(map[string]any)["isRequired"] = "yes"
			})
		}},
		{"parameter-string-default-type", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-metadata", func(metadata map[string]any) {
				parameters := metadata["parameters"].([]any)
				parameters[0].(map[string]any)["defaultValue"] = 3
			})
		}},
		{"parameter-boolean-default-type", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-metadata", func(metadata map[string]any) {
				parameters := metadata["parameters"].([]any)
				parameters[3].(map[string]any)["defaultValue"] = "no"
			})
		}},
		{"parameter-number-default-type", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-metadata", func(metadata map[string]any) {
				parameters := metadata["parameters"].([]any)
				parameters[2].(map[string]any)["defaultValue"] = "2.5"
			})
		}},
		{"parameter-unknown-type-with-default", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-metadata", func(metadata map[string]any) {
				parameters := metadata["parameters"].([]any)
				parameters[1].(map[string]any)["type"] = "array"
			})
		}},
		{"parameter-duplicate-id", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-metadata", func(metadata map[string]any) {
				parameters := metadata["parameters"].([]any)
				parameters[1].(map[string]any)["id"] = "status"
			})
		}},
		{"parameter-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-metadata", func(metadata map[string]any) {
				metadata["parameters"] = []any{"x"}
			})
		}},
		{"metadata-id-empty", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-metadata", func(metadata map[string]any) {
				metadata["id"] = ""
			})
		}},
		{"metadata-connection-empty", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-metadata", func(metadata map[string]any) {
				metadata["connectionId"] = ""
			})
		}},
		{"records-child-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAsset(t, input, "orders-records", func([]byte) []byte { return []byte("[]") })
		}},
		{"records-child-invalid-json", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAsset(t, input, "orders-records", func([]byte) []byte { return []byte("{") })
		}},
		{"records-child-invalid-utf8", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAsset(t, input, "orders-records", func([]byte) []byte { return []byte{0xff} })
		}},
		{"records-child-nested-json-error", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "orders-records", func(records map[string]any) {
				records["columns"] = 1
			})
		}},
		{"records-child-syntax-error", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAsset(t, input, "orders-records", func([]byte) []byte { return []byte(`{"columns":}`) })
		}},
		{"evidence-text-invalid-utf8", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAsset(t, input, "narrative-text", func([]byte) []byte { return []byte{0xff} })
		}},
		{"metadata-child-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAsset(t, input, "retry-metadata", func([]byte) []byte { return []byte("[]") })
		}},
		{"check-child-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAsset(t, input, "retry-check", func([]byte) []byte { return []byte("[]") })
		}},
		{"records-column-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "orders-records", func(records map[string]any) {
				records["columns"] = []any{"x"}
			})
		}},
		{"records-column-name-empty", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "orders-records", func(records map[string]any) {
				columns := records["columns"].([]any)
				columns[0].(map[string]any)["name"] = ""
			})
		}},
		{"records-row-not-array", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "orders-records", func(records map[string]any) {
				records["rows"] = []any{"x"}
			})
		}},
		{"records-typed-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "orders-records", func(records map[string]any) {
				rows := records["rows"].([]any)
				rows[0].([]any)[0] = 1
			})
		}},
		{"records-typed-type-missing", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "orders-records", func(records map[string]any) {
				rows := records["rows"].([]any)
				delete(rows[0].([]any)[0].(map[string]any), "type")
			})
		}},
		{"records-typed-type-nonstring", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "orders-records", func(records map[string]any) {
				rows := records["rows"].([]any)
				rows[0].([]any)[0].(map[string]any)["type"] = 1
			})
		}},
		{"records-typed-value-missing", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "orders-records", func(records map[string]any) {
				rows := records["rows"].([]any)
				delete(rows[0].([]any)[0].(map[string]any), "value")
			})
		}},
		{"records-typed-number-not-number", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "orders-records", func(records map[string]any) {
				rows := records["rows"].([]any)
				cell := rows[0].([]any)[0].(map[string]any)
				cell["type"] = "number"
			})
		}},
		{"records-typed-number-infinite", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "orders-records", func(records map[string]any) {
				rows := records["rows"].([]any)
				cell := rows[0].([]any)[0].(map[string]any)
				cell["type"] = "number"
				cell["value"] = "INFINITE"
			})
			mutateAsset(t, input, "orders-records", func(body []byte) []byte {
				return []byte(replaceOnce(string(body), `"INFINITE"`, `1e999`))
			})
		}},
		{"records-typed-boolean-not-bool", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "orders-records", func(records map[string]any) {
				rows := records["rows"].([]any)
				cell := rows[0].([]any)[0].(map[string]any)
				cell["type"] = "boolean"
				cell["value"] = "true"
			})
		}},
		{"records-typed-null-not-null", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "orders-records", func(records map[string]any) {
				rows := records["rows"].([]any)
				cell := rows[0].([]any)[0].(map[string]any)
				cell["type"] = "null"
			})
		}},
		{"records-typed-unknown-type", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "orders-records", func(records map[string]any) {
				rows := records["rows"].([]any)
				rows[0].([]any)[0].(map[string]any)["type"] = "uuid"
			})
		}},
		{"log-invalid-utf8", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAsset(t, input, "ops-log", func([]byte) []byte { return []byte{0xff} })
		}},
		{"log-id-empty", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSONFile(t, input, "ops-log", func(lines []map[string]any) {
				lines[0]["id"] = ""
			})
		}},
		{"watch-child-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAsset(t, input, "retry-watch", func([]byte) []byte { return []byte("[]") })
		}},
		{"watch-check-version-empty", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) { watch["checkVersion"] = "" })
		}},
		{"watch-initial-observation", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) { watch["initialObservation"] = "later" })
		}},
		{"watch-mode", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) { watch["mode"] = "loop" })
		}},
		{"watch-schedule-missing", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) { delete(watch, "schedule") })
		}},
		{"watch-schedule-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) { watch["schedule"] = 1 })
		}},
		{"watch-policy-missing", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) { delete(watch, "notificationPolicy") })
		}},
		{"watch-policy-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) { watch["notificationPolicy"] = 1 })
		}},
		{"watch-policy-cooldown-missing", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) {
				delete(watch["notificationPolicy"].(map[string]any), "cooldownSeconds")
			})
		}},
		{"watch-inputs-missing", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) { delete(watch, "inputs") })
		}},
		{"watch-inputs-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) { watch["inputs"] = 1 })
		}},
		{"watch-input-name-invalid", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) {
				watch["inputs"] = map[string]any{"bad name": map[string]any{"type": "string", "value": "x"}}
			})
		}},
		{"watch-input-not-object", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) {
				watch["inputs"] = map[string]any{"status": 1}
			})
		}},
		{"watch-input-type-missing", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) {
				watch["inputs"] = map[string]any{"status": map[string]any{"value": "x"}}
			})
		}},
		{"watch-input-type-nonstring", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) {
				watch["inputs"] = map[string]any{"status": map[string]any{"type": 1}}
			})
		}},
		{"watch-input-range-start-missing", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) {
				watch["inputs"] = map[string]any{"status": map[string]any{
					"type": "range", "end": map[string]any{"type": "string", "value": "z"},
				}}
			})
		}},
		{"watch-input-set", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) {
				watch["inputs"] = map[string]any{"status": map[string]any{
					"type":   "set",
					"values": []any{map[string]any{"type": "string", "value": "pending"}},
				}}
			})
		}},
		{"watch-input-range", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateAssetJSON(t, input, "retry-watch", func(watch map[string]any) {
				watch["inputs"] = map[string]any{"status": map[string]any{
					"type":  "range",
					"start": map[string]any{"type": "string", "value": "a"},
					"end":   map[string]any{"type": "string", "value": "z"},
				}}
			})
		}},
		{"context-typed-values", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				cases := index["cases"].([]any)
				cases[0].(map[string]any)["context"] = map[string]any{
					"number":  map[string]any{"type": "number", "value": 2.5},
					"boolean": map[string]any{"type": "boolean", "value": true},
					"set": map[string]any{"type": "set", "values": []any{
						map[string]any{"type": "string", "value": "pending"},
					}},
					"range": map[string]any{"type": "range",
						"start": map[string]any{"type": "integer", "value": "1"},
						"end":   map[string]any{"type": "integer", "value": "5"}},
				}
			})
		}},
		{"context-range-inverted", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				cases := index["cases"].([]any)
				cases[0].(map[string]any)["context"] = map[string]any{
					"status": map[string]any{"type": "range",
						"start": map[string]any{"type": "integer", "value": "5"},
						"end":   map[string]any{"type": "integer", "value": "1"}},
				}
			})
		}},
		{"query-metadata-kind-wrong", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				index["queries"].([]any)[0].(map[string]any)["metadataAssetId"] = "orders-records"
			})
		}},
		{"challenge-manifest-kind-wrong", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				index["challenges"].([]any)[0].(map[string]any)["manifestAssetId"] = "orders-records"
			})
		}},
		{"challenge-fixture-kind-wrong", "full", func(t *testing.T, input *TrainingPackageInput) {
			mutateIndex(t, input, func(index map[string]any) {
				fixtures := index["challenges"].([]any)[0].(map[string]any)["fixtures"].([]any)
				fixtures[0].(map[string]any)["assetId"] = "retry-check"
			})
		}},
	}
	for _, testCase := range cases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			input := corpusInput(t, testCase.base)
			testCase.mutate(t, &input)
			result := AdmitTrainingPackage(input)
			// The positive typed-context case remains admitted; every other
			// mutation must refuse without exposing a partial snapshot.
			if testCase.name == "context-typed-values" {
				require.True(t, result.Admitted, "issues: %v", result.Issues)
				return
			}
			require.False(t, result.Admitted, "unexpected admission")
			require.Nil(t, result.Package)
			require.NotEmpty(t, result.Issues)
		})
	}
}

// mutateAssetJSONFile rewrites a JSONL asset one object per line.
func mutateAssetJSONFile(t *testing.T, input *TrainingPackageInput, assetID string, mutate func([]map[string]any)) {
	t.Helper()
	mutateAsset(t, input, assetID, func(body []byte) []byte {
		lines := make([]map[string]any, 0)
		for _, line := range splitLines(body) {
			var object map[string]any
			require.NoError(t, json.Unmarshal(line, &object))
			lines = append(lines, object)
		}
		mutate(lines)
		raw := make([]byte, 0)
		for _, object := range lines {
			encoded, err := json.Marshal(object)
			require.NoError(t, err)
			raw = append(raw, encoded...)
			raw = append(raw, '\n')
		}
		return raw
	})
}

func splitLines(body []byte) [][]byte {
	lines := make([][]byte, 0)
	start := 0
	for index, char := range body {
		if char == '\n' {
			lines = append(lines, body[start:index])
			start = index + 1
		}
	}
	if start < len(body) {
		lines = append(lines, body[start:])
	}
	filtered := lines[:0]
	for _, line := range lines {
		if len(line) > 0 {
			filtered = append(filtered, line)
		}
	}
	return filtered
}

func replaceOnce(text, old, replacement string) string {
	for index := 0; index+len(old) <= len(text); index++ {
		if text[index:index+len(old)] == old {
			return text[:index] + replacement + text[index+len(old):]
		}
	}
	return text
}

func TestTrainingPackageReferenceAndBindingRefusals(t *testing.T) {
	input := corpusInput(t, "minimal")
	bad := input
	bad.Reference.ID = "BAD"
	bad.Reference.Revision = "0"
	bad.Reference.Path = "../escape"
	bad.Reference.SHA256 = "xyz"
	bad.Reference.Bytes = "0"
	result := AdmitTrainingPackage(bad)
	require.False(t, result.Admitted)
	require.Len(t, result.Issues, 6)

	pinMismatch := input
	pinMismatch.IndexBytes = append([]byte{}, input.IndexBytes...)
	pinMismatch.IndexBytes[0] = ' '
	result = AdmitTrainingPackage(pinMismatch)
	require.False(t, result.Admitted)
	require.Equal(t, CodeDigestMismatch, result.Issues[0].Code)

	bindingCases := []struct {
		name   string
		mutate func(*TrainingPackageBinding)
		code   Code
		path   string
	}{
		{"narrative-digest", func(binding *TrainingPackageBinding) { binding.NarrativeSHA256 = "x" }, CodeDigestMismatch, "/binding/narrativeSha256"},
		{"package-digest-shape", func(binding *TrainingPackageBinding) { binding.PackageSHA256 = "x" }, CodeDigestMismatch, "/binding/packageSha256"},
		{"scenario-id", func(binding *TrainingPackageBinding) { binding.ScenarioID = "BAD ID" }, CodeInvalidValue, "/binding/scenarioId"},
		{"scenario-revision", func(binding *TrainingPackageBinding) { binding.ScenarioRevision = "0" }, CodeInvalidValue, "/binding/scenarioRevision"},
	}
	for _, testCase := range bindingCases {
		t.Run(testCase.name, func(t *testing.T) {
			failed := input
			testCase.mutate(&failed.Binding)
			result := AdmitTrainingPackage(failed)
			require.False(t, result.Admitted)
			require.Equal(t, testCase.code, result.Issues[0].Code)
			require.Equal(t, testCase.path, result.Issues[0].Path)
		})
	}
}

func TestTrainingPackageDuplicateSuppliedAsset(t *testing.T) {
	input := corpusInput(t, "minimal")
	input.Assets = append(input.Assets, input.Assets[0])
	result := AdmitTrainingPackage(input)
	require.False(t, result.Admitted)
	require.Equal(t, CodeMismatch, result.Issues[0].Code)
}

// TestTrainingPackageProjectionCoversOptionalShapes exercises the defensive
// projection directly so absent codecs and every typed presentation branch are
// rendered without inventing a package that could be admitted.
func TestTrainingPackageProjectionCoversOptionalShapes(t *testing.T) {
	expected := investigation.NewIntegerValue("1")
	check := &detective.Check{
		ID: "check", Title: "Check", Version: 2,
		Steps: []detective.Step{{
			ID: "child", Kind: "check", CheckID: "other",
			Bindings: []detective.Binding{{Input: "i", FromInput: "f", Part: "start", FromColumn: "c"}},
			Validators: []detective.Validator{{
				ID: "v", Expected: &expected,
				Bindings: []detective.Binding{{Input: "i", FromInput: "f"}},
			}},
		}},
		UpdatedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
		UpdatedBy: "actor",
	}
	packageValue := &AdmittedTrainingPackage{
		Schema: PackageSchemaV1,
		ID:     "projection",
		Binding: TrainingPackageBinding{
			ContentPin: ContentPin{Kind: PinDigest, SHA256: "cc"},
		},
		Assets: []TrainingPackageAsset{
			{ID: "text", Kind: "evidence-text", Text: "x"},
			{ID: "records", Kind: "evidence-records"},
			{ID: "log", Kind: "evidence-log"},
			{ID: "metadata", Kind: "query-metadata"},
			{ID: "check-asset", Kind: "check-definition", Check: check},
			{ID: "check-nil", Kind: "check-definition"},
			{ID: "manifest", Kind: "challenge-manifest"},
			{ID: "watch", Kind: "watch-recommendation"},
			{ID: "binary", Kind: "challenge-fixture"},
			{ID: "records-typed", Kind: "evidence-records", Records: &TrainingRecords{
				Columns: []TrainingRecordColumn{{Name: "n", Type: investigation.ValueTypeNumber}},
				Rows: [][]investigation.TypedValue{{
					investigation.NewNumberValue(2.5),
					investigation.NewBooleanValue(true),
					investigation.NewNullValue(),
				}},
			}},
		},
		Checks: []TrainingCheckRecord{{ID: "check", DefinitionAssetID: "check-asset"}},
		Challenges: []TrainingChallengeRecord{{
			ID: "challenge", Revision: "1", ManifestAssetID: "manifest",
			Fixtures: []TrainingChallengeFixture{{FixtureID: "f", AssetID: "binary"}},
		}},
		FictionalActors: []TrainingFictionalActor{{ID: "actor", Name: "Actor"}},
		Sources:         []TrainingSourceBinding{{ID: "s", FixtureAssetID: "binary"}},
		Cases: []TrainingCase{{
			ID: "c", Title: "C", Status: "active",
			Context: map[string]detective.InputValue{
				"set": {Set: &apicontract.TypedValueSet{Values: []investigation.TypedValue{
					investigation.NewStringValue("a"),
				}}},
				"range": {Range: &detective.ValueRange{Type: "range", Start: expected, End: expected}},
				"empty": {},
			},
			Findings: []TrainingFinding{{ID: "f", Text: "T", Status: "hypothesis", EvidenceRefs: []string{}, HasEvidenceRefs: true}},
			Timeline: []TrainingTimelineEntry{{ID: "t", At: "2020-01-01T00:00:00Z", ActorID: "actor",
				Type: "evidence-revealed", Ref: TrainingTimelineRef{Kind: "evidence", ID: "text"}}},
		}},
		Proposals:           []TrainingProposal{{ID: "p", Kind: "query", DefinitionID: "q", EvidenceIDs: []string{"text"}, Rationale: "r"}},
		PresentationTargets: []TrainingPresentationTarget{{ID: "pt", Kind: "case", RefID: "c"}},
		Contributions:       []TrainingAssetContribution{{Path: "x", SHA256: "cc", Class: "text", Bytes: "1", AssetIDs: []string{"text"}}},
	}
	projected := packageValue.Projection()
	require.Equal(t, "projection", projected["id"])
	codecs := projected["codecs"].([]any)
	require.Len(t, codecs, len(packageValue.Assets))
}

func TestTrainingPackageTrustedInventoryProjections(t *testing.T) {
	admitted := mustAdmit(t, corpusInput(t, "full"))
	packageInventory := admitted.TrustedPackageInventory()
	require.True(t, packageInventory.Ref.Equal(admitted.Reference))
	require.Len(t, packageInventory.Assets, len(admitted.Assets))
	require.Equal(t, []string{"retry-case"}, packageInventory.Cases)

	narrative := admitted.TrustedNarrativeInventory()
	require.Equal(t, admitted.Binding.ContentPin, narrative.Binding.ContentPin)
	require.Equal(t, admitted.Reference, narrative.Binding.Package)
	require.Len(t, narrative.Assets, len(admitted.Assets))
	require.Equal(t, "retry-fixture", narrative.Assets[7].AssetID)
	require.Equal(t, "challenge-fixture", narrative.Assets[7].Kind)
	require.Equal(t, []string{"retry-case"}, narrative.Cases)
	require.Equal(t, []string{"retry-incident"}, narrative.Incidents)
	require.Equal(t, []string{"retry-observation"}, narrative.Checks)
	require.Equal(t, []string{"retries/by-status"}, narrative.Queries)
	require.Equal(t, []string{"query-proposal", "check-proposal", "watch-proposal"}, narrative.Proposals)
	require.Len(t, narrative.PresentationTargets, 5)
	require.Equal(t, "training-fixture", admitted.Sources[0].ID)
}

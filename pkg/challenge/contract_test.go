package challenge

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/require"
)

func TestGoldenManifestsOpenWithPinnedPristineFixtures(t *testing.T) {
	for _, challengeID := range []string{"deduplicate-customers", "retry-evidence"} {
		t.Run(challengeID, func(t *testing.T) {
			opened := openGolden(t, challengeID)
			require.Equal(t, SchemaVersion, opened.Manifest.SchemaVersion)
			require.Equal(t, GradingVersion, opened.Manifest.GradingVersion)
			require.Len(t, opened.DefinitionDigest, 64)
			require.NotEmpty(t, opened.Manifest.Parts)

			asset, err := AssetPath(opened.Manifest.ChallengeID, opened.Manifest.Revision)
			require.NoError(t, err)
			require.Equal(t, "challenges/"+challengeID+"/1.json", asset)

			route, err := GuidedRouteSuffix(opened.Manifest.ChallengeID, opened.Manifest.Revision)
			require.NoError(t, err)
			require.Equal(t, "challenge/"+challengeID+"/v/1", route)

			// Opening the same golden bytes again is deterministic.
			again := openGolden(t, challengeID)
			require.Equal(t, opened.DefinitionDigest, again.DefinitionDigest)

			// Every fixture pin is verified against a pristine copy of the bytes.
			for _, fixture := range opened.Manifest.FixtureSuite.Fixtures {
				bytesOfFixture, ok := opened.Fixture(fixture.ID)
				require.True(t, ok)
				require.Len(t, bytesOfFixture, fixture.Bytes)
				againBytes, ok := again.Fixture(fixture.ID)
				require.True(t, ok)
				require.True(t, bytes.Equal(bytesOfFixture, againBytes))
			}
		})
	}
}

func TestDefinitionRefusalsAreRecoverableAndCannotOpen(t *testing.T) {
	const challengeID = "deduplicate-customers"
	valid := goldenManifestData(t, challengeID)
	validFixtures := goldenFixtures(t, challengeID)
	firstFixture := "nonsequential"

	tests := []struct {
		name     string
		manifest []byte
		fixtures map[string][]byte
		code     Code
	}{
		{
			name:     "unsupported schema version",
			manifest: mutateManifestJSON(t, valid, func(value map[string]any) { value["schemaVersion"] = 2 }),
			fixtures: validFixtures,
			code:     CodeUnsupportedSchema,
		},
		{
			name: "unsupported schema with future fields",
			manifest: mutateManifestJSON(t, valid, func(value map[string]any) {
				value["schemaVersion"] = 2
				value["futureField"] = true
			}),
			fixtures: validFixtures,
			code:     CodeUnsupportedSchema,
		},
		{
			name:     "unsupported grading semantics",
			manifest: mutateManifestJSON(t, valid, func(value map[string]any) { value["gradingVersion"] = "challenge-grading-v2" }),
			fixtures: validFixtures,
			code:     CodeUnsupportedGradingVersion,
		},
		{
			name:     "unsupported dialect",
			manifest: mutateManifestJSON(t, valid, func(value map[string]any) { value["dialect"] = "postgres" }),
			fixtures: validFixtures,
			code:     CodeUnsupportedDialect,
		},
		{
			name: "mismatched engine pin",
			manifest: mutateManifestJSON(t, valid, func(value map[string]any) {
				value["engine"].(map[string]any)["version"] = "0.0.0"
			}),
			fixtures: validFixtures,
			code:     CodeUnsupportedEngine,
		},
		{
			name:     "unknown top-level field",
			manifest: mutateManifestJSON(t, valid, func(value map[string]any) { value["debug"] = true }),
			fixtures: validFixtures,
			code:     CodeInvalidManifest,
		},
		{
			name: "unknown nested answer field",
			manifest: mutateManifestJSON(t, valid, func(value map[string]any) {
				value["parts"].([]any)[0].(map[string]any)["sql"].(map[string]any)["debug"] = true
			}),
			fixtures: validFixtures,
			code:     CodeInvalidManifest,
		},
		{
			name:     "duplicate JSON key",
			manifest: append([]byte(`{"schemaVersion":1,`), valid[1:]...),
			fixtures: validFixtures,
			code:     CodeInvalidManifest,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opened, err := Open(test.manifest, test.fixtures)
			require.Nil(t, opened)
			requireRefused(t, err, test.code)
		})
	}

	t.Run("mismatched fixture hash", func(t *testing.T) {
		fixtures := cloneFixtures(validFixtures)
		corrupted := append([]byte(nil), fixtures[firstFixture]...)
		corrupted[len(corrupted)-1] ^= 0xff
		fixtures[firstFixture] = corrupted
		opened, err := Open(valid, fixtures)
		require.Nil(t, opened)
		requireRefused(t, err, CodeFixtureHash)
	})

	t.Run("mismatched fixture byte length", func(t *testing.T) {
		fixtures := cloneFixtures(validFixtures)
		fixtures[firstFixture] = fixtures[firstFixture][:len(fixtures[firstFixture])-1]
		opened, err := Open(valid, fixtures)
		require.Nil(t, opened)
		requireRefused(t, err, CodeFixtureBytes)
	})

	t.Run("missing fixture bytes", func(t *testing.T) {
		fixtures := cloneFixtures(validFixtures)
		delete(fixtures, firstFixture)
		opened, err := Open(valid, fixtures)
		require.Nil(t, opened)
		requireRefused(t, err, CodeFixtureMissing)
	})

	// A refused definition cannot be turned into a completed attempt.
	attempt, gradeErr := GradeAttempt(Manifest{}, nil)
	require.Error(t, gradeErr)
	require.False(t, attempt.Complete)

	t.Run("unpinned fixture bytes", func(t *testing.T) {
		fixtures := cloneFixtures(validFixtures)
		fixtures["surprise"] = []byte("not pinned")
		opened, err := Open(valid, fixtures)
		require.Nil(t, opened)
		requireRefused(t, err, CodeFixtureUnknown)
	})
}

func TestManifestRangeAndReferenceChecks(t *testing.T) {
	const challengeID = "deduplicate-customers"
	valid := goldenManifestData(t, challengeID)

	mutations := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"revision below range", func(value map[string]any) { value["revision"] = 0 }},
		{"empty title", func(value map[string]any) { value["title"] = " " }},
		{"unknown timing mode", func(value map[string]any) { value["timing"] = "paused" }},
		{"unknown operation", func(value map[string]any) { value["allowedOperation"] = "admin" }},
		{"no parts", func(value map[string]any) { value["parts"] = []any{} }},
		{"duplicate part id", func(value map[string]any) {
			parts := value["parts"].([]any)
			parts = append(parts, parts[0])
			value["parts"] = parts
		}},
		{"unknown case fixture", func(value map[string]any) {
			value["parts"].([]any)[0].(map[string]any)["sql"].(map[string]any)["cases"].([]any)[0].(map[string]any)["fixture"] = "missing"
		}},
		{"duplicate case fixture", func(value map[string]any) {
			sqlPart := value["parts"].([]any)[0].(map[string]any)["sql"].(map[string]any)
			cases := sqlPart["cases"].([]any)
			sqlPart["cases"] = append(cases, cases[0])
		}},
		{"fixture runs out of range", func(value map[string]any) {
			value["fixtureSuite"].(map[string]any)["fixtures"].([]any)[0].(map[string]any)["runs"] = 9
		}},
		{"fixture hash malformed", func(value map[string]any) {
			value["fixtureSuite"].(map[string]any)["fixtures"].([]any)[0].(map[string]any)["sha256"] = "not-a-sha"
		}},
		{"visible schema unknown type", func(value map[string]any) {
			value["visibleSchema"].(map[string]any)["tables"].([]any)[0].(map[string]any)["columns"].([]any)[0].(map[string]any)["type"] = "blob"
		}},
		{"hint duplicate id", func(value map[string]any) {
			hints := value["hints"].([]any)
			value["hints"] = append(hints, hints[0])
		}},
		{"reference to missing part", func(value map[string]any) {
			value["referenceSolutions"].([]any)[0].(map[string]any)["partId"] = "missing"
		}},
		{"reference answer kind mismatch", func(value map[string]any) {
			value["referenceSolutions"].([]any)[0].(map[string]any)["answer"] = map[string]any{"structured": map[string]any{"selections": map[string]any{}}}
		}},
		{"too much SQL", func(value map[string]any) {
			value["referenceSolutions"].([]any)[0].(map[string]any)["answer"] = map[string]any{"sql": strings.Repeat("x", MaxSQLBytes+1)}
		}},
		{"no required part", func(value map[string]any) {
			for _, raw := range value["parts"].([]any) {
				raw.(map[string]any)["required"] = false
			}
		}},
	}

	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			manifest, err := LoadManifest(mutateManifestJSON(t, valid, test.mutate))
			require.Error(t, err)
			var refusal *Error
			require.ErrorAs(t, err, &refusal)
			require.Equal(t, CodeInvalidManifest, refusal.Code)
			require.Empty(t, manifest.Parts)
		})
	}
}

func TestManifestAcceptsEveryDeclaredTimingAndOperationValue(t *testing.T) {
	valid := goldenManifestData(t, "deduplicate-customers")
	for _, timing := range []string{"off", "optional", "required"} {
		for _, operation := range []string{"read", "write"} {
			for _, difficulty := range []string{"intro", "intermediate", "advanced"} {
				manifest, err := LoadManifest(mutateManifestJSON(t, valid, func(value map[string]any) {
					value["timing"] = timing
					value["allowedOperation"] = operation
					value["difficulty"] = difficulty
				}))
				require.NoError(t, err, "%s/%s/%s", timing, operation, difficulty)
				require.Equal(t, TimingMode(timing), manifest.Timing)
			}
		}
	}
	for _, runs := range []int{1, 4} {
		_, err := LoadManifest(mutateManifestJSON(t, valid, func(value map[string]any) {
			value["fixtureSuite"].(map[string]any)["fixtures"].([]any)[0].(map[string]any)["runs"] = runs
		}))
		require.NoError(t, err)
	}
}

func TestStructuredManifestReferencesAreClosed(t *testing.T) {
	valid := goldenManifestData(t, "retry-evidence")
	partIndex := -1
	var value map[string]any
	require.NoError(t, json.Unmarshal(valid, &value))
	for index, raw := range value["parts"].([]any) {
		if raw.(map[string]any)["answerKind"] == "structured" {
			partIndex = index
			break
		}
	}
	require.GreaterOrEqual(t, partIndex, 0)

	mutations := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"expected unknown field", func(value map[string]any) {
			structured := value["parts"].([]any)[partIndex].(map[string]any)["structured"].(map[string]any)
			structured["expected"] = map[string]any{"selections": map[string]any{"not-a-field": []any{"x"}}}
		}},
		{"expected unknown option", func(value map[string]any) {
			structured := value["parts"].([]any)[partIndex].(map[string]any)["structured"].(map[string]any)
			fields := structured["fields"].([]any)
			structured["expected"] = map[string]any{"selections": map[string]any{
				fields[0].(map[string]any)["id"].(string): []any{"not-an-option"},
			}}
		}},
		{"expected cardinality out of range", func(value map[string]any) {
			structured := value["parts"].([]any)[partIndex].(map[string]any)["structured"].(map[string]any)
			fields := structured["fields"].([]any)
			structured["expected"] = map[string]any{"selections": map[string]any{
				fields[0].(map[string]any)["id"].(string): []any{},
			}}
		}},
		{"expected duplicate field id", func(value map[string]any) {
			structured := value["parts"].([]any)[partIndex].(map[string]any)["structured"].(map[string]any)
			fields := structured["fields"].([]any)
			structured["fields"] = append(fields, fields[0])
		}},
	}

	for _, test := range mutations {
		t.Run(test.name, func(t *testing.T) {
			_, err := LoadManifest(mutateManifestJSON(t, valid, test.mutate))
			requireRefused(t, err, CodeInvalidManifest)
		})
	}
}

func TestOpenedFixturesAreImmutableCopies(t *testing.T) {
	const challengeID = "deduplicate-customers"
	manifest := goldenManifestData(t, challengeID)
	fixtures := goldenFixtures(t, challengeID)
	opened, err := Open(manifest, fixtures)
	require.NoError(t, err)
	before, ok := opened.Fixture("nonsequential")
	require.True(t, ok)
	original := append([]byte(nil), before...)

	// The caller may keep and mutate its own buffers; the opened definition
	// must execute the pinned snapshot.
	fixtures["nonsequential"][0] ^= 0xff
	manifest[0] = ' '

	after, ok := opened.Fixture("nonsequential")
	require.True(t, ok)
	require.True(t, bytes.Equal(original, after))
	after[0] ^= 0xff
	again, ok := opened.Fixture("nonsequential")
	require.True(t, ok)
	require.True(t, bytes.Equal(original, again))
	require.Equal(t, opened.Manifest.ChallengeID, challengeID)
}

func TestAssetPathsRejectTraversalAndBadRevisions(t *testing.T) {
	_, err := AssetPath("../escape", 1)
	requireRefused(t, err, CodeInvalidManifest)
	_, err = AssetPath("ok", 0)
	requireRefused(t, err, CodeInvalidManifest)
	_, err = GuidedRouteSuffix("ok", 0)
	requireRefused(t, err, CodeInvalidManifest)
	_, err = FixtureAssetPath("ok", "../escape")
	requireRefused(t, err, CodeInvalidManifest)

	path, err := FixtureAssetPath("deduplicate-customers", "nonsequential")
	require.NoError(t, err)
	require.Equal(t, "challenges/deduplicate-customers/fixtures/nonsequential.db", path)
}

func TestOpenedHelpersAndStableErrorText(t *testing.T) {
	opened := openGolden(t, "deduplicate-customers")
	ids := opened.FixtureIDs()
	require.Len(t, ids, len(opened.Manifest.FixtureSuite.Fixtures))
	require.Equal(t, []string{"no-duplicates", "nonsequential", "null-text", "repaired", "repaired-mutated", "three-member"}, ids)
	require.True(t, opened.HasFixture("nonsequential"))
	require.False(t, opened.HasFixture("missing"))
	bytesOfFixture, ok := opened.Fixture("nonsequential")
	require.True(t, ok)
	require.True(t, opened.FixtureBytesEqual("nonsequential", bytesOfFixture))
	require.False(t, opened.FixtureBytesEqual("nonsequential", []byte("different")))

	_, err := LoadManifest([]byte(`{"schemaVersion":1}`))
	require.Error(t, err)
	require.Equal(t, CodeInvalidManifest, ErrorCode(err))
	require.Contains(t, err.Error(), string(CodeInvalidManifest))
	require.Contains(t, err.Error(), "schemaVersion")
}

func TestRequireEngineRejectsDifferentRuntime(t *testing.T) {
	opened := openGolden(t, "deduplicate-customers")
	require.NoError(t, opened.RequireEngine(SupportedEngine))
	different := SupportedEngine
	different.Version = "3.99.0-build1"
	requireRefused(t, opened.RequireEngine(different), CodeUnsupportedEngine)
}

func TestResultOracleRequiresTypedRows(t *testing.T) {
	base := `{"kind":"result","columns":[{"name":"id","type":"integer"}],"rows":[[{"type":"string","value":"1"}]],"ordering":"unordered"}`
	var oracle Oracle
	err := json.Unmarshal([]byte(base), &oracle)
	require.Error(t, err, "a string cell cannot satisfy an integer column")

	good := `{"kind":"result","columns":[{"name":"id","type":"integer"}],"rows":[[{"type":"integer","value":"1"}]],"ordering":"unordered"}`
	require.NoError(t, json.Unmarshal([]byte(good), &oracle))
	require.NoError(t, oracle.Validate())

	unknown := `{"kind":"result","columns":[{"name":"id","type":"integer"}],"rows":[],"ordering":"unordered","extra":true}`
	require.Error(t, json.Unmarshal([]byte(unknown), &oracle))
}

func TestManifestRoundTripsThroughItsWireForm(t *testing.T) {
	for _, challengeID := range []string{"deduplicate-customers", "retry-evidence"} {
		opened := openGolden(t, challengeID)
		encoded, err := json.Marshal(opened.Manifest)
		require.NoError(t, err)
		decoded, err := LoadManifest(encoded)
		require.NoError(t, err)
		require.Equal(t, opened.Manifest, decoded)
		again, err := Open(encoded, goldenFixtures(t, challengeID))
		require.NoError(t, err)
		require.Equal(t, opened.DefinitionDigest, again.DefinitionDigest)
	}
}

func TestFixturePinAndExpectedValueHelpers(t *testing.T) {
	ratio, err := parseTolerance("0.001")
	require.NoError(t, err)
	require.NotNil(t, ratio)
	_, err = parseTolerance("-1")
	require.Error(t, err)
	_, err = parseTolerance("1e-3")
	require.Error(t, err)

	require.True(t, typedValueEqual(apicontract.NewNullValue(), apicontract.NewNullValue(), nil))
	require.False(t, typedValueEqual(apicontract.NewNullValue(), apicontract.NewStringValue(""), nil))
	require.False(t, typedValueEqual(apicontract.NewIntegerValue("2"), apicontract.NewNumberValue(2), nil))
	tolerance, err := parseTolerance("0.01")
	require.NoError(t, err)
	require.True(t, typedValueEqual(apicontract.NewIntegerValue("2"), apicontract.NewNumberValue(2), tolerance))
}

func cloneFixtures(source map[string][]byte) map[string][]byte {
	cloned := make(map[string][]byte, len(source))
	for id, data := range source {
		cloned[id] = append([]byte(nil), data...)
	}
	return cloned
}

package datatug

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Leaf validators are public contracts too: callers need not first run the
// enclosing QueryDef storage screen, which intercepts the same credential.
func TestQuerySourceMetadataLeafIdentifierRefusals(t *testing.T) {
	for _, field := range []string{"identity", "pair", "identity-envelope", "pair-envelope"} {
		t.Run(field, func(t *testing.T) {
			binding := sourceQuery(t).RelationshipBindings[0]
			if field == "identity" || field == "identity-envelope" {
				binding.ID = "Password=secret"
				if field == "identity-envelope" {
					binding.ID = " padded"
				}
				require.ErrorContains(t, binding.Validate(), "relationship identifier")
			} else {
				binding.Pairs = []QueryRelationshipFieldPair{{FromField: "Password=secret", ToField: "ID"}}
				if field == "pair-envelope" {
					binding.Pairs[0].FromField = " padded"
				}
				require.ErrorContains(t, binding.Validate(), "relationship field")
			}
		})
	}
}

// Exercise the exported decoder entry point directly, as well as normal JSON
// admission: syntax/trailing failures must leave an existing identity intact.
func TestQuerySourceMetadataFederationMalformedJSONKeepsReceiver(t *testing.T) {
	for _, raw := range []string{`[`, `[]`, `{} {}`, `{} true`, `{} garbage`} {
		t.Run(raw, func(t *testing.T) {
			original := *sourceQuery(t).Federation
			got := original
			require.Error(t, got.UnmarshalJSON([]byte(raw)))
			require.Equal(t, original, got)
			require.Error(t, json.Unmarshal([]byte(raw), &got))
			require.Equal(t, original, got)
		})
	}
}

func TestQuerySourceMetadataNullBindingsJSONKeepsReceiver(t *testing.T) {
	original := sourceQuery(t).RelationshipBindings
	got := original
	require.ErrorContains(t, json.Unmarshal([]byte(" null "), &got), "null relationship bindings")
	require.Equal(t, original, got)
}

type sourceMetadataYAMLDecoder interface {
	UnmarshalYAML(*yaml.Node) error
}

// yaml.v3 intentionally skips custom unmarshallers for a null scalar. Calling
// the interface entry point also verifies its own nonnull guarantee; nonnull
// malformed leaf shapes are separately checked through yaml.Unmarshal below.
func TestQuerySourceMetadataYAMLLeafRefusalsKeepReceiver(t *testing.T) {
	for _, tc := range []struct {
		name, raw, reason string
		target            func() sourceMetadataYAMLDecoder
	}{
		{"identity-sequence", `[value]`, "cannot unmarshal", func() sourceMetadataYAMLDecoder {
			return &QueryExpectedServerIdentity{BaseURL: "https://example.com", ServerID: "original"}
		}},
		{"identity-null", `null`, "must be a mapping", func() sourceMetadataYAMLDecoder {
			return &QueryExpectedServerIdentity{BaseURL: "https://example.com", ServerID: "original"}
		}},
		{"identity-unknown", `{baseUrl: https://example.com, serverId: original, grant: true}`, "unknown/noncanonical", func() sourceMetadataYAMLDecoder {
			return &QueryExpectedServerIdentity{BaseURL: "https://example.com", ServerID: "original"}
		}},
		{"endpoint-missing", `{schema: main}`, "missing/null", func() sourceMetadataYAMLDecoder { return &QueryRelationshipEndpoint{Schema: "main", Table: "original"} }},
		{"pair-missing", `{fromField: ID}`, "missing/null", func() sourceMetadataYAMLDecoder {
			return &QueryRelationshipFieldPair{FromField: "ID", ToField: "original"}
		}},
		{"bindings-mapping", `{id: original}`, "cannot unmarshal", func() sourceMetadataYAMLDecoder { v := sourceQuery(t).RelationshipBindings; return &v }},
		{"bindings-null", `null`, "null relationship bindings", func() sourceMetadataYAMLDecoder { v := sourceQuery(t).RelationshipBindings; return &v }},
		{"bindings-null-member", `[null]`, "null relationship binding", func() sourceMetadataYAMLDecoder { v := sourceQuery(t).RelationshipBindings; return &v }},
		{"federation-sequence", `[value]`, "cannot unmarshal", func() sourceMetadataYAMLDecoder { return sourceQuery(t).Federation }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.target()
			before, err := json.Marshal(got)
			require.NoError(t, err)
			var document yaml.Node
			require.NoError(t, yaml.Unmarshal([]byte(tc.raw), &document))
			require.ErrorContains(t, got.UnmarshalYAML(document.Content[0]), tc.reason)
			after, err := json.Marshal(got)
			require.NoError(t, err)
			require.JSONEq(t, string(before), string(after))
			if tc.raw != "null" {
				require.Error(t, yaml.Unmarshal([]byte(tc.raw), got))
				after, err = json.Marshal(got)
				require.NoError(t, err)
				require.JSONEq(t, string(before), string(after))
			}
		})
	}
}

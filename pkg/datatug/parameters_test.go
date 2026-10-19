package datatug

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParameterDef_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		assert.NoError(t, ParameterDef{ID: "p1", Type: "string"}.Validate())
	})
	t.Run("missing_id", func(t *testing.T) {
		assert.Error(t, ParameterDef{Type: "string"}.Validate())
	})
	t.Run("missing_type", func(t *testing.T) {
		assert.Error(t, ParameterDef{ID: "p1"}.Validate())
	})
	t.Run("valid_default_string", func(t *testing.T) {
		assert.NoError(t, ParameterDef{ID: "p1", Type: "string", DefaultValue: "val"}.Validate())
	})
	t.Run("invalid_default_string", func(t *testing.T) {
		assert.Error(t, ParameterDef{ID: "p1", Type: "string", DefaultValue: 123}.Validate())
	})
	t.Run("valid_default_integer", func(t *testing.T) {
		assert.NoError(t, ParameterDef{ID: "p1", Type: "integer", DefaultValue: 123}.Validate())
	})
	t.Run("valid_default_number", func(t *testing.T) {
		assert.NoError(t, ParameterDef{ID: "p1", Type: "number", DefaultValue: 123.45}.Validate())
	})
	t.Run("valid_default_boolean", func(t *testing.T) {
		assert.NoError(t, ParameterDef{ID: "p1", Type: "boolean", DefaultValue: true}.Validate())
	})
	t.Run("valid_default_bit", func(t *testing.T) {
		assert.NoError(t, ParameterDef{ID: "p1", Type: "bit", DefaultValue: 1}.Validate())
	})
}

func TestIntegerAndBitDefaultsSurviveSafeJSONRoundTrip(t *testing.T) {
	for _, kind := range []string{"integer", "bit"} {
		for _, value := range []int{0, 10, -10} {
			original := ParameterDef{ID: "p1", Type: kind, DefaultValue: value}
			require.NoError(t, original.Validate())
			encoded, err := json.Marshal(original)
			require.NoError(t, err)
			var decoded ParameterDef
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			require.NoError(t, decoded.Validate(), "%s %s", kind, encoded)
			assert.Equal(t, float64(value), decoded.DefaultValue)
		}
		for _, number := range []string{"9007199254740991", "-9007199254740991"} {
			var decoded ParameterDef
			require.NoError(t, json.Unmarshal([]byte(`{"id":"p1","type":"`+kind+`","defaultValue":`+number+`}`), &decoded))
			require.NoError(t, decoded.Validate(), "%s %s", kind, number)
		}
		for _, number := range []string{"1.5", "9007199254740992", "-9007199254740992", `"10"`} {
			var decoded ParameterDef
			require.NoError(t, json.Unmarshal([]byte(`{"id":"p1","type":"`+kind+`","defaultValue":`+number+`}`), &decoded))
			require.ErrorContains(t, decoded.Validate(), "defaultValue", "%s %s", kind, number)
		}
		for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			require.ErrorContains(t, (ParameterDef{ID: "p1", Type: kind, DefaultValue: value}).Validate(), "defaultValue")
		}
		// Existing Go int defaults remain valid even when too large for a JSON
		// consumer; the unsafe JSON-decoded float64 above is still rejected.
		require.NoError(t, (ParameterDef{ID: "p1", Type: kind, DefaultValue: int(9007199254740992)}).Validate())
	}
	assert.NoError(t, (ParameterDef{ID: "p1", Type: "number", DefaultValue: 1.5}).Validate())
	assert.NoError(t, (ParameterDef{ID: "p1", Type: "custom", DefaultValue: "legacy"}).Validate())
}

func TestParameters_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		assert.NoError(t, Parameters{{ID: "p1", Type: "string"}}.Validate())
	})
	t.Run("invalid", func(t *testing.T) {
		assert.Error(t, Parameters{{ID: ""}}.Validate())
	})
}

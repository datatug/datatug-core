package apicontract

import (
	"strings"
	"testing"
)

type decodeStrictFixture struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

func TestDecodeStrict_Valid(t *testing.T) {
	var v decodeStrictFixture
	if err := DecodeStrict([]byte(`{"name":"alice","age":30}`), &v); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Name != "alice" || v.Age != 30 {
		t.Errorf("got %+v", v)
	}
}

func TestDecodeStrict_RejectsUnknownField(t *testing.T) {
	var v decodeStrictFixture
	err := DecodeStrict([]byte(`{"name":"alice","age":30,"role":"admin"}`), &v)
	if err == nil {
		t.Fatal("expected an error for an unknown field")
	}
}

func TestDecodeStrict_RejectsDuplicateTopLevelKey(t *testing.T) {
	var v decodeStrictFixture
	err := DecodeStrict([]byte(`{"name":"alice","name":"bob","age":30}`), &v)
	if err == nil {
		t.Fatal("expected an error for a duplicate key")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected a duplicate-key error, got: %v", err)
	}
}

func TestDecodeStrict_RejectsDuplicateNestedKey(t *testing.T) {
	type nested struct {
		Inner decodeStrictFixture `json:"inner"`
	}
	var v nested
	err := DecodeStrict([]byte(`{"inner":{"name":"alice","age":1,"age":2}}`), &v)
	if err == nil {
		t.Fatal("expected an error for a duplicate key nested inside an object")
	}
	if !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("expected a duplicate-key error, got: %v", err)
	}
}

func TestDecodeStrict_RejectsDuplicateKeyInArrayElement(t *testing.T) {
	type withList struct {
		Items []decodeStrictFixture `json:"items"`
	}
	var v withList
	err := DecodeStrict([]byte(`{"items":[{"name":"a","age":1},{"name":"b","age":2,"age":3}]}`), &v)
	if err == nil {
		t.Fatal("expected an error for a duplicate key inside an array element")
	}
}

func TestDecodeStrict_RejectsTrailingData(t *testing.T) {
	var v decodeStrictFixture
	err := DecodeStrict([]byte(`{"name":"alice","age":30}{"name":"bob","age":1}`), &v)
	if err == nil {
		t.Fatal("expected an error for trailing data after the JSON value")
	}
}

func TestDecodeStrict_RejectsMalformedJSON(t *testing.T) {
	var v decodeStrictFixture
	err := DecodeStrict([]byte(`{"name":`), &v)
	if err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestDecodeStrict_AllowsEmptyNestedObjectsAndArrays(t *testing.T) {
	type withCollections struct {
		Tags  []string          `json:"tags"`
		Attrs map[string]string `json:"attrs"`
	}
	var v withCollections
	if err := DecodeStrict([]byte(`{"tags":[],"attrs":{}}`), &v); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecodeStrict_RejectsForbiddenField(t *testing.T) {
	var v decodeStrictFixture
	err := DecodeStrict([]byte(`{"name":"alice","age":30}`), &v, "name")
	if err == nil {
		t.Fatal("expected an error: 'name' was declared forbidden")
	}
	if !strings.Contains(err.Error(), "name") {
		t.Errorf("expected the error to name the forbidden field, got: %v", err)
	}
}

func TestDecodeStrict_ForbiddenFieldAbsentIsFine(t *testing.T) {
	var v decodeStrictFixture
	if err := DecodeStrict([]byte(`{"name":"alice","age":30}`), &v, "role"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecodeStrict_RejectsForbiddenFieldInsideArrayElement(t *testing.T) {
	type item struct {
		Name string `json:"name"`
	}
	type withList struct {
		Items []item `json:"items"`
	}
	var v withList
	err := DecodeStrict([]byte(`{"items":[{"name":"a"},{"name":"b","role":"admin"}]}`), &v, "role")
	if err == nil {
		t.Fatal("expected an error: forbidden field inside an array element")
	}
}

func TestDecodeStrict_RejectsMalformedJSONDuringDuplicateCheck(t *testing.T) {
	var v decodeStrictFixture
	if err := DecodeStrict([]byte(`{"name":"a",`), &v); err == nil {
		t.Fatal("expected an error for malformed JSON during the duplicate-key pre-pass")
	}
}

func TestDecodeStrict_RejectsMalformedJSONDuringForbiddenFieldCheck(t *testing.T) {
	var v decodeStrictFixture
	if err := DecodeStrict([]byte(`{"name":`), &v, "role"); err == nil {
		t.Fatal("expected an error for malformed JSON during the forbidden-field pre-pass")
	}
}

func TestDecodeStrict_ScalarValueIsFine(t *testing.T) {
	var s string
	if err := DecodeStrict([]byte(`"hello"`), &s); err != nil {
		t.Fatalf("unexpected error decoding a bare scalar: %v", err)
	}
}

func TestCheckNoDuplicateKeys_TruncatedObjectMissingClosingBrace(t *testing.T) {
	if err := checkNoDuplicateKeys([]byte(`{"a":1`)); err == nil {
		t.Fatal("expected an error for a truncated object")
	}
}

func TestCheckNoDuplicateKeys_TruncatedArrayMissingClosingBracket(t *testing.T) {
	if err := checkNoDuplicateKeys([]byte(`[1,2`)); err == nil {
		t.Fatal("expected an error for a truncated array")
	}
}

func TestCheckNoDuplicateKeys_MismatchedClosingDelimiter(t *testing.T) {
	// An object closed by "]" (or an array closed by "}") is the one shape
	// that makes dec.More() correctly report false and the explicit
	// "consume the closing delimiter" Token() call itself fail - proving
	// that check is not dead code.
	if err := checkNoDuplicateKeys([]byte(`{"a":1]`)); err == nil {
		t.Fatal("expected an error: object closed with ']'")
	}
	if err := checkNoDuplicateKeys([]byte(`[1,2}`)); err == nil {
		t.Fatal("expected an error: array closed with '}'")
	}
}

func TestCheckNoForbiddenFields_TruncatedAtEntry(t *testing.T) {
	// Called directly (not through DecodeStrict, whose duplicate-key
	// pre-pass runs first and would already reject the same malformed
	// bytes) so walkNoForbiddenFields's own error paths are exercised.
	if err := checkNoForbiddenFields([]byte(`{"a":`), []string{"role"}); err == nil {
		t.Fatal("expected an error for malformed JSON at the entry token")
	}
}

func TestCheckNoForbiddenFields_NestedObject(t *testing.T) {
	if err := checkNoForbiddenFields([]byte(`{"outer":{"role":"admin"}}`), []string{"role"}); err == nil {
		t.Fatal("expected an error: forbidden field nested one object deep")
	}
	if err := checkNoForbiddenFields([]byte(`{"outer":{"safe":"value"}}`), []string{"role"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckNoForbiddenFields_NestedArray(t *testing.T) {
	if err := checkNoForbiddenFields([]byte(`{"items":[{"safe":1},{"role":"admin"}]}`), []string{"role"}); err == nil {
		t.Fatal("expected an error: forbidden field nested inside an array element")
	}
}

func TestCheckNoForbiddenFields_TruncatedObjectMissingClosingBrace(t *testing.T) {
	if err := checkNoForbiddenFields([]byte(`{"a":1`), []string{"role"}); err == nil {
		t.Fatal("expected an error for a truncated object")
	}
}

func TestCheckNoForbiddenFields_TruncatedArrayMissingClosingBracket(t *testing.T) {
	if err := checkNoForbiddenFields([]byte(`{"items":[1,2`), []string{"role"}); err == nil {
		t.Fatal("expected an error for a truncated array")
	}
}

func TestCheckNoForbiddenFields_MismatchedClosingDelimiter(t *testing.T) {
	if err := checkNoForbiddenFields([]byte(`{"a":1]`), []string{"role"}); err == nil {
		t.Fatal("expected an error: object closed with ']'")
	}
	if err := checkNoForbiddenFields([]byte(`{"items":[1,2}`), []string{"role"}); err == nil {
		t.Fatal("expected an error: array closed with '}'")
	}
}

func TestDecodeStrict_RejectsClientSuppliedPrincipalOrRole(t *testing.T) {
	// The contract: "a client-supplied principal or role are rejected, never
	// reconciled by precedence." None of this package's request types declare
	// principal/role fields at all, so DisallowUnknownFields already refuses
	// them - this test proves that specific, named security-relevant case
	// stays refused, not just "some" unknown field.
	var v Scope
	body := `{"project":"p1","environment":"local","securityContextId":"sc1","principal":"admin"}`
	err := DecodeStrict([]byte(body), &v)
	if err == nil {
		t.Fatal("expected an error: client-supplied principal must be rejected")
	}

	body = `{"project":"p1","environment":"local","securityContextId":"sc1","role":"admin"}`
	if err := DecodeStrict([]byte(body), &v); err == nil {
		t.Fatal("expected an error: client-supplied role must be rejected")
	}
}

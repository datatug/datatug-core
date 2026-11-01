package dataaccess

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/condeval"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/tugql"
	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/datatug"
	"gopkg.in/yaml.v3"
)

// StructuredReadProfile is a single-connection structured recordset operation.
// It is distinct from the closed all-column table preview and from raw SQL.
const StructuredReadProfile = "structured-recordset/1"

// StructuredRead contains immutable serialized intent, never a caller query or
// authority. TugQL is the existing serialized YAML/JSON query syntax. Parameters
// is a strict JSON object of apicontract.TypedValueOrSet. Empty means no bindings.
// The output is bounded without changing the query's limit, offset or ordering.
// MaxRows and MaxBytes have the same bounds as TableRead.
type StructuredRead struct {
	TugQL      string `json:"tugql"`
	Parameters string `json:"parameters"`
	MaxRows    uint32 `json:"maxRows"`
	MaxBytes   uint32 `json:"maxBytes"`
}

func (op StructuredRead) valid() bool {
	return len(op.TugQL) > 0 && len(op.TugQL) <= 131072 && len(op.Parameters) <= 65536 &&
		op.MaxRows > 0 && op.MaxRows <= 100 && op.MaxBytes > 0 && op.MaxBytes <= 262144
}

func (op StructuredRead) query() (dal.StructuredQuery, *access.Request, error) {
	if !op.valid() {
		return nil, nil, fmt.Errorf("dataaccess: unsupported structured read")
	}
	if err := (datatug.QueryDef{Type: datatug.QueryTypeTugQL, Text: op.TugQL, StructuredBody: true}).RefuseContextVariables(); err != nil {
		return nil, nil, err
	}
	query, err := tugql.Deserialize([]byte(op.TugQL))
	if err != nil {
		return nil, nil, fmt.Errorf("dataaccess: unsupported structured query")
	}
	variables, err := structuredParameters(op.Parameters)
	if err != nil {
		return nil, nil, err
	}
	// Reuse the existing public substitution engine. It binds the existing
	// supported RHS-WHERE parameter profile; other parameter positions fail
	// below, rather than passing unresolved parameters to a native compiler.
	if where := query.Where(); where != nil && len(variables) != 0 {
		info, validationErr := condeval.Validate(where)
		if validationErr == nil && len(info.Params) != 0 {
			where, err = condeval.Substitute(where, func(name string) (any, bool) { value, ok := variables[name]; return value, ok })
			if err != nil {
				return nil, nil, fmt.Errorf("dataaccess: unresolved structured parameter")
			}
			query = dal.WithWhere(query, where)
		}
	}
	return freezeStructuredOperation(query)
}

// This is also the admission point for a trusted subordinate policy's effective
// query. No unchecked implementation or unresolved parameter reaches a driver.
func freezeStructuredOperation(query dal.StructuredQuery) (dal.StructuredQuery, *access.Request, error) {
	serialized, err := tugql.Serialize(query)
	if err != nil {
		return nil, nil, fmt.Errorf("dataaccess: unsupported structured query")
	}
	defer clear(serialized)
	return parseStructuredOperation(serialized)
}

func parseStructuredOperation(serialized []byte) (dal.StructuredQuery, *access.Request, error) {
	query, err := tugql.Deserialize(serialized)
	if err != nil {
		return nil, nil, err
	}
	// Admission applies to authored intent AND the complete effective query
	// after subordinate policies. A known resource set does not establish the
	// separately owned leaf/intermediate lifetime required by recursive routes.
	if dal.HasSubquery(query) {
		return nil, nil, fmt.Errorf("dataaccess: recursive query requires a separately owned execution profile")
	}
	var tree yaml.Node
	// The same YAML decoder has just accepted this document, including strict
	// shape validation. Its node representation cannot now be malformed YAML.
	_ = yaml.Unmarshal(serialized, &tree)
	if hasStructuredParameter(&tree) {
		return nil, nil, fmt.Errorf("dataaccess: unsupported structured parameter position")
	}
	request := &access.Request{Operation: access.Query, Resources: access.ResourcesForQuery(query)}
	if _, err := structuredIdentities(request.Resources); err != nil {
		return nil, nil, err
	}
	return query, request, nil
}

// This walks only the already validated serialized expression syntax to find
// unresolved bindings. Resource discovery remains DALgo's single shared walker.
func hasStructuredParameter(node *yaml.Node) bool {
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			if node.Content[i].Value == "param" {
				return true
			}
		}
	}
	for _, child := range node.Content {
		if hasStructuredParameter(child) {
			return true
		}
	}
	return false
}

func structuredParameters(data string) (map[string]any, error) {
	values := map[string]apicontract.TypedValueOrSet{}
	if data != "" {
		if err := tableDecode([]byte(data), &values); err != nil || values == nil {
			return nil, fmt.Errorf("dataaccess: invalid structured bindings")
		}
	}
	result := make(map[string]any, len(values))
	for name, value := range values {
		if !dal.ValidParamName(name) || strings.Contains(name, ".") || value.Validate() != nil {
			return nil, fmt.Errorf("dataaccess: unsupported structured binding")
		}
		if value.IsSet() {
			items := make([]any, len(value.Set.Values))
			for i, item := range value.Set.Values {
				var err error
				items[i], err = structuredScalar(item)
				if err != nil {
					return nil, err
				}
			}
			result[name] = items
		} else {
			var err error
			result[name], err = structuredScalar(*value.Scalar)
			if err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

func structuredScalar(value apicontract.TypedValue) (any, error) {
	switch value.Type {
	case apicontract.ValueTypeString, apicontract.ValueTypeDate, apicontract.ValueTypeDatetime:
		return value.Str, nil
	case apicontract.ValueTypeBoolean:
		return value.Bool, nil
	case apicontract.ValueTypeNull:
		return nil, nil
	case apicontract.ValueTypeInteger:
		number, err := strconv.ParseInt(value.Str, 10, 64)
		// DALgo's generic comparison engine uses floating comparisons. Refuse
		// outside its exact range instead of silently changing a typed value.
		if err == nil && number > -9007199254740992 && number < 9007199254740992 {
			return number, nil
		}
	case apicontract.ValueTypeNumber:
		if value.Num > -9007199254740992 && value.Num < 9007199254740992 {
			return value.Num, nil
		}
	case apicontract.ValueTypeDecimal:
		exact, _ := new(big.Rat).SetString(value.Str) // TypedValue validation precedes this call.
		number, _ := strconv.ParseFloat(value.Str, 64)
		actual := new(big.Rat).SetFloat64(number)
		if actual != nil && actual.Cmp(exact) == 0 && number > -9007199254740992 && number < 9007199254740992 {
			return number, nil
		}
	}
	return nil, fmt.Errorf("dataaccess: exact structured parameter is unsupported")
}

// RequestFor binds full operation bytes and bounds to an independently selected
// fresh nonce. Server and consumer call this from their own original intent.
func (op StructuredRead) RequestFor(nonce string) (ResultRequest, error) {
	if !op.valid() {
		return ResultRequest{}, fmt.Errorf("dataaccess: unsupported structured read")
	}
	data, _ := json.Marshal(op) // closed scalar DTO cannot invoke custom marshaling
	defer clear(data)
	request := ResultRequest{Nonce: nonce, RequestSHA256: hashBytes(bytes.TrimSpace(data))}
	if !request.valid() {
		return ResultRequest{}, fmt.Errorf("dataaccess: fresh structured nonce required")
	}
	return request, nil
}

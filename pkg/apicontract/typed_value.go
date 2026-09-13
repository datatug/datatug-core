package apicontract

import "github.com/datatug/datatug-core/pkg/investigation"

// ValueType and TypedValue remain source-compatible aliases while the single
// canonical implementation is owned by the Investigation Context package.
type ValueType = investigation.ValueType

const (
	ValueTypeString   = investigation.ValueTypeString
	ValueTypeNumber   = investigation.ValueTypeNumber
	ValueTypeInteger  = investigation.ValueTypeInteger
	ValueTypeDecimal  = investigation.ValueTypeDecimal
	ValueTypeBoolean  = investigation.ValueTypeBoolean
	ValueTypeDate     = investigation.ValueTypeDate
	ValueTypeDatetime = investigation.ValueTypeDatetime
	ValueTypeNull     = investigation.ValueTypeNull
)

type TypedValue = investigation.TypedValue

func NewStringValue(v string) TypedValue   { return investigation.NewStringValue(v) }
func NewNumberValue(v float64) TypedValue  { return investigation.NewNumberValue(v) }
func NewIntegerValue(v string) TypedValue  { return investigation.NewIntegerValue(v) }
func NewDecimalValue(v string) TypedValue  { return investigation.NewDecimalValue(v) }
func NewBooleanValue(v bool) TypedValue    { return investigation.NewBooleanValue(v) }
func NewDateValue(v string) TypedValue     { return investigation.NewDateValue(v) }
func NewDatetimeValue(v string) TypedValue { return investigation.NewDatetimeValue(v) }
func NewNullValue() TypedValue             { return investigation.NewNullValue() }

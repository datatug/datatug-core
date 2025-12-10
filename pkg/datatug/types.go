package datatug

const (
	// TypeString = "string"
	TypeString = "string"
	// TypeText = "text"
	TypeText = "text"
	// TypeJSON = "JSON"
	TypeJSON = "JSON"

	// TypeBit     = "bit"
	TypeBit = "bit"
	// TypeBoolean = "boolean"
	TypeBoolean = "boolean"

	// TypeNumber  = "number"
	TypeNumber = "number"
	// TypeInteger = "integer"
	TypeInteger = "integer"
	// TypeDecimal = "decimal"
	TypeDecimal = "decimal"
	// TypeFloat   = "float"
	TypeFloat = "float"
	// TypeMoney   = "money"
	TypeMoney = "money"

	// TypeDate     = "date"
	TypeDate = "date"
	// TypeDateTime = "datetime"
	TypeDateTime = "datetime"
	// TypeTime     = "time"
	TypeTime = "time"

	// TypeGUID = "GUID"
	TypeGUID = "GUID"
	// TypeUUID = "UUID"
	TypeUUID = "UUID"

	// TypeBinary = "binary"
	TypeBinary = "binary"
	// TypeImage  = "image"
	TypeImage = "image"
)

// KnownTypes enumerates list of known types
var KnownTypes = []string{
	TypeString,
	TypeText,
	TypeJSON,

	TypeBit,
	TypeBoolean,

	TypeNumber,
	TypeInteger,
	TypeDecimal,
	TypeFloat,
	TypeMoney,

	TypeDate,
	TypeDateTime,
	TypeTime,

	TypeGUID,
	TypeUUID,

	TypeBinary,
	TypeImage,
}

package dbconnection

const (
	// ModeReadOnly specifies a read-only mode
	ModeReadOnly Mode = "ro"
	// ModeReadWrite specifies read-write mode
	ModeReadWrite Mode = "rw"
)

// Mode holds read/write mode
type Mode = string

// Params holds params
type Params interface {
	Driver() string
	Mode() Mode
	Server() string
	Port() int
	Catalog() string
	User() string
	ConnectionString() string
	String() string
}

package dbconnection

const (
	ModeReadOnly  = "ro"
	ModeReadWrite = "rw"
)

type Mode = string

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
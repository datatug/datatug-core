package dbconnection

const (
	ModeReadOnly  Mode = "ro"
	ModeReadWrite Mode = "rw"
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

package models

// Credentials holds username & password
type Credentials struct {
	Username string `json:"username,omitempty" yaml:"username,omitempty"`
	Password string `json:"password,omitempty"  yaml:"password,omitempty"`
}

// Validate returns error if failed
func (v Credentials) Validate() error {
	return nil
}

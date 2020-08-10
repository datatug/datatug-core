package dto

// Credentials holds username & password
type Credentials struct {
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
}

// Validate returns error if failed
func (v Credentials) Validate() error {
	return nil
}

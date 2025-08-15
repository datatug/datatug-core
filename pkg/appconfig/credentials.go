package appconfig

type AuthCredential struct {
	ID    string `json:"ID"` // Like USER or service account ID
	Title string `json:"Title,omitempty"`
}

func (v AuthCredential) Validate() error {
	return nil
}

package dtconfig

type AuthCredential struct {
	ID    string `json:"GetID"` // Like USER or service account ID
	Title string `json:"Title,omitempty"`
}

func (v AuthCredential) Validate() error {
	return nil
}

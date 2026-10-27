package datatug

import (
	"fmt"
	"net/url"
	"strings"
)

// ProjectDatabase is a named database and its explicit anonymous OVDB browser
// binding. It grants no credentials, backend source binding or mutation rights.
type ProjectDatabase struct {
	ID           string             `json:"id" firestore:"id"`
	Title        string             `json:"title,omitempty" firestore:"title,omitempty"`
	ConnectionID string             `json:"connectionId" firestore:"connectionId"`
	OVDB         PublicOVDBDatabase `json:"ovdb" firestore:"ovdb"`
}
type PublicOVDBDatabase struct {
	BaseURL    string `json:"baseUrl" firestore:"baseUrl"`
	DatabaseID string `json:"databaseId" firestore:"databaseId"`
}
type ProjectDatabases []ProjectDatabase

func (items ProjectDatabases) Validate() error {
	if len(items) > 500 {
		return fmt.Errorf("too many project databases")
	}
	seen := map[string]bool{}
	for _, item := range items {
		if !ValidConnectionID(item.ID) || seen[item.ID] {
			return fmt.Errorf("invalid or duplicate database id")
		}
		seen[item.ID] = true
		if !ValidConnectionID(item.ConnectionID) || !ValidConnectionID(item.OVDB.DatabaseID) || len(item.Title) > 200 {
			return fmt.Errorf("invalid project database reference")
		}
		u, err := url.Parse(item.OVDB.BaseURL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(item.OVDB.BaseURL, "#") {
			return fmt.Errorf("database OVDB base must use HTTPS without credentials, query or fragment")
		}
	}
	return nil
}

package fbauth

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestProjectIDsFromServiceAccount(t *testing.T) {
	dir := t.TempDir()
	// Standard service account JSON with project_id
	saPath := filepath.Join(dir, "sa.json")
	if err := os.WriteFile(saPath, []byte(`{"project_id":"p1","type":"service_account"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	projs, err := projectsFromServiceAccount(saPath)
	if err != nil {
		t.Fatalf("projectsFromServiceAccount err: %v", err)
	}
	if len(projs) != 1 || projs[0].ProjectId != "p1" {
		t.Fatalf("unexpected projects: %#v", projs)
	}

	// Impersonated service account JSON without project_id but with impersonation URL
	impPath := filepath.Join(dir, "imp.json")
	impJSON := `{
	  "delegates": [],
	  "service_account_impersonation_url": "https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/datatug-alex@sneat-eur3-1.iam.gserviceaccount.com:generateAccessToken",
	  "source_credentials": {
	    "account": "",
	    "client_id": "764086051850-6qr4p6gpi6hn506pt8ejuq83di341hur.apps.googleusercontent.com",
	    "client_secret": "redacted",
	    "refresh_token": "redacted",
	    "type": "authorized_user",
	    "universe_domain": "googleapis.com"
	  },
	  "type": "impersonated_service_account"
	}`
	if err := os.WriteFile(impPath, []byte(impJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	projs, err = projectsFromServiceAccount(impPath)
	if err != nil {
		t.Fatalf("projectsFromServiceAccount(imp) err: %v", err)
	}
	if len(projs) != 1 || projs[0].ProjectId != "sneat-eur3-1" {
		t.Fatalf("unexpected projects from imp: %#v", projs)
	}

	// Empty path should error
	if _, err := projectsFromServiceAccount(""); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestNewServiceAccountMenu_PopulatesList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sa.json")
	if err := os.WriteFile(path, []byte(`{"project_id":"p1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	m := newServiceAccountMenu(ServiceAccountDbo{Name: "acc", Path: path})
	sam, ok := m.(*serviceAccountMenu)
	if !ok {
		t.Fatalf("unexpected model type: %T", m)
	}
	// Kick Init/measure cycle minimally
	_ = sam.Init()
	var msg tea.Msg
	m2, _ := sam.Update(msg)
	_ = m2
	// Check visible items length via View content sanity check
	view := sam.View()
	if view == "" {
		t.Fatalf("view is empty")
	}
}

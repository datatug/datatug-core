package datatug

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvironmentSummary_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		v := EnvironmentSummary{ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}}}
		assert.NoError(t, v.Validate())
	})
	t.Run("invalid_project_item", func(t *testing.T) {
		v := EnvironmentSummary{}
		assert.Error(t, v.Validate())
	})
	t.Run("invalid_servers", func(t *testing.T) {
		v := EnvironmentSummary{
			ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "e1"}},
			Servers:     EnvDbServers{{}},
		}
		assert.Error(t, v.Validate())
	})
}

func TestEnvironmentSummaryCopiesValidConnectionIDsWithoutCredentials(t *testing.T) {
	var environment Environment
	if err := json.Unmarshal([]byte(`{"id":"dev","dbServers":[],"editionConnections":["music","other"],"credentials":{"password":"private-secret"}}`), &environment); err != nil {
		t.Fatal(err)
	}
	summary, err := environment.Summary()
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "private-secret") || strings.Contains(string(body), "credentials") {
		t.Fatal("opaque credentials escaped summary")
	}
	var wire EnvironmentSummary
	if err = json.Unmarshal(body, &wire); err != nil || len(wire.EditionConnections) != 2 || wire.EditionConnections[0] != "music" {
		t.Fatalf("safe IDs lost: %s %v", body, err)
	}
	summary.EditionConnections[0] = "mutated"
	if environment.EditionConnections[0] != "music" {
		t.Fatal("summary aliases source IDs")
	}
	for _, ids := range [][]string{{"../music"}, {"music", "music"}, {"secret@host"}} {
		environment.EditionConnections = ids
		if _, err = environment.Summary(); err == nil {
			t.Fatalf("unsafe IDs projected: %v", ids)
		}
		wire.EditionConnections = ids
		if err = wire.Validate(); err == nil {
			t.Fatalf("unsafe summary accepted: %v", ids)
		}
	}
}

package datatug

import (
	"encoding/json"
	"testing"
)

func TestSQLWidgetSettings_rowLinkRoundTrips(t *testing.T) {
	in := `{"queryId":"q","rowLink":{"boardId":"{storage}-project","parameters":[{"id":"projectId","column":"__key__"},{"id":"k","value":"v"}]}}`
	var settings SQLWidgetSettings
	if err := json.Unmarshal([]byte(in), &settings); err != nil {
		t.Fatal(err)
	}
	if err := settings.Validate(); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != in {
		t.Fatalf("got %s", out)
	}
}

func TestSQLRowLink_Validate(t *testing.T) {
	for name, link := range map[string]SQLRowLink{
		"no board":     {},
		"no id":        {BoardID: "b", Parameters: []SQLRowLinkParameter{{Column: "c"}}},
		"both sources": {BoardID: "b", Parameters: []SQLRowLinkParameter{{ID: "p", Column: "c", Value: "v"}}},
		"no source":    {BoardID: "b", Parameters: []SQLRowLinkParameter{{ID: "p"}}},
	} {
		if err := link.Validate(); err == nil {
			t.Errorf("%s: expected an error", name)
		}
		if err := (SQLWidgetSettings{QueryID: "q", RowLink: &link}).Validate(); err == nil {
			t.Errorf("%s: expected the widget to refuse it", name)
		}
	}
}

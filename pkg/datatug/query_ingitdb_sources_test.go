package datatug

import (
	"encoding/json"
	"strings"
	"testing"
)

const geoDescriptor = "https://raw.githubusercontent.com/datatug/datatug-demo-project/50ed33afd0aee803588b186a2a09b36760a7c99e/demo-project-1/fixtures/ovdb/geo-github.ovdb.yaml"

func tugqlWithIngitdbSources(sources ...QueryIngitdbSource) QueryDef {
	return QueryDef{
		ProjectItem: ProjectItem{ProjItemBrief: ProjItemBrief{ID: "q", Title: "Q"}},
		Type:        "TugQL",
		Federation:  &QueryFederation{OVDBBaseURL: "https://cloud.openvaultdb.com", IngitdbSources: sources},
	}
}

func TestQueryFederation_IngitdbSourcesDecode(t *testing.T) {
	var q QueryDef
	if err := json.Unmarshal([]byte(`{"id":"q","type":"TugQL","federation":{"ingitdbSources":[{"database":"geo","descriptor":"`+geoDescriptor+`"}]}}`), &q); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got := q.Federation.IngitdbSources; len(got) != 1 || got[0].Database != "geo" || got[0].Descriptor != geoDescriptor {
		t.Fatalf("unexpected sources: %+v", got)
	}
}

func TestQueryFederation_IngitdbSourcesValidate(t *testing.T) {
	if err := tugqlWithIngitdbSources(QueryIngitdbSource{Database: "geo", Descriptor: geoDescriptor}).Validate(); err != nil {
		t.Fatalf("valid source refused: %v", err)
	}
	for name, tc := range map[string]struct {
		sources []QueryIngitdbSource
		want    string
	}{
		"database with a dot":   {[]QueryIngitdbSource{{Database: "geo.x", Descriptor: geoDescriptor}}, "ingitdbSources[0].database"},
		"empty database":        {[]QueryIngitdbSource{{Database: "", Descriptor: geoDescriptor}}, "ingitdbSources[0].database"},
		"plain http":            {[]QueryIngitdbSource{{Database: "geo", Descriptor: "http://example.com/d.yaml"}}, "ingitdbSources[0].descriptor"},
		"query string":          {[]QueryIngitdbSource{{Database: "geo", Descriptor: geoDescriptor + "?token=1"}}, "ingitdbSources[0].descriptor"},
		"empty query":           {[]QueryIngitdbSource{{Database: "geo", Descriptor: geoDescriptor + "?"}}, "ingitdbSources[0].descriptor"},
		"fragment":              {[]QueryIngitdbSource{{Database: "geo", Descriptor: geoDescriptor + "#x"}}, "ingitdbSources[0].descriptor"},
		"credentials":           {[]QueryIngitdbSource{{Database: "geo", Descriptor: "https://user:pass@example.com/d.yaml"}}, "ingitdbSources[0].descriptor"},
		"port":                  {[]QueryIngitdbSource{{Database: "geo", Descriptor: "https://example.com:8443/d.yaml"}}, "ingitdbSources[0].descriptor"},
		"not a URL":             {[]QueryIngitdbSource{{Database: "geo", Descriptor: "://"}}, "ingitdbSources[0].descriptor"},
		"database listed twice": {[]QueryIngitdbSource{{Database: "geo", Descriptor: geoDescriptor}, {Database: "geo", Descriptor: geoDescriptor}}, "listed twice"},
	} {
		t.Run(name, func(t *testing.T) {
			err := tugqlWithIngitdbSources(tc.sources...).Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestQueryIngitdbSource_Validate(t *testing.T) {
	if err := (QueryIngitdbSource{Database: "geo", Descriptor: geoDescriptor}).Validate(); err != nil {
		t.Fatalf("valid source refused: %v", err)
	}
	if err := (QueryIngitdbSource{Database: "geo", Descriptor: "ftp://example.com/d"}).Validate(); err == nil || !strings.Contains(err.Error(), "descriptor") {
		t.Fatalf("want descriptor refusal, got %v", err)
	}
}

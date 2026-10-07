package datatug

import (
	"errors"
	"strings"
	"testing"
)

func TestConnectionResolutionError(t *testing.T) {
	for _, tc := range []struct {
		err  ConnectionResolutionError
		text string
	}{
		{ConnectionResolutionError{Kind: ErrAmbiguousConnection, ConnectionID: "one"}, "ambiguous connection catalog: one"},
		{ConnectionResolutionError{Kind: ErrConnectionNotReady, ConnectionID: "one", EnvironmentID: "QA", Readiness: "setup-required"}, "connection is not ready: one in QA (setup-required)"},
		{ConnectionResolutionError{Kind: ErrConnectionNotFound, ConnectionID: "one", EnvironmentID: "QA"}, "connection not found: one in QA"},
	} {
		if got := tc.err.Error(); got != tc.text {
			t.Fatalf("got %q, want %q", got, tc.text)
		}
		if !errors.Is(&tc.err, tc.err.Kind) {
			t.Fatalf("error kind was lost: %v", tc.err.Unwrap())
		}
	}
}

func TestConnectionIdentityValidation(t *testing.T) {
	for _, id := range []string{"QA", "chinook-postgresql", "a.b_1"} {
		if !ValidConnectionID(id) {
			t.Fatalf("rejected ID %q", id)
		}
	}
	for _, id := range []string{"", "../QA", "white space", strings.Repeat("a", 129)} {
		if ValidConnectionID(id) {
			t.Fatalf("accepted unsafe ID %q", id)
		}
	}
	for _, label := range []string{"chinook", "Café"} {
		if !validConnectionLabel(label) {
			t.Fatalf("rejected label %q", label)
		}
	}
	for _, label := range []string{"", " padded", strings.Repeat("x", 257), "bad\nlabel", "bad\u202elabel", string([]byte{0xff})} {
		if validConnectionLabel(label) {
			t.Fatalf("accepted unsafe label %q", label)
		}
	}
	if err := uniqueConnectionIDs([]string{"dev", "QA"}); err != nil {
		t.Fatal(err)
	}
	if err := uniqueConnectionIDs([]string{"../bad"}); err == nil {
		t.Fatal("accepted invalid environment ID")
	}
	if err := uniqueConnectionIDs([]string{"QA", "QA"}); !errors.Is(err, ErrAmbiguousConnection) {
		t.Fatalf("duplicate: %v", err)
	}
	env := Environment{EditionConnections: []string{"QA", "QA"}}
	env.SetID("QA")
	if err := env.Validate(); !errors.Is(err, ErrAmbiguousConnection) {
		t.Fatalf("environment duplicate: %v", err)
	}
}

func sampleEditionConnection() EditionConnection {
	return EditionConnection{ID: "chinook-sqlite", Dataset: "chinook", Storage: "sqlite", Readiness: "public-api", Query: "ovdb-read", Tags: []string{"chinook"}, Environments: []string{"dev"}}
}

func TestEditionConnectionValidationAndReadiness(t *testing.T) {
	valid := sampleEditionConnection()
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, readiness := range []string{"public-api", "hosted-repository", "public-read-user-project-required"} {
		c := valid
		c.Readiness = readiness
		if !c.ReadyForBinding() {
			t.Fatalf("actionable readiness refused: %s", readiness)
		}
	}
	for _, readiness := range []string{"hosted-api-pending", "setup-required", "unexpected"} {
		c := valid
		c.Readiness = readiness
		if c.ReadyForBinding() {
			t.Fatalf("pending/unknown readiness accepted: %s", readiness)
		}
	}
	for _, mutate := range []func(*EditionConnection){
		func(c *EditionConnection) { c.ID = "../bad" },
		func(c *EditionConnection) { c.Dataset = "" },
		func(c *EditionConnection) { c.Storage = "bad/storage" },
		func(c *EditionConnection) { c.Readiness = "bad readiness" },
		func(c *EditionConnection) { c.Query = "bad query" },
		func(c *EditionConnection) { c.Environments = []string{"dev", "dev"} },
		func(c *EditionConnection) { c.Tags = []string{"bad tag"} },
		func(c *EditionConnection) { c.SourceProjectID = "demodb-dev" },
	} {
		c := valid
		mutate(&c)
		if err := c.Validate(); err == nil {
			t.Fatalf("invalid connection accepted: %+v", c)
		}
	}
	bq := valid
	bq.Storage, bq.SourceProjectID, bq.DatasetID, bq.Location = "bigquery", "demodb-dev", "chinook", "US"
	if err := bq.Validate(); err != nil {
		t.Fatal(err)
	}
	bq.Location = "US/unsafe"
	if err := bq.Validate(); err == nil {
		t.Fatal("invalid BigQuery location accepted")
	}
}

func TestProjectConnectionsValidation(t *testing.T) {
	valid := sampleEditionConnection()
	bq := EditionConnection{ID: "chinook-bigquery", Dataset: "chinook", Storage: "bigquery", Readiness: "public-read-user-project-required", SourceProjectID: "demodb-dev", DatasetID: "chinook", Location: "US"}
	base := ProjectConnections{Format: ConnectionCatalogFormat, Connections: []EditionConnection{valid}, BigQueryEditions: []EditionConnection{bq}, BigQueryPlans: []ConnectionPlan{{ID: "planned", Readiness: "setup-required"}}}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*ProjectConnections)
		kind   error
	}{
		{"format", func(c *ProjectConnections) { c.Format = "future/v2" }, ErrUnsupportedConnectionFile},
		{"duplicate connection", func(c *ProjectConnections) { c.Connections = append(c.Connections, valid) }, ErrAmbiguousConnection},
		{"duplicate bigquery", func(c *ProjectConnections) { c.BigQueryEditions = append(c.BigQueryEditions, bq) }, ErrAmbiguousConnection},
		{"duplicate plan", func(c *ProjectConnections) { c.BigQueryPlans = append(c.BigQueryPlans, c.BigQueryPlans[0]) }, ErrAmbiguousConnection},
		{"invalid connection ID", func(c *ProjectConnections) { c.Connections[0].ID = "../bad" }, nil},
		{"invalid connection", func(c *ProjectConnections) { c.Connections[0].Dataset = "" }, nil},
		{"invalid BigQuery ID", func(c *ProjectConnections) { c.BigQueryEditions[0].ID = "../bad" }, nil},
		{"invalid BigQuery declaration", func(c *ProjectConnections) { c.BigQueryEditions[0].Readiness = "bad readiness" }, nil},
		{"invalid BigQuery location", func(c *ProjectConnections) { c.BigQueryEditions[0].DatasetID = "" }, nil},
		{"missing BigQuery location", func(c *ProjectConnections) {
			c.BigQueryEditions[0].SourceProjectID, c.BigQueryEditions[0].DatasetID, c.BigQueryEditions[0].Location = "", "", ""
		}, nil},
		{"invalid BigQuery storage", func(c *ProjectConnections) { c.BigQueryEditions[0].Storage = "sqlite" }, nil},
		{"invalid plan ID", func(c *ProjectConnections) { c.BigQueryPlans[0].ID = "../bad" }, nil},
		{"invalid plan readiness", func(c *ProjectConnections) { c.BigQueryPlans[0].Readiness = "bad readiness" }, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			c.Connections = append([]EditionConnection(nil), base.Connections...)
			c.BigQueryEditions = append([]EditionConnection(nil), base.BigQueryEditions...)
			c.BigQueryPlans = append([]ConnectionPlan(nil), base.BigQueryPlans...)
			tc.mutate(&c)
			err := c.Validate()
			if err == nil {
				t.Fatal("invalid catalog accepted")
			}
			if tc.kind != nil && !errors.Is(err, tc.kind) {
				t.Fatalf("got %v, want %v", err, tc.kind)
			}
		})
	}
}

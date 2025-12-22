package datatug

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChecks_Validate(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		var checks = make(Checks, 0)
		assert.Nil(t, checks.Validate())
	})
	t.Run("invalid_item", func(t *testing.T) {
		checks := Checks{{ID: ""}}
		assert.NotNil(t, checks.Validate())
	})
}

func TestCheck_Validate(t *testing.T) {
	t.Run("missing_id", func(t *testing.T) {
		check := Check{ID: ""}
		assert.NotNil(t, check.Validate())
	})
	t.Run("missing_title", func(t *testing.T) {
		check := Check{ID: "id", Title: ""}
		assert.NotNil(t, check.Validate())
	})
	t.Run("missing_data", func(t *testing.T) {
		check := Check{ID: "id", Title: "title", Data: nil}
		assert.NotNil(t, check.Validate())
	})
	t.Run("missing_type", func(t *testing.T) {
		check := Check{ID: "id", Title: "title", Data: []byte("{}")}
		assert.NotNil(t, check.Validate())
	})
	t.Run("unknown_type", func(t *testing.T) {
		check := Check{ID: "id", Title: "title", Type: "unknown", Data: []byte("{}")}
		assert.NotNil(t, check.Validate())
	})
	t.Run("invalid_json", func(t *testing.T) {
		check := Check{ID: "id", Title: "title", Type: "sql", Data: []byte("invalid")}
		assert.NotNil(t, check.Validate())
	})
}

func TestSqlCheck_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		var check = Check{ID: "check-id", Title: "Check title", Type: "sql"}
		var err error
		if check.Data, err = json.Marshal(SQLCheck{Query: "select * from table"}); err != nil {
			t.Fatal(err)
		}
		assert.Nil(t, check.Validate())
	})
	t.Run("missing_query", func(t *testing.T) {
		check := SQLCheck{Query: ""}
		assert.NotNil(t, check.Validate())
	})
}

func TestOptionsValueCheck_Validate(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		var check = Check{ID: "check-id", Title: "Check title", Type: "options"}
		var err error
		if check.Data, err = json.Marshal(OptionsValueCheck{Type: "string", Options: []interface{}{"option1", "option2"}}); err != nil {
			t.Fatal(err)
		}
		assert.Nil(t, check.Validate())
	})
	t.Run("missing_type", func(t *testing.T) {
		check := OptionsValueCheck{Type: "", Options: []interface{}{"opt"}}
		assert.NotNil(t, check.Validate())
	})
	t.Run("missing_options", func(t *testing.T) {
		check := OptionsValueCheck{Type: "string", Options: nil}
		assert.NotNil(t, check.Validate())
	})
	t.Run("invalid_option_type", func(t *testing.T) {
		check := OptionsValueCheck{Type: "string", Options: []interface{}{1}}
		assert.NotNil(t, check.Validate())
	})
}

func TestRegexpValueCheck_Validate(t *testing.T) {
	var check = Check{ID: "check-id", Title: "Check title", Type: "regexp"}
	var err error

	t.Run("missing regexp", func(t *testing.T) {
		if check.Data, err = json.Marshal(RegexpValueCheck{}); err != nil {
			t.Fatal(err)
		}
		assert.NotNil(t, check.Validate())
	})
	t.Run("valid regexp", func(t *testing.T) {
		if check.Data, err = json.Marshal(RegexpValueCheck{Regexp: `\w+`}); err != nil {
			t.Fatal(err)
		}
		assert.Nil(t, check.Validate())
	})
	t.Run("not valid regexp", func(t *testing.T) {
		if check.Data, err = json.Marshal(RegexpValueCheck{Regexp: `(\w+`}); err != nil {
			t.Fatal(err)
		}
		assert.NotNil(t, check.Validate())
	})
}

package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestChecks_Validate(t *testing.T) {
	var checks = make(Checks, 0)
	assert.Nil(t, checks.Validate())
}

func TestSqlCheck_Validate(t *testing.T) {
	var check = Check{ID: "check-id", Title: "Check title", Type: "sql"}
	var err error
	if check.Data, err = json.Marshal(SQLCheck{Query: "select * from table"}); err != nil {
		t.Fatal(err)
	}
	assert.Nil(t, check.Validate())
}

func TestOptionsValueCheck_Validate(t *testing.T) {
	var check = Check{ID: "check-id", Title: "Check title", Type: "options"}
	var err error
	if check.Data, err = json.Marshal(OptionsValueCheck{Type: "string", Options: []interface{}{"option1", "option2"}}); err != nil {
		t.Fatal(err)
	}
	assert.Nil(t, check.Validate())
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

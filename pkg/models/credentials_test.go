package models

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCredentials_Validate(t *testing.T) {
	t.Run("should_pass", func(t *testing.T) {
		t.Run("on_empty", func(t *testing.T) {
			var credentials = Credentials{}
			assert.Nil(t, credentials.Validate())
		})
		t.Run("on_non_empty", func(t *testing.T) {
			var credentials = Credentials{Username: "some_user", Password: "some_password"}
			assert.Nil(t, credentials.Validate())
		})
	})
}

func TestCredentials_JSON(t *testing.T) {
	t.Run("should_be_empty_if_empty", func(t *testing.T) {
		content, err := json.Marshal(Credentials{})
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, []byte("{}"), content)
	})
	t.Run("Username", func(t *testing.T) {
		t.Run("should_persist", func(t *testing.T) {
			content, err := json.Marshal(Credentials{Username: "some_user"})
			if err != nil {
				t.Fatal(err)
			}
			s := string(content)
			assert.True(t, strings.Contains(s, `"username"`))
			assert.True(t, strings.Contains(s, `"some_user"`))
		})
	})
	t.Run("Password", func(t *testing.T) {
		t.Run("should_persist", func(t *testing.T) {
			content, err := json.Marshal(Credentials{Password: "some_pwd"})
			if err != nil {
				t.Fatal(err)
			}
			s := string(content)
			assert.True(t, strings.Contains(s, `"password"`))
			assert.True(t, strings.Contains(s, `"some_pwd"`))
		})
	})
}

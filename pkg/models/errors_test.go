package models

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProjectDoesNotExist(t *testing.T) {
	type args struct {
		err error
	}
	tests := []struct {
		name string
		args args
		want bool
	}{
		{
			name: "project does not exist: false",
			args: args{
				err: errors.New("project does not exist"),
			},
			want: false,
		},
		{
			name: "project does not exist: true",
			args: args{
				err: ErrProjectDoesNotExist,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equalf(t, tt.want, ProjectDoesNotExist(tt.args.err), "ProjectDoesNotExist(%v)", tt.args.err)
		})
	}
}

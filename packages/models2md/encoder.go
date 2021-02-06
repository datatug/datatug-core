package models2md

import (
	"github.com/datatug/datatug/packages/models"
)

// NewEncoder creates new encoder
func NewEncoder() models.ReadmeEncoder {
	return encoder{}
}

type encoder struct {
}

package commands

import "errors"

var (
	//ErrUnknownProjectName signals an unknown project was requested/referred
	ErrUnknownProjectName = errors.New("unknown project name")
)

package models

type validatable interface {
	Validate() error
}

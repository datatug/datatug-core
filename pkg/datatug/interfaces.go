package datatug

type validatable interface {
	Validate() error
}

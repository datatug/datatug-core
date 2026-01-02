package datatug

func GetStoreOptions(opts ...StoreOption) (o StoreOptions) {
	for _, opt := range opts {
		opt(&o)
	}
	return
}

type StoreOptions struct {
	depth int
}

func (o StoreOptions) ToSlice() (options []StoreOption) {
	if o.depth != 0 {
		options = append(options, Depth(o.depth))
	}
	return
}

func (o StoreOptions) Depth() int {
	return o.depth
}

func (o StoreOptions) Next() StoreOptions {
	if o.depth > 0 {
		o.depth--
	}
	return o
}

type StoreOption func(op *StoreOptions)

func Depth(depth int) StoreOption {
	return func(op *StoreOptions) {
		op.depth = depth
	}
}

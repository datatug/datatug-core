package datatug

func GetStoreOptions(opts ...StoreOption) (o StoreOptions) {
	for _, opt := range opts {
		opt(&o)
	}
	return
}

type StoreOptions struct {
	deep bool
}

func (o StoreOptions) Deep() bool {
	return o.deep
}

type StoreOption func(op *StoreOptions)

func Deep() StoreOption {
	return func(op *StoreOptions) {
		op.deep = true
	}
}

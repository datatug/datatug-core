package datatug

func GetStoreOptions(opts ...StoreOption) (o StoreOptions) {
	for _, opt := range opts {
		opt(&o)
	}
	return
}

type StoreOptions struct {
	depth           int
	activeItemsOnly bool
}

func (o StoreOptions) ToSlice() (options []StoreOption) {
	if o.depth != 0 {
		options = append(options, Depth(o.depth))
	}
	if o.activeItemsOnly {
		options = append(options, ActiveItemsOnly())
	}
	return
}

// ActiveItemsOnly selects a user-facing list projection. Internal readers and
// direct object access omit this option so archived references still resolve.
func ActiveItemsOnly() StoreOption {
	return func(op *StoreOptions) { op.activeItemsOnly = true }
}

func (o StoreOptions) ActiveItemsOnly() bool { return o.activeItemsOnly }

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

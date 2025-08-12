package apps

type Stack[T any] struct {
	items []T
}

func (s *Stack[T]) Push(v T) {
	s.items = append(s.items, v)
}

func (s *Stack[T]) Pop() (T, bool) {
	if len(s.items) == 0 {
		var zero T
		return zero, false
	}
	lastIndex := len(s.items) - 1
	v := s.items[lastIndex]
	s.items = s.items[:lastIndex]
	return v, true
}

func (s *Stack[T]) peek() (T, bool) {
	if len(s.items) == 0 {
		var zero T
		return zero, false
	}
	return s.items[len(s.items)-1], true
}

func (s *Stack[T]) Len() int {
	return len(s.items)
}

func (s *Stack[T]) Current() T {
	if count := len(s.items); count == 0 {
		var zero T
		return zero
	} else {
		return s.items[count-1]
	}
}

func (s *Stack[T]) SetRoot(item T) {
	s.items = []T{item}
}

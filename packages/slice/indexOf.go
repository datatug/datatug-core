package slice

// IndexOfString returns index of a string in a slice
func IndexOfString(slice []string, v string) int {
	for i, a := range slice {
		if a == v {
			return i
		}
	}
	return -1
}

// IndexOfInt returns index of an int in a slice
func IndexOfInt(slice []int, v int) int {
	for i, a := range slice {
		if a == v {
			return i
		}
	}
	return -1
}

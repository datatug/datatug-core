//go:build !js || !wasm

package main

import "testing"

func TestNativeEntrypointIsNoop(t *testing.T) {
	main()
}

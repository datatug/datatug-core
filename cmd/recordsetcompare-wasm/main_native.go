//go:build !js || !wasm

package main

// The package exists in native module walks, while its executable is built
// only for the linked JavaScript/WebAssembly target.
func main() {}

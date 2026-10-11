//go:build js && wasm

package main

import (
	"syscall/js"

	"github.com/datatug/datatug-core/pkg/recordsetcompare/bridge"
)

func main() {
	compare := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) != 1 || args[0].Type() != js.TypeString {
			return bridge.HandleJSON(`{"abi":"recordsetcompare/1"}`)
		}
		return bridge.HandleJSON(args[0].String())
	})
	js.Global().Set("datatugRecordsetCompare", compare)
	js.Global().Call("datatugRecordsetCompareReady", bridge.ABI)
	select {}
}

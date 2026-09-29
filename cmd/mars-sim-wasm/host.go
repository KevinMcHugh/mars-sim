//go:build !js || !wasm

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "mars-sim-wasm is the browser build; build it with GOOS=js GOARCH=wasm (web/build-wasm.sh)")
	os.Exit(1)
}

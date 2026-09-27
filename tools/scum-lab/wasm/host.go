//go:build !js || !wasm

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "scum lab's browser module is built with GOOS=js GOARCH=wasm")
	os.Exit(1)
}

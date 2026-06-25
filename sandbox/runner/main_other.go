//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprintln(os.Stderr, "sandbox runner is linux-only (it runs inside the sandbox container)")
	os.Exit(1)
}

//go:build !linux

package main

import (
	"fmt"
	"os"
)

func main() { fmt.Fprintln(os.Stderr, "offline trial cache probe requires Linux"); os.Exit(1) }

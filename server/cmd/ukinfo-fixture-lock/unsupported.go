//go:build !unix

package main

import "os"

func main() { os.Exit(2) }

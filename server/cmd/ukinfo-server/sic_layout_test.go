package main

import (
	"os"
	"strings"
	"testing"
)

func TestSICLayoutFlagRequiresHostedParade(t *testing.T) {
	old := os.Args
	defer func() { os.Args = old }()
	for _, args := range [][]string{{"serve", "--experimental-sic-unpartitioned"}, {"verify", "--search-backend=paradedb", "--experimental-sic-unpartitioned"}, {"serve", "--search-backend=paradedb", "--desktop-control", "--experimental-sic-unpartitioned"}} {
		os.Args = append([]string{"ukinfo-server"}, args...)
		if err := run(); err == nil || !strings.Contains(err.Error(), "experimental-sic-unpartitioned requires hosted ParadeDB serve") {
			t.Fatalf("unsafe/unknown experimental flag: %v", err)
		}
	}
}

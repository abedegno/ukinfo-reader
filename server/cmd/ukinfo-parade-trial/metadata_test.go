package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/abedegno/ukinfo-reader/server/internal/catalog"
)

func TestCaseMetadataExcludesValues(t *testing.T) {
	directory := t.TempDir()
	input := filepath.Join(directory, "cases.json")
	output := filepath.Join(directory, "metadata.json")
	cases := []catalog.ParadeTrialCase{
		{Path: "/api/search", Query: "dataset=PRIVATE_EDITION&surname=PRIVATE_NAME&postcode=ZZ11AA&town=PRIVATE_TOWN&sort=name"},
		{Path: "/api/search", Query: "dataset=all&surname=PRIVATE_NAME&group=listing"},
		{Path: "/api/record", Query: "dataset=PRIVATE_EDITION&id=123456789&source_id=PRIVATE_SOURCE"},
		{Path: "/api/search", Query: "dataset=all&postcode=ZZ1"},
	}
	b, _ := json.Marshal(cases)
	if e := os.WriteFile(input, b, 0600); e != nil {
		t.Fatal(e)
	}
	if e := metadata(input, output); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(output)
	if e != nil {
		t.Fatal(e)
	}
	for _, value := range []string{"PRIVATE_EDITION", "PRIVATE_NAME", "PRIVATE_TOWN", "ZZ11AA", "123456789", "PRIVATE_SOURCE"} {
		if strings.Contains(string(b), value) {
			t.Fatal("private query value entered metadata report")
		}
	}
	var result struct {
		Cases, Editions int
		Metadata        []caseMetadata
	}
	if e = json.Unmarshal(b, &result); e != nil || result.Cases != 4 || result.Editions != 1 {
		t.Fatal("incorrect metadata coverage", e)
	}
	if result.Metadata[0].Backend != "paradedb" || result.Metadata[1].Backend != "native" || result.Metadata[2].Backend != "native" || result.Metadata[3].Backend != "native" {
		t.Fatal("incorrect serving selection metadata")
	}
	info, e := os.Stat(output)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("report permissions", e)
	}
	if e = metadata(input, output); e == nil {
		t.Fatal("existing evidence was overwritten")
	}
}

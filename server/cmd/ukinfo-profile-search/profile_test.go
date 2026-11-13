package main

import (
	"testing"
	"time"
)

func TestQueryWallTimeDoesNotDoubleCountConcurrentReads(t *testing.T) {
	start := time.Unix(100, 0)
	rows := []*query{
		{Started: start, Milliseconds: 100, Phase: "records"},
		{Started: start.Add(50 * time.Millisecond), Milliseconds: 100, Phase: "records"},
		{Started: start.Add(300 * time.Millisecond), Milliseconds: 10, Phase: "metadata"},
	}
	if got := queryWallMS(rows, "records"); got != 150 {
		t.Fatalf("concurrent reads counted twice: %v", got)
	}
}

func TestProfilePhaseOnlyAcceptsBoundedStageNames(t *testing.T) {
	for _, name := range []string{"search.records", "search.vocabulary", "search.address_blocks"} {
		if profilePhase(name) == "metadata" {
			t.Fatal("lost known stage", name)
		}
	}
	for _, name := range []string{"PRIVATE_QUERY_VALUE", "search.PRIVATE_EDITION", "http.request"} {
		if got := profilePhase(name); got != "metadata" {
			t.Fatal("private stage escaped", got)
		}
	}
}

func TestIODeltaRejectsResetCounters(t *testing.T) {
	before := ioStats{Available: true, Reads: 20, Hits: 100, ReadMS: 5}
	after := ioStats{Available: true, Reads: 23, Hits: 110, ReadMS: 7}
	if got := after.minus(before); !got.Available || got.Reads != 3 || got.Hits != 10 || got.ReadMS != 2 {
		t.Fatal("invalid isolated database IO delta", got)
	}
	after.Reads = 1
	if after.minus(before).Available {
		t.Fatal("reset counters reported as valid IO")
	}
}

func TestRejectUnboundedPrivateProfilesBeforeReadingInputs(t *testing.T) {
	for _, args := range [][]string{
		{"--repeats", "3"}, {"--page", "0"}, {"--page", "11"}, {"--backend", "PRIVATE"},
		{"--max-cases", "100", "--repeats", "2", "--page", "10"},
	} {
		if run(append(args, "--output", "/unreachable")) == nil {
			t.Fatal("unbounded profile accepted", args)
		}
	}
}

package main

import (
	"errors"
	"strings"
	"testing"
)

type fakeRows struct {
	rows     [][]any
	position int
	closed   bool
	err      error
}

func (f *fakeRows) Next() bool {
	if f.position >= len(f.rows) {
		return false
	}
	f.position++
	return true
}
func (f *fakeRows) Values() ([]any, error) { return f.rows[f.position-1], nil }
func (f *fakeRows) Err() error             { return f.err }
func (f *fakeRows) Close()                 { f.closed = true }
func TestRowDigestsPreserveOrderNullsAndBoundedConsumption(t *testing.T) {
	a := &fakeRows{rows: [][]any{{1, "PRIVATE", nil}, {2, "second", "null"}}}
	n, h, err := digestRows(a)
	if err != nil || n != 2 || !a.closed {
		t.Fatal("rows not consumed and closed")
	}
	b := &fakeRows{rows: [][]any{{2, "second", "null"}, {1, "PRIVATE", nil}}}
	_, other, err := digestRows(b)
	if err != nil || h == other {
		t.Fatal("row order lost")
	}
	for _, bad := range []*fakeRows{{rows: make([][]any, 130)}, {rows: [][]any{{strings.Repeat("PRIVATE", 10000)}}}, {err: errors.New("PRIVATE_ERROR")}} {
		if _, _, err := digestRows(bad); err == nil || !bad.closed {
			t.Fatal("unbounded or failed rows accepted")
		}
	}
}

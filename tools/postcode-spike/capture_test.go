package main

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

const testSQL = `SELECT id,name_id,first_id,postcode_id,name,first_full FROM ukinfo_search.d1_aaaaaaaaaaaaaaaaaaaaaaaa WHERE kind=$1 AND ((postcode >= $2 AND postcode < $3)) AND (postcode_length=$4 OR postcode=$5) AND postcode @@@ pdb.regex($6) AND id @@@ pdb.all() ORDER BY name,first_full,id LIMIT $7`

func TestKnownPredicateOnlyChangesRedundantClause(t *testing.T) {
	args := []any{int16(0), "AA1", "AA1\U0010ffff", 6, "AA1", "AA1(.{3})?", 51}
	scalar, err := scalarVariant(testSQL, args)
	if err != nil {
		t.Fatal(err)
	}
	expected := `SELECT id,name_id,first_id,postcode_id,name,first_full FROM ukinfo_search.d1_aaaaaaaaaaaaaaaaaaaaaaaa WHERE kind=$1 AND ((postcode >= $2 AND postcode < $3)) AND (postcode_length=$4 OR postcode=$5) AND ($6::text IS NOT NULL) AND id @@@ pdb.all() ORDER BY name,first_full,id LIMIT $7`
	if scalar != expected {
		t.Fatal("unrelated predicate, precision, ordering or parameter changed")
	}
	if !reflect.DeepEqual(args, []any{int16(0), "AA1", "AA1\U0010ffff", 6, "AA1", "AA1(.{3})?", 51}) {
		t.Fatal("arguments mutated")
	}
	for _, sql := range []string{testSQL + "; SELECT 1", testSQL + " AND postcode @@@ pdb.regex($6)", "SELECT 1", "DELETE FROM ukinfo_search.d1_aaaaaaaaaaaaaaaaaaaaaaaa"} {
		if _, err := scalarVariant(sql, args); err == nil {
			t.Fatal("unknown statement accepted")
		}
	}
	for _, arg := range []any{nil, "", []byte("PRIVATE")} {
		bad := append([]any(nil), args...)
		bad[5] = arg
		if _, err := scalarVariant(testSQL, bad); err == nil {
			t.Fatal("nonconstant nonnull regex accepted")
		}
	}
}

func TestCaptureCancelsOnlyEligibleTargetAndCopiesArguments(t *testing.T) {
	c := &capture{}
	parent := context.Background()
	metadata := c.TraceQueryStart(parent, nil, pgx.TraceQueryStartData{SQL: "SELECT version()"})
	if metadata.Err() != nil {
		t.Fatal("metadata cancelled")
	}
	args := []any{int16(0), "AA1", "AA1\U0010ffff", 6, "AA1", "AA1(.{3})?", 51}
	captured := c.TraceQueryStart(parent, nil, pgx.TraceQueryStartData{SQL: testSQL, Args: args})
	if captured.Err() != context.Canceled || parent.Err() != nil {
		t.Fatal("capture must cancel its query, not the parent request")
	}
	args[5] = "CHANGED"
	item, err := c.only()
	if err != nil || item.SQL != testSQL || item.Args[5] != "AA1(.{3})?" {
		t.Fatal("capture missing or mutable")
	}
	c.TraceQueryStart(parent, nil, pgx.TraceQueryStartData{SQL: testSQL, Args: args})
	if _, err := c.only(); err == nil {
		t.Fatal("multiple target captures accepted")
	}
}

func TestOnlyTwoUnrefinedFirstPagePostcodePersonCases(t *testing.T) {
	valid := []workloadCase{{Path: "/api/search", Query: "dataset=invented-one&kind=person&postcode=AA1&sort=name&limit=50"}, {Path: "/api/search", Query: "dataset=invented-two&postcode=BB2&sort=name"}}
	if err := validateCases(valid); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{"&surname=PRIVATE", "&first_name=PRIVATE", "&town=PRIVATE", "&group=listing", "&cursor=PRIVATE", "&postcode=BB2", "&unknown=PRIVATE", "&near=AA11AA"} {
		bad := append([]workloadCase(nil), valid...)
		bad[0].Query += extra
		if err := validateCases(bad); err == nil {
			t.Fatal("unsupported shape accepted")
		}
	}
	for _, query := range []string{"dataset=all&postcode=AA1&sort=name", "dataset=invented&postcode=AA1&kind=business&sort=name", "dataset=invented&postcode=AA1", "dataset=invented&sort=name"} {
		bad := append([]workloadCase(nil), valid...)
		bad[0].Query = query
		if err := validateCases(bad); err == nil {
			t.Fatal("unsupported scope accepted")
		}
	}
	if err := validateCases(valid[:1]); err == nil {
		t.Fatal("truncated cohort accepted")
	}
}

func TestRegexMustBeTheCanonicalRedundantPostcodePredicate(t *testing.T) {
	original := []any{int16(0), "AA1", "AA1\U0010ffff", 6, "AA1", "AA1(.{3})?", 51}
	for _, tc := range []struct {
		index int
		value any
	}{{0, int16(1)}, {1, "AB2"}, {2, "wrong_bound"}, {3, 7}, {4, "different"}, {5, "different_regex"}, {6, 10000}} {
		args := append([]any(nil), original...)
		args[tc.index] = tc.value
		if _, err := scalarVariant(testSQL, args); err == nil {
			t.Fatal("unverified semantic binding accepted")
		}
	}
}

func TestUnknownConjunctionsAndExtraClausesAreRejected(t *testing.T) {
	args := []any{int16(0), "AA1", "AA1\U0010ffff", 6, "AA1", "AA1(.{3})?", 51}
	for _, sql := range []string{strings.Replace(testSQL, "kind=$1 AND", "kind=$1 OR", 1), strings.Replace(testSQL, " ORDER BY", " AND true ORDER BY", 1)} {
		if _, err := scalarVariant(sql, args); err == nil {
			t.Fatal("unknown statement structure accepted")
		}
	}
}

func TestFailedCaptureExplainsOnlyClosedStructureAndStatus(t *testing.T) {
	c := &capture{}
	d := c.failure(503)
	if d.Status != 503 || d.Statements != 0 || d.KnownSQL || d.KnownArguments {
		t.Fatal("missing-query diagnostic changed")
	}
	args := []any{int16(0), "AA1", "AA1\U0010ffff", 6, "AA1", "AA1(.{3})?", 51}
	c.TraceQueryStart(context.Background(), nil, pgx.TraceQueryStartData{SQL: testSQL, Args: args})
	d = c.failure(503)
	if !d.KnownSQL || !d.KnownArguments || d.Statements != 1 {
		t.Fatal("canonical capture missing")
	}
	c.found[0].Args[5] = "SECRET"
	d = c.failure(503)
	if !d.KnownSQL || d.KnownArguments {
		t.Fatal("bad binding hidden")
	}
	if strings.Contains(d.Error(), "SECRET") || strings.Contains(d.Error(), "AA1") {
		t.Fatal("private capture escaped")
	}
}

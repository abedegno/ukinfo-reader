package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/jackc/pgx/v5"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParadePostcodeScalarEligibility(t *testing.T) {
	for _, s := range []string{"postcode=AA1&sort=name", "kind=business&postcode=AA1+1AA&sort=name", "dataset=all&postcode=AA1+1&sort=name"} {
		q, err := parse(mustValues("dataset=invented&" + s))
		if err != nil {
			t.Fatal(err)
		}
		if !paradePostcodeScalar(q) {
			t.Fatal("postcode-only ordered search rejected")
		}
	}
	base, err := parse(mustValues("dataset=invented&postcode=AA1&sort=name"))
	if err != nil {
		t.Fatal(err)
	}
	bound := base
	bound.Catalogue = "invented-catalogue-hash"
	if !paradePostcodeScalar(bound) {
		t.Fatal("cursor catalogue binding changed the selected SQL route")
	}
	changes := []func(*query){func(q *query) { q.Postcode = "" }, func(q *query) { q.Sort = "" }, func(q *query) { q.Sort = "edition" }, func(q *query) { q.Group = "listing" }, func(q *query) { q.Surname = "Invented" }, func(q *query) { q.First = "June" }, func(q *query) { q.Organisation = "Invented" }, func(q *query) { q.Middle = "J" }, func(q *query) { q.House = "1" }, func(q *query) { q.Building = "Invented" }, func(q *query) { q.Flat = "1" }, func(q *query) { q.Street = "Invented" }, func(q *query) { q.Locality = "Invented" }, func(q *query) { q.Town = "Invented" }, func(q *query) { q.County = "Invented" }, func(q *query) { q.Near = "AA11AA" }, func(q *query) { q.SIC = "1234" }, func(q *query) { q.Industry = "Invented" }}
	for i, f := range changes {
		q := base
		f(&q)
		if paradePostcodeScalar(q) {
			t.Fatalf("unqualified refinement %d used scalar route", i)
		}
	}
}

func TestParadePostcodeScalarPreservesPrecisionAndSeekBindings(t *testing.T) {
	q, err := parse(mustValues("dataset=invented&postcode=AA1&sort=name&limit=37"))
	if err != nil {
		t.Fatal(err)
	}
	d := paradeEdition{ID: 1, Key: "invented"}
	for _, after := range []paradePosition{{}, {Dataset: 1, ID: 7, Name: "Invented", First: "June"}, {Dataset: 2, ID: 7, Name: "Invented", First: "June"}} {
		sql, args := paradeSQL(q, d, after)
		if strings.Contains(sql, "@@@") || !strings.Contains(sql, "postcode >= $") || !strings.Contains(sql, " OR postcode=$") || !strings.Contains(sql, "ORDER BY name,first_full,id") || args[len(args)-1] != 37 {
			t.Fatal("scalar precision/order/limit changed")
		}
		legacy, legacyArgs := paradeSQLPrefix(q, d, after, true)
		if !strings.Contains(legacy, "pdb.regex") || !strings.Contains(legacy, "pdb.all()") {
			t.Fatal("legacy diagnostic/refined path changed")
		}
		if len(legacyArgs) != len(args)+1 {
			t.Fatal("bindings were dropped beyond redundant prefilter")
		}
		if after.ID > 0 && (!strings.Contains(sql, "name>$") || !strings.Contains(sql, "first_full>$")) {
			t.Fatal("seek ordering lost")
		}
	}
}

func TestPostcodeIndexPolicyDoesNotChangeSourceGeneration(t *testing.T) {
	d := paradeEdition{ID: 1, Key: "invented", Manifest: json.RawMessage(`{"edition":"invented-2008-03"}`), Entries: 14, Imported: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)}
	golden := `{"Version":1,"Edition":{"ID":1,"Key":"invented","Manifest":{"edition":"invented-2008-03"},"Entries":14,"Imported":"2000-01-01T00:00:00Z"}}`
	digest := sha256.Sum256([]byte(golden))
	if d.generation() != hex.EncodeToString(digest[:]) {
		t.Fatal("source generation schema changed")
	}
	if paradePostcodeIndexName(d) != "p8_pc_"+d.table()+"_cover" {
		t.Fatal("verified physical index compatibility lost")
	}
}

func TestPostcodeIndexContractUsesCanonicalCollationIdentity(t *testing.T) {
	if !strings.Contains(paradePostcodeIndexStateSQL, `'pg_catalog."C"'::regcollation`) || strings.Contains(paradePostcodeIndexStateSQL, "co.collname") {
		t.Fatal("foreign same-name collation can impersonate canonical C")
	}
}

// Assert executed handler candidate SQL, not an admission-time metric which can
// precede catalogue binding and therefore hide an incorrect execution route.
type postcodeCandidateTracer struct {
	enabled atomic.Bool
	native  atomic.Int64
	bm25    atomic.Int64
}

func (t *postcodeCandidateTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if t.enabled.Load() && strings.HasPrefix(d.SQL, "SELECT id,name_id,first_id,postcode_id,name,first_full FROM ukinfo_search.") {
		if strings.Contains(d.SQL, "@@@") {
			t.bm25.Add(1)
		} else {
			t.native.Add(1)
		}
	}
	return ctx
}
func (t *postcodeCandidateTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

package catalog

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Only the measured postcode-only ordered shape uses the covering path. The
// existing builder owns precision, seek order, limits and cross-edition merges.
func paradePostcodeScalar(q query) bool {
	return (q.Kind == "person" || q.Kind == "business") && q.Postcode != "" && q.Sort == "name" && q.Group == "" && q.Surname == "" && q.First == "" && q.Organisation == "" && q.Middle == "" && q.House == "" && q.Building == "" && q.Flat == "" && !q.hasAddress() && q.Near == "" && !q.hasBusinessFilters()
}

// Keep the verified prototype's physical name as the version-one compatibility
// name. This avoids duplicating or renaming the existing full-edition index.
func paradePostcodeIndexName(d paradeEdition) string { return "p8_pc_" + d.table() + "_cover" }

var paradePostcodeCoverColumns = []string{"kind", "postcode", "name", "first_full", "id", "name_id", "first_id", "postcode_id", "postcode_length"}

// Rebuild vector arrays with unnest so PostgreSQL's zero-based catalog array
// bounds do not prevent equality with the expected one-based arrays.
const paradePostcodeIndexStateSQL = `SELECT to_regclass($1) IS NOT NULL, EXISTS(
 SELECT FROM pg_class c JOIN pg_index i ON i.indexrelid=c.oid
 JOIN pg_class t ON t.oid=i.indrelid JOIN pg_am am ON am.oid=c.relam
 WHERE c.oid=to_regclass($1) AND t.oid=to_regclass($2)
 AND c.relkind='i' AND t.relkind='r' AND t.relam=(SELECT oid FROM pg_am WHERE amname='heap') AND c.relpersistence='p' AND t.relpersistence='p'
 AND c.relowner=t.relowner AND c.reltablespace=t.reltablespace AND am.amname='btree'
 AND i.indisvalid AND i.indisready AND i.indislive AND NOT i.indisunique
 AND NOT EXISTS(SELECT FROM unnest(i.indclass) k(opc) JOIN pg_opclass o ON o.oid=k.opc WHERE NOT o.opcdefault OR o.opcmethod<>c.relam)
 AND i.indpred IS NULL AND i.indexprs IS NULL AND i.indnkeyatts=5 AND i.indnatts=9
 AND ARRAY(SELECT a.attname::text FROM unnest(i.indkey) WITH ORDINALITY k(attnum,ord)
 JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attnum=k.attnum ORDER BY k.ord)=$3::text[]
 AND ARRAY(SELECT k.coll FROM unnest(i.indcollation) WITH ORDINALITY k(coll,ord) ORDER BY k.ord)
 =ARRAY[0::oid,'pg_catalog."C"'::regcollation::oid,'pg_catalog."C"'::regcollation::oid,'pg_catalog."C"'::regcollation::oid,0::oid]
 AND ARRAY(SELECT unnest(i.indoption))=ARRAY[0,0,0,0,0]::smallint[]
)`

type paradeIndexReader interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}
type paradeIndexBuilder interface {
	paradeIndexReader
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func paradePostcodeIndexState(ctx context.Context, db paradeIndexReader, d paradeEdition) (exists, valid bool, err error) {
	err = db.QueryRow(ctx, paradePostcodeIndexStateSQL, "ukinfo_search."+paradePostcodeIndexName(d), "ukinfo_search."+d.table(), paradePostcodeCoverColumns).Scan(&exists, &valid)
	return
}
func ensureParadePostcodeIndex(ctx context.Context, db paradeIndexBuilder, d paradeEdition, concurrent bool) error {
	exists, valid, err := paradePostcodeIndexState(ctx, db, d)
	if err != nil {
		return errors.New("postcode index metadata unavailable")
	}
	if exists {
		if valid {
			return nil
		}
		return errors.New("postcode index definition differs")
	}
	mode := ""
	if concurrent {
		mode = " CONCURRENTLY"
	}
	sql := "CREATE INDEX" + mode + " " + pgx.Identifier{paradePostcodeIndexName(d)}.Sanitize() + " ON " + pgx.Identifier{"ukinfo_search", d.table()}.Sanitize() + " (" + strings.Join(paradePostcodeCoverColumns[:5], ",") + ") INCLUDE (" + strings.Join(paradePostcodeCoverColumns[5:], ",") + ")"
	if _, err = db.Exec(ctx, sql); err != nil {
		return errors.New("postcode index creation incomplete")
	}
	exists, valid, err = paradePostcodeIndexState(ctx, db, d)
	if err != nil || !exists || !valid {
		return errors.New("postcode index validation failed")
	}
	return nil
}

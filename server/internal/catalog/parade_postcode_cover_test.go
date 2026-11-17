package catalog

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type postcodeIndexRow struct {
	exists, valid bool
	err           error
}

func (r postcodeIndexRow) Scan(out ...any) error {
	if r.err != nil {
		return r.err
	}
	*out[0].(*bool) = r.exists
	*out[1].(*bool) = r.valid
	return nil
}

type postcodeIndexDB struct {
	t                *testing.T
	states           []postcodeIndexRow
	queries, creates int
	ddl              string
	createErr        error
}

func (db *postcodeIndexDB) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	db.queries++
	if len(args) != 3 || args[0] == args[1] || !strings.Contains(sql, "to_regclass($1)") {
		db.t.Fatal("index/table identity not parameterized separately")
	}
	if len(db.states) == 0 {
		db.t.Fatal("unexpected metadata retry")
	}
	r := db.states[0]
	db.states = db.states[1:]
	return r
}
func (db *postcodeIndexDB) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	db.creates++
	db.ddl = sql
	return pgconn.NewCommandTag("CREATE INDEX"), db.createErr
}
func TestPostcodeIndexLifecycleReusesOnlyExactReadyObjects(t *testing.T) {
	d := paradeEdition{ID: 1, Key: "invented"}
	for _, concurrent := range []bool{false, true} {
		db := &postcodeIndexDB{t: t, states: []postcodeIndexRow{{}, {exists: true, valid: true}}}
		if err := ensureParadePostcodeIndex(context.Background(), db, d, concurrent); err != nil || db.creates != 1 || db.queries != 2 {
			t.Fatal("completed index was not validated")
		}
		if strings.Contains(db.ddl, "IF NOT EXISTS") || strings.Contains(db.ddl, " WHERE ") || strings.Contains(db.ddl, "CONCURRENTLY") != concurrent || !strings.Contains(db.ddl, "INCLUDE (name_id,first_id,postcode_id,postcode_length)") {
			t.Fatal("index recipe or concurrency changed")
		}
	}
	for _, valid := range []bool{false, true} {
		db := &postcodeIndexDB{t: t, states: []postcodeIndexRow{{exists: true, valid: valid}}}
		err := ensureParadePostcodeIndex(context.Background(), db, d, true)
		if (err == nil) != valid || db.creates != 0 {
			t.Fatal("existing object was silently replaced or invalid object reused")
		}
	}
}
func TestPostcodeIndexFailureNeverRetriesOrPublishesCompletion(t *testing.T) {
	for _, mode := range []string{"metadata", "create", "post_validation"} {
		db := &postcodeIndexDB{t: t, states: []postcodeIndexRow{{}, {exists: true, valid: false}}}
		if mode == "metadata" {
			db.states = []postcodeIndexRow{{err: errors.New("PRIVATE")}}
		}
		if mode == "create" {
			db.createErr = errors.New("PRIVATE")
		}
		err := ensureParadePostcodeIndex(context.Background(), db, paradeEdition{ID: 1}, true)
		if err == nil || strings.Contains(err.Error(), "PRIVATE") || db.creates > 1 {
			t.Fatal("failed index creation accepted or retried")
		}
	}
}

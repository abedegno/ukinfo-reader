package catalog

import (
	"context"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klauspost/compress/zstd"
)

//go:embed addresses.sql
var addressSchema string

// Decode only searchable display fields. Unknown source fields remain lossless
// in record_blocks and need no allocations during this offline indexing pass.
type addressSource struct {
	Street    string `json:"street"`
	Street1   string `json:"street_1"`
	Street2   string `json:"street_2"`
	Locality  string `json:"locality"`
	Locality1 string `json:"locality_1"`
	Locality2 string `json:"locality_2"`
	Town      string `json:"town"`
	County    string `json:"county"`
}

func (r addressSource) project() (addressLocation, error) {
	first := func(a, b string) string {
		if a != "" {
			return Normalize(a)
		}
		return Normalize(b)
	}
	location := addressLocation{first(r.Street, r.Street1), Normalize(r.Street2), first(r.Locality, r.Locality1), Normalize(r.Locality2), Normalize(r.Town), Normalize(r.County)}
	for _, v := range location {
		if len(v) > 256 || strings.ContainsRune(v, 0) {
			return location, errors.New("invalid or oversized displayed address component")
		}
	}
	return location, nil
}

type addressLocation [6]string

// Index only displayed, resolved fields. Raw references and suppressed precision
// fields must never restore information absent from the displayed source address.
func projectAddress(row map[string]any) (addressLocation, error) {
	return (addressSource{Street: Text(row, "street"), Street1: Text(row, "street_1"), Street2: Text(row, "street_2"), Locality: Text(row, "locality"), Locality1: Text(row, "locality_1"), Locality2: Text(row, "locality_2"), Town: Text(row, "town"), County: Text(row, "county")}).project()

}
func (l addressLocation) key() string { b, _ := json.Marshal(l); return string(b) }

type AddressIndexOptions struct {
	// SampleBlocks selects evenly spread whole blocks for a rollback-only pilot.
	// Zero builds and publishes the complete edition.
	SampleBlocks   int
	CheckResources func() error
	Progress       func(Progress)
}
type AddressIndexReport struct {
	Dataset         string           `json:"dataset"`
	Pilot           bool             `json:"pilot"`
	Records         int64            `json:"records"`
	Locations       int64            `json:"locations"`
	Nonempty        map[string]int64 `json:"nonempty_fields"`
	DictionaryBytes int64            `json:"dictionary_bytes"`
	MembershipBytes int64            `json:"membership_bytes"`
	Seconds         float64          `json:"seconds"`
}

// IndexAddresses builds from lossless catalogue blocks, without duplicating them.
// Sorted occurrence-ID arrays share each street/locality/town/county tuple. A
// bounded dictionary cache and TEMP staging heap keep Go memory independent of
// edition size. Commit publishes only a complete, count-checked index.
func IndexAddresses(ctx context.Context, db *pgxpool.Pool, key string, options AddressIndexOptions) (report AddressIndexReport, err error) {
	ctx, cancel := context.WithCancelCause(ctx)
	var watcherDone chan struct{}
	defer func() {
		cancel(nil)
		if watcherDone != nil {
			<-watcherDone
		}
		if err != nil && context.Cause(ctx) != nil && !errors.Is(context.Cause(ctx), context.Canceled) {
			err = context.Cause(ctx)
		}
	}()
	var resourceMu sync.Mutex
	if options.CheckResources != nil {
		original := options.CheckResources
		options.CheckResources = func() error { resourceMu.Lock(); defer resourceMu.Unlock(); return original() }
		watcherDone = make(chan struct{})
		go func() {
			defer close(watcherDone)
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if e := options.CheckResources(); e != nil {
						cancel(e)
						return
					}
				}
			}
		}()
	}
	started := time.Now()
	report = AddressIndexReport{Dataset: key, Pilot: options.SampleBlocks > 0, Nonempty: map[string]int64{}}
	if options.SampleBlocks < 0 {
		return report, errors.New("sample blocks must be nonnegative")
	}
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if options.CheckResources != nil {
			return options.CheckResources()
		}
		return nil
	}
	if err = check(); err != nil {
		return report, err
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return report, err
	}
	defer tx.Rollback(context.Background())
	// Ingestion/index creation is bounded by the caller's cancellable context,
	// not the serving pool's short search-statement deadline. SET LOCAL ensures
	// ordinary queries keep their deadline when this connection is returned.
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout=0"); err != nil {
		return report, err
	}
	var locked bool
	if err = tx.QueryRow(ctx, "SELECT pg_try_advisory_xact_lock(742918306,hashtext(current_schema()))").Scan(&locked); err != nil {
		return report, err
	}
	if !locked {
		return report, errors.New("another address index build is running")
	}
	// Bound external sorts; staging is on disk rather than in an edition-sized map.
	if _, err = tx.Exec(ctx, "SET LOCAL work_mem='32MB'"); err != nil {
		return report, err
	}
	var dataset int16
	var expected int64
	if err = tx.QueryRow(ctx, "SELECT id,entries FROM datasets WHERE key=$1", key).Scan(&dataset, &expected); err != nil {
		return report, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM address_indexes WHERE dataset_id=$1)", dataset).Scan(&exists); err != nil {
		return report, err
	}
	if exists && !report.Pilot {
		return report, errors.New("edition already has an address index")
	}
	locationsTable, membersTable := "address_locations", "address_members"
	if report.Pilot {
		// Separate names also prevent cached prepared statements from resolving a
		// permanent table when a temporary shadow is created on a pooled connection.
		// Shadow the permanent dictionary/members so a pilot consumes no permanent
		// sequence values and changes no published index, including existing editions.
		if _, err = tx.Exec(ctx, `CREATE TEMP TABLE address_pilot_locations (LIKE address_locations INCLUDING ALL) ON COMMIT DROP;
   CREATE TEMP TABLE address_pilot_members (LIKE address_members INCLUDING ALL) ON COMMIT DROP`); err != nil {
			return report, err
		}
		locationsTable, membersTable = "pg_temp.address_pilot_locations", "pg_temp.address_pilot_members"
	}
	if _, err = tx.Exec(ctx, "CREATE TEMP TABLE address_stage(location_id integer NOT NULL,id integer NOT NULL) ON COMMIT DROP"); err != nil {
		return report, err
	}
	decoder, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(16<<20))
	if err != nil {
		return report, err
	}
	defer decoder.Close()
	cache := newTermCache(100000)
	pending := make([]struct {
		id       int32
		location addressLocation
	}, 0, importBatchRows)
	flush := func() error {
		if err := check(); err != nil {
			return err
		}
		missing := map[string]addressLocation{}
		for _, p := range pending {
			k := p.location.key()
			if _, ok := cache.values[k]; !ok {
				missing[k] = p.location
			}
		}
		// Look up before INSERT to avoid spending identities for existing locations.
		keys := make([]string, 0, len(missing))
		for k := range missing {
			keys = append(keys, k)
		}
		resolved := map[string]int32{}
		for _, p := range pending {
			k := p.location.key()
			if id, ok := cache.values[k]; ok {
				resolved[k] = id
			}
		}
		consume := func(rows pgx.Rows, e error) error {
			if e != nil {
				return e
			}
			defer rows.Close()
			for rows.Next() {
				var k string
				var id int32
				if e = rows.Scan(&k, &id); e != nil {
					return e
				}
				resolved[k] = id
				cache.put(k, id)
			}
			return rows.Err()
		}
		if len(keys) > 0 {
			if e := consume(tx.Query(ctx, "SELECT key,id FROM "+locationsTable+" WHERE key=ANY($1::text[])", keys)); e != nil {
				return e
			}
			columns := make([][]string, 7)
			for k, l := range missing {
				if _, ok := resolved[k]; ok {
					continue
				}
				columns[0] = append(columns[0], k)
				for i, v := range l {
					columns[i+1] = append(columns[i+1], v)
				}
			}
			if len(columns[0]) > 0 {
				if e := consume(tx.Query(ctx, `INSERT INTO `+locationsTable+`(key,street,street_2,locality,locality_2,town,county)
    SELECT * FROM unnest($1::text[],$2::text[],$3::text[],$4::text[],$5::text[],$6::text[],$7::text[]) RETURNING key,id`, columns[0], columns[1], columns[2], columns[3], columns[4], columns[5], columns[6])); e != nil {
					return e
				}
			}
		}
		values := make([][]any, 0, len(pending))
		for _, p := range pending {
			k := p.location.key()
			id, ok := resolved[k]
			if !ok {
				id, ok = cache.values[k]
			}
			if !ok {
				return errors.New("unresolved address location")
			}
			values = append(values, []any{id, p.id})
		}
		if _, e := tx.CopyFrom(ctx, pgx.Identifier{"address_stage"}, []string{"location_id", "id"}, pgx.CopyFromRows(values)); e != nil {
			return e
		}
		pending = pending[:0]
		if options.Progress != nil {
			options.Progress(Progress{"indexing addresses", report.Records, expected})
		}
		return nil
	}
	blocks := (expected + blockRows - 1) / blockRows
	sample := int64(options.SampleBlocks)
	if sample == 0 || sample > blocks {
		sample = blocks
	}
	// Sequential windows avoid holding a source reader while writing through the
	// same transaction. Full builds fetch 256 compressed blocks at a time.
	next := int64(0)
	for next < sample {
		ids := make([]int32, 0, 256)
		for len(ids) < 256 && next < sample {
			ids = append(ids, int32(next*blocks/sample))
			next++
		}
		rows, e := tx.Query(ctx, "SELECT id,payload FROM record_blocks WHERE dataset_id=$1 AND id=ANY($2::integer[]) ORDER BY id", dataset, ids)
		if e != nil {
			return report, e
		}
		type block struct {
			id      int32
			payload []byte
		}
		batch := []block{}
		for rows.Next() {
			var b block
			if e = rows.Scan(&b.id, &b.payload); e != nil {
				break
			}
			batch = append(batch, b)
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return report, e
		}
		if len(batch) != len(ids) {
			return report, errors.New("missing source blocks during address indexing")
		}
		for _, b := range batch {
			if e = check(); e != nil {
				return report, e
			}
			raw, e := unpackBlock(decoder, b.payload)
			if e != nil {
				return report, e
			}
			n := int(binary.LittleEndian.Uint16(raw[5:7]))
			pos := 7
			for i := 0; i < n; i++ {
				sourceLen := int(binary.LittleEndian.Uint16(raw[pos:]))
				recordLen := int(binary.LittleEndian.Uint32(raw[pos+2:]))
				pos += 6
				var source addressSource
				if e = json.Unmarshal(raw[pos+sourceLen:pos+sourceLen+recordLen], &source); e != nil {
					return report, e
				}
				pos += sourceLen + recordLen
				location, e := source.project()
				if e != nil {
					return report, e
				}
				for j, name := range []string{"street", "street_2", "locality", "locality_2", "town", "county"} {
					if location[j] != "" {
						report.Nonempty[name]++
					}
				}
				report.Records++
				pending = append(pending, struct {
					id       int32
					location addressLocation
				}{b.id*blockRows + int32(i) + 1, location})
				if len(pending) == cap(pending) {
					if e = flush(); e != nil {
						return report, e
					}
				}
			}
		}
	}
	if len(pending) > 0 {
		if err = flush(); err != nil {
			return report, err
		}
	}
	if !report.Pilot && report.Records != expected {
		return report, errors.New("address index source count mismatch")
	}
	if err = check(); err != nil {
		return report, err
	}
	if options.Progress != nil {
		options.Progress(Progress{"grouping address postings", report.Records, expected})
	}
	if _, err = tx.Exec(ctx, `INSERT INTO `+membersTable+`(dataset_id,location_id,ids) SELECT $1,location_id,array_agg(id ORDER BY id) FROM address_stage GROUP BY location_id`, dataset); err != nil {
		return report, err
	}
	var counted int64
	if err = tx.QueryRow(ctx, "SELECT count(*),COALESCE(sum(cardinality(ids)),0)::bigint FROM "+membersTable+" WHERE dataset_id=$1", dataset).Scan(&report.Locations, &counted); err != nil {
		return report, err
	}
	if counted != report.Records {
		return report, errors.New("address membership count mismatch")
	}
	if _, err = tx.Exec(ctx, "ANALYZE "+locationsTable+"; ANALYZE "+membersTable); err != nil {
		return report, err
	}
	if err = tx.QueryRow(ctx, "SELECT pg_total_relation_size($1::regclass),pg_total_relation_size($2::regclass)", locationsTable, membersTable).Scan(&report.DictionaryBytes, &report.MembershipBytes); err != nil {
		return report, err
	}
	if err = check(); err != nil {
		return report, err
	}
	report.Seconds = time.Since(started).Seconds()
	if report.Pilot {
		return report, nil
	} // deferred rollback removes all pilot tables
	if _, err = tx.Exec(ctx, "INSERT INTO address_indexes(dataset_id,version,entries,locations) VALUES($1,1,$2,$3)", dataset, report.Records, report.Locations); err != nil {
		return report, err
	}
	if _, err = tx.Exec(ctx, "SELECT build_address_record_lookup($1)", key); err != nil {
		return report, err
	}
	err = tx.Commit(ctx)
	return report, err
}

// The serving role normally has a two-second SQL deadline. Broad cross-edition
// address queries get four seconds inside their own read-only transaction; normal
// name/postcode queries retain their existing budget. Caller closes rows first.
func (a *API) addressRows(ctx context.Context, sql string, args []any) (pgx.Rows, func(), error) {
	tx, err := a.DB.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, func() {}, err
	}
	finish := func() { _ = tx.Rollback(context.Background()) }
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout='4000'"); err != nil {
		finish()
		return nil, func() {}, err
	}
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		finish()
		return nil, func() {}, err
	}
	return rows, finish, nil
}

func (a *API) addresses(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	rows, err := a.DB.Query(ctx, `SELECT d.key,i.entries,i.locations FROM address_indexes i JOIN datasets d ON d.id=i.dataset_id ORDER BY d.key`)
	if err != nil {
		respond(w, 200, map[string]any{"available": false, "datasets": []any{}})
		return
	}
	defer rows.Close()
	type coverage struct {
		Dataset   string `json:"dataset"`
		Entries   int64  `json:"entries"`
		Locations int64  `json:"locations"`
	}
	items := []coverage{}
	for rows.Next() {
		var c coverage
		if err = rows.Scan(&c.Dataset, &c.Entries, &c.Locations); err != nil {
			fail(w, 503, "Address coverage unavailable")
			return
		}
		items = append(items, c)
	}
	if rows.Err() != nil {
		fail(w, 503, "Address coverage unavailable")
		return
	}
	respond(w, 200, map[string]any{"available": len(items) > 0, "datasets": items})
}

func (q query) hasAddress() bool {
	return q.Street != "" || q.Locality != "" || q.Town != "" || q.County != ""
}

// Address predicates use parameterized C-collation ranges, never SQL wildcards.
func addressPredicate(q query, args *[]any) string {
	conditions := []string{}
	for _, filter := range []struct {
		value   string
		columns []string
	}{{q.Street, []string{"street", "street_2"}}, {q.Locality, []string{"locality", "locality_2"}}, {q.Town, []string{"town"}}, {q.County, []string{"county"}}} {
		if filter.value == "" {
			continue
		}
		key := strings.TrimSuffix(filter.value, "*")
		*args = append(*args, key)
		low := len(*args)
		parts := []string{}
		prefix := strings.HasSuffix(filter.value, "*")
		high := 0
		if prefix {
			*args = append(*args, key+"\U0010ffff")
			high = len(*args)
		}
		for _, col := range filter.columns {
			if prefix {
				parts = append(parts, fmt.Sprintf("(l.%s >= $%d AND l.%s < $%d)", col, low, col, high))
			} else {
				parts = append(parts, fmt.Sprintf("l.%s=$%d", col, low))
			}
		}
		conditions = append(conditions, "("+strings.Join(parts, " OR ")+")")
	}
	return strings.Join(conditions, " AND ")
}

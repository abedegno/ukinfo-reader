package catalog

import (
	"context"
	"encoding/binary"
	"errors"
	"math"
	"slices"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klauspost/compress/zstd"
)

// ParadeBuildOptions configures a derived build. Supply a SELECT-only source.
// ReaderRole must already exist in the target; no login secrets are managed.
type ParadeBuildOptions struct {
	ReaderRole     string
	CheckResources func() error
	Progress       func(string, int64, int64)
}

// BuildParadeIndex publishes one complete generation at a time. Committed COPY
// checkpoints resume after interruption; serving requires the ready marker and
// exact source generation. Failed/stale editions cannot cause partial all-search.
func BuildParadeIndex(ctx context.Context, src, dst *pgxpool.Pool, scope string, options ParadeBuildOptions) (int64, error) {
	if scope == "" {
		return 0, errors.New("choose an edition or all")
	}
	a, b := src.Config().ConnConfig, dst.Config().ConnConfig
	if a.Host == b.Host && a.Port == b.Port && a.Database == b.Database {
		return 0, errors.New("ParadeDB target must be a separate database")
	}
	var sourceIdentity, targetIdentity string
	identity := `SELECT current_database()||':'||COALESCE(inet_server_addr()::text,'local')||':'||COALESCE(inet_server_port(),0)::text`
	if err := src.QueryRow(ctx, identity).Scan(&sourceIdentity); err != nil {
		return 0, errors.New("cannot verify source isolation")
	}
	if err := dst.QueryRow(ctx, identity).Scan(&targetIdentity); err != nil {
		return 0, errors.New("cannot verify target isolation")
	}
	if sourceIdentity == targetIdentity {
		return 0, errors.New("ParadeDB target resolves to source database")
	}
	source, err := src.Acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer source.Release()
	var locked bool
	if err = source.QueryRow(ctx, "SELECT pg_try_advisory_lock_shared(742918304)").Scan(&locked); err != nil || !locked {
		return 0, errors.New("source maintenance is running")
	}
	defer source.Exec(context.Background(), "SELECT pg_advisory_unlock_shared(742918304)")
	target, err := dst.Acquire(ctx)
	if err != nil {
		return 0, err
	}
	defer target.Release()
	if err = target.QueryRow(ctx, "SELECT pg_try_advisory_lock(20261026)").Scan(&locked); err != nil || !locked {
		return 0, errors.New("another ParadeDB build is running")
	}
	defer target.Exec(context.Background(), "SELECT pg_advisory_unlock(20261026)")
	if options.CheckResources != nil {
		if err = options.CheckResources(); err != nil {
			return 0, err
		}
	}
	_, err = target.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS pg_search CASCADE;
 CREATE SCHEMA IF NOT EXISTS ukinfo_search;
 REVOKE ALL ON SCHEMA ukinfo_search FROM PUBLIC;
 CREATE TABLE IF NOT EXISTS ukinfo_search.editions(dataset_id smallint NOT NULL,generation text NOT NULL,entries bigint NOT NULL,version integer NOT NULL,last_id bigint NOT NULL DEFAULT 0,ready boolean NOT NULL DEFAULT false,PRIMARY KEY(dataset_id,generation));
 REVOKE ALL ON ALL TABLES IN SCHEMA ukinfo_search FROM PUBLIC;`)
	if err != nil {
		return 0, errors.New("cannot initialize derived search schema")
	}
	var version string
	if err = target.QueryRow(ctx, "SELECT extversion FROM pg_extension WHERE extname='pg_search'").Scan(&version); err != nil || version != "0.26.3" {
		return 0, errors.New("this preview requires pg_search 0.26.3")
	}
	if options.ReaderRole != "" {
		// Identifiers are quoted; this grants no CREATE, TEMP, owner or write powers.
		role := pgx.Identifier{options.ReaderRole}.Sanitize()
		if _, err = target.Exec(ctx, "GRANT USAGE ON SCHEMA ukinfo_search TO "+role+"; GRANT SELECT ON ukinfo_search.editions TO "+role); err != nil {
			return 0, errors.New("cannot grant derived reader access")
		}
	}
	editions, err := searchEditions(ctx, src, scope)
	if err != nil {
		return 0, errors.New("source edition inventory unavailable")
	}
	if len(editions) == 0 {
		return 0, errors.New("edition not imported")
	}
	var total int64
	for _, d := range editions {
		if d.ID < 1 || d.Entries < 1 || d.Entries > math.MaxInt32 {
			return total, errors.New("unsupported source edition size")
		}
		if err = buildParadeEdition(ctx, source.Conn(), target.Conn(), d, options); err != nil {
			return total, err
		}
		total += d.Entries
	}
	return total, nil
}
func buildParadeEdition(ctx context.Context, source, target *pgx.Conn, d paradeEdition, options ParadeBuildOptions) error {
	table := pgx.Identifier{"ukinfo_search", d.table()}.Sanitize()
	var last int64
	var ready bool
	err := target.QueryRow(ctx, "SELECT last_id,ready FROM ukinfo_search.editions WHERE dataset_id=$1 AND generation=$2 AND version=$3", d.ID, d.generation(), paradeVersion).Scan(&last, &ready)
	if errors.Is(err, pgx.ErrNoRows) {
		tx, e := target.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(context.Background())
		_, e = tx.Exec(ctx, "CREATE TABLE "+table+`(id integer PRIMARY KEY,kind smallint NOT NULL,name text COLLATE "C" NOT NULL,first_full text COLLATE "C" NOT NULL,given text COLLATE "C" NOT NULL,postcode text COLLATE "C" NOT NULL,street text COLLATE "C" NOT NULL,street_2 text COLLATE "C" NOT NULL,locality text COLLATE "C" NOT NULL,locality_2 text COLLATE "C" NOT NULL,town text COLLATE "C" NOT NULL,county text COLLATE "C" NOT NULL,sic text COLLATE "C" NOT NULL,industry text COLLATE "C" NOT NULL,name_id integer NOT NULL,first_id integer NOT NULL,postcode_id integer NOT NULL,postcode_length smallint GENERATED ALWAYS AS(length(postcode)) STORED) WITH(autovacuum_enabled=false)`)
		if e != nil {
			return errors.New("cannot create derived edition")
		}
		if _, e = tx.Exec(ctx, "INSERT INTO ukinfo_search.editions(dataset_id,generation,entries,version) VALUES($1,$2,$3,$4)", d.ID, d.generation(), d.Entries, paradeVersion); e != nil {
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
	} else if err != nil {
		return errors.New("derived checkpoint unavailable")
	}
	if ready {
		if options.ReaderRole != "" {
			_, err = target.Exec(ctx, "GRANT SELECT ON "+table+" TO "+pgx.Identifier{options.ReaderRole}.Sanitize())
		}
		return err
	}
	if last < 0 || last > d.Entries || (last%32768 != 0 && last != d.Entries) {
		return errors.New("invalid derived checkpoint")
	}
	dec, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(16<<20))
	if err != nil {
		return err
	}
	defer dec.Close()
	for first := last + 1; first <= d.Entries; first += 32768 {
		if options.CheckResources != nil {
			if err = options.CheckResources(); err != nil {
				return err
			}
		}
		end := min(d.Entries, first+32767)
		rows, e := source.Query(ctx, "SELECT id,payload FROM record_blocks WHERE dataset_id=$1 AND id BETWEEN $2 AND $3 ORDER BY id", d.ID, int32((first-1)/128), int32((end-1)/128))
		if e != nil {
			return errors.New("source blocks unavailable")
		}
		vals := make([][]any, 0, end-first+1)
		next := first
		for rows.Next() {
			var block int32
			var payload []byte
			if e = rows.Scan(&block, &payload); e != nil {
				break
			}
			if int64(block) != (next-1)/128 {
				e = recordIntegrity("source block sequence gap")
				break
			}
			raw, failure := unpackBlock(dec, payload)
			if failure != nil {
				e = failure
				break
			}
			count := int64(binary.LittleEndian.Uint16(raw[5:7]))
			if count != min(int64(128), d.Entries-int64(block)*128) {
				e = recordIntegrity("source block count mismatch")
				break
			}
			for j := 0; j < int(count); j++ {
				r, failure := recordAt(raw, j)
				if failure != nil {
					e = failure
					break
				}
				v, failure := flattenSearchRecord(int32(next), r.Record)
				if failure != nil {
					e = failure
					break
				}
				vals = append(vals, v)
				next++
			}
			if e != nil {
				break
			}
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return e
		}
		if next != end+1 {
			return recordIntegrity("incomplete source batch")
		}
		// Preserve the compact backend's dictionary-ID ordering, rather than silently
		// substituting lexical or source-ID order when the operator flips the flag.
		rows, e = source.Query(ctx, `SELECT e.id,e.kind,e.name_id,e.first_id,e.postcode_id,n.key,f.key,p.key FROM entries e JOIN terms n ON n.id=e.name_id JOIN terms f ON f.id=e.first_id JOIN terms p ON p.id=e.postcode_id WHERE dataset_id=$1 AND e.id BETWEEN $2 AND $3 ORDER BY e.id`, d.ID, first, end)
		if e != nil {
			return errors.New("source ordering projection unavailable")
		}
		seen := 0
		for rows.Next() {
			var id, n, f, p int32
			var kind int16
			var name, full, postcode string
			if e = rows.Scan(&id, &kind, &n, &f, &p, &name, &full, &postcode); e != nil {
				break
			}
			if seen >= len(vals) || int64(id) != first+int64(seen) || vals[seen][1] != kind || vals[seen][2] != name || vals[seen][3] != full || vals[seen][5] != postcode {
				e = recordIntegrity("source search fields differ from original records")
				break
			}
			vals[seen] = append(vals[seen], n, f, p)
			seen++
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return e
		}
		if seen != len(vals) {
			return recordIntegrity("missing source ordering row")
		}
		tx, e := target.Begin(ctx)
		if e != nil {
			return e
		}
		columns := append(slices.Clone(searchDocumentColumns), "name_id", "first_id", "postcode_id")
		copied, e := tx.CopyFrom(ctx, pgx.Identifier{"ukinfo_search", d.table()}, columns, pgx.CopyFromRows(vals))
		if e == nil && copied != int64(len(vals)) {
			e = recordIntegrity("derived copy count mismatch")
		}
		if e == nil {
			_, e = tx.Exec(ctx, "UPDATE ukinfo_search.editions SET last_id=$1 WHERE dataset_id=$2 AND generation=$3", end, d.ID, d.generation())
		}
		if e != nil {
			tx.Rollback(context.Background())
			return errors.New("derived batch failed")
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
		if options.Progress != nil {
			options.Progress(d.Key, end, d.Entries)
		}
	}
	if options.CheckResources != nil {
		if err = options.CheckResources(); err != nil {
			return err
		}
	}
	index := pgx.Identifier{d.table() + "_search"}.Sanitize()
	if _, err = target.Exec(ctx, "CREATE INDEX IF NOT EXISTS "+index+" ON "+table+` USING paradedb(id,kind,name_id,first_id,postcode_id,(name::pdb.literal),(first_full::pdb.literal),(given::pdb.literal),(postcode::pdb.literal),(street::pdb.literal),(street_2::pdb.literal),(locality::pdb.literal),(locality_2::pdb.literal),(town::pdb.literal),(county::pdb.literal),(sic::pdb.literal),(industry::pdb.literal),postcode_length)`); err != nil {
		return errors.New("derived search index failed")
	}
	var valid bool
	if err = target.QueryRow(ctx, "SELECT indisvalid AND indisready AND indrelid=$1::regclass FROM pg_index WHERE indexrelid=$2::regclass", table, "ukinfo_search."+d.table()+"_search").Scan(&valid); err != nil || !valid {
		return errors.New("derived index is not valid")
	}
	if _, err = target.Exec(ctx, "VACUUM(ANALYZE) "+table); err != nil {
		return errors.New("derived vacuum failed")
	}
	var count, lo, hi int64
	if err = target.QueryRow(ctx, "SELECT count(*),min(id),max(id) FROM "+table).Scan(&count, &lo, &hi); err != nil || count != d.Entries || lo != 1 || hi != d.Entries {
		return recordIntegrity("derived coverage mismatch")
	}
	var current paradeEdition
	if err = source.QueryRow(ctx, "SELECT id,key,manifest,entries,imported_at FROM datasets WHERE id=$1", d.ID).Scan(&current.ID, &current.Key, &current.Manifest, &current.Entries, &current.Imported); err != nil || current.generation() != d.generation() {
		return errors.New("source generation changed during build")
	}
	tx, err := target.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "ALTER TABLE "+table+" SET(autovacuum_enabled=true)"); err != nil {
		return err
	}
	if options.ReaderRole != "" {
		if _, err = tx.Exec(ctx, "GRANT SELECT ON "+table+" TO "+pgx.Identifier{options.ReaderRole}.Sanitize()); err != nil {
			return errors.New("cannot grant edition reader access")
		}
	}
	if _, err = tx.Exec(ctx, "UPDATE ukinfo_search.editions SET ready=true WHERE dataset_id=$1 AND generation=$2 AND last_id=entries", d.ID, d.generation()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

package catalog

// Private operator experiment. No serving route, migration or source write.
import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/klauspost/compress/zstd"
)

var spikeColumns = searchDocumentColumns

func spikeFlat(id int32, raw []byte) ([]any, error) { return flattenSearchRecord(id, raw) }

// BuildSearchEngineSpike reads original blocks sequentially with a SELECT-only
// source connection. Batches commit only into the explicitly separate target DB.
// Its published completion marker requires all records, sample-field comparisons
// against independently stored source indexes, and both candidate indexes.
func BuildSearchEngineSpike(ctx context.Context, src, dst *pgxpool.Pool, key string, check func() error, progress func(string, int64, int64)) error {
	if key != "se2003-dpcs0104" {
		return errors.New("this experiment is restricted to the complete 2003 edition")
	}
	// Refuse a misconfigured target before issuing any target DDL. Separate
	// database names on the same server are insufficient for this experiment.
	a, b := src.Config().ConnConfig, dst.Config().ConnConfig
	if a.Host == b.Host && a.Port == b.Port {
		return errors.New("source and target must use separate PostgreSQL servers")
	}
	var sourceAddress, targetAddress string
	var sourcePort, targetPort int
	if err := src.QueryRow(ctx, "SELECT COALESCE(inet_server_addr()::text,'local'),COALESCE(inet_server_port(),0)").Scan(&sourceAddress, &sourcePort); err != nil {
		return errors.New("cannot verify source isolation")
	}
	if err := dst.QueryRow(ctx, "SELECT COALESCE(inet_server_addr()::text,'local'),COALESCE(inet_server_port(),0)").Scan(&targetAddress, &targetPort); err != nil {
		return errors.New("cannot verify target isolation")
	}
	if sourceAddress == targetAddress && sourcePort == targetPort {
		return errors.New("source and target resolve to the same PostgreSQL server")
	}
	source, err := src.Acquire(ctx)
	if err != nil {
		return err
	}
	defer source.Release()
	var locked bool
	if err = source.QueryRow(ctx, "SELECT pg_try_advisory_lock_shared(742918304)").Scan(&locked); err != nil || !locked {
		return errors.New("source import/index maintenance is running")
	}
	defer source.Exec(context.Background(), "SELECT pg_advisory_unlock_shared(742918304)")
	var edition int16
	var expected int64
	var manifest []byte
	if err = source.QueryRow(ctx, "SELECT id,entries,manifest FROM datasets WHERE key=$1", key).Scan(&edition, &expected, &manifest); err != nil {
		return err
	}
	if expected != 59886841 {
		return errors.New("unexpected complete 2003 entry count")
	}
	sum := sha256.Sum256(manifest)
	fingerprint := hex.EncodeToString(sum[:])
	conn, err := dst.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	var guard bool
	if err = conn.QueryRow(ctx, "SELECT pg_try_advisory_lock(20261025)").Scan(&guard); err != nil || !guard {
		return errors.New("target build already running")
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock(20261025)")
	_, err = conn.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS pg_search CASCADE;
 CREATE TABLE IF NOT EXISTS spike_build(dataset_key text PRIMARY KEY,source_fingerprint text NOT NULL,expected bigint NOT NULL,last_id bigint NOT NULL DEFAULT 0,sampled bigint NOT NULL DEFAULT 0,started_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz);
 CREATE TABLE IF NOT EXISTS spike_build_steps(phase text PRIMARY KEY,seconds double precision NOT NULL,finished_at timestamptz NOT NULL DEFAULT now());`)
	if err != nil {
		return err
	}
	var last int64
	var old string
	var want int64
	err = conn.QueryRow(ctx, "SELECT source_fingerprint,expected,last_id FROM spike_build WHERE dataset_key=$1", key).Scan(&old, &want, &last)
	if errors.Is(err, pgx.ErrNoRows) {
		tx, e := conn.Begin(ctx)
		if e != nil {
			return e
		}
		defer tx.Rollback(context.Background())
		_, e = tx.Exec(ctx, `CREATE TABLE native(id integer NOT NULL,kind smallint NOT NULL,name text COLLATE "C" NOT NULL,first_full text COLLATE "C" NOT NULL,given text COLLATE "C" NOT NULL,postcode text COLLATE "C" NOT NULL,street text COLLATE "C" NOT NULL,street_2 text COLLATE "C" NOT NULL,locality text COLLATE "C" NOT NULL,locality_2 text COLLATE "C" NOT NULL,town text COLLATE "C" NOT NULL,county text COLLATE "C" NOT NULL,sic text COLLATE "C" NOT NULL,industry text COLLATE "C" NOT NULL) WITH(autovacuum_enabled=false)`)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, "INSERT INTO spike_build(dataset_key,source_fingerprint,expected) VALUES($1,$2,$3)", key, fingerprint, expected)
		if e != nil {
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
	} else if err != nil {
		return err
	} else if old != fingerprint || want != expected {
		return errors.New("source generation changed; refuse resume")
	}
	// Source postcode precision is an indexed field, rather than a runtime
	// length() post-filter that could prevent search-engine filter pushdown.
	if _, err = conn.Exec(ctx, "ALTER TABLE native ADD COLUMN IF NOT EXISTS postcode_length smallint GENERATED ALWAYS AS (length(postcode)) STORED"); err != nil {
		return err
	}
	if last%32768 != 0 && last != expected {
		return errors.New("invalid target checkpoint")
	}
	dec, err := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(16<<20))
	if err != nil {
		return err
	}
	defer dec.Close()
	for first := last + 1; first <= expected; first += 32768 {
		if check != nil {
			if err = check(); err != nil {
				return err
			}
		}
		end := min(expected, first+32767)
		rows, e := source.Query(ctx, "SELECT id,payload FROM record_blocks WHERE dataset_id=$1 AND id BETWEEN $2 AND $3 ORDER BY id", edition, int32((first-1)/128), int32((end-1)/128))
		if e != nil {
			return errors.New("source block batch unavailable")
		}
		vals := make([][]any, 0, end-first+1)
		sample := map[int32][]any{}
		next := first
		for rows.Next() {
			var block int32
			var payload []byte
			if e = rows.Scan(&block, &payload); e != nil {
				break
			}
			if int64(block) != (next-1)/128 {
				e = errors.New("source block sequence gap")
				break
			}
			raw, err := unpackBlock(dec, payload)
			if err != nil {
				e = err
				break
			}
			count := int64(binary.LittleEndian.Uint16(raw[5:7]))
			if count != min(int64(128), expected-int64(block)*128) {
				e = errors.New("source block count mismatch")
				break
			}
			for j := 0; j < int(count); j++ {
				r, err := recordAt(raw, j)
				if err != nil {
					e = err
					break
				}
				v, err := spikeFlat(int32(next), r.Record)
				if err != nil {
					e = err
					break
				}
				vals = append(vals, v)
				if (next-1)%1024 == 0 {
					sample[int32(next)] = v
				}
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
			return errors.New("incomplete source batch")
		}
		if e = spikeCheckSamples(ctx, source.Conn(), edition, sample); e != nil {
			return e
		}
		tx, e := conn.Begin(ctx)
		if e != nil {
			return e
		}
		n, e := tx.CopyFrom(ctx, pgx.Identifier{"native"}, spikeColumns, pgx.CopyFromRows(vals))
		if e != nil {
			tx.Rollback(context.Background())
			return errors.New("target copy failed")
		}
		if n != end-first+1 {
			tx.Rollback(context.Background())
			return errors.New("target copy count mismatch")
		}
		_, e = tx.Exec(ctx, "UPDATE spike_build SET last_id=$1,sampled=sampled+$2 WHERE dataset_key=$3", end, len(sample), key)
		if e != nil {
			tx.Rollback(context.Background())
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
		if progress != nil {
			progress("flattening originals", end, expected)
		}
	}
	// Idempotent phases publish their own markers only after successful completion.
	phases := []struct{ name, sql string }{
		{"native_pk", `ALTER TABLE native ADD PRIMARY KEY(id)`},
		{"native_name_town", `CREATE INDEX native_name_town ON native(kind,name,given,town,id)`},
		{"native_lexical", `CREATE INDEX native_lexical ON native(kind,name,first_full,id)`},
		{"native_name_county", `CREATE INDEX native_name_county ON native(kind,name,given,county,id)`},
		{"native_name", `CREATE INDEX native_name ON native(kind,name,given,id) INCLUDE(first_full,postcode,town,county)`},
		{"native_postcode", `CREATE INDEX native_postcode ON native(kind,postcode,name,given,id)`},
		{"native_town", `CREATE INDEX native_town ON native(kind,town,id)`},
		{"native_county", `CREATE INDEX native_county ON native(kind,county,id)`},
		{"native_sic", `CREATE INDEX native_sic ON native(kind,sic,id) WHERE sic<>''`},
		{"native_industry", `CREATE INDEX native_industry ON native(kind,industry,id) WHERE industry<>''`},
		{"native_street", `CREATE INDEX native_street ON native(kind,street,id) WHERE street<>''; CREATE INDEX native_street2 ON native(kind,street_2,id) WHERE street_2<>''`},
		{"native_locality", `CREATE INDEX native_locality ON native(kind,locality,id) WHERE locality<>''; CREATE INDEX native_locality2 ON native(kind,locality_2,id) WHERE locality_2<>''`},
		{"parade_copy", `CREATE TABLE parade (LIKE native INCLUDING DEFAULTS INCLUDING CONSTRAINTS) WITH(autovacuum_enabled=false); INSERT INTO parade SELECT * FROM native; ALTER TABLE parade ADD PRIMARY KEY(id)`},
		{"parade_search", `CREATE INDEX parade_search ON parade USING paradedb(id,kind,(name::pdb.literal),(first_full::pdb.literal),(given::pdb.literal),(postcode::pdb.literal),(street::pdb.literal),(street_2::pdb.literal),(locality::pdb.literal),(locality_2::pdb.literal),(town::pdb.literal),(county::pdb.literal),(sic::pdb.literal),(industry::pdb.literal),postcode_length)`},
	}
	for _, p := range phases {
		var done bool
		if err = conn.QueryRow(ctx, "SELECT EXISTS(SELECT FROM spike_build_steps WHERE phase=$1)", p.name).Scan(&done); err != nil {
			return err
		}
		if done {
			continue
		}
		if check != nil {
			if err = check(); err != nil {
				return err
			}
		}
		if progress != nil {
			progress(p.name, 0, expected)
		}
		start := time.Now()
		tx, e := conn.Begin(ctx)
		if e != nil {
			return e
		}
		_, e = tx.Exec(ctx, p.sql)
		if e != nil {
			tx.Rollback(context.Background())
			return fmt.Errorf("phase %s failed: %w", p.name, e)
		}
		_, e = tx.Exec(ctx, "INSERT INTO spike_build_steps(phase,seconds) VALUES($1,$2)", p.name, time.Since(start).Seconds())
		if e != nil {
			tx.Rollback(context.Background())
			return e
		}
		if e = tx.Commit(ctx); e != nil {
			return e
		}
	}
	for _, table := range []string{"native", "parade"} {
		if _, err = conn.Exec(ctx, "VACUUM ANALYZE "+table); err != nil {
			return err
		}
		var count, lo, hi int64
		if err = conn.QueryRow(ctx, "SELECT count(*),min(id),max(id) FROM "+table).Scan(&count, &lo, &hi); err != nil {
			return err
		}
		if count != expected || lo != 1 || hi != expected {
			return errors.New("final source coverage mismatch")
		}
	}
	_, err = conn.Exec(ctx, "ALTER TABLE native SET(autovacuum_enabled=true); ALTER TABLE parade SET(autovacuum_enabled=true); DO $role$ BEGIN IF NOT EXISTS(SELECT FROM pg_roles WHERE rolname='spike_search') THEN CREATE ROLE spike_search LOGIN; END IF; END $role$; GRANT CONNECT ON DATABASE spike TO spike_search; GRANT USAGE ON SCHEMA public TO spike_search; GRANT SELECT ON native,parade,spike_build,spike_build_steps TO spike_search;")
	if err != nil {
		return err
	}
	_, err = conn.Exec(ctx, "UPDATE spike_build SET finished_at=now() WHERE dataset_key=$1 AND last_id=expected", key)
	return err
}

func spikeCheckSamples(ctx context.Context, c *pgx.Conn, edition int16, expected map[int32][]any) error {
	ids := make([]int32, 0, len(expected))
	for id := range expected {
		ids = append(ids, id)
	}
	rows, err := c.Query(ctx, `SELECT e.id,e.kind,n.key,f.key,p.key,l.street,l.street_2,l.locality,l.locality_2,l.town,l.county,r.sic,r.industry
 FROM entries e JOIN terms n ON n.id=e.name_id JOIN terms f ON f.id=e.first_id JOIN terms p ON p.id=e.postcode_id
 JOIN address_record_blocks b ON b.dataset_id=e.dataset_id AND b.block_id=(e.id-1)/128
 JOIN address_locations l ON l.id=b.location_ids[(e.id-1)%128+1] JOIN research_keys r ON r.dataset_id=e.dataset_id AND r.id=e.id
 WHERE e.dataset_id=$1 AND e.id=ANY($2::integer[])`, edition, ids)
	if err != nil {
		return errors.New("independent source-field checks unavailable")
	}
	defer rows.Close()
	seen := 0
	for rows.Next() {
		var id int32
		var kind int16
		ss := make([]string, 11)
		args := []any{&id, &kind}
		for i := range ss {
			args = append(args, &ss[i])
		}
		if err = rows.Scan(args...); err != nil {
			return errors.New("source-field check decode failed")
		}
		v, ok := expected[id]
		if !ok || v[1] != kind {
			return errors.New("source kind mismatch")
		}
		cols := []int{2, 3, 5, 6, 7, 8, 9, 10, 11, 12, 13}
		for i, col := range cols {
			if v[col] != ss[i] {
				return fmt.Errorf("source field mismatch in column %d", col)
			}
		}
		seen++
	}
	if err = rows.Err(); err != nil {
		return errors.New("source-field checks interrupted")
	}
	if seen != len(ids) {
		return errors.New("missing source-field check rows")
	}
	return nil
}

\set ON_ERROR_STOP on
\timing on
-- Experimental, owner-only, single-edition projection. Run with -v dataset=KEY.
-- Refuses to overwrite a previous spike. The serving role receives no grants.
SET application_name='ukinfo-hosted-projection-spike';
SET search_path=public;
SET lock_timeout='5s';
SET statement_timeout='20min';
SET work_mem='64MB';
SET maintenance_work_mem='256MB';
SET max_parallel_workers_per_gather=0;
SET max_parallel_maintenance_workers=0;
SET temp_file_limit='4GB';
SET jit=off;
SELECT id AS edition_id, entries AS edition_entries FROM datasets WHERE key=:'dataset' \gset
CREATE SCHEMA ukinfo_projection_spike;
REVOKE ALL ON SCHEMA ukinfo_projection_spike FROM PUBLIC;
CREATE TABLE ukinfo_projection_spike.build (
 dataset_id smallint PRIMARY KEY, expected bigint NOT NULL,
 started_at timestamptz NOT NULL DEFAULT clock_timestamp(), finished_at timestamptz,
 wal_bytes_started numeric NOT NULL DEFAULT (0), wal_bytes_finished numeric);
INSERT INTO ukinfo_projection_spike.build(dataset_id,expected,wal_bytes_started) SELECT :edition_id,:edition_entries,wal_bytes FROM pg_stat_wal;
DO $guard$
DECLARE d ukinfo_projection_spike.build%ROWTYPE;
BEGIN
 SELECT * INTO STRICT d FROM ukinfo_projection_spike.build;
 IF d.expected<1 OR NOT EXISTS(SELECT FROM research_indexes r JOIN address_record_indexes a USING(dataset_id)
 WHERE r.dataset_id=d.dataset_id AND r.version=1 AND a.version=1
 AND r.entries=d.expected AND a.entries=d.expected AND a.blocks=(d.expected+127)/128) THEN
 RAISE EXCEPTION 'Spike requires complete, verified source/research/address coverage'; END IF;
END $guard$;
CREATE TABLE ukinfo_projection_spike.entries (
 id integer NOT NULL, dataset_id smallint NOT NULL, kind smallint NOT NULL, name_id integer NOT NULL,
 first_id integer NOT NULL, postcode_id integer NOT NULL, location_id integer NOT NULL CHECK(location_id>0),
 group_key bytea NOT NULL CHECK(octet_length(group_key)=32),
 sic text COLLATE "C" NOT NULL, industry text COLLATE "C" NOT NULL) WITH (autovacuum_enabled=false);
CREATE TABLE ukinfo_projection_spike.steps (
 first_id integer PRIMARY KEY, rows bigint NOT NULL,
 finished_at timestamptz NOT NULL DEFAULT clock_timestamp());
-- 8,192 address blocks per committed batch (~1 million source records). Explicit
-- bounds on all three inputs avoid repeatedly hashing/scanning the full edition.
-- Each source record remains in the original compressed block store.
SELECT format($batch$
 WITH inserted AS (
 INSERT INTO ukinfo_projection_spike.entries
 SELECT e.id,e.dataset_id,e.kind,e.name_id,e.first_id,e.postcode_id,
 b.location_ids[(e.id-1)%%128+1],r.group_key,r.sic,r.industry
 FROM entries e JOIN research_keys r USING(dataset_id,id)
 JOIN address_record_blocks b ON b.dataset_id=e.dataset_id AND b.block_id=(e.id-1)/128
 WHERE e.dataset_id=%1$s AND e.id BETWEEN %2$s AND %3$s
 AND r.id BETWEEN %2$s AND %3$s AND b.block_id BETWEEN (%2$s-1)/128 AND (%3$s-1)/128
 RETURNING 1)
 INSERT INTO ukinfo_projection_spike.steps(first_id,rows) SELECT %2$s,count(*) FROM inserted;
 $batch$, :edition_id, first_id, least(first_id+1048575,:edition_entries))
 FROM generate_series(1::bigint,:edition_entries::bigint,1048576) first_id \gexec
DO $counts$
BEGIN
 IF (SELECT sum(rows) FROM ukinfo_projection_spike.steps)<>(SELECT expected FROM ukinfo_projection_spike.build) THEN
 RAISE EXCEPTION 'Incomplete spike; do not benchmark or publish it'; END IF;
END $counts$;
\ir finish.sql

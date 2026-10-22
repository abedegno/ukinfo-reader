\set ON_ERROR_STOP on
\timing on
-- Private, single-edition experiment. Refuses replacement. No serving grants.
SET application_name='ukinfo-place-postings-spike';
SET search_path=public;
SET lock_timeout='5s';
SET statement_timeout='20min';
SET work_mem='64MB';
SET maintenance_work_mem='256MB';
SET max_parallel_workers_per_gather=0;
SET max_parallel_maintenance_workers=0;
SET temp_file_limit='4GB';
SET jit=off;
SELECT id AS edition_id,entries AS edition_entries FROM datasets WHERE key=:'dataset' \gset
CREATE SCHEMA ukinfo_place_spike;
REVOKE ALL ON SCHEMA ukinfo_place_spike FROM PUBLIC;
CREATE TABLE ukinfo_place_spike.build (
 dataset_id smallint PRIMARY KEY, expected bigint NOT NULL CHECK(expected BETWEEN 1 AND 2147483647),
 chunk_rows integer NOT NULL DEFAULT 8192 CHECK(chunk_rows=8192),
 started_at timestamptz NOT NULL DEFAULT clock_timestamp(), finished_at timestamptz);
INSERT INTO ukinfo_place_spike.build(dataset_id,expected) VALUES(:edition_id,:edition_entries);
DO $guard$
BEGIN
 IF NOT EXISTS(SELECT FROM ukinfo_place_spike.build b JOIN address_record_indexes a USING(dataset_id)
 JOIN address_indexes i USING(dataset_id) WHERE a.version=1 AND i.version=1
 AND a.entries=b.expected AND i.entries=b.expected AND a.blocks=(b.expected+127)/128) THEN
 RAISE EXCEPTION 'Complete address projections required'; END IF;
END $guard$;
CREATE TABLE ukinfo_place_spike.postings (
 dataset_id smallint NOT NULL, field smallint NOT NULL CHECK(field IN (1,2)),
 key text COLLATE "C" NOT NULL CHECK(key<>''), chunk integer NOT NULL CHECK(chunk>=0),
 ids integer[] NOT NULL CHECK(cardinality(ids) BETWEEN 1 AND 8192),
 PRIMARY KEY(dataset_id,field,key,chunk)) WITH (autovacuum_enabled=false);
CREATE TABLE ukinfo_place_spike.steps (
 first_block integer PRIMARY KEY, source_rows bigint NOT NULL,
 town_rows bigint NOT NULL, county_rows bigint NOT NULL);
-- Both field lists are built from exactly the same bounded authoritative block
-- expansion. Blank fields are counted but intentionally have no searchable list.
SELECT format($batch$
 WITH expanded AS MATERIALIZED (
 SELECT b.block_id*128+u.slot::integer AS id,l.town,l.county
 FROM address_record_blocks b CROSS JOIN LATERAL unnest(b.location_ids) WITH ORDINALITY u(location_id,slot)
 JOIN address_locations l ON l.id=u.location_id
 WHERE b.dataset_id=%1$s AND b.block_id BETWEEN %2$s AND %3$s),
 inserted AS (
 INSERT INTO ukinfo_place_spike.postings
 SELECT %1$s,field,key,(id-1)/8192,array_agg(id ORDER BY id)
 FROM expanded CROSS JOIN LATERAL (VALUES(1,town),(2,county)) f(field,key)
 WHERE key<>'' GROUP BY field,key,(id-1)/8192 RETURNING 1)
 INSERT INTO ukinfo_place_spike.steps
 SELECT %2$s,count(*),count(*) FILTER(WHERE town<>''),count(*) FILTER(WHERE county<>'') FROM expanded;
 $batch$, :edition_id, first_block, least(first_block+8191,(:edition_entries-1)/128))
 FROM generate_series(0::bigint,(:edition_entries::bigint-1)/128,8192) first_block \gexec
DO $verify$
BEGIN
 IF (SELECT sum(source_rows) FROM ukinfo_place_spike.steps)<>(SELECT expected FROM ukinfo_place_spike.build)
 OR (SELECT COALESCE(sum(cardinality(ids)),0) FROM ukinfo_place_spike.postings WHERE field=1) IS DISTINCT FROM (SELECT sum(town_rows) FROM ukinfo_place_spike.steps)
 OR (SELECT COALESCE(sum(cardinality(ids)),0) FROM ukinfo_place_spike.postings WHERE field=2) IS DISTINCT FROM (SELECT sum(county_rows) FROM ukinfo_place_spike.steps)
 THEN RAISE EXCEPTION 'Incomplete place postings'; END IF;
END $verify$;
VACUUM (ANALYZE) ukinfo_place_spike.postings;
ALTER TABLE ukinfo_place_spike.postings RESET(autovacuum_enabled);
UPDATE ukinfo_place_spike.build SET finished_at=clock_timestamp();
SELECT expected,extract(epoch FROM finished_at-started_at) AS seconds,
 pg_total_relation_size('ukinfo_place_spike.postings') AS bytes FROM ukinfo_place_spike.build;

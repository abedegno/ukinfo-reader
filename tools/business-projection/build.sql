\set ON_ERROR_STOP on
\timing on
SET application_name='ukinfo-business-projection-spike';
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
CREATE SCHEMA ukinfo_business_spike;
REVOKE ALL ON SCHEMA ukinfo_business_spike FROM PUBLIC;
CREATE TABLE ukinfo_business_spike.build (
 dataset_id smallint PRIMARY KEY,expected bigint NOT NULL CHECK(expected BETWEEN 1 AND 2147483647),business_rows bigint NOT NULL,
 started_at timestamptz NOT NULL DEFAULT clock_timestamp(),finished_at timestamptz);
INSERT INTO ukinfo_business_spike.build(dataset_id,expected,business_rows)
 SELECT :edition_id,:edition_entries,count(*) FROM entries WHERE dataset_id=:edition_id AND kind=1;
DO $guard$
BEGIN
 IF NOT EXISTS(SELECT FROM ukinfo_business_spike.build b JOIN research_indexes r USING(dataset_id)
 JOIN address_record_indexes a USING(dataset_id) WHERE r.version=1 AND a.version=1
 AND r.entries=b.expected AND a.entries=b.expected AND a.blocks=(b.expected+127)/128) THEN
 RAISE EXCEPTION 'Complete source/research/address projections required'; END IF;
END $guard$;
CREATE TABLE ukinfo_business_spike.entries (
 id integer PRIMARY KEY,dataset_id smallint NOT NULL,kind smallint NOT NULL CHECK(kind=1),
 name_id integer NOT NULL,first_id integer NOT NULL,postcode_id integer NOT NULL,
 location_id integer NOT NULL CHECK(location_id>0),group_key bytea NOT NULL CHECK(octet_length(group_key)=32),
 sic text COLLATE "C" NOT NULL,industry text COLLATE "C" NOT NULL) WITH(autovacuum_enabled=false);
-- Fix the business source set before joins. Lateral fences avoid a full research
-- partition scan for this small projection, and preserve index-only source reads.
WITH business AS MATERIALIZED (
 SELECT dataset_id,id,kind,name_id,first_id,postcode_id FROM entries WHERE dataset_id=:edition_id AND kind=1)
INSERT INTO ukinfo_business_spike.entries
 SELECT e.id,e.dataset_id,e.kind,e.name_id,e.first_id,e.postcode_id,
 b.location_ids[(e.id-1)%128+1],r.group_key,r.sic,r.industry
 FROM business e
 CROSS JOIN LATERAL (SELECT group_key,sic,industry FROM research_keys r WHERE r.dataset_id=e.dataset_id AND r.id=e.id OFFSET 0) r
 CROSS JOIN LATERAL (SELECT location_ids FROM address_record_blocks b WHERE b.dataset_id=e.dataset_id AND b.block_id=(e.id-1)/128 OFFSET 0) b;
DO $verify$
BEGIN
 IF (SELECT count(*) FROM ukinfo_business_spike.entries)<>(SELECT business_rows FROM ukinfo_business_spike.build)
 THEN RAISE EXCEPTION 'Incomplete business projection'; END IF;
END $verify$;
CREATE INDEX business_sic ON ukinfo_business_spike.entries(sic,id) INCLUDE(kind,name_id,first_id,postcode_id,location_id) WHERE sic<>'';
CREATE INDEX business_industry ON ukinfo_business_spike.entries(industry,id) INCLUDE(kind,name_id,first_id,postcode_id,location_id) WHERE industry<>'';
-- Classifications are the scope of this experiment; duplicate name/postcode and
-- location indexes from the full-edition projection are deliberately omitted.
VACUUM(ANALYZE) ukinfo_business_spike.entries;
ALTER TABLE ukinfo_business_spike.entries RESET(autovacuum_enabled);
UPDATE ukinfo_business_spike.build SET finished_at=clock_timestamp();
SELECT expected,business_rows,extract(epoch FROM finished_at-started_at) AS seconds,
 pg_table_size('ukinfo_business_spike.entries') AS table_bytes,
 pg_indexes_size('ukinfo_business_spike.entries') AS index_bytes FROM ukinfo_business_spike.build;

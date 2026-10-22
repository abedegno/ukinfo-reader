\set ON_ERROR_STOP on
\timing on
SET application_name='ukinfo-hosted-projection-spike';
SET search_path=public;
-- Only this private experiment's DDL waits longer; serving budgets are unchanged.
SET lock_timeout='2min';
SET statement_timeout='20min';
SET work_mem='64MB';
SET maintenance_work_mem='256MB';
SET max_parallel_workers_per_gather=0;
SET max_parallel_maintenance_workers=0;
SET temp_file_limit='4GB';
SET jit=off;
DO $guard$
BEGIN
 IF (SELECT count(*) FROM ukinfo_projection_spike.build)<>1
 OR EXISTS(SELECT FROM ukinfo_projection_spike.build WHERE finished_at IS NOT NULL)
 OR (SELECT sum(rows) FROM ukinfo_projection_spike.steps)<>(SELECT expected FROM ukinfo_projection_spike.build) THEN
 RAISE EXCEPTION 'Finalization requires one complete, unfinished private spike'; END IF;
END $guard$;
ALTER TABLE ukinfo_projection_spike.entries SET (autovacuum_enabled=false);
SELECT 'ALTER TABLE ukinfo_projection_spike.entries ADD PRIMARY KEY(id)' WHERE NOT EXISTS(SELECT FROM pg_constraint WHERE conrelid='ukinfo_projection_spike.entries'::regclass AND contype='p') \gexec
CREATE INDEX IF NOT EXISTS projection_name ON ukinfo_projection_spike.entries(kind,name_id,first_id,id) INCLUDE(postcode_id,location_id);
CREATE INDEX IF NOT EXISTS projection_postcode ON ukinfo_projection_spike.entries(kind,postcode_id,name_id,first_id,id) INCLUDE(location_id);
CREATE INDEX IF NOT EXISTS projection_location ON ukinfo_projection_spike.entries(location_id,kind,name_id,first_id,id) INCLUDE(postcode_id);
CREATE INDEX IF NOT EXISTS projection_sic ON ukinfo_projection_spike.entries(sic,id) INCLUDE(kind,name_id,first_id,postcode_id,location_id) WHERE sic<>'';
CREATE INDEX IF NOT EXISTS projection_industry ON ukinfo_projection_spike.entries(industry,id) INCLUDE(kind,name_id,first_id,postcode_id,location_id) WHERE industry<>'';
DO $indexes$
BEGIN
 IF EXISTS(
 SELECT FROM (VALUES
 ('entries_pkey',ARRAY['id'],1,true,NULL::text),
 ('projection_name',ARRAY['kind','name_id','first_id','id','postcode_id','location_id'],4,false,NULL::text),
 ('projection_postcode',ARRAY['kind','postcode_id','name_id','first_id','id','location_id'],5,false,NULL::text),
 ('projection_location',ARRAY['location_id','kind','name_id','first_id','id','postcode_id'],5,false,NULL::text),
 ('projection_sic',ARRAY['sic','id','kind','name_id','first_id','postcode_id','location_id'],2,false,$predicate$(sic <> ''::text)$predicate$),
 ('projection_industry',ARRAY['industry','id','kind','name_id','first_id','postcode_id','location_id'],2,false,$predicate$(industry <> ''::text)$predicate$)
 ) e(name,columns,keys,unique_index,predicate)
 LEFT JOIN pg_index i ON i.indexrelid=to_regclass('ukinfo_projection_spike.'||e.name)
 LEFT JOIN pg_class c ON c.oid=i.indexrelid LEFT JOIN pg_am am ON am.oid=c.relam
 WHERE i.indexrelid IS NULL OR NOT i.indisvalid OR NOT i.indisready OR am.amname<>'btree'
 OR i.indrelid<>'ukinfo_projection_spike.entries'::regclass OR i.indisunique<>e.unique_index
 OR i.indnkeyatts<>e.keys OR i.indexprs IS NOT NULL
 OR pg_get_expr(i.indpred,i.indrelid) IS DISTINCT FROM e.predicate
 OR ARRAY(SELECT pg_get_indexdef(i.indexrelid,n,true) FROM generate_series(1,i.indnatts)n)<>e.columns
 ) THEN RAISE EXCEPTION 'Incompatible or incomplete spike indexes; inspect before benchmarking'; END IF;
END $indexes$;
VACUUM(ANALYZE) ukinfo_projection_spike.entries;
ALTER TABLE ukinfo_projection_spike.entries RESET (autovacuum_enabled);
UPDATE ukinfo_projection_spike.build SET finished_at=clock_timestamp(), wal_bytes_finished=(SELECT wal_bytes FROM pg_stat_wal);
SELECT expected,finished_at-started_at AS build_time,
 pg_size_pretty(pg_table_size('ukinfo_projection_spike.entries')) AS table_size,
 pg_size_pretty(pg_indexes_size('ukinfo_projection_spike.entries')) AS index_size
 FROM ukinfo_projection_spike.build;

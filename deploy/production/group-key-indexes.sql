-- Optional HDD-hosted source optimisation; never run by application startup.
-- One explicit edition per invocation: psql -v dataset=EDITION -f this-file.sql.
-- Original research keys, existing indexes and HTTP/SQL serving budgets are unchanged.
\set ON_ERROR_STOP on
\if :{?dataset}
\else
\echo 'An explicit dataset is required'
\quit 3
\endif
SET application_name = 'ukinfo-group-key-cover';
SET maintenance_work_mem = '256MB';
SET max_parallel_maintenance_workers = 0;
SET lock_timeout = '5s';
SET statement_timeout = 0;
SELECT pg_try_advisory_lock_shared(742918304) AS source_locked \gset
\if :source_locked
\else
\echo 'Source maintenance is active'
\quit 3
\endif
SELECT set_config('ukinfo.group_cover_dataset', :'dataset', false) AS selected_dataset \gset

CREATE OR REPLACE FUNCTION pg_temp.check_group_cover(required boolean) RETURNS void LANGUAGE plpgsql AS $check$
DECLARE part oid; existing oid; part_name text;
BEGIN
 SELECT c.oid,c.relname INTO part,part_name
 FROM public.datasets d JOIN public.research_indexes r ON r.dataset_id=d.id
 JOIN pg_class c ON c.oid=to_regclass(format('public.research_d%s',d.id))
 JOIN pg_inherits h ON h.inhrelid=c.oid AND h.inhparent='public.research_keys'::regclass
 WHERE d.key=current_setting('ukinfo.group_cover_dataset')
   AND r.version=1 AND r.entries=d.entries AND d.entries>0;
 IF part IS NULL THEN
  RAISE EXCEPTION 'Require one complete research edition and its source partition';
 END IF;
 existing := to_regclass(format('public.%I','nas_'||part_name||'_id_group_cover'));
 IF existing IS NULL THEN
  IF required THEN RAISE EXCEPTION 'Source group covering index was not created'; END IF;
  RETURN;
 END IF;
 IF NOT EXISTS (
  SELECT FROM pg_index i JOIN pg_class c ON c.oid=i.indexrelid JOIN pg_am a ON a.oid=c.relam
  WHERE i.indexrelid=existing AND i.indrelid=part AND i.indisvalid AND i.indisready
    AND NOT i.indisunique AND a.amname='btree' AND i.indpred IS NULL AND i.indexprs IS NULL
    AND i.indnkeyatts=2 AND i.indnatts=3
    AND ARRAY(SELECT x.attname::text FROM unnest(i.indkey) WITH ORDINALITY k(attnum,n)
              JOIN pg_attribute x ON x.attrelid=part AND x.attnum=k.attnum ORDER BY k.n)
        = ARRAY['dataset_id','id','group_key']
 ) THEN
  RAISE EXCEPTION 'Existing source group covering index is invalid or incompatible';
 END IF;
END
$check$;
SELECT pg_temp.check_group_cover(false);
SELECT format('CREATE INDEX CONCURRENTLY IF NOT EXISTS %I ON %s (dataset_id,id) INCLUDE(group_key);',
              'nas_'||c.relname||'_id_group_cover',c.oid::regclass)
FROM public.datasets d JOIN pg_class c ON c.oid=to_regclass(format('public.research_d%s',d.id))
WHERE d.key=current_setting('ukinfo.group_cover_dataset')
\gexec
SELECT pg_temp.check_group_cover(true);
SELECT pg_advisory_unlock_shared(742918304);

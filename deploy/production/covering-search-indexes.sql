-- Optional HDD-hosted optimisation. Keeps the compact portable indexes intact.
-- Run as the catalogue owner using psql (not inside a transaction).
-- Concurrent builds keep serving reads/writes available; one partition at a time.
\set ON_ERROR_STOP on
SET application_name = 'ukinfo-covering-search-indexes';
SET maintenance_work_mem = '256MB';
SET max_parallel_maintenance_workers = 0;
SET lock_timeout = '5s';
SET statement_timeout = 0;

-- IF NOT EXISTS must never silently accept an invalid/incompatible prior build.
DO $check$
DECLARE partition record; existing oid;
BEGIN
 FOR partition IN
  SELECT c.oid,c.relname FROM pg_inherits i JOIN pg_class c ON c.oid=i.inhrelid
  WHERE i.inhparent='public.entries'::regclass
 LOOP
  existing := to_regclass(format('public.%I','nas_'||partition.relname||'_name_postcode_cover'));
  IF existing IS NOT NULL AND NOT EXISTS (
   SELECT FROM pg_index idx WHERE idx.indexrelid=existing AND idx.indrelid=partition.oid
    AND idx.indisvalid AND idx.indisready AND NOT idx.indisunique
    AND idx.indpred IS NULL AND idx.indexprs IS NULL
    AND idx.indnkeyatts=5 AND idx.indnatts=6
    AND ARRAY(SELECT a.attname::text FROM unnest(idx.indkey) WITH ORDINALITY k(attnum,n)
              JOIN pg_attribute a ON a.attrelid=idx.indrelid AND a.attnum=k.attnum ORDER BY k.n)
        = ARRAY['dataset_id','kind','name_id','first_id','id','postcode_id']
  ) THEN
   RAISE EXCEPTION 'Existing covering index for % is invalid or incompatible; inspect before retrying',partition.relname;
  END IF;
 END LOOP;
END
$check$;

SELECT format('CREATE INDEX CONCURRENTLY IF NOT EXISTS %I ON %s (dataset_id,kind,name_id,first_id,id) INCLUDE (postcode_id);',
              'nas_'||c.relname||'_name_postcode_cover',c.oid::regclass)
FROM pg_inherits i JOIN pg_class c ON c.oid=i.inhrelid
WHERE i.inhparent='public.entries'::regclass
ORDER BY c.reltuples,c.oid
\gexec

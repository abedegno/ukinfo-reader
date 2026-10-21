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
 existing := to_regclass('public.nas_terms_key_id_cover');
 IF existing IS NOT NULL AND NOT EXISTS (
  SELECT FROM pg_index idx JOIN pg_class ic ON ic.oid=idx.indexrelid
   JOIN pg_am am ON am.oid=ic.relam
  WHERE idx.indexrelid=existing AND idx.indrelid='public.terms'::regclass
   AND idx.indisvalid AND idx.indisready AND NOT idx.indisunique
   AND am.amname='btree' AND idx.indpred IS NULL AND idx.indexprs IS NULL
   AND idx.indnkeyatts=1 AND idx.indnatts=2
   AND idx.indcollation[0]=(SELECT attcollation FROM pg_attribute WHERE attrelid=idx.indrelid AND attname='key')
   AND ARRAY(SELECT a.attname::text FROM unnest(idx.indkey) WITH ORDINALITY k(attnum,n)
             JOIN pg_attribute a ON a.attrelid=idx.indrelid AND a.attnum=k.attnum ORDER BY k.n)
       = ARRAY['key','id']
 ) THEN
  RAISE EXCEPTION 'Existing vocabulary covering index is invalid or incompatible; inspect before retrying';
 END IF;
 existing := to_regclass('public.nas_terms_id_key_cover');
 IF existing IS NOT NULL AND NOT EXISTS (
  SELECT FROM pg_index idx JOIN pg_class ic ON ic.oid=idx.indexrelid
   JOIN pg_am am ON am.oid=ic.relam
  WHERE idx.indexrelid=existing AND idx.indrelid='public.terms'::regclass
   AND idx.indisvalid AND idx.indisready AND NOT idx.indisunique
   AND am.amname='btree' AND idx.indpred IS NULL AND idx.indexprs IS NULL
   AND idx.indnkeyatts=1 AND idx.indnatts=2 AND idx.indcollation[0]=0
   AND ARRAY(SELECT a.attname::text FROM unnest(idx.indkey) WITH ORDINALITY k(attnum,n)
             JOIN pg_attribute a ON a.attrelid=idx.indrelid AND a.attnum=k.attnum ORDER BY k.n)
       = ARRAY['id','key']
 ) THEN
  RAISE EXCEPTION 'Existing reverse vocabulary covering index is invalid or incompatible; inspect before retrying';
 END IF;
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

-- Prefix expansion needs both key and ID, before any entry search begins.
-- Keep the original unique constraint/index; this optional index avoids random
-- vocabulary heap reads for names, postcodes and materialized broad expansions.
CREATE INDEX CONCURRENTLY IF NOT EXISTS nas_terms_key_id_cover ON public.terms (key) INCLUDE (id);

-- Sorting and grouping resolve IDs back to labels. The prefix index above
-- cannot cover those probes; keep both directions without duplicating the heap.
CREATE INDEX CONCURRENTLY IF NOT EXISTS nas_terms_id_key_cover ON public.terms (id) INCLUDE (key);

SELECT format('CREATE INDEX CONCURRENTLY IF NOT EXISTS %I ON %s (dataset_id,kind,name_id,first_id,id) INCLUDE (postcode_id);',
              'nas_'||c.relname||'_name_postcode_cover',c.oid::regclass)
FROM pg_inherits i JOIN pg_class c ON c.oid=i.inhrelid
WHERE i.inhparent='public.entries'::regclass
ORDER BY c.reltuples,c.oid
\gexec

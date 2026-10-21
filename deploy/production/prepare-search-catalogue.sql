-- Owner maintenance after an immutable edition's import/verification transactions
-- finish, before publication. psql -v dataset='exact-complete-edition-key' -f ...
-- Ordinary VACUUM only: no source rewrite, blocking FULL vacuum or new privileges.
\set ON_ERROR_STOP on
\if :{?dataset}
\else
 DO $missing$ BEGIN RAISE EXCEPTION 'Set -v dataset to one complete edition key'; END $missing$;
\endif
SET application_name='ukinfo-prepare-search-catalogue';
SET search_path=public;
SET lock_timeout='5s';
SET statement_timeout='20min';
SET maintenance_work_mem='256MB';
SET max_parallel_maintenance_workers=0;
SET vacuum_cost_delay='2ms';
SET vacuum_cost_limit=1000;
CREATE TEMP TABLE ukinfo_search_maintenance_datasets AS
 SELECT d.id FROM datasets d
 JOIN research_indexes r ON r.dataset_id=d.id AND r.version=1 AND r.entries=d.entries
 JOIN address_indexes a ON a.dataset_id=d.id AND a.version=1 AND a.entries=d.entries
 JOIN address_record_indexes b ON b.dataset_id=d.id AND b.version=1
  AND b.entries=d.entries AND b.blocks=(d.entries+127)/128
 WHERE d.key=:'dataset' AND d.entries>0;
DO $check$
BEGIN
 IF (SELECT count(*) FROM ukinfo_search_maintenance_datasets)<>1
 OR (SELECT count(*) FROM pg_inherits i JOIN pg_class c ON c.oid=i.inhrelid
       JOIN ukinfo_search_maintenance_datasets d
         ON c.relname IN ('entries_d'||d.id,'research_d'||d.id)
       WHERE i.inhparent IN ('entries'::regclass,'research_keys'::regclass))<>2
 THEN RAISE EXCEPTION 'Expected one complete edition with source and research partitions'; END IF;
END
$check$;
-- Finish the visibility map after old snapshots release, including research
-- projections. Partial visibility can turn a covered classification probe back
-- into many random HDD heap reads despite a valid index and fresh statistics.
SELECT format('VACUUM (ANALYZE) %s;',c.oid::regclass)
FROM pg_inherits i JOIN pg_class c ON c.oid=i.inhrelid
JOIN ukinfo_search_maintenance_datasets d
 ON c.relname IN ('entries_d'||d.id,'research_d'||d.id)
WHERE i.inhparent IN ('entries'::regclass,'research_keys'::regclass)
ORDER BY c.relpages
\gexec
VACUUM (ANALYZE) terms;
VACUUM (ANALYZE) address_locations;
VACUUM (ANALYZE) address_record_blocks;
DROP TABLE ukinfo_search_maintenance_datasets;

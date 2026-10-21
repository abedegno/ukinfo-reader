-- Owner maintenance with psql, not a transaction. Install address_lookup.sql
-- beside this file. Use -v dataset='all' or an exact complete catalogue key.
-- Each edition commits independently; existing searches remain available.
\set ON_ERROR_STOP on
SET application_name='ukinfo-address-record-lookups';
SET lock_timeout='5s';
SET statement_timeout=0;
SET work_mem='128MB';
SET maintenance_work_mem='256MB';
SET max_parallel_maintenance_workers=0;
\ir address_lookup.sql
SELECT 'GRANT SELECT ON address_record_blocks,address_record_indexes TO ukinfo_search'
WHERE EXISTS (SELECT FROM pg_roles WHERE rolname='ukinfo_search')
\gexec
CREATE INDEX CONCURRENTLY IF NOT EXISTS address_town_lookup ON address_locations(town,id);
CREATE INDEX CONCURRENTLY IF NOT EXISTS address_county_lookup ON address_locations(county,id);
SELECT format('SELECT build_address_record_lookup(%L);',d.key)
FROM datasets d JOIN address_indexes a ON a.dataset_id=d.id AND a.entries=d.entries AND a.version=1
WHERE :'dataset'='all' OR d.key=:'dataset'
ORDER BY CASE WHEN d.key='se2003-dpcs0104' THEN 0 ELSE 1 END,d.id
\gexec
VACUUM (ANALYZE) address_record_blocks;
ANALYZE address_record_indexes;
-- Serving grants above cover only these parents; no write or ownership rights.

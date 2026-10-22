\set ON_ERROR_STOP on
-- Owner-only, additive preparation for one immutable published edition.
-- psql -v dataset=EDITION -f deploy/production/business-search.sql
SET application_name='ukinfo-business-search-build';
SET lock_timeout='5s';
SET statement_timeout='20min';
SET work_mem='64MB';
SET maintenance_work_mem='256MB';
SET max_parallel_workers_per_gather=0;
SET max_parallel_maintenance_workers=0;
SET temp_file_limit='4GB';
SET jit=off;
\ir ../../server/internal/catalog/business_search.sql
SELECT build_business_search(:'dataset');
SELECT format('VACUUM(ANALYZE) %I','business_d'||id) FROM datasets WHERE key=:'dataset' \gexec
SELECT 'GRANT SELECT ON business_search,business_search_indexes TO ukinfo_search'
WHERE EXISTS(SELECT FROM pg_roles WHERE rolname='ukinfo_search') \gexec

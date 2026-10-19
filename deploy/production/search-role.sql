\set ON_ERROR_STOP on
\getenv search_password UKINFO_SEARCH_PASSWORD
BEGIN;
SELECT 'CREATE ROLE ukinfo_search LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS'
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname='ukinfo_search') \gexec
ALTER ROLE ukinfo_search PASSWORD :'search_password';
ALTER ROLE ukinfo_search SET default_transaction_read_only=on;
ALTER ROLE ukinfo_search SET statement_timeout='5s';
SELECT format('REVOKE ALL ON DATABASE %I FROM PUBLIC',current_database()) \gexec
SELECT format('GRANT CONNECT ON DATABASE %I TO ukinfo_search',current_database()) \gexec
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
GRANT USAGE ON SCHEMA public TO ukinfo_search;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO ukinfo_search;
ALTER DEFAULT PRIVILEGES FOR ROLE ukinfo IN SCHEMA public GRANT SELECT ON TABLES TO ukinfo_search;
COMMIT;

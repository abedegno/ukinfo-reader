-- Derived DB only; execute privately as owner. No password enters arguments.
\set ON_ERROR_STOP on
\getenv reader_password UKINFO_PARADE_SEARCH_PASSWORD
SELECT length(:'reader_password')>=32 AS password_valid \gset
\if :password_valid
\else
  \echo 'Set a random reader password of at least 32 characters'
  \quit 1
\endif
BEGIN;
SELECT 'CREATE ROLE ukinfo_parade_search LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS'
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname='ukinfo_parade_search') \gexec
DO $policy$
DECLARE serving oid := 'ukinfo_parade_search'::regrole;
BEGIN
 IF EXISTS (SELECT FROM pg_auth_members WHERE member=serving)
    OR EXISTS (SELECT FROM pg_class WHERE relowner=serving)
    OR EXISTS (SELECT FROM pg_namespace WHERE nspowner=serving)
    OR EXISTS (SELECT FROM pg_proc WHERE proowner=serving)
    OR EXISTS (SELECT FROM pg_database WHERE datdba=serving) THEN
  RAISE EXCEPTION 'Use a dedicated reader with no ownership or memberships';
 END IF;
END
$policy$;
ALTER ROLE ukinfo_parade_search LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
ALTER ROLE ukinfo_parade_search PASSWORD :'reader_password';
ALTER ROLE ukinfo_parade_search SET default_transaction_read_only=on;
ALTER ROLE ukinfo_parade_search SET statement_timeout='4000';
SELECT format('REVOKE ALL ON DATABASE %I FROM PUBLIC',current_database()) \gexec
SELECT format('REVOKE ALL ON DATABASE %I FROM ukinfo_parade_search',current_database()) \gexec
SELECT format('GRANT CONNECT ON DATABASE %I TO ukinfo_parade_search',current_database()) \gexec
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
REVOKE ALL ON SCHEMA public FROM ukinfo_parade_search;
SELECT format('REVOKE ALL ON SCHEMA %I FROM ukinfo_parade_search',nspname)
FROM pg_namespace WHERE nspname='ukinfo_search' \gexec
GRANT USAGE ON SCHEMA public TO ukinfo_parade_search;
SELECT format('REVOKE ALL ON TABLE %I.%I FROM ukinfo_parade_search',schemaname,tablename)
FROM pg_tables WHERE schemaname IN ('public','ukinfo_search') \gexec
SELECT format('REVOKE ALL (%I) ON TABLE %I.%I FROM ukinfo_parade_search',column_name,table_schema,table_name)
FROM information_schema.column_privileges WHERE grantee='ukinfo_parade_search'
AND table_schema IN ('public','ukinfo_search') \gexec
SELECT format('REVOKE ALL ON SEQUENCE %I.%I FROM ukinfo_parade_search',schemaname,sequencename)
FROM pg_sequences WHERE schemaname IN ('public','ukinfo_search') \gexec
-- Refuse write privileges inherited through PUBLIC; defaults alone are insufficient.
DO $policy$
BEGIN
 IF EXISTS (SELECT FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
   WHERE n.nspname IN ('public','ukinfo_search') AND CASE WHEN c.relkind IN ('r','p','v','m','f') THEN
   (has_table_privilege('ukinfo_parade_search',c.oid,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER,MAINTAIN')
    OR has_any_column_privilege('ukinfo_parade_search',c.oid,'INSERT,UPDATE,REFERENCES'))
   WHEN c.relkind='S' THEN has_sequence_privilege('ukinfo_parade_search',c.oid,'USAGE,UPDATE') ELSE false END) THEN
  RAISE EXCEPTION 'Reader inherits write privileges; review PUBLIC grants';
 END IF;
END
$policy$;
COMMIT;
-- The index builder grants only schema USAGE and SELECT on published objects.

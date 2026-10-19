\set ON_ERROR_STOP on
\getenv search_password UKINFO_SEARCH_PASSWORD
BEGIN;
SELECT 'CREATE ROLE ukinfo_search LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS'
WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname='ukinfo_search') \gexec
-- Existing roles need the same policy as freshly created roles. Refuse ambiguous
-- ownership/membership rather than treating a read-only default as protection.
DO $policy$
DECLARE serving oid := 'ukinfo_search'::regrole;
BEGIN
 IF EXISTS (SELECT FROM pg_auth_members WHERE member=serving)
    OR EXISTS (SELECT FROM pg_class WHERE relowner=serving)
    OR EXISTS (SELECT FROM pg_namespace WHERE nspowner=serving)
    OR EXISTS (SELECT FROM pg_database WHERE datname=current_database() AND datdba=serving) THEN
  RAISE EXCEPTION 'Serving role has ownership or role memberships; use a dedicated role and review its grants';
 END IF;
END
$policy$;
ALTER ROLE ukinfo_search LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
ALTER ROLE ukinfo_search PASSWORD :'search_password';
ALTER ROLE ukinfo_search SET default_transaction_read_only=on;
ALTER ROLE ukinfo_search SET statement_timeout='5s';
SELECT format('REVOKE ALL ON DATABASE %I FROM PUBLIC',current_database()) \gexec
SELECT format('REVOKE ALL ON DATABASE %I FROM ukinfo_search',current_database()) \gexec
SELECT format('GRANT CONNECT ON DATABASE %I TO ukinfo_search',current_database()) \gexec
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
REVOKE ALL ON SCHEMA public FROM ukinfo_search;
GRANT USAGE ON SCHEMA public TO ukinfo_search;
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM ukinfo_search;
REVOKE ALL ON ALL SEQUENCES IN SCHEMA public FROM ukinfo_search;
SELECT format('REVOKE ALL (%I) ON TABLE %I.%I FROM ukinfo_search',column_name,table_schema,table_name)
FROM information_schema.column_privileges WHERE grantee='ukinfo_search' AND table_schema='public' \gexec
GRANT SELECT ON ALL TABLES IN SCHEMA public TO ukinfo_search;
ALTER DEFAULT PRIVILEGES FOR ROLE ukinfo IN SCHEMA public REVOKE ALL ON TABLES FROM ukinfo_search;
ALTER DEFAULT PRIVILEGES FOR ROLE ukinfo IN SCHEMA public GRANT SELECT ON TABLES TO ukinfo_search;
-- CASE guards relation-specific functions; WHERE evaluation can be reordered.
DO $policy$
BEGIN
 IF EXISTS (SELECT FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
            WHERE n.nspname='public' AND CASE WHEN c.relkind IN ('r','p','v','m','f') THEN
            (has_table_privilege('ukinfo_search',c.oid,'INSERT,UPDATE,DELETE,TRUNCATE,REFERENCES,TRIGGER,MAINTAIN')
             OR has_any_column_privilege('ukinfo_search',c.oid,'INSERT,UPDATE,REFERENCES')) ELSE false END)
    OR EXISTS (SELECT FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
               WHERE n.nspname='public' AND CASE WHEN c.relkind='S' THEN has_sequence_privilege('ukinfo_search',c.oid,'USAGE,UPDATE') ELSE false END) THEN
  RAISE EXCEPTION 'Serving role still has write privileges through PUBLIC or other grants; review the catalogue permissions';
 END IF;
END
$policy$;
COMMIT;

\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout='5s';
DO $guard$
BEGIN
 IF (SELECT count(*) FROM ukinfo_business_spike.build)<>1
 OR NOT EXISTS(SELECT FROM ukinfo_business_spike.build b JOIN public.datasets d ON d.id=b.dataset_id AND d.entries=b.expected)
 OR (SELECT nspowner FROM pg_namespace WHERE nspname='ukinfo_business_spike')<>(SELECT oid FROM pg_roles WHERE rolname=current_user)
 THEN RAISE EXCEPTION 'Refuse ambiguous private business cleanup'; END IF;
END $guard$;
DROP TABLE ukinfo_business_spike.entries RESTRICT;
DROP TABLE ukinfo_business_spike.build RESTRICT;
DROP SCHEMA ukinfo_business_spike RESTRICT;
COMMIT;

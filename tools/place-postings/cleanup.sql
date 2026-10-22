\set ON_ERROR_STOP on
BEGIN;
SET LOCAL lock_timeout='5s';
DO $guard$
BEGIN
 IF (SELECT count(*) FROM ukinfo_place_spike.build)<>1
 OR NOT EXISTS(SELECT FROM ukinfo_place_spike.build b JOIN public.datasets d ON d.id=b.dataset_id AND d.entries=b.expected)
 OR (SELECT nspowner FROM pg_namespace WHERE nspname='ukinfo_place_spike')<>(SELECT oid FROM pg_roles WHERE rolname=current_user)
 THEN RAISE EXCEPTION 'Refuse ambiguous private place cleanup'; END IF;
END $guard$;
DROP TABLE ukinfo_place_spike.postings RESTRICT;
DROP TABLE ukinfo_place_spike.steps RESTRICT;
DROP TABLE ukinfo_place_spike.build RESTRICT;
DROP SCHEMA ukinfo_place_spike RESTRICT;
COMMIT;

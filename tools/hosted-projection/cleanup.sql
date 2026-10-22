\set ON_ERROR_STOP on
-- Remove only this disposable experiment, never its source edition. No CASCADE:
-- any external dependency or additional schema object causes a rollback.
\if :{?dataset}
\else
 DO $missing$ BEGIN RAISE EXCEPTION 'Set -v dataset to the measured edition key'; END $missing$;
\endif
BEGIN;
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='2min';
DO $guard$
BEGIN
 IF (SELECT count(*) FROM ukinfo_projection_spike.build)<>1
 OR (SELECT nspowner FROM pg_namespace WHERE nspname='ukinfo_projection_spike')<>current_user::regrole THEN
 RAISE EXCEPTION 'Cleanup requires one operator-owned spike'; END IF;
END $guard$;
SELECT count(*)=1 AS cleanup_target_ok FROM ukinfo_projection_spike.build b
JOIN public.datasets d ON d.id=b.dataset_id AND d.entries=b.expected
WHERE d.key=:'dataset' AND b.finished_at IS NOT NULL \gset
\if :cleanup_target_ok
\else
 DO $target$ BEGIN RAISE EXCEPTION 'Measured edition does not match cleanup target'; END $target$;
\endif
DROP TABLE IF EXISTS ukinfo_projection_spike.tuning RESTRICT;
DROP TABLE ukinfo_projection_spike.entries,ukinfo_projection_spike.steps,ukinfo_projection_spike.build RESTRICT;
DROP SCHEMA ukinfo_projection_spike RESTRICT;
COMMIT;

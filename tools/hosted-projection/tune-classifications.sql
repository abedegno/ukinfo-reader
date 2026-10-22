\set ON_ERROR_STOP on
\timing on
-- Recovery for the initial measured prototype only. No public schema is changed.
SET application_name='ukinfo-projection-classification-tuning';
SET lock_timeout='5s';
SET statement_timeout='10min';
SET maintenance_work_mem='256MB';
SET max_parallel_maintenance_workers=0;
SET temp_file_limit='4GB';
BEGIN;
DO $guard$
BEGIN
 IF (SELECT count(*) FROM ukinfo_projection_spike.build)<>1 OR NOT EXISTS(SELECT FROM ukinfo_projection_spike.build WHERE finished_at IS NOT NULL) THEN
 RAISE EXCEPTION 'Tuning requires a complete private spike'; END IF;
END $guard$;
CREATE TABLE ukinfo_projection_spike.tuning (
 started_at timestamptz NOT NULL DEFAULT clock_timestamp(),finished_at timestamptz);
INSERT INTO ukinfo_projection_spike.tuning DEFAULT VALUES;
DROP INDEX ukinfo_projection_spike.projection_sic;
DROP INDEX ukinfo_projection_spike.projection_industry;
CREATE INDEX projection_sic ON ukinfo_projection_spike.entries(sic,id) INCLUDE(kind,name_id,first_id,postcode_id,location_id) WHERE sic<>'';
CREATE INDEX projection_industry ON ukinfo_projection_spike.entries(industry,id) INCLUDE(kind,name_id,first_id,postcode_id,location_id) WHERE industry<>'';
UPDATE ukinfo_projection_spike.tuning SET finished_at=clock_timestamp();
COMMIT;

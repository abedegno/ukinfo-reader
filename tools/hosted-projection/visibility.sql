\set ON_ERROR_STOP on
-- Read-only on-disk header sampling for an immutable, completed edition.
-- Requires the operator's server-file read permission. Returns no tuple bytes.
-- Headers can lag dirty buffers; this is not a full visibility-map audit.
\if :{?dataset}
\else
 DO $missing$ BEGIN RAISE EXCEPTION 'Set -v dataset to a complete edition key'; END $missing$;
\endif
BEGIN READ ONLY;
SET LOCAL search_path=public;
SET LOCAL statement_timeout='30s';
SET LOCAL lock_timeout='100ms';
DO $layout$
BEGIN
 IF current_setting('server_version_num')::integer NOT BETWEEN 180000 AND 189999
 OR current_setting('block_size')<>'8192' OR current_setting('segment_size')<>'1GB' THEN
 RAISE EXCEPTION 'Header sampler supports PostgreSQL 18, 8-KiB pages and 1-GiB segments'; END IF;
END $layout$;
SELECT id AS edition_id FROM datasets WHERE key=:'dataset' AND entries>0 \gset
SELECT c.relname,c.relpages,c.relallvisible,
 round(100.0*c.relallvisible/nullif(c.relpages,0),2) visible_percent
FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
WHERE n.nspname='public' AND c.relname IN ('entries_d'||:edition_id,'research_d'||:edition_id);
WITH samples AS MATERIALIZED (
 SELECT c.relname,pg_relation_filepath(c.oid) path,((c.relpages-1)::bigint*i/255) block
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace CROSS JOIN generate_series(0,255)i
 WHERE n.nspname='public' AND c.relpages>0
 AND c.relname IN('entries_d'||:edition_id,'research_d'||:edition_id)
), headers AS MATERIALIZED (
 SELECT relname,pg_read_binary_file(path||CASE WHEN block/131072>0 THEN '.'||(block/131072)::text ELSE '' END,
 (block%131072)*8192,24,true) h FROM samples
)
SELECT relname,count(*) samples,
 count(*) FILTER(WHERE get_byte(h,14)=0 AND get_byte(h,15)=0) uninitialized_pages,
 count(*) FILTER(WHERE get_byte(h,12)+256*get_byte(h,13)>24) tuple_pages,
 count(*) FILTER(WHERE get_byte(h,12)+256*get_byte(h,13)>24 AND (get_byte(h,10)&4)=4) tuple_pages_marked_visible,
 count(*) FILTER(WHERE get_byte(h,12)+256*get_byte(h,13)>24 AND (get_byte(h,18)<>4 OR get_byte(h,19)<>32)) unsupported_header_layouts
FROM headers GROUP BY relname;
-- Interpret the flags only when unsupported_header_layouts=0 (little-endian,
-- 8-KiB layout version 4). Data files are never modified by this diagnostic.
ROLLBACK;

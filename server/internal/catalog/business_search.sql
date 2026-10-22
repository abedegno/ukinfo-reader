-- Optional additive projection for classified businesses. Source blocks remain
-- authoritative; complete per-edition metadata alone enables serving.
CREATE TABLE IF NOT EXISTS business_search (
 dataset_id smallint NOT NULL,id integer NOT NULL,kind smallint NOT NULL CHECK(kind=1),
 name_id integer NOT NULL,first_id integer NOT NULL,postcode_id integer NOT NULL,
 location_id integer NOT NULL CHECK(location_id>0),
 group_key bytea NOT NULL CHECK(octet_length(group_key)=32),
 sic text COLLATE "C" NOT NULL,industry text COLLATE "C" NOT NULL,
 PRIMARY KEY(dataset_id,id)) PARTITION BY LIST(dataset_id);
CREATE INDEX IF NOT EXISTS business_search_sic ON business_search(dataset_id,sic,id)
 INCLUDE(kind,name_id,first_id,postcode_id,location_id) WHERE sic<>'';
CREATE INDEX IF NOT EXISTS business_search_industry ON business_search(dataset_id,industry,id)
 INCLUDE(kind,name_id,first_id,postcode_id,location_id) WHERE industry<>'';
DO $indexes$
BEGIN
 IF EXISTS(SELECT FROM (VALUES
 ('business_search_pkey',ARRAY['dataset_id','id'],2,true,NULL::text),
 ('business_search_sic',ARRAY['dataset_id','sic','id','kind','name_id','first_id','postcode_id','location_id'],3,false,$predicate$(sic <> ''::text)$predicate$),
 ('business_search_industry',ARRAY['dataset_id','industry','id','kind','name_id','first_id','postcode_id','location_id'],3,false,$predicate$(industry <> ''::text)$predicate$)
 ) e(name,columns,keys,unique_index,predicate)
 LEFT JOIN pg_index i ON i.indexrelid=to_regclass(format('%I.%I',current_schema(),e.name))
 LEFT JOIN pg_class c ON c.oid=i.indexrelid LEFT JOIN pg_am am ON am.oid=c.relam
 WHERE i.indexrelid IS NULL OR NOT i.indisvalid OR NOT i.indisready OR am.amname<>'btree'
 OR i.indrelid<>'business_search'::regclass OR i.indisunique<>e.unique_index
 OR i.indnkeyatts<>e.keys OR i.indexprs IS NOT NULL
 OR pg_get_expr(i.indpred,i.indrelid) IS DISTINCT FROM e.predicate
 OR ARRAY(SELECT pg_get_indexdef(i.indexrelid,n,true) FROM generate_series(1,i.indnatts)n)<>e.columns
 ) THEN RAISE EXCEPTION 'Incompatible or incomplete business search indexes'; END IF;
END $indexes$;
CREATE TABLE IF NOT EXISTS business_search_indexes (
 dataset_id smallint PRIMARY KEY REFERENCES datasets(id),version integer NOT NULL CHECK(version=1),
 entries bigint NOT NULL,businesses bigint NOT NULL CHECK(businesses>=0),
 indexed_at timestamptz NOT NULL DEFAULT now());
CREATE OR REPLACE FUNCTION build_business_search(dataset_key text)
RETURNS bigint LANGUAGE plpgsql AS $build$
DECLARE edition smallint; expected bigint; actual bigint; inserted bigint;
 existing business_search_indexes%ROWTYPE; partition_name text;
BEGIN
 IF NOT pg_try_advisory_xact_lock(742918304) OR NOT pg_try_advisory_xact_lock(742918306,hashtext(current_schema())) THEN
 RAISE EXCEPTION 'An import or index build is running'; END IF;
 SELECT id,entries INTO STRICT edition,expected FROM datasets WHERE key=dataset_key FOR UPDATE;
 IF expected NOT BETWEEN 1 AND 2147483647 OR NOT EXISTS(
 SELECT FROM research_indexes r JOIN address_record_indexes a USING(dataset_id)
 JOIN address_indexes i USING(dataset_id) WHERE r.dataset_id=edition AND r.version=1 AND a.version=1 AND i.version=1
 AND r.entries=expected AND a.entries=expected AND i.entries=expected AND a.blocks=(expected+127)/128) THEN
 RAISE EXCEPTION 'Complete research and address indexes required'; END IF;
 SELECT * INTO existing FROM business_search_indexes WHERE dataset_id=edition;
 IF FOUND THEN
 IF existing.version<>1 OR existing.entries<>expected OR NOT EXISTS(SELECT FROM pg_inherits WHERE inhparent='business_search'::regclass AND inhrelid=to_regclass(format('%I.%I',current_schema(),'business_d'||edition))) THEN RAISE EXCEPTION 'Incompatible or detached business projection'; END IF;
 RETURN existing.businesses;
 END IF;
 partition_name:='business_d'||edition;
 -- No IF NOT EXISTS: an unfinished/unexpected relation must be inspected.
 EXECUTE format('CREATE TABLE %I (LIKE business_search INCLUDING DEFAULTS INCLUDING CONSTRAINTS, CHECK(dataset_id=%s))',partition_name,edition);
 SELECT count(*) INTO actual FROM entries WHERE dataset_id=edition AND kind=1;
 EXECUTE format($sql$
 WITH business AS MATERIALIZED (SELECT dataset_id,id,kind,name_id,first_id,postcode_id FROM entries WHERE dataset_id=$1 AND kind=1)
 INSERT INTO %I SELECT e.dataset_id,e.id,e.kind,e.name_id,e.first_id,e.postcode_id,
 b.location_ids[(e.id-1)%%128+1],r.group_key,r.sic,r.industry
 FROM business e
 CROSS JOIN LATERAL (SELECT group_key,sic,industry FROM research_keys r WHERE r.dataset_id=e.dataset_id AND r.id=e.id OFFSET 0) r
 CROSS JOIN LATERAL (SELECT location_ids FROM address_record_blocks b WHERE b.dataset_id=e.dataset_id AND b.block_id=(e.id-1)/128 OFFSET 0) b
 $sql$,partition_name) USING edition;
 GET DIAGNOSTICS inserted=ROW_COUNT;
 IF actual<>inserted THEN RAISE EXCEPTION 'Incomplete business projection'; END IF;
 EXECUTE format('ALTER TABLE business_search ATTACH PARTITION %I FOR VALUES IN (%s)',partition_name,edition);
 EXECUTE format('ANALYZE %I',partition_name);
 INSERT INTO business_search_indexes(dataset_id,version,entries,businesses) VALUES(edition,1,expected,actual);
 RETURN actual;
END $build$;
REVOKE ALL ON FUNCTION build_business_search(text) FROM PUBLIC;

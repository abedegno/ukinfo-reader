-- Optional reverse address projection. One small array per 128 source records,
-- not one heap/index tuple per occurrence. Original record blocks stay unchanged.
CREATE TABLE IF NOT EXISTS address_record_blocks (
 -- Explicit dataset cleanup avoids taking a parent-table FK DDL lock while
 -- another immutable edition is importing in a long transaction.
 dataset_id smallint NOT NULL CHECK(dataset_id>0),
 block_id integer NOT NULL CHECK (block_id >= 0),
 location_ids integer[] NOT NULL CHECK (
  array_ndims(location_ids)=1 AND array_lower(location_ids,1)=1
  AND cardinality(location_ids) BETWEEN 1 AND 128),
 PRIMARY KEY (dataset_id,block_id) INCLUDE (location_ids)
);
CREATE TABLE IF NOT EXISTS address_record_indexes (
 dataset_id smallint PRIMARY KEY CHECK(dataset_id>0),
 version integer NOT NULL CHECK(version=1),
 entries bigint NOT NULL,
 blocks integer NOT NULL,
 indexed_at timestamptz NOT NULL DEFAULT now()
);

-- Owner-only maintenance; each invocation publishes a complete edition in its
-- caller's transaction. The dataset row lock also serializes deletion/replacement.
CREATE OR REPLACE FUNCTION build_address_record_lookup(dataset_key text)
RETURNS integer LANGUAGE plpgsql AS $lookup$
DECLARE edition smallint; expected bigint; expected_blocks integer;
        inserted bigint; existing address_record_indexes%ROWTYPE;
BEGIN
 SELECT d.id,d.entries INTO STRICT edition,expected
 FROM datasets d JOIN address_indexes a ON a.dataset_id=d.id
  AND a.entries=d.entries AND a.version=1
 WHERE d.key=dataset_key FOR UPDATE OF d;
 IF expected<1 OR expected>2147483647 THEN
  RAISE EXCEPTION 'Address lookup requires a complete nonempty edition';
 END IF;
 expected_blocks := ((expected+127)/128)::integer;
 SELECT * INTO existing FROM address_record_indexes WHERE dataset_id=edition;
 IF FOUND THEN
  IF existing.version<>1 OR existing.entries<>expected OR existing.blocks<>expected_blocks THEN
   RAISE EXCEPTION 'Incompatible address record lookup; inspect before rebuilding';
  END IF;
  RETURN existing.blocks;
 END IF;
 IF EXISTS (SELECT FROM address_record_blocks WHERE dataset_id=edition) THEN
  RAISE EXCEPTION 'Unpublished address lookup rows exist; inspect before rebuilding';
 END IF;
 INSERT INTO address_record_blocks(dataset_id,block_id,location_ids)
 SELECT edition,(entry_id-1)/128,array_agg(m.location_id ORDER BY entry_id)
 FROM address_members m CROSS JOIN LATERAL unnest(m.ids) entry_id
 WHERE m.dataset_id=edition
 GROUP BY (entry_id-1)/128
 HAVING min(entry_id)=((entry_id-1)/128)::bigint*128+1
  AND max(entry_id)=least(((entry_id-1)/128)::bigint*128+128,expected)
  AND count(*)=least(128,expected-((entry_id-1)/128)::bigint*128)
  AND count(DISTINCT entry_id)=count(*);
 GET DIAGNOSTICS inserted=ROW_COUNT;
 IF inserted<>expected_blocks THEN
  RAISE EXCEPTION 'Address lookup has missing, duplicate or out-of-range occurrences';
 END IF;
 INSERT INTO address_record_indexes(dataset_id,version,entries,blocks)
 VALUES(edition,1,expected,expected_blocks);
 RETURN expected_blocks;
END
$lookup$;
REVOKE ALL ON FUNCTION build_address_record_lookup(text) FROM PUBLIC;

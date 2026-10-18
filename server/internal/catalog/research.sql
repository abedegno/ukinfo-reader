-- Additive projections; original record blocks remain the authority.
CREATE TABLE IF NOT EXISTS research_indexes (
 dataset_id smallint PRIMARY KEY REFERENCES datasets(id),
 version integer NOT NULL CHECK(version=1),
 entries bigint NOT NULL,
 indexed_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS research_keys (
 dataset_id smallint NOT NULL,
 id integer NOT NULL,
 source_hash bytea NOT NULL CHECK(octet_length(source_hash)=32),
 group_key bytea NOT NULL CHECK(octet_length(group_key)=32),
 sic text COLLATE "C" NOT NULL,
 industry text COLLATE "C" NOT NULL,
 PRIMARY KEY(dataset_id,id)
) PARTITION BY LIST(dataset_id);
CREATE INDEX IF NOT EXISTS research_source ON research_keys(dataset_id,source_hash,id);
CREATE INDEX IF NOT EXISTS research_groups ON research_keys(group_key,dataset_id,id);
CREATE INDEX IF NOT EXISTS research_sic ON research_keys(dataset_id,sic,id) WHERE sic<>'';
CREATE INDEX IF NOT EXISTS research_industry ON research_keys(dataset_id,industry,id) WHERE industry<>'';

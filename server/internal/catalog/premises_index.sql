-- Optional additive projection. Original records remain authoritative.
CREATE TABLE IF NOT EXISTS premises_keys (
 dataset_id smallint NOT NULL,
 id integer NOT NULL,
 premises_key bytea CHECK(premises_key IS NULL OR octet_length(premises_key)=32),
 PRIMARY KEY(dataset_id,id)
) PARTITION BY LIST(dataset_id);
CREATE INDEX IF NOT EXISTS premises_lookup ON premises_keys(dataset_id,premises_key,id) WHERE premises_key IS NOT NULL;
CREATE TABLE IF NOT EXISTS premises_indexes (
 dataset_id smallint PRIMARY KEY REFERENCES datasets(id),
 version integer NOT NULL CHECK(version=1),
 entries bigint NOT NULL,
 matchable bigint NOT NULL,
 indexed_at timestamptz NOT NULL DEFAULT now()
);

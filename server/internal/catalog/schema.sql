-- Schema 1: occurrences remain distinct; JSON preserves all source fields.
CREATE TABLE IF NOT EXISTS catalog_schema (version integer PRIMARY KEY);
INSERT INTO catalog_schema VALUES (1) ON CONFLICT DO NOTHING;
CREATE TABLE IF NOT EXISTS datasets (
    id smallint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    key text NOT NULL UNIQUE,
    manifest jsonb NOT NULL,
    entries bigint NOT NULL,
    imported_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS entries (
    dataset_id smallint NOT NULL REFERENCES datasets(id),
    id bigint NOT NULL,
    source_id text NOT NULL,
    kind text NOT NULL CHECK (kind IN ('person','business')),
    surname text COLLATE "C" NOT NULL,
    first_name text COLLATE "C" NOT NULL,
    organisation text COLLATE "C" NOT NULL,
    postcode text COLLATE "C" NOT NULL,
    record jsonb NOT NULL,
    PRIMARY KEY (dataset_id, id),
    UNIQUE (dataset_id, source_id)
);
CREATE INDEX IF NOT EXISTS entries_people ON entries
    (dataset_id, surname, first_name, id) WHERE kind = 'person';
CREATE INDEX IF NOT EXISTS entries_business ON entries
    (dataset_id, organisation, id) WHERE kind = 'business';
CREATE INDEX IF NOT EXISTS entries_postcode ON entries
    (dataset_id, postcode, surname, first_name, id) WHERE kind = 'person';
CREATE INDEX IF NOT EXISTS entries_business_postcode ON entries
    (dataset_id, postcode, organisation, id) WHERE kind = 'business';

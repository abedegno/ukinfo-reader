-- Optional additive index. Publication is atomic per edition; source blocks are unchanged.
CREATE TABLE IF NOT EXISTS address_locations (
 id integer GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 key text COLLATE "C" NOT NULL UNIQUE,
 street text COLLATE "C" NOT NULL,
 street_2 text COLLATE "C" NOT NULL,
 locality text COLLATE "C" NOT NULL,
 locality_2 text COLLATE "C" NOT NULL,
 town text COLLATE "C" NOT NULL,
 county text COLLATE "C" NOT NULL
);
CREATE INDEX IF NOT EXISTS address_street ON address_locations(street);
CREATE INDEX IF NOT EXISTS address_street_2 ON address_locations(street_2);
CREATE INDEX IF NOT EXISTS address_locality ON address_locations(locality);
CREATE INDEX IF NOT EXISTS address_locality_2 ON address_locations(locality_2);
CREATE INDEX IF NOT EXISTS address_town ON address_locations(town);
CREATE INDEX IF NOT EXISTS address_county ON address_locations(county);
CREATE TABLE IF NOT EXISTS address_members (
 dataset_id smallint NOT NULL REFERENCES datasets(id),
 location_id integer NOT NULL REFERENCES address_locations(id),
 ids integer[] NOT NULL,
 PRIMARY KEY(dataset_id,location_id)
);
CREATE TABLE IF NOT EXISTS address_indexes (
 dataset_id smallint PRIMARY KEY REFERENCES datasets(id),
 version integer NOT NULL CHECK(version=1),
 entries bigint NOT NULL,
 locations bigint NOT NULL,
 indexed_at timestamptz NOT NULL DEFAULT now()
);

-- Optional versioned postcode reference; source occurrence tables are unchanged.
CREATE TABLE IF NOT EXISTS geography_releases (
    release text PRIMARY KEY,
    source_url text NOT NULL,
    sha256 text NOT NULL,
    scope text NOT NULL CHECK (scope = 'GB'),
    entries bigint NOT NULL,
    located bigint NOT NULL,
    excluded_ni bigint NOT NULL,
    excluded_other bigint NOT NULL,
    imported_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS geography_active (
    singleton boolean PRIMARY KEY CHECK(singleton),
    release text NOT NULL REFERENCES geography_releases(release)
);
CREATE TABLE IF NOT EXISTS postcode_locations (
    release text NOT NULL,
    postcode text NOT NULL,
    introduced text NOT NULL,
    terminated text NOT NULL,
    country text NOT NULL,
    quality smallint NOT NULL,
    latitude double precision,
    longitude double precision,
    easting integer,
    northing integer,
    position public.cube GENERATED ALWAYS AS (
        CASE WHEN latitude IS NULL THEN NULL ELSE public.cube(ARRAY[
            cos(radians(latitude))*cos(radians(longitude)),
            cos(radians(latitude))*sin(radians(longitude)),
            sin(radians(latitude))]) END
    ) STORED,
    PRIMARY KEY(release,postcode),
    CHECK ((latitude IS NULL) = (longitude IS NULL)),
    CHECK (latitude BETWEEN -90 AND 90 AND longitude BETWEEN -180 AND 180),
    CHECK (quality BETWEEN 1 AND 9)
) PARTITION BY LIST(release);
CREATE INDEX IF NOT EXISTS postcode_locations_position ON postcode_locations USING gist(position);

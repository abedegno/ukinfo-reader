# ONSPD postcode reference and nearby search

The optional geographic layer uses the **ONS Postcode Directory (ONSPD)**. Source
addresses and historical source JSON remain unchanged. Reference coordinates, quality,
release and distances are separate enrichment fields, not reconstructed house positions.

## Pinned source

- [ONS Postcode Directory, August 2026](https://geoportal.statistics.gov.uk/datasets/9e5a92a3cfb14dc7ad43d6ea7a7b8c7f/about).
- Publisher: ONS Geography (`ONSGeography_data`).
- Official archive: [ArcGIS item data](https://www.arcgis.com/sharing/rest/content/items/9e5a92a3cfb14dc7ad43d6ea7a7b8c7f/data).
- Archive: **253,928,895 bytes**, including the **7 September 2026 header correction**.
- SHA-256: `ab51f1e82bd5beeb1089c3732ea6e11d62eb5693d619e2c4f4dd8c4baaa69293`.
- CSV read directly inside the ZIP: `Data/ONSPD_AUG_2026_UK.csv`.
- [Official release user guide](https://geoportal.statistics.gov.uk/datasets/920df475305b4b6baa9c3cab10813ca7/about).

This is ONSPD, not the NSPL best-fit lookup or the current-only live postcode service.
The ZIP is kept in ignored local storage, not committed or bundled in the app.

## Import

Use PostgreSQL 18 with the supplied `cube` extension. The existing hosted Alpine and
staged native development runtimes include it. Migrations enable it in `public` using
the owner connection. This is an additive schema-2 migration; source partitions are
not rewritten. Run migrations as the trusted database owner.

```sh
mkdir -p private/geography
curl --fail --location \
  --output private/geography/ONSPD_AUG_2026.zip \
  'https://www.arcgis.com/sharing/rest/content/items/9e5a92a3cfb14dc7ad43d6ea7a7b8c7f/data'

# DATABASE_URL must use an owner connection, as for normal edition imports.
build/desktop/ukinfo-server import-onspd \
  --onspd-zip private/geography/ONSPD_AUG_2026.zip
```

The default release, expected hash and source URL are pinned in the executable. Hashes
are checked before and after decoding. ZIP CRCs, CSV structure and duplicate postcode
identities are checked too. Invalid/truncated inputs roll back. No bulk CSV extraction
or temporary JSON export is needed. The importer reads the single UK CSV, not both it
and the duplicate per-area CSVs.

The importer retains a 5 GiB local filesystem reserve and monitors it throughout the
job, including indexing. This checks the ZIP/tmp host, not a separate remote database
volume or the Docker VM's internal free space; monitor that host too. Allow room for
WAL and index construction beyond the final reference size.

For desktop provisioning, close the app and use its staged or bundled `ukinfo-server`
with `--data-dir /path/to/portable-archive/catalogue --pg-bin /path/to/pg18/bin` instead
of `DATABASE_URL`. The app's normal PostgreSQL owner must be stopped first. Geographic
reference import is currently a provisioning command, not a GUI file-picker workflow.
After provisioning, reopen the app: the reference and searches work offline.

A later monthly release can be selected with `--release YYYY-MM --sha256 HASH
--source-url HTTPS_URL`. Supply the official ZIP for that month and verify its schema.
Each release imports into an indexed standalone table and is attached/activated in one
transaction. Older releases remain intact. Re-importing the same month is rejected;
there is no automatic same-month correction/replace or deletion workflow.

Serving uses SELECT only:

```sql
GRANT SELECT ON geography_releases, geography_active, postcode_locations TO ukinfo_search;
```

## Coverage and licence

Only Great Britain is imported. The validated source produced:

| Item | Count |
| --- | ---: |
| Imported GB postcodes, current and retired | 2,652,624 |
| Usable for nearby lookup | 2,573,343 |
| Excluded Northern Ireland rows | 63,293 |
| Excluded other-country/Crown dependency rows | 13,173 |

The reference occupies approximately **840 MiB**, including its postcode and spatial
indexes. Usable lookup excludes positional quality 6 (non-geographic/PO-box positions),
quality 9 (unlocated), and missing/invalid coordinates. These exclusions do not remove
any occurrences from the original archive or from ordinary name/postcode searches.

Great Britain reuse requires the source owners' attribution. Northern Ireland data
has separate licensing conditions and is deliberately unsupported by this importer.
See [ONS licensing](https://www.ons.gov.uk/methodology/geography/licences/).
The UI displays these statements when the reference is available:

- Contains OS data © Crown copyright and database right 2026.
- Contains Royal Mail data © Royal Mail copyright and database right 2026.
- Source: Office for National Statistics licensed under the Open Government Licence v3.0.

The project's MIT licence does not relicense the reference data.

## Search, accuracy and pagination

Expand **Search near a postcode**, enter a full centre postcode, and select a radius.
A name is optional. The UI offers 1/2/5/10/25 miles; the API accepts 50–50,000 integer
metres. Existing name, business, edition and postcode filters can be combined with it.

```text
/api/search?dataset=all&near=WS137SW&radius_m=8047&surname=Smith
```

Results include approximate distance, coordinate positional quality and postcode
termination date where present. The response and original JSONL exports include the
reference release/hash/source. Readable CSV includes distance and coordinate source.
The original decoder record is never modified.

Coordinates are representative postcode points, not addresses or postcode polygons.
Distances are great-circle estimates on a sphere with mean radius 6,371,008.8 metres,
not road distances or WGS84 ellipsoidal measurements. Quality 5 is imputed; quality 8
is a retired postcode's last known location. Displayed distance precision is not a
claim about positional accuracy. A valid coordinate can cover multiple addresses.

Historical postcode reuse and movement can make a modern point misleading for an
older listing. Termination dates and quality are retained, but the lookup does not
claim to reconstruct geography at the cover-disc date. Missing coordinates stay
unlocated. **Only the displayed source postcode is used**: a hidden/raw full postcode
never upgrades a partial source postcode to a precise point. A retired centre is
supported when it has a usable coordinate and is labelled as retired.

Results retain stable catalogue order, **not nearest-first order**. Nearby paging uses
postcode ID, name/first-name codes, import ordinal and dataset ID, with existing keyset
cursors. Radius-only queries visit 512-postcode batches in that order; empty batches
are traversed, not treated as the end of results. This bounds entry candidate work
without truncating nearby postcodes. The active reference release is part of a nearby
cursor's fingerprint; changing it requires restarting the search.

## Implementation and operating limits

`postcode_locations` stores unit-sphere XYZ points using PostgreSQL's bundled
[`cube` GiST index](https://www.postgresql.org/docs/18/cube.html). An indexed bounding
cube selects candidate postcode points, followed by an exact chord-distance predicate
on that sphere. Matching postcode IDs join the existing archive postcode index.
The `earthdistance` and PostGIS extensions are not required by this implementation.

A shared cache contains only postcode links/distances and reference metadata, never
names or directory records. It retains at most eight completed lookups and 500,000
postcode links in total; oversized results still work but are not retained. Concurrent
requests for the same circle share its lookup. New edition imports invalidate links
through the catalogue generation, and new reference releases use separate keys.

Ordinary search deadlines remain two seconds. Nearby requests have a five-second total
budget; uncached spatial SQL has a four-second deadline in a read-only transaction.
At most two uncached spatial lookups run per API process, preserving database capacity
for other searches. Entry SQL retains the serving connection's two-second deadline.
The existing eight-connection pool, 32-request admission and maximum 100-result page
remain. Large/cold circles cost more than cached or exact searches; failures are errors,
never silently incomplete pages. Validate actual hosting capacity before deployment.

See [reference and radius checks](research/onspd-nearby.md) for the measured limits.

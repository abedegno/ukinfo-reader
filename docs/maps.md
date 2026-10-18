# Optional postcode maps

Use **Map this page**, **Show postcode map** in a record, or **Map selected** in
research lists. Maps use the catalogue's existing ONSPD reference and do not require
another directory index or remote geocoding service.

Markers represent approximate **postcode positions**, not houses. Multiple source
listings at one postcode share a marker; its popup/location list retains their names
and editions. Green markers contain one edition, blue markers multiple editions.
The list includes latitude/longitude, ONSPD position quality and retired-postcode date.
It remains usable with a keyboard and when the optional street background fails.

A nearby search also shows its radius and centre. District/sector centres are explicitly
labelled as approximations from postcode points, including retired-only fallback where
applicable. The radius is a circle from that centre, not a district/sector boundary.
Source listings with incomplete postcodes are not assigned a house position using raw
or hidden fields. Unknown positions, Northern Ireland and unusable position qualities
are excluded and counted. **Map this page** covers only the current bounded result page.

## Offline and online behaviour

The map library (Leaflet 1.9.4, BSD-2-Clause) is bundled locally with its license and
checked against upstream published integrity hashes. It loads only when a map is opened.
Coordinates resolve against the local PostgreSQL reference; no names or addresses are
sent to a geocoder. No ONSPD dataset or map tiles are bundled in release packages.
[Import postcode positions](geography.md) separately if they are absent.

The **Street-map background (online)** checkbox is off initially. Turning it on loads
only the interactive viewport's raster tiles. The provider receives tile coordinates,
your IP address and an origin-only referrer; names, saved notes and search query strings
are not sent. Turning it off removes that layer. Closing the view disposes the map.
There is no tile prefetch, bulk download or offline tile-pack feature.

The default background is OpenStreetMap's standard tile service, with visible licence
attribution and normal browser HTTP caching. It is a best-effort community service,
not a guaranteed free hosting plan or SLA. Public deployments must review the current
[OSMF tile policy](https://operations.osmfoundation.org/policies/tiles/) and move to an
appropriate provider as traffic requires. See [Leaflet's official download/hashes](https://leafletjs.com/download.html).

Operators can switch an OSM-derived raster provider without rebuilding:

```sh
export UKINFO_MAP_TILES_URL='https://tiles.example.org/{z}/{x}/{y}.png'
export UKINFO_MAP_ATTRIBUTION='© OpenStreetMap contributors · Example provider'
```

Use a valid HTTPS XYZ template and the required provider attribution. Any client-side
provider token in the URL is public. The server adds only that tile origin to `img-src`;
no third-party scripts or styles are enabled. `UKINFO_MAP_TILES_URL=none` disables online
backgrounds. The native app inherits these settings and identifies itself in its
User-Agent. Browser tiles explicitly use an origin-only referrer; the application keeps
its ordinary no-referrer policy elsewhere.

## API and validation

`GET /api/map?postcode=WS137SW&postcode=WS137SE` resolves 1–100 full displayed postcodes.
Duplicates normalize to one lookup. The response has `reference`, `points`, `unlocated`
and a precision notice. Each point includes postcode, latitude, longitude, quality and
termination date. Invalid/partial codes return 400; unavailable reference/data or busy
admission returns 503. The lookup shares two geography slots and a two-second deadline,
using the existing release/postcode index and read-only role.

Invented backend fixtures cover duplicates, retired postcodes, excluded qualities,
Northern Ireland, partial codes and request limits. Browser tests cover postcode
aggregation, radius/centre labels, partial-code exclusion, tile opt-in, origin-only
referrers, failure behaviour, attribution and mobile width. Automated tests intercept
tile requests; they do not download tiles from the community service.

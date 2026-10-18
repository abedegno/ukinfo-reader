package catalog

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed geography.sql
var geographySchema string

const ONSPDRelease = "2026-08"
const ONSPDSHA256 = "ab51f1e82bd5beeb1089c3732ea6e11d62eb5693d619e2c4f4dd8c4baaa69293"
const ONSPDURL = "https://www.arcgis.com/sharing/rest/content/items/9e5a92a3cfb14dc7ad43d6ea7a7b8c7f/data"
const earthRadius = 6371008.8 // IUGG mean Earth radius; spherical postcode-point distances.
const geographyNotice = "Approximate postcode locations, not house positions. Modern reference coordinates may differ from historical locations. Great Britain coverage; Northern Ireland is excluded."

type GeographyRelease struct {
	Release       string    `json:"release"`
	SourceURL     string    `json:"source_url"`
	SHA256        string    `json:"sha256"`
	Scope         string    `json:"scope"`
	Entries       int64     `json:"entries"`
	Located       int64     `json:"located"`
	ExcludedNI    int64     `json:"excluded_ni"`
	ExcludedOther int64     `json:"excluded_other"`
	ImportedAt    time.Time `json:"imported_at"`
	Generation    int16     `json:"-"`
}
type GeographySearch struct {
	Reference         GeographyRelease `json:"reference"`
	Near              string           `json:"near"`
	CentreType        string           `json:"centre_type"`
	CentrePostcodes   int              `json:"centre_postcodes"`
	CentreRetiredOnly bool             `json:"centre_retired_only"`
	Radius            int              `json:"radius_m"`
	Latitude          float64          `json:"latitude"`
	Longitude         float64          `json:"longitude"`
	Quality           int16            `json:"quality"`
	Terminated        string           `json:"terminated"`
	Notice            string           `json:"notice"`
}
type GeographyImportOptions struct {
	Release, SHA256, SourceURL string
	CheckResources             func() error
	Progress                   func(Progress)
}

func migrateGeography(ctx context.Context, tx pgx.Tx) error {
	// cube is shipped with the hosted and native PostgreSQL runtimes. All calls
	// are qualified, so temporary catalogue schemas need not change search_path.
	if _, err := tx.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS cube WITH SCHEMA public"); err != nil {
		return fmt.Errorf("postcode geography requires the PostgreSQL cube extension: %w", err)
	}
	_, err := tx.Exec(ctx, geographySchema)
	return err
}

func fileSHA256(path string) (string, error) {
	return fileSHA256Context(context.Background(), path)
}

func fileSHA256Context(ctx context.Context, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	buffer := make([]byte, 1<<20)
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := f.Read(buffer)
		if n > 0 {
			_, _ = h.Write(buffer[:n])
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ImportGeography loads one verified ONSPD ZIP without extracting its CSVs.
// Great Britain is intentionally the only scope; BT data has separate licence terms.
func ImportGeography(ctx context.Context, db *pgxpool.Pool, path string, options GeographyImportOptions) (report GeographyRelease, err error) {
	report = GeographyRelease{Release: options.Release, SourceURL: options.SourceURL, SHA256: strings.ToLower(options.SHA256), Scope: "GB"}
	ctx, cancel := context.WithCancelCause(ctx)
	var watcherDone chan struct{}
	defer func() {
		cancel(nil)
		if watcherDone != nil {
			<-watcherDone
		}
	}()
	defer func() {
		if err != nil && ctx.Err() != nil {
			err = context.Cause(ctx)
		}
	}()
	var resourceMu sync.Mutex
	checkResources := func() error {
		resourceMu.Lock()
		defer resourceMu.Unlock()
		if options.CheckResources != nil {
			return options.CheckResources()
		}
		return nil
	}
	if options.CheckResources != nil {
		watcherDone = make(chan struct{})
		go func() {
			defer close(watcherDone)
			ticker := time.NewTicker(2 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if e := checkResources(); e != nil {
						cancel(e)
						return
					}
				}
			}
		}()
	}
	date, err := time.Parse("2006-01", report.Release)
	if err != nil || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(report.SHA256) || !strings.HasPrefix(report.SourceURL, "https://") {
		return report, errors.New("provide release YYYY-MM, expected SHA-256 and HTTPS source URL")
	}
	check := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if options.CheckResources != nil {
			return checkResources()
		}
		return nil
	}
	if err = check(); err != nil {
		return report, err
	}
	if options.Progress != nil {
		options.Progress(Progress{Phase: "verifying postcode archive"})
	}
	hash, err := fileSHA256Context(ctx, path)
	if err != nil {
		return report, err
	}
	if hash != report.SHA256 {
		return report, errors.New("ONSPD ZIP SHA-256 mismatch")
	}
	archive, err := zip.OpenReader(path)
	if err != nil {
		return report, err
	}
	defer archive.Close()
	filename := "Data/ONSPD_" + strings.ToUpper(date.Format("Jan_2006")) + "_UK.csv"
	var source *zip.File
	for _, entry := range archive.File {
		if entry.Name == filename {
			if source != nil {
				return report, errors.New("duplicate ONSPD CSV")
			}
			source = entry
		}
	}
	if source == nil || source.UncompressedSize64 > 2<<30 {
		return report, errors.New("expected single UK ONSPD CSV missing or oversized")
	}
	input, err := source.Open()
	if err != nil {
		return report, err
	}
	defer input.Close()
	csvReader := csv.NewReader(io.LimitReader(input, 2<<30))
	header, err := csvReader.Read()
	if err != nil {
		return report, err
	}
	columns := map[string]int{}
	for i, name := range header {
		name = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(name)), "\ufeff")
		if _, ok := columns[name]; ok {
			return report, errors.New("duplicate ONSPD column")
		}
		columns[name] = i
	}
	find := func(names ...string) int {
		for _, name := range names {
			if i, ok := columns[name]; ok {
				return i
			}
		}
		return -1
	}
	pc, lat, lon := find("pcd7", "pcd"), find("lat"), find("long")
	country, quality, east, north := find("ctry"), find("gridind", "osgrdind"), find("east1m", "oseast1m"), find("north1m", "osnrth1m")
	if country < 0 {
		for name, i := range columns {
			if regexp.MustCompile(`^ctry[0-9]{2}cd$`).MatchString(name) {
				country = i
			}
		}
	}
	introduced, terminated := find("dointr"), find("doterm")
	for _, i := range []int{pc, lat, lon, country, quality, east, north, introduced, terminated} {
		if i < 0 {
			return report, errors.New("unsupported ONSPD CSV headers")
		}
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		return report, err
	}
	defer tx.Rollback(context.Background())
	// Ingestion/index creation is bounded by the caller's cancellable context,
	// not the serving pool's short search-statement deadline. SET LOCAL ensures
	// ordinary queries keep their deadline when this connection is returned.
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout=0"); err != nil {
		return report, err
	}
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(742918305)"); err != nil {
		return report, err
	}
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM geography_releases WHERE release=$1)", report.Release).Scan(&exists); err != nil {
		return report, err
	}
	if exists {
		return report, errors.New("ONSPD release already imported; existing reference unchanged")
	}
	table := pgx.Identifier{"onspd_" + date.Format("200601")}
	if _, err = tx.Exec(ctx, fmt.Sprintf("CREATE TABLE %s (LIKE postcode_locations INCLUDING GENERATED INCLUDING CONSTRAINTS)", table.Sanitize())); err != nil {
		return report, err
	}
	pending := make([][]any, 0, importBatchRows)
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		if err := check(); err != nil {
			return err
		}
		_, err := tx.CopyFrom(ctx, table, []string{"release", "postcode", "introduced", "terminated", "country", "quality", "latitude", "longitude", "easting", "northing"}, pgx.CopyFromRows(pending))
		pending = pending[:0]
		return err
	}
	integer := func(s string) (any, error) {
		s = strings.TrimSpace(s)
		if s == "" {
			return nil, nil
		}
		n, err := strconv.ParseInt(s, 10, 32)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			return nil, nil
		}
		return int32(n), nil
	}
	var sourceRows int64
	for {
		row, e := csvReader.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			return report, fmt.Errorf("ONSPD CSV: %w", e)
		}
		sourceRows++
		if sourceRows > 10000000 {
			return report, errors.New("too many ONSPD rows")
		}
		c := strings.TrimSpace(row[country])
		code := Postcode(row[pc])
		if c == "N92000002" || strings.HasPrefix(code, "BT") {
			report.ExcludedNI++
			continue
		}
		if c != "E92000001" && c != "W92000004" && c != "S92000003" {
			report.ExcludedOther++
			continue
		}
		if len(code) < 5 || len(code) > 8 || strings.ContainsAny(code, "\x00\n\r") {
			return report, errors.New("invalid ONSPD postcode")
		}
		pq, e := strconv.ParseInt(strings.TrimSpace(row[quality]), 10, 16)
		if e != nil || pq < 1 || pq > 9 {
			return report, errors.New("invalid ONSPD positional quality")
		}
		var latitude, longitude any
		a, e1 := strconv.ParseFloat(strings.TrimSpace(row[lat]), 64)
		b, e2 := strconv.ParseFloat(strings.TrimSpace(row[lon]), 64)
		if pq != 9 && e1 == nil && e2 == nil && !math.IsNaN(a) && !math.IsNaN(b) && a >= -90 && a <= 90 && b >= -180 && b <= 180 {
			latitude = a
			longitude = b
			if pq != 6 {
				report.Located++
			}
		}
		x, e := integer(row[east])
		if e != nil {
			return report, errors.New("invalid ONSPD easting")
		}
		y, e := integer(row[north])
		if e != nil {
			return report, errors.New("invalid ONSPD northing")
		}
		report.Entries++
		pending = append(pending, []any{report.Release, code, strings.TrimSpace(row[introduced]), strings.TrimSpace(row[terminated]), c, int16(pq), latitude, longitude, x, y})
		if len(pending) == cap(pending) {
			if err = flush(); err != nil {
				return report, err
			}
			if options.Progress != nil {
				options.Progress(Progress{Phase: "loading postcodes", Done: report.Entries})
			}
		}
	}
	if err = flush(); err != nil {
		return report, err
	}
	if report.Entries == 0 {
		return report, errors.New("no Great Britain postcodes in source")
	}
	if err = check(); err != nil {
		return report, err
	}
	if options.Progress != nil {
		options.Progress(Progress{Phase: "checking postcode archive", Done: report.Entries})
	}
	hash, err = fileSHA256Context(ctx, path)
	if err != nil {
		return report, err
	}
	if hash != report.SHA256 {
		return report, errors.New("ONSPD source changed during import")
	}
	if options.Progress != nil {
		options.Progress(Progress{Phase: "indexing postcodes", Done: report.Entries})
	}
	for _, sql := range []string{fmt.Sprintf("ALTER TABLE %s ADD PRIMARY KEY(release,postcode)", table.Sanitize()), fmt.Sprintf("CREATE INDEX ON %s USING gist(position)", table.Sanitize()), fmt.Sprintf("ALTER TABLE %s ADD CHECK(release='%s')", table.Sanitize(), report.Release), fmt.Sprintf("ANALYZE %s", table.Sanitize())} {
		if _, err = tx.Exec(ctx, sql); err != nil {
			return report, err
		}
		if err = check(); err != nil {
			return report, err
		}
	}
	if err = tx.QueryRow(ctx, `INSERT INTO geography_releases(release,source_url,sha256,scope,entries,located,excluded_ni,excluded_other) VALUES($1,$2,$3,$4,$5,$6,$7,$8) RETURNING imported_at`, report.Release, report.SourceURL, report.SHA256, report.Scope, report.Entries, report.Located, report.ExcludedNI, report.ExcludedOther).Scan(&report.ImportedAt); err != nil {
		return report, err
	}
	if _, err = tx.Exec(ctx, fmt.Sprintf("ALTER TABLE postcode_locations ATTACH PARTITION %s FOR VALUES IN('%s')", table.Sanitize(), report.Release)); err != nil {
		return report, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO geography_active VALUES(true,$1) ON CONFLICT(singleton) DO UPDATE SET release=excluded.release", report.Release); err != nil {
		return report, err
	}
	if err = tx.Commit(ctx); err != nil {
		return report, err
	}
	return report, nil
}

func activeGeography(ctx context.Context, db *pgxpool.Pool) (GeographyRelease, error) {
	var release GeographyRelease
	err := db.QueryRow(ctx, `SELECT g.release,g.source_url,g.sha256,g.scope,g.entries,g.located,g.excluded_ni,g.excluded_other,g.imported_at,COALESCE((SELECT max(id) FROM datasets),0) FROM geography_releases g JOIN geography_active a USING(release)`).Scan(&release.Release, &release.SourceURL, &release.SHA256, &release.Scope, &release.Entries, &release.Located, &release.ExcludedNI, &release.ExcludedOther, &release.ImportedAt, &release.Generation)
	return release, err
}
func (a *API) geography(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	reference, err := activeGeography(ctx, a.DB)
	if errors.Is(err, pgx.ErrNoRows) {
		respond(w, 200, map[string]any{"available": false, "notice": "Nearby search needs an imported ONSPD reference."})
		return
	}
	if err != nil {
		fail(w, 503, "Postcode reference unavailable; run migrate and import-onspd with an owner connection")
		return
	}
	respond(w, 200, map[string]any{"available": true, "reference": reference, "notice": geographyNotice})
}
func unitPosition(lat, lon float64) []float64 {
	a, b := lat*math.Pi/180, lon*math.Pi/180
	return []float64{math.Cos(a) * math.Cos(b), math.Cos(a) * math.Sin(b), math.Sin(a)}
}

type geographyError struct {
	Status  int
	Message string
}

func (e *geographyError) Error() string { return e.Message }
func geographyUnavailable() error {
	return &geographyError{503, "Nearby postcode lookup unavailable or timed out; retry shortly"}
}

type postcodeDistance struct {
	Distance   float64
	Quality    int16
	Terminated string
}

func (a *API) nearby(ctx context.Context, q *query) (map[int32]postcodeDistance, *GeographySearch, error) {
	reference, err := activeGeography(ctx, a.DB)
	if err != nil {
		return nil, nil, &geographyError{503, "Nearby search needs an imported ONSPD reference"}
	}
	q.GeoRelease = reference.Release
	return a.geographyCache.lookup(ctx, geographyKey{release: reference.Release, near: q.Near, radius: q.Radius, nearLength: q.NearLength, generation: reference.Generation}, func() (map[int32]postcodeDistance, *GeographySearch, error) { return a.loadNearby(ctx, q, reference) })
}
func (a *API) loadNearby(ctx context.Context, q *query, reference GeographyRelease) (map[int32]postcodeDistance, *GeographySearch, error) {
	select {
	case a.geographySlots <- struct{}{}:
		defer func() { <-a.geographySlots }()
	default:
		return nil, nil, &geographyError{503, "Nearby postcode lookup is busy; retry shortly"}
	}
	tx, err := a.DB.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, nil, geographyUnavailable()
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SET LOCAL statement_timeout='4000'"); err != nil {
		return nil, nil, geographyUnavailable()
	}
	info := &GeographySearch{Reference: reference, Near: q.Near, Radius: q.Radius, Notice: geographyNotice, CentreType: "postcode", CentrePostcodes: 1}
	if q.NearLength == 0 {
		err = tx.QueryRow(ctx, `SELECT latitude,longitude,quality,terminated FROM postcode_locations WHERE release=$1 AND postcode=$2 AND position IS NOT NULL AND quality NOT IN (6,9)`, reference.Release, q.Near).Scan(&info.Latitude, &info.Longitude, &info.Quality, &info.Terminated)
	} else {
		info.CentreType = "district"
		if q.NearLength == len(q.Near)+2 {
			info.CentreType = "sector"
			info.Near = q.Near[:len(q.Near)-1] + " " + q.Near[len(q.Near)-1:]
		}
		// Equal weight per usable postcode point, not per resident or area. Prefer
		// current postcodes; retain a documented fallback for wholly retired areas.
		err = tx.QueryRow(ctx, `WITH candidates AS MATERIALIZED (
   SELECT latitude,longitude,terminated FROM postcode_locations
   WHERE release=$1 AND postcode >= $2 AND postcode < $3
   AND length(postcode)=$4 AND position IS NOT NULL AND quality NOT IN (6,9)
  ), selected AS (
   SELECT * FROM candidates WHERE terminated='' OR NOT EXISTS
    (SELECT 1 FROM candidates WHERE terminated='')
  ) SELECT avg(latitude),avg(longitude),count(*)::integer,bool_and(terminated<>'')
    FROM selected HAVING count(*)>0`, reference.Release, q.Near, q.Near+"~", q.NearLength).
			Scan(&info.Latitude, &info.Longitude, &info.CentrePostcodes, &info.CentreRetiredOnly)
	}

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil, &geographyError{400, "Centre postcode has no usable Great Britain location in this ONSPD release"}
		}
		return nil, nil, geographyUnavailable()
	}

	origin := unitPosition(info.Latitude, info.Longitude)
	chord := 2 * math.Sin(float64(q.Radius)/(2*earthRadius))
	rows, err := tx.Query(ctx, `SELECT t.id,public.cube_distance(p.position,public.cube($2::double precision[])),p.quality,p.terminated FROM postcode_locations p JOIN terms t ON t.key=p.postcode WHERE p.release=$1 AND p.quality NOT IN (6,9) AND p.position OPERATOR(public.<@) public.cube_enlarge(public.cube($2::double precision[]),$3,3) AND public.cube_distance(p.position,public.cube($2::double precision[])) <= $3`, reference.Release, origin, chord)
	if err != nil {
		return nil, nil, geographyUnavailable()
	}
	defer rows.Close()
	distances := map[int32]postcodeDistance{}
	for rows.Next() {
		var id int32
		var d float64
		var quality int16
		var terminated string
		if err = rows.Scan(&id, &d, &quality, &terminated); err != nil {
			return nil, nil, geographyUnavailable()
		}
		distances[id] = postcodeDistance{2 * earthRadius * math.Asin(math.Min(1, d/2)), quality, terminated}
	}
	if err = rows.Err(); err != nil {
		return nil, nil, geographyUnavailable()
	}
	return distances, info, nil
}

package catalog

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestNearbyParse(t *testing.T) {
	for _, raw := range []string{"dataset=all&near=WS13", "dataset=all&near=WS13+7", "dataset=all&near=AA1+1AA", "dataset=all&kind=business&near=AA11AA&radius_m=1000"} {
		if _, err := parse(mustValues(raw)); err != nil {
			t.Fatal(err)
		}
	}
	for _, raw := range []string{"dataset=all&near=???", "dataset=all&near=AA11AA&radius_m=0", "dataset=all&near=AA11AA&radius_m=50001", "dataset=all&near=AA11AA&radius_m=NaN", "dataset=all&surname=Smith&radius_m=1000"} {
		if _, err := parse(mustValues(raw)); err == nil {
			t.Fatal("accepted invalid radius search", raw)
		}
	}
	for _, tc := range []struct {
		input  string
		length int
	}{{"W11", 6}, {"W1+1", 5}, {"WS13", 7}, {"WS13+7", 7}, {"WS137SW", 0}} {
		q, err := parse(mustValues("dataset=all&near=" + tc.input))
		if err != nil || q.NearLength != tc.length {
			t.Fatalf("centre boundary %s: %+v %v", tc.input, q, err)
		}
	}
	district, _ := parse(mustValues("dataset=all&near=W11"))
	sector, _ := parse(mustValues("dataset=all&near=W1+1"))
	if district.fingerprint() == sector.fingerprint() {
		t.Fatal("ambiguous centres share cursor")
	}
	p := unitPosition(51, -1)
	length := math.Sqrt(p[0]*p[0] + p[1]*p[1] + p[2]*p[2])
	if math.Abs(length-1) > 1e-12 {
		t.Fatal("unit sphere position", p)
	}
}
func testGeography(t *testing.T, db *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	fixture := func(release, rows string) (string, GeographyImportOptions) {
		path := filepath.Join(t.TempDir(), "onspd.zip")
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		z := zip.NewWriter(f)
		month := "AUG_2026"
		if release == "2026-09" {
			month = "SEP_2026"
		}
		w, err := z.Create("Data/ONSPD_" + month + "_UK.csv")
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprint(w, "pcd7,dointr,doterm,ctry26cd,gridind,lat,long,east1m,north1m\n"+rows)
		if err = z.Close(); err != nil {
			t.Fatal(err)
		}
		if err = f.Close(); err != nil {
			t.Fatal(err)
		}
		sha, err := fileSHA256(path)
		if err != nil {
			t.Fatal(err)
		}
		return path, GeographyImportOptions{Release: release, SHA256: sha, SourceURL: "https://example.org/invented-onspd"}
	}
	rows := "AA1 1AA,198001,,E92000001,1,51,-1,100,100\nBB1 1BB,198001,200102,E92000001,8,51.001,-1,100,101\nCC1 1CC,198001,,E92000001,1,52,-1,100,200\nDD1 1DD,198001,,E92000001,6,51,-1,100,100\nEE1 1EE,198001,,E92000001,9,99.999999,0,0,0\nBT1 1AA,198001,,N92000002,1,54,-6,100,100\nJE1 1AA,198001,,L93000001,1,49,-2,100,100\n"
	for i := 0; i < 600; i++ {
		rows += fmt.Sprintf("ZX1 0%c%c,198001,,E92000001,1,51,-1,100,100\n", 'A'+i/26, 'A'+i%26)
	}
	path, options := fixture("2026-08", rows)
	wrong := options
	wrong.SHA256 = strings.Repeat("0", 64)
	if _, err := ImportGeography(ctx, db, path, wrong); err == nil {
		t.Fatal("accepted wrong source hash")
	}
	report, err := ImportGeography(ctx, db, path, options)
	if err != nil || report.Entries != 605 || report.Located != 603 || report.ExcludedNI != 1 || report.ExcludedOther != 1 {
		t.Fatal("reference import", report, err)
	}
	if _, err = ImportGeography(ctx, db, path, options); err == nil {
		t.Fatal("duplicate reference replaced")
	}
	badPath, badOptions := fixture("2026-09", strings.Replace(rows, ",1,51,", ",0,51,", 1))
	if _, err = ImportGeography(ctx, db, badPath, badOptions); err == nil {
		t.Fatal("invalid quality was published")
	}
	current, err := activeGeography(ctx, db)
	if err != nil || current.Release != "2026-08" {
		t.Fatal("failed import changed active reference", current, err)
	}
	var badCount int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM geography_releases WHERE release='2026-09'").Scan(&badCount); err != nil || badCount != 0 {
		t.Fatal("failed reference import left metadata", badCount, err)
	}
	handler := NewAPI(db).Handler()
	fetch := func(path string, status int) []byte {
		t.Helper()
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != status {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		return w.Body.Bytes()
	}
	if !bytes.Contains(fetch("/api/geography", 200), []byte(`"available":true`)) {
		t.Fatal("missing reference capability")
	}
	var response struct {
		Items     []result         `json:"items"`
		Next      string           `json:"next_cursor"`
		Geography *GeographySearch `json:"geography"`
	}
	// Distinct district/sector centres, current-point preference, and retired fallback.
	if _, err = db.Exec(ctx, `INSERT INTO postcode_locations(release,postcode,introduced,terminated,country,quality,latitude,longitude) VALUES
 ('2026-08','W11AA','198001','','E92000001',1,51,-1),
 ('2026-08','W11AB','198001','','E92000001',1,51.002,-1),
 ('2026-08','W11AC','198001','200001','E92000001',8,53,-1),
 ('2026-08','W110AA','198001','','E92000001',1,52,-1)`); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		input, kind string
		lat         float64
		count       int
		retired     bool
	}{
		{"W11", "district", 52, 1, false}, {"W1+1", "sector", 51.001, 2, false},
		{"BB1", "district", 51.001, 1, true}, {"AA1+1", "sector", 51, 1, false},
	} {
		if err = json.Unmarshal(fetch("/api/search?dataset=invented&near="+tc.input, 200), &response); err != nil {
			t.Fatal(err)
		}
		g := response.Geography
		if g == nil || g.CentreType != tc.kind || g.CentrePostcodes != tc.count || math.Abs(g.Latitude-tc.lat) > 1e-9 || g.CentreRetiredOnly != tc.retired {
			t.Fatalf("partial centre %s: %+v", tc.input, g)
		}
	}
	fetch("/api/search?dataset=invented&near=ZZ99", 400)
	seen := map[string]bool{}
	cursor := ""
	for pages := 0; ; pages++ {
		if pages > 5 {
			t.Fatal("nearby paging did not terminate")
		}
		if err = json.Unmarshal(fetch("/api/search?dataset=invented&near=AA11AA&radius_m=500&limit=1&cursor="+cursor, 200), &response); err != nil {
			t.Fatal(err)
		}
		if response.Geography == nil || response.Geography.Reference.Release != "2026-08" {
			t.Fatal("lost coordinate provenance")
		}
		for _, item := range response.Items {
			if seen[item.SourceID] || item.Distance == nil || *item.Distance > 500 {
				t.Fatal("duplicate or out-of-radius occurrence", item)
			}
			seen[item.SourceID] = true
			if item.SourceID == "c" && (*item.Distance < 110 || *item.Distance > 113) {
				t.Fatal("great-circle distance", *item.Distance)
			}
		}
		cursor = response.Next
		if cursor == "" {
			break
		}
	}
	if len(seen) != 4 {
		t.Fatal("nearby occurrence count", len(seen))
	}
	fetch("/api/search?dataset=invented&near=DD11DD", 400)
	fetch("/api/search?dataset=invented&near=EE11EE", 400)
	fetch("/api/search?dataset=invented&near=BT11AA", 400)
	exported := fetch("/api/export?dataset=invented&near=AA11AA&radius_m=500&limit=1", 200)
	if !bytes.Contains(exported, []byte(`"distance_m":`)) || !bytes.Contains(exported, []byte(`"geography":`)) || !bytes.Contains(exported, []byte(`"building":null`)) {
		t.Fatal("nearby export lost source fields or provenance", string(exported))
	}
	// More than one postcode batch, with no matching occurrence in the first
	// batch. Later pages must seek directly to their postcode cursor.
	if _, err = db.Exec(ctx, `INSERT INTO terms(key) SELECT 'ZX10'||chr(65+i/26)||chr(65+i%26) FROM generate_series(0,599) i ORDER BY i ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	var batchInput bytes.Buffer
	batchEncoder := json.NewEncoder(&batchInput)
	_ = batchEncoder.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "spatial-batches", Edition: "invented-batches", Title: "Spatial batches", SHA256: map[string]string{"invented": "fake"}, Expected: 3})
	for i := 0; i < 3; i++ {
		n := 599 - i
		code := fmt.Sprintf("ZX1 0%c%c", 'A'+n/26, 'A'+n%26)
		_ = batchEncoder.Encode(map[string]any{"type": "record", "source_id": fmt.Sprint(i), "record": map[string]any{"surname": "Spatial", "postcode": code}})
	}
	_ = batchEncoder.Encode(map[string]any{"type": "end", "count": 3})
	if _, err = Import(ctx, db, &batchInput); err != nil {
		t.Fatal(err)
	}
	seenBatch := map[string]bool{}
	batchCursor := ""
	for n := 0; ; n++ {
		if n > 3 {
			t.Fatal("spatial batching failed to terminate")
		}
		if err = json.Unmarshal(fetch("/api/search?dataset=spatial-batches&near=AA11AA&radius_m=500&limit=1&cursor="+batchCursor, 200), &response); err != nil {
			t.Fatal(err)
		}
		for _, item := range response.Items {
			if seenBatch[item.SourceID] {
				t.Fatal("duplicate spatial batch occurrence")
			}
			seenBatch[item.SourceID] = true
		}
		batchCursor = response.Next
		if batchCursor == "" {
			break
		}
	}
	if len(seenBatch) != 3 {
		t.Fatal("spatial batches skipped matches or reused stale links", len(seenBatch))
	}
	// A source's hidden full postcode must never place its displayed partial postcode.
	var input bytes.Buffer
	enc := json.NewEncoder(&input)
	_ = enc.Encode(Manifest{Type: "manifest", Protocol: 1, Dataset: "partial-location", Edition: "invented", Title: "Partial location", SHA256: map[string]string{"invented": "fake"}, Expected: 1})
	_ = enc.Encode(map[string]any{"type": "record", "source_id": "partial", "record": map[string]any{"surname": "Partial", "postcode": "AA1", "postcode_raw": "AA11AA"}})
	_ = enc.Encode(map[string]any{"type": "end", "count": 1})
	if _, err = Import(ctx, db, &input); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(fetch("/api/search?dataset=partial-location&near=AA11AA", 200), &response); err != nil || len(response.Items) != 0 {
		t.Fatal("raw postcode silently geocoded", response, err)
	}
}

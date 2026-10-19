package catalog

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const premisesNotice = "Listings share the same recorded address components. They do not establish family relationships or continuous residence. Edition dates refer to cover discs; spelling or address variations may be missed."

type premisesIdentity struct {
	Postcode                     string
	House, Building, Subbuilding string
	Streets, Localities          []string
	Town, County                 string
}

func premises(row map[string]any) (premisesIdentity, string) {
	for _, field := range []string{"postcode", "street", "street_1", "street_2", "locality", "locality_1", "locality_2", "town", "county", "building", "subbuilding", "source_warning"} {
		if value, exists := row[field]; exists && value != nil {
			if _, ok := value.(string); !ok {
				return premisesIdentity{}, "A recorded address field has an unsupported value; its premises cannot be matched reliably."
			}
		}
	}
	if value, exists := row["houseno"]; exists && value != nil {
		switch value.(type) {
		case string, json.Number, float64:
		default:
			return premisesIdentity{}, "The recorded house identifier cannot be matched reliably."
		}
	}
	identity := premisesIdentity{Postcode: Postcode(Text(row, "postcode")), Building: Normalize(Text(row, "building")), Subbuilding: Normalize(Text(row, "subbuilding")), Town: Normalize(Text(row, "town")), County: Normalize(Text(row, "county"))}
	switch v := row["houseno"].(type) {
	case string:
		identity.House = Normalize(v)
	case json.Number:
		identity.House = v.String()
		if number, err := v.Float64(); err == nil && number == 0 {
			identity.House = ""
		}
	case float64:
		identity.House = strconv.FormatFloat(v, 'f', -1, 64)
	}
	if identity.House == "0" {
		identity.House = ""
	}
	components := func(values ...string) []string {
		result := []string{}
		for _, v := range values {
			v = Normalize(v)
			if v != "" && (len(result) == 0 || result[len(result)-1] != v) {
				result = append(result, v)
			}
		}
		return result
	}
	street := Text(row, "street")
	if street == "" {
		street = Text(row, "street_1")
	}
	locality := Text(row, "locality")
	if locality == "" {
		locality = Text(row, "locality_1")
	}
	identity.Streets = components(street, Text(row, "street_2"))
	identity.Localities = components(locality, Text(row, "locality_2"))
	if !fullPostcodeSearch.MatchString(identity.Postcode) {
		return identity, "A complete displayed postcode is needed to identify this address."
	}
	if len(identity.Streets) == 0 || (identity.House == "" && identity.Building == "") {
		return identity, "The listing needs a recorded street and house number or building name to identify its premises."
	}
	for _, warning := range strings.Split(Text(row, "source_warning"), ";") {
		if warning != "" && warning != "synthetic_demo_not_a_real_person" {
			return identity, "This source listing has an unresolved data warning; its premises cannot be matched reliably."
		}
	}
	if precision, ok := row["postcode_precision"]; ok && precision != nil {
		if value := fmt.Sprint(precision); value != "0" && value != "10" {
			return identity, "The source postcode has limited precision; its premises cannot be matched reliably."
		}
	}
	return identity, ""
}
func premisesRecord(raw json.RawMessage) (premisesIdentity, string, error) {
	var row map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&row); err != nil {
		return premisesIdentity{}, "", err
	}
	identity, reason := premises(row)
	return identity, reason, nil
}
func (p premisesIdentity) key() string { b, _ := json.Marshal(p); return string(b) }

type recordReference struct {
	Archive string
	Dataset string
	ID      int32
	Source  string
}

func parseRecordReference(v url.Values) (recordReference, error) {
	if format := v.Get("format"); format != "" && format != "jsonl" {
		return recordReference{}, errors.New("record format must be jsonl or omitted")
	}
	ref := recordReference{Archive: v.Get("archive_id"), Dataset: strings.TrimSpace(v.Get("dataset")), Source: v.Get("source_id")}
	if ref.Archive != "" {
		decoded, e := hex.DecodeString(ref.Archive)
		if e != nil || len(decoded) != 32 || ref.Source == "" {
			return ref, errors.New("Invalid archival source reference")
		}
		ref.Archive = strings.ToLower(ref.Archive)
	}

	id, err := strconv.ParseInt(v.Get("id"), 10, 32)
	if err != nil || id < 1 || ref.Dataset == "" || ref.Dataset == "all" || len(ref.Dataset) > 256 || len(ref.Source) > maxSourceIDBytes || strings.ContainsRune(ref.Dataset+ref.Source, 0) {
		return ref, errors.New("choose a source edition and a valid record link")
	}
	ref.ID = int32(id)
	return ref, nil
}
func (a *API) referencedRecord(ctx context.Context, ref recordReference) (result, int, error) {
	if ref.Archive != "" {
		return a.archivalRecord(ctx, ref)
	}
	var dataset int16
	var count int64
	err := a.DB.QueryRow(ctx, "SELECT id,entries FROM datasets WHERE key=$1", ref.Dataset).Scan(&dataset, &count)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && int64(ref.ID) > count) {
		return result{}, 404, errors.New("Source record not found in this catalogue")
	}
	if err != nil {
		return result{}, 503, errors.New("Catalogue unavailable")
	}
	records, err := a.loadRecords(ctx, dataset, []int32{ref.ID})
	if err != nil {
		return result{}, 503, sourceReadError(err)
	}
	item := records[0]
	item.Dataset = ref.Dataset
	if ref.Source != "" && item.SourceID != ref.Source {
		return result{}, 409, errors.New("This link no longer identifies the same source entry; search the edition again")
	}
	return item, 200, nil
}
func (a *API) record(w http.ResponseWriter, r *http.Request) {
	ref, err := parseRecordReference(r.URL.Query())
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	item, status, err := a.referencedRecord(ctx, ref)
	if err != nil {
		failError(w, status, err)
		return
	}
	_, reason, err := premisesRecord(item.Record)
	if err != nil {
		fail(w, 503, "Source record unavailable")
		return
	}
	if r.URL.Query().Get("format") == "jsonl" {
		writeRecordLines(w, []result{item})
		return
	}
	respond(w, 200, struct {
		Item             result `json:"item"`
		AddressAvailable bool   `json:"address_available"`
		Reason           string `json:"address_reason,omitempty"`
	}{item, reason == "", reason})
}

type premisesDataset struct {
	ID           int16
	Key, Edition string
}
type premisesCursor struct {
	Query   string `json:"q"`
	Dataset int16  `json:"d"`
	ID      int32  `json:"i"`
}

func (a *API) address(w http.ResponseWriter, r *http.Request) {
	v := r.URL.Query()
	ref, err := parseRecordReference(v)
	if err != nil {
		fail(w, 400, err.Error())
		return
	}
	scope := v.Get("scope")
	if scope == "" {
		scope = "edition"
	}
	if scope != "edition" && scope != "all" {
		fail(w, 400, "address scope must be edition or all")
		return
	}
	limit := 50
	if v.Has("limit") {
		limit, err = strconv.Atoi(v.Get("limit"))
		if err != nil || limit < 1 || limit > 100 {
			fail(w, 400, "limit must be 1–100")
			return
		}
	}
	if len(v.Get("cursor")) > 2048 {
		fail(w, 400, "Invalid address cursor")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	select {
	case a.addressSlots <- struct{}{}:
		defer func() { <-a.addressSlots }()
	case <-ctx.Done():
		fail(w, 503, "Address lookup is busy; retry shortly")
		return
	}
	anchor, status, err := a.referencedRecord(ctx, ref)
	if err != nil {
		failError(w, status, err)
		return
	}
	ref.Dataset, ref.ID = anchor.Dataset, int32(anchor.ID)
	identity, reason, err := premisesRecord(anchor.Record)
	if err != nil {
		fail(w, 503, "Source record unavailable")
		return
	}
	if reason != "" {
		fail(w, 422, reason)
		return
	}
	sql := `SELECT id,key,manifest->>'edition' FROM datasets d WHERE key=$1`
	args := []any{ref.Dataset}
	if scope == "all" {
		sql = `SELECT id,key,manifest->>'edition' FROM datasets d WHERE NOT (
 COALESCE((d.manifest->>'sample')::boolean,false) AND EXISTS(SELECT 1 FROM datasets f WHERE f.manifest->>'edition'=d.manifest->>'edition' AND NOT COALESCE((f.manifest->>'sample')::boolean,false)))`
		args = nil
	}
	rows, err := a.DB.Query(ctx, sql, args...)
	if err != nil {
		fail(w, 503, "Catalogue unavailable")
		return
	}
	datasets := []premisesDataset{}
	for rows.Next() {
		var d premisesDataset
		if err = rows.Scan(&d.ID, &d.Key, &d.Edition); err != nil {
			break
		}
		datasets = append(datasets, d)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		fail(w, 503, "Catalogue unavailable")
		return
	}
	// Supported edition IDs end in YYYY-MM. This puts snapshots in source-edition
	// order while retaining deterministic ordering for additional/demo editions.
	sort.Slice(datasets, func(i, j int) bool {
		date := func(s string) string {
			if len(s) >= 7 {
				return s[len(s)-7:]
			}
			return s
		}
		a, b := date(datasets[i].Edition), date(datasets[j].Edition)
		if a != b {
			return a < b
		}
		return datasets[i].Key < datasets[j].Key
	})
	fingerprintRaw, _ := json.Marshal(struct {
		Anchor   recordReference
		Identity string
		Scope    string
		Datasets []premisesDataset
	}{ref, identity.key(), scope, datasets})
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(fingerprintRaw))
	after := premisesCursor{Query: fingerprint}
	start := 0
	if token := v.Get("cursor"); token != "" {
		raw, e := base64.RawURLEncoding.DecodeString(token)
		if e != nil || json.Unmarshal(raw, &after) != nil || after.Query != fingerprint || after.ID < 1 {
			fail(w, 400, "Cursor does not match this address view")
			return
		}
		start = -1
		for i, d := range datasets {
			if d.ID == after.Dataset {
				start = i
				break
			}
		}
		if start < 0 {
			fail(w, 400, "Cursor does not match this address view")
			return
		}
	}
	identityKey := identity.key()
	var postcode int32
	if err = a.DB.QueryRow(ctx, "SELECT id FROM terms WHERE key=$1", identity.Postcode).Scan(&postcode); err != nil {
		fail(w, 503, "Source postcode index unavailable; catalogue needs verification")
		return
	}
	items := []result{}
	next := ""
	last := after
	for _, dataset := range datasets[start:] {
		scanned := int32(0)
		if dataset.ID == after.Dataset {
			scanned = after.ID
		}
		for {
			// Explicit kind branches preserve the existing postcode B-tree prefix. Only
			// this displayed full postcode is scanned; names and raw postcodes are unused.
			rows, e := a.DB.Query(ctx, `SELECT id FROM (
    (SELECT id FROM entries WHERE dataset_id=$1 AND kind=0 AND postcode_id=$2 AND id>$3 ORDER BY id LIMIT 256)
    UNION ALL
    (SELECT id FROM entries WHERE dataset_id=$1 AND kind=1 AND postcode_id=$2 AND id>$3 ORDER BY id LIMIT 256)
   ) candidates ORDER BY id LIMIT 256`, dataset.ID, postcode, scanned)
			if e != nil {
				fail(w, 503, "Address lookup unavailable or timed out")
				return
			}
			ids := []int32{}
			for rows.Next() {
				var id int32
				if e = rows.Scan(&id); e != nil {
					break
				}
				ids = append(ids, id)
			}
			if e == nil {
				e = rows.Err()
			}
			rows.Close()
			if e != nil {
				fail(w, 503, "Address lookup unavailable or timed out")
				return
			}
			if len(ids) == 0 {
				break
			}
			records, e := a.loadRecords(ctx, dataset.ID, ids)
			if e != nil {
				failError(w, 503, sourceReadError(e))
				return
			}
			for _, item := range records {
				key, reason, e := premisesRecord(item.Record)
				if e != nil {
					fail(w, 503, "Source record unavailable")
					return
				}
				if reason != "" || key.key() != identityKey {
					continue
				}
				if len(items) == limit {
					raw, _ := json.Marshal(last)
					next = base64.RawURLEncoding.EncodeToString(raw)
					break
				}
				item.Dataset = dataset.Key
				items = append(items, item)
				last = premisesCursor{fingerprint, dataset.ID, int32(item.ID)}
			}
			if next != "" {
				break
			}
			scanned = ids[len(ids)-1]
			if len(ids) < 256 || scanned == math.MaxInt32 {
				break
			}
		}
		if next != "" {
			break
		}
	}
	if v.Get("format") == "jsonl" {
		writeRecordLines(w, items)
		return
	}
	respond(w, 200, struct {
		Anchor result   `json:"anchor"`
		Scope  string   `json:"scope"`
		Items  []result `json:"items"`
		Next   string   `json:"next_cursor"`
		Notice string   `json:"notice"`
	}{anchor, scope, items, next, premisesNotice})
}

func writeRecordLines(w http.ResponseWriter, items []result) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Disposition", `attachment; filename="ukinfo-records.jsonl"`)
	encoder := json.NewEncoder(w)
	for _, item := range items {
		if err := encoder.Encode(item); err != nil {
			return
		}
	}
}

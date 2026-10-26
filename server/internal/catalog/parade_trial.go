package catalog

// Private operator checks for the hosted preview. Values, identities and query
// hashes remain inside NAS files; only aggregate counters leave these functions.
import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ParadeTrialCase struct {
	Path       string `json:"path,omitempty"`
	Query      string `json:"query"`
	DigestMode string `json:"digest_mode,omitempty"`
}

// ParadeTrialBackend keeps value-free operator metadata aligned with serving.
func ParadeTrialBackend(c ParadeTrialCase) (string, error) {
	if c.Path != "/api/search" && c.Path != "/api/export" {
		return "native", nil
	}
	v, e := url.ParseQuery(c.Query)
	if e != nil {
		return "", errors.New("invalid private query")
	}
	q, e := parse(v)
	if e != nil {
		return "", errors.New("invalid private query")
	}
	if paradeEligible(q) {
		return "paradedb", nil
	}
	return "native", nil
}

type ParadeEditionVerification struct {
	Dataset            string `json:"dataset"`
	Entries            int64  `json:"entries"`
	Samples            int    `json:"samples"`
	Ready              bool   `json:"ready"`
	IndexValid         bool   `json:"index_valid"`
	FieldsMatched      int    `json:"fields_matched"`
	IndependentMatched int    `json:"independent_matched"`
}
type ParadeTrialVerification struct {
	Passed   bool                        `json:"passed"`
	Editions []ParadeEditionVerification `json:"editions"`
}

// VerifyParadeTrial checks complete counts and spread samples against original
// records AND the independently stored compact/address/research indexes.
func VerifyParadeTrial(ctx context.Context, src, dst *pgxpool.Pool, samples int) (ParadeTrialVerification, error) {
	return verifyParadeTrial(ctx, src, dst, "all", samples)
}

// VerifyParadeEditionTrial permits an early functional pilot while other
// generations are still building. It is not an all-edition readiness gate.
func VerifyParadeEditionTrial(ctx context.Context, src, dst *pgxpool.Pool, key string, samples int) (ParadeTrialVerification, error) {
	if key == "" || key == "all" {
		return ParadeTrialVerification{}, errors.New("choose one visible edition")
	}
	return verifyParadeTrial(ctx, src, dst, key, samples)
}
func verifyParadeTrial(ctx context.Context, src, dst *pgxpool.Pool, scope string, samples int) (ParadeTrialVerification, error) {
	out := ParadeTrialVerification{Editions: []ParadeEditionVerification{}}
	if samples < 2 || samples > 1024 {
		return out, errors.New("sample count must be 2..1024")
	}
	editions, err := searchEditions(ctx, src, scope)
	if err != nil {
		return out, errors.New("source inventory unavailable")
	}
	if len(editions) == 0 {
		return out, errors.New("no visible editions")
	}
	api := NewAPI(src)
	for _, d := range editions {
		item := ParadeEditionVerification{Dataset: d.Key, Entries: d.Entries}
		if err = dst.QueryRow(ctx, `SELECT EXISTS(SELECT FROM ukinfo_search.editions WHERE dataset_id=$1 AND generation=$2 AND entries=$3 AND version=$4 AND ready AND last_id=entries)`, d.ID, d.generation(), d.Entries, paradeVersion).Scan(&item.Ready); err != nil || !item.Ready {
			return out, errors.New("edition is not completely published")
		}
		table := pgx.Identifier{"ukinfo_search", d.table()}.Sanitize()
		var count, lo, hi int64
		if err = dst.QueryRow(ctx, "SELECT count(*),min(id),max(id) FROM "+table).Scan(&count, &lo, &hi); err != nil || count != d.Entries || lo != 1 || hi != d.Entries {
			return out, errors.New("edition coverage mismatch")
		}
		if err = dst.QueryRow(ctx, "SELECT indisvalid AND indisready AND indrelid=$1::regclass FROM pg_index WHERE indexrelid=$2::regclass", table, "ukinfo_search."+d.table()+"_search").Scan(&item.IndexValid); err != nil || !item.IndexValid {
			return out, errors.New("invalid search index")
		}
		ids := []int32{}
		for i := 0; i < samples; i++ {
			id := int32(1 + (d.Entries-1)*int64(i)/int64(samples-1))
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		expected := map[int32][]any{}
		records, e := api.loadRecords(ctx, d.ID, ids)
		if e != nil {
			return out, errors.New("original samples unavailable")
		}
		for _, r := range records {
			v, e := flattenSearchRecord(int32(r.ID), r.Record)
			if e != nil {
				return out, errors.New("original sample could not be projected")
			}
			expected[int32(r.ID)] = v
		}
		if len(expected) != len(ids) {
			return out, errors.New("incomplete original sample coverage")
		}
		fields := append(slices.Clone(searchDocumentColumns), "name_id", "first_id", "postcode_id")
		rows, e := dst.Query(ctx, "SELECT "+strings.Join(fields, ",")+" FROM "+table+" WHERE id=ANY($1::integer[]) ORDER BY id", ids)
		if e != nil {
			return out, errors.New("derived samples unavailable")
		}
		ordering := map[int32][3]int32{}
		for rows.Next() {
			var id, n, f, p int32
			var kind int16
			values := make([]string, 12)
			args := []any{&id, &kind}
			for i := range values {
				args = append(args, &values[i])
			}
			args = append(args, &n, &f, &p)
			if e = rows.Scan(args...); e != nil {
				break
			}
			v := []any{id, kind}
			for _, s := range values {
				v = append(v, s)
			}
			if !reflect.DeepEqual(v, expected[id]) {
				e = errors.New("derived sample field mismatch")
				break
			}
			ordering[id] = [3]int32{n, f, p}
			item.FieldsMatched++
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return out, errors.New("derived sample validation failed")
		}
		if item.FieldsMatched != len(ids) {
			return out, errors.New("incomplete derived sample coverage")
		}
		rows, e = src.Query(ctx, `SELECT e.id,e.kind,e.name_id,e.first_id,e.postcode_id,n.key,f.key,p.key,l.street,l.street_2,l.locality,l.locality_2,l.town,l.county,r.sic,r.industry
 FROM entries e JOIN terms n ON n.id=e.name_id JOIN terms f ON f.id=e.first_id JOIN terms p ON p.id=e.postcode_id
 JOIN address_record_blocks b ON b.dataset_id=e.dataset_id AND b.block_id=(e.id-1)/128
 JOIN address_locations l ON l.id=b.location_ids[(e.id-1)%128+1]
 JOIN research_keys r ON r.dataset_id=e.dataset_id AND r.id=e.id
 WHERE e.dataset_id=$1 AND e.id=ANY($2::integer[])`, d.ID, ids)
		if e != nil {
			return out, errors.New("independent source samples unavailable")
		}
		for rows.Next() {
			var id, n, f, p int32
			var kind int16
			ss := make([]string, 11)
			args := []any{&id, &kind, &n, &f, &p}
			for i := range ss {
				args = append(args, &ss[i])
			}
			if e = rows.Scan(args...); e != nil {
				break
			}
			v, ok := expected[id]
			if !ok || v[1] != kind || ordering[id] != [3]int32{n, f, p} {
				e = errors.New("independent source kind/order mismatch")
				break
			}
			for i, col := range []int{2, 3, 5, 6, 7, 8, 9, 10, 11, 12, 13} {
				if v[col] != ss[i] {
					e = errors.New("independent source field mismatch")
					break
				}
			}
			if e != nil {
				break
			}
			item.IndependentMatched++
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return out, errors.New("independent source validation failed")
		}
		if item.IndependentMatched != len(ids) {
			return out, errors.New("incomplete independent sample coverage")
		}
		item.Samples = len(ids)
		out.Editions = append(out.Editions, item)
	}
	out.Passed = true
	return out, nil
}

// CreateParadeTrialCases samples the serving role's visible original records.
// The returned workload contains personal data: write mode 0600 inside the NAS.
func CreateParadeTrialCases(ctx context.Context, src *pgxpool.Pool) ([]ParadeTrialCase, error) {
	editions, err := searchEditions(ctx, src, "all")
	if err != nil {
		return nil, errors.New("source inventory unavailable")
	}
	api := NewAPI(src)
	out := []ParadeTrialCase{}
	seen := map[string]bool{}
	add := func(path string, v url.Values) {
		if path == "/api/search" || path == "/api/export" {
			if _, e := parse(v); e != nil {
				return
			}
		}
		s := v.Encode()
		key := path + "?" + s
		if seen[key] {
			return
		}
		seen[key] = true
		mode := "cursor-neutral-v1"
		if path == "/api/export" {
			mode = ""
		}
		out = append(out, ParadeTrialCase{path, s, mode})
	}
	clone := func(v url.Values) url.Values {
		out := url.Values{}
		for k, vs := range v {
			out[k] = slices.Clone(vs)
		}
		return out
	}
	for _, d := range editions {
		ids := []int32{1, int32(d.Entries/3 + 1), int32(d.Entries*2/3 + 1), int32(d.Entries)}
		rows, e := src.Query(ctx, `SELECT id FROM entries WHERE dataset_id=$1 AND kind=1 ORDER BY name_id,first_id,id LIMIT 1`, d.ID)
		if e != nil {
			return nil, errors.New("business sample selection failed")
		}
		for rows.Next() {
			var id int32
			if e = rows.Scan(&id); e != nil {
				break
			}
			if !slices.Contains(ids, id) {
				ids = append(ids, id)
			}
		}
		if e == nil {
			e = rows.Err()
		}
		rows.Close()
		if e != nil {
			return nil, errors.New("business samples unavailable")
		}
		records, e := api.loadRecords(ctx, d.ID, ids)
		if e != nil {
			return nil, errors.New("case source records unavailable")
		}
		broadPostcode := false
		for i, r := range records {
			var doc searchDocument
			if json.Unmarshal(r.Record, &doc) != nil {
				return nil, errors.New("case record could not be projected")
			}
			loc, e := doc.addressSource.project()
			if e != nil {
				return nil, errors.New("case address could not be projected")
			}
			base := url.Values{"dataset": {d.Key}, "kind": {"person"}, "match": {"exact"}, "limit": {"50"}, "surname": {doc.Surname}, "first_name": {doc.First}, "first_match": {"given"}}
			if doc.EntryType == "business" {
				base = url.Values{"dataset": {d.Key}, "kind": {"business"}, "match": {"exact"}, "limit": {"50"}, "organisation": {doc.Organisation}}
			}
			// Every search is based on displayed fields; no raw references are restored.
			if loc[4] != "" {
				base.Set("town", loc[4])
			} else if doc.Postcode != "" {
				base.Set("postcode", doc.Postcode)
			}
			add("/api/search", base)
			v := clone(base)
			v.Set("sort", "name")
			add("/api/search", v)
			if i < 2 {
				v = clone(base)
				v.Set("dataset", "all")
				add("/api/search", v)
				v.Set("group", "listing")
				add("/api/search", v)
				v.Del("group")
				v.Set("sort", "edition")
				add("/api/search", v)
			}
			if doc.Postcode != "" {
				v = clone(base)
				v.Del("town")
				v.Set("postcode", doc.Postcode)
				if q, e := parse(v); e == nil && q.PostcodeLength == 0 {
					compact := Postcode(doc.Postcode)
					v.Set("postcode", compact[:len(compact)-3])
				}
				add("/api/search", v)
				v.Set("sort", "name")
				add("/api/search", v)
				if i == 0 {
					v = clone(base)
					v.Del("postcode")
					v.Set("near", doc.Postcode)
					v.Set("radius_m", "2000")
					add("/api/search", v)
				}
			}
			// These broader cases deliberately fill later pages and exercise the
			// cross-edition cursor, rather than validating only unique-name lookups.
			if !broadPostcode && doc.Postcode != "" {
				v := url.Values{"dataset": {d.Key}, "kind": {"person"}, "postcode": {doc.Postcode}, "limit": {"50"}}
				if parsed, e := parse(v); e == nil {
					if parsed.PostcodeLength == 0 {
						compact := Postcode(doc.Postcode)
						v.Set("postcode", compact[:len(compact)-3])
					}
					add("/api/search", v)
					v.Set("sort", "name")
					add("/api/search", v)
					v.Del("sort")
					v.Set("dataset", "all")
					add("/api/search", v)
					broadPostcode = true
				}
			}
			if i == 1 && doc.EntryType != "business" && doc.Surname != "" {
				v := url.Values{"dataset": {d.Key}, "kind": {"person"}, "surname": {doc.Surname}, "limit": {"50"}}
				add("/api/search", v)
			}
			if i == 0 && loc[5] != "" {
				add("/api/search", url.Values{"dataset": {d.Key}, "kind": {"person"}, "county": {loc[5]}, "limit": {"50"}})
			}
			if loc[5] != "" {
				v = clone(base)
				v.Del("town")
				v.Set("county", loc[5])
				add("/api/search", v)
			}
			street := loc[0]
			if street == "" {
				street = loc[1]
			}
			if street != "" {
				v = clone(base)
				v.Set("street", street)
				add("/api/search", v)
			}
			if i == 0 {
				add("/api/export", base)
				ref := url.Values{"dataset": {d.Key}, "id": {strconv.FormatInt(r.ID, 10)}, "source_id": {r.SourceID}}
				add("/api/record", ref)
				if _, reason, e := premisesRecord(r.Record); e == nil && reason == "" {
					v = clone(ref)
					v.Set("scope", "all")
					v.Set("limit", "50")
					add("/api/address", v)
				}
				var fields map[string]any
				dec := json.NewDecoder(bytes.NewReader(r.Record))
				dec.UseNumber()
				if dec.Decode(&fields) == nil {
					if group, e := listingKey(fields, r.Archive, r.SourceID, d.Key); e == nil {
						add("/api/group", url.Values{"dataset": {"all"}, "group_key": {hex.EncodeToString(group)}})
					}
					if house := Text(fields, "houseno"); house != "" && house != "0" {
						v = clone(base)
						v.Set("house", house)
						add("/api/search", v)
					}
				}
			}
			if doc.EntryType == "business" {
				if doc.SIC != "" {
					v = clone(base)
					v.Set("sic", doc.SIC)
					add("/api/search", v)
					v.Set("dataset", "all")
					add("/api/search", v)
				}
				if doc.Industry != "" {
					v = clone(base)
					v.Set("industry", doc.Industry)
					add("/api/search", v)
				}
			} else if name := []rune(Normalize(doc.Surname)); len(name) >= 3 && i == 1 {
				v = clone(base)
				v.Set("surname", string(name[:3])+"*")
				add("/api/search", v)
			}
		}
	}
	// A fixed negative stays in the cohort even when a backend fails to prove it.
	for _, sort := range []string{"", "name"} {
		add("/api/search", url.Values{"dataset": {"all"}, "surname": {"ukinfo trial absent surname 20261026"}, "town": {"ukinfo trial absent town 20261026"}, "sort": {sort}, "limit": {"50"}})
	}
	if len(out) == 0 || len(out) > 500 {
		return nil, fmt.Errorf("invalid bounded case cohort")
	}
	return out, nil
}

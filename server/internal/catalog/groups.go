package catalog

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"
)

func (a *API) groupSources(w http.ResponseWriter, r *http.Request) {
	group, err := hex.DecodeString(r.URL.Query().Get("group_key"))
	scope := r.URL.Query().Get("dataset")
	if err != nil || len(group) != 32 || scope == "" || len(scope) > 256 {
		fail(w, 400, "Choose an indexed source group and edition scope")
		return
	}
	if format := r.URL.Query().Get("format"); format != "" && format != "jsonl" {
		fail(w, 400, "Group format must be jsonl or omitted")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	f, status, err := a.candidateFilters(ctx, query{Dataset: scope, Kind: "person"})
	if err != nil {
		failError(w, status, err)
		return
	}
	var after cursor
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		b, e := base64.RawURLEncoding.DecodeString(raw)
		if len(raw) > 2048 || e != nil || json.Unmarshal(b, &after) != nil || after.Query != f.q.fingerprint()+":"+hex.EncodeToString(group) || after.ID < 1 || f.datasetKeys[after.Dataset] == "" {
			fail(w, 400, "Invalid source group cursor")
			return
		}
	}
	rows, err := a.DB.Query(ctx, "SELECT dataset_id,id FROM research_keys WHERE group_key=$1 AND dataset_id=ANY($2::smallint[]) AND (dataset_id,id)>($3,$4) ORDER BY dataset_id,id LIMIT 51", group, f.datasetIDs, after.Dataset, after.ID)
	if err != nil {
		fail(w, 503, "Source groups unavailable; build research indexes")
		return
	}
	type key struct {
		dataset int16
		id      int32
	}
	keys := []key{}
	next := ""
	for rows.Next() {
		var k key
		if err = rows.Scan(&k.dataset, &k.id); err != nil {
			break
		}
		if len(keys) == 50 {
			last := keys[len(keys)-1]
			raw, _ := json.Marshal(cursor{Query: f.q.fingerprint() + ":" + hex.EncodeToString(group), Dataset: last.dataset, ID: last.id})
			next = base64.RawURLEncoding.EncodeToString(raw)
			break
		}
		keys = append(keys, k)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		fail(w, 503, "Group sources unavailable")
		return
	}
	requested := map[int16][]int32{}
	for _, k := range keys {
		requested[k.dataset] = append(requested[k.dataset], k.id)
	}
	found := map[key]result{}
	for dataset, ids := range requested {
		records, e := a.loadRecords(ctx, dataset, ids)
		if e != nil {
			failError(w, 503, sourceReadError(e))
			return
		}
		for _, record := range records {
			record.Dataset = f.datasetKeys[dataset]
			found[key{dataset, int32(record.ID)}] = record
		}
	}
	items := []result{}
	for _, k := range keys {
		item := found[k]
		var row map[string]any
		decoder := json.NewDecoder(bytes.NewReader(item.Record))
		decoder.UseNumber()
		if decoder.Decode(&row) != nil {
			fail(w, 503, "Group source needs verification")
			return
		}
		key, _ := listingKey(row, item.Archive, item.SourceID, item.Dataset)
		if !bytes.Equal(key, group) {
			fail(w, 503, "Source group index differs from its records; rebuild the index")
			return
		}
		items = append(items, item)
	}
	if r.URL.Query().Get("format") == "jsonl" {
		writeRecordLines(w, items)
		return
	}
	respond(w, 200, map[string]any{"items": items, "next_cursor": next, "notice": "Identical recorded listings, not verified identities. Source occurrences remain separate."})
}

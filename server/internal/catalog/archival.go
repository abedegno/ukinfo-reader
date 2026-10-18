package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
)

// A catalogue ID is a fast hint only. Verified source identity plus original entry
// identity controls resolution when keys, sample scope or ingest order change.
func (a *API) archivalRecord(ctx context.Context, ref recordReference) (result, int, error) {
	rows, err := a.DB.Query(ctx, "SELECT id,key,manifest,entries FROM datasets ORDER BY entries DESC,key")
	if err != nil {
		return result{}, 503, errors.New("Catalogue unavailable")
	}
	type source struct {
		id      int16
		key     string
		entries int64
	}
	sources := []source{}
	for rows.Next() {
		var d source
		var raw []byte
		var manifest Manifest
		if err = rows.Scan(&d.id, &d.key, &raw, &d.entries); err != nil {
			break
		}
		if err = json.Unmarshal(raw, &manifest); err != nil {
			break
		}
		if archiveIdentity(manifest) == ref.Archive {
			sources = append(sources, d)
		}
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return result{}, 503, errors.New("Catalogue unavailable")
	}
	if len(sources) == 0 {
		return result{}, 404, errors.New("The original source components for this archival reference are not imported")
	}
	sort.SliceStable(sources, func(i, j int) bool { return sources[i].key == ref.Dataset && sources[j].key != ref.Dataset })
	for _, d := range sources {
		if int64(ref.ID) > d.entries {
			continue
		}
		item, status, e := a.referencedRecord(ctx, recordReference{Dataset: d.key, ID: ref.ID, Source: ref.Source})
		if e == nil {
			return item, status, nil
		}
	}
	missingIndex := false
	for _, d := range sources {
		var ready bool
		if err = a.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM research_indexes WHERE dataset_id=$1 AND entries=$2 AND version=1)", d.id, d.entries).Scan(&ready); err != nil {
			return result{}, 503, errors.New("Research index inventory unavailable")
		}
		if !ready {
			missingIndex = true
			continue
		}
		rows, e := a.DB.Query(ctx, "SELECT id FROM research_keys WHERE dataset_id=$1 AND source_hash=$2 ORDER BY id", d.id, sourceHash(ref.Source))
		if e != nil {
			return result{}, 503, errors.New("Archival reference lookup unavailable")
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
			return result{}, 503, e
		}
		if len(ids) == 0 {
			continue
		}
		records, e := a.loadRecords(ctx, d.id, ids)
		if e != nil {
			return result{}, 503, errors.New("Archival source record unavailable")
		}
		for _, item := range records {
			if item.SourceID == ref.Source {
				item.Dataset = d.key
				return item, 200, nil
			}
		}
	}
	if missingIndex {
		return result{}, 409, errors.New("Build the source's research index to resolve this archival entry after its catalogue position changed")
	}
	return result{}, 404, errors.New("This original entry is not present in the imported source scope")
}

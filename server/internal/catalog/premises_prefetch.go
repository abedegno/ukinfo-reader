package catalog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"sync"
)

// collectPremisesEditions overlaps only the current and next edition. It consumes
// results in catalogue order, so completion order cannot affect paging. Joining
// cancelled workers before returning also keeps admission-slot and cache lifetimes
// within the original HTTP request.
func collectPremisesEditions(ctx context.Context, datasets []premisesDataset, after premisesCursor, limit int, fingerprint string, read func(context.Context, premisesDataset, int32, int) ([]result, error)) ([]result, string, error) {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	type reply struct {
		items []result
		err   error
	}
	pending := make([]chan reply, len(datasets))
	start := func(index int) {
		channel := make(chan reply, 1)
		pending[index] = channel
		workers.Add(1)
		go func() {
			defer workers.Done()
			d := datasets[index]
			var cursor int32
			if d.ID == after.Dataset {
				cursor = after.ID
			}
			items, err := read(ctx, d, cursor, limit+1)
			channel <- reply{items, err}
		}()
	}
	items := []result{}
	last := after
	for i := 0; i < len(datasets) && i < 2; i++ {
		start(i)
	}
	for i, d := range datasets {
		var page reply
		select {
		case page = <-pending[i]:
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		if page.err != nil {
			return nil, "", page.err
		}
		if len(page.items) > limit+1 {
			return nil, "", recordIntegrity("oversized premises edition page")
		}
		for _, item := range page.items {
			if len(items) == limit {
				raw, _ := json.Marshal(last)
				return items, base64.RawURLEncoding.EncodeToString(raw), nil
			}
			item.Dataset = d.Key
			items = append(items, item)
			last = premisesCursor{fingerprint, d.ID, int32(item.ID)}
		}
		if i+2 < len(datasets) {
			start(i + 2)
		}
	}
	return items, "", nil
}

// Each scan retains at most one page plus its lookahead. Nonmatching postcode
// batches are still exhausted; a sparse batch never establishes end of results.
func (a *API) premisesEdition(ctx context.Context, dataset premisesDataset, postcode int32, identityKey string, scanned int32, want int) ([]result, error) {
	matches := []result{}
	for {
		rows, err := a.DB.Query(ctx, `SELECT id FROM (
    (SELECT id FROM entries WHERE dataset_id=$1 AND kind=0 AND postcode_id=$2 AND id>$3 ORDER BY id LIMIT 256)
    UNION ALL
    (SELECT id FROM entries WHERE dataset_id=$1 AND kind=1 AND postcode_id=$2 AND id>$3 ORDER BY id LIMIT 256)
   ) candidates ORDER BY id LIMIT 256`, dataset.ID, postcode, scanned)
		if err != nil {
			return nil, errors.New("Address lookup unavailable or timed out")
		}
		ids := []int32{}
		for rows.Next() {
			var id int32
			if err = rows.Scan(&id); err != nil {
				break
			}
			ids = append(ids, id)
		}
		if err == nil {
			err = rows.Err()
		}
		rows.Close()
		if err != nil {
			return nil, errors.New("Address lookup unavailable or timed out")
		}
		if len(ids) == 0 {
			return matches, nil
		}
		records, err := a.loadRecords(ctx, dataset.ID, ids)
		if err != nil {
			return nil, sourceReadError(err)
		}
		for _, item := range records {
			key, reason, err := premisesRecord(item.Record)
			if err != nil {
				return nil, errors.New("Source record unavailable")
			}
			if reason != "" || key.key() != identityKey {
				continue
			}
			matches = append(matches, item)
			if len(matches) == want {
				return matches, nil
			}
		}
		scanned = ids[len(ids)-1]
		if len(ids) < 256 || scanned == math.MaxInt32 {
			return matches, nil
		}
	}
}

package catalog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"sync"
)

// collectPremisesEditions streams only the current and next edition. Results
// remain in catalogue order. A terminal error follows its valid prefix; it is
// relevant only if that prefix cannot fill the page and its lookahead.
// Cancellation and joining keep workers inside the request's admission slot.
func collectPremisesEditions(ctx context.Context, datasets []premisesDataset, after premisesCursor, limit int, fingerprint string, read func(context.Context, premisesDataset, int32, func(result) error) error) ([]result, string, error) {
	ctx, cancel := context.WithCancel(ctx)
	var workers sync.WaitGroup
	defer func() { cancel(); workers.Wait() }()
	type reply struct {
		item result
		err  error
		done bool
	}
	pending := make([]chan reply, len(datasets))
	start := func(index int) {
		channel := make(chan reply)
		pending[index] = channel
		workers.Add(1)
		go func() {
			defer workers.Done()
			d := datasets[index]
			var scanned int32
			if d.ID == after.Dataset {
				scanned = after.ID
			}
			emit := func(item result) error {
				select {
				case channel <- reply{item: item}:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			err := read(ctx, d, scanned, emit)
			select {
			case channel <- reply{err: err, done: true}:
			case <-ctx.Done():
			}
		}()
	}
	items := []result{}
	last := after
	for i := 0; i < len(datasets) && i < 2; i++ {
		start(i)
	}
	for i, d := range datasets {
		for {
			var event reply
			select {
			case event = <-pending[i]:
			case <-ctx.Done():
				return nil, "", ctx.Err()
			}
			if ctx.Err() != nil {
				return nil, "", ctx.Err()
			}
			if event.done {
				if event.err != nil {
					return nil, "", event.err
				}
				break
			}
			if len(items) == limit {
				raw, _ := json.Marshal(last)
				return items, base64.RawURLEncoding.EncodeToString(raw), nil
			}
			event.item.Dataset = d.Key
			items = append(items, event.item)
			last = premisesCursor{fingerprint, d.ID, int32(event.item.ID)}
		}
		if i+2 < len(datasets) {
			start(i + 2)
		}
	}
	return items, "", nil
}

// Postcode batches remain bounded at256 records; no matching-record page is
// buffered ahead of the coordinator. Empty batches do not establish exhaustion.
func (a *API) premisesEdition(ctx context.Context, dataset premisesDataset, postcode int32, identityKey string, scanned int32, emit func(result) error) error {
	for {
		rows, err := a.DB.Query(ctx, `SELECT id FROM (
    (SELECT id FROM entries WHERE dataset_id=$1 AND kind=0 AND postcode_id=$2 AND id>$3 ORDER BY id LIMIT 256)
    UNION ALL
    (SELECT id FROM entries WHERE dataset_id=$1 AND kind=1 AND postcode_id=$2 AND id>$3 ORDER BY id LIMIT 256)
   ) candidates ORDER BY id LIMIT 256`, dataset.ID, postcode, scanned)
		if err != nil {
			return errors.New("Address lookup unavailable or timed out")
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
			return errors.New("Address lookup unavailable or timed out")
		}
		if len(ids) == 0 {
			return nil
		}
		records, err := a.loadRecords(ctx, dataset.ID, ids)
		if err != nil {
			return sourceReadError(err)
		}
		for _, item := range records {
			key, reason, err := premisesRecord(item.Record)
			if err != nil {
				return errors.New("Source record unavailable")
			}
			if reason != "" || key.key() != identityKey {
				continue
			}
			if err := emit(item); err != nil {
				return err
			}
		}
		scanned = ids[len(ids)-1]
		if len(ids) < 256 || scanned == math.MaxInt32 {
			return nil
		}
	}
}

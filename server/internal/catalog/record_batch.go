package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"
)

type sourceRecordKey struct {
	dataset int16
	id      int32
}

// loadPageRecords batches a native page across editions. Four blocks per SQL
// statement and two in-flight statements are global to the page, rather than
// multiplied by its edition count. Candidate order is restored by the caller.
func (a *API) loadPageRecords(ctx context.Context, requested map[int16][]int32) (out map[sourceRecordKey]result, loadErr error) {
	ctx, end := a.searchPhase(ctx, "records")
	defer func() { end(loadErr) }()
	out = make(map[sourceRecordKey]result)
	datasets := make([]int16, 0, len(requested))
	for dataset, ids := range requested {
		if len(ids) > 0 {
			datasets = append(datasets, dataset)
		}
	}
	if len(datasets) == 0 {
		return out, nil
	}
	sort.Slice(datasets, func(i, j int) bool { return datasets[i] < datasets[j] })
	archives := make(map[int16]string, len(datasets))
	rows, err := a.DB.Query(ctx, "SELECT id,manifest FROM datasets WHERE id=ANY($1::smallint[])", datasets)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var dataset int16
		var raw []byte
		var manifest Manifest
		if err = rows.Scan(&dataset, &raw); err != nil {
			break
		}
		if err = json.Unmarshal(raw, &manifest); err != nil {
			err = recordIntegrity("invalid stored manifest")
			break
		}
		archives[dataset] = archiveIdentity(manifest)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		return nil, err
	}
	if len(archives) != len(datasets) {
		return nil, recordIntegrity("missing stored manifest")
	}
	cache := recordCache(ctx)
	groups := map[recordBlockKey][]int32{}
	keys := []recordBlockKey{}
	for _, dataset := range datasets {
		for _, id := range requested[dataset] {
			if id < 1 {
				return nil, recordIntegrity("invalid source record ID")
			}
			key := recordBlockKey{dataset, (id - 1) / blockRows}
			if raw := cache.get(key); raw != nil {
				r, e := recordAt(raw, int((id-1)%blockRows))
				if e != nil {
					return nil, e
				}
				out[sourceRecordKey{dataset, id}] = result{ID: int64(id), SourceID: r.SourceID, Record: r.Record, Archive: archives[dataset]}
			} else {
				if _, ok := groups[key]; !ok {
					keys = append(keys, key)
				}
				groups[key] = append(groups[key], id)
			}
		}
	}
	group, childCtx := errgroup.WithContext(ctx)
	group.SetLimit(2)
	var mu sync.Mutex
	for offset := 0; offset < len(keys); offset += recordBlocksPerRead {
		batch := keys[offset:min(offset+recordBlocksPerRead, len(keys))]
		group.Go(func() error {
			// Explicit edition branches retain partition pruning and indexed point
			// access; no cross-product of edition IDs and block IDs is fetched.
			branches := []string{}
			args := []any{}
			for _, key := range batch {
				args = append(args, key.dataset, key.block)
				branches = append(branches, fmt.Sprintf("SELECT dataset_id,id,payload FROM record_blocks WHERE dataset_id=$%d AND id=$%d", len(args)-1, len(args)))
			}
			rows, e := a.DB.Query(childCtx, strings.Join(branches, " UNION ALL "), args...)
			if e != nil {
				return e
			}
			defer rows.Close()
			type block struct {
				key     recordBlockKey
				payload []byte
			}
			pending := make([]block, 0, len(batch))
			for rows.Next() {
				var b block
				if e = rows.Scan(&b.key.dataset, &b.key.block, &b.payload); e != nil {
					return e
				}
				pending = append(pending, b)
			}
			if e = rows.Err(); e != nil {
				return e
			}
			rows.Close()
			if len(pending) != len(batch) {
				return recordIntegrity("missing source record block")
			}
			local := make(map[sourceRecordKey]result)
			for _, b := range pending {
				if e = childCtx.Err(); e != nil {
					return e
				}
				raw, e := unpackBlock(a.decoder, b.payload)
				if e != nil {
					return e
				}
				cache.put(b.key, raw)
				if cache != nil && cache.observe != nil {
					cache.observe.decodedBlocks.Add(1)
				}
				for _, id := range groups[b.key] {
					r, e := recordAt(raw, int((id-1)%blockRows))
					if e != nil {
						return e
					}
					local[sourceRecordKey{b.key.dataset, id}] = result{ID: int64(id), SourceID: r.SourceID, Record: r.Record, Archive: archives[b.key.dataset]}
				}
			}
			mu.Lock()
			for key, r := range local {
				out[key] = r
			}
			mu.Unlock()
			return nil
		})
	}
	if err = group.Wait(); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

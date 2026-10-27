package catalog

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
)

// Deep pages can touch a distinct compressed source block per result. Bound
// random I/O per statement and overlap at most two reads within the existing
// pool. The original HTTP/SQL deadlines still apply; no retries or partial pages.
const recordBlocksPerRead = 4

func (a *API) readRecordBlocks(ctx context.Context, dataset int16, blockIDs []int32, groups map[int32][]int32, cache *requestBlockCache) (map[int32]storedRecord, error) {
	found := make(map[int32]storedRecord)
	var mu sync.Mutex
	group, ctx := errgroup.WithContext(ctx)
	group.SetLimit(2)
	for offset := 0; offset < len(blockIDs); offset += recordBlocksPerRead {
		batch := blockIDs[offset:min(offset+recordBlocksPerRead, len(blockIDs))]
		group.Go(func() error {
			rows, err := a.DB.Query(ctx, "SELECT id,payload FROM record_blocks WHERE dataset_id=$1 AND id=ANY($2::integer[])", dataset, batch)
			if err != nil {
				return err
			}
			defer rows.Close()
			type block struct {
				id         int32
				compressed []byte
			}
			pending := make([]block, 0, len(batch))
			for rows.Next() {
				var b block
				if err = rows.Scan(&b.id, &b.compressed); err != nil {
					return err
				}
				pending = append(pending, b)
			}
			if err = rows.Err(); err != nil {
				return err
			}
			// Release the connection and finish the SQL statement before
			// decoding. Go CPU time must not extend a streaming DB statement.
			rows.Close()
			local := make(map[int32]storedRecord)
			for _, b := range pending {
				if err = ctx.Err(); err != nil {
					return err
				}
				raw, err := unpackBlock(a.decoder, b.compressed)
				if err != nil {
					return err
				}
				cache.put(recordBlockKey{dataset, b.id}, raw)
				if cache != nil && cache.observe != nil {
					cache.observe.decodedBlocks.Add(1)
				}
				for _, id := range groups[b.id] {
					record, err := recordAt(raw, int((id-1)%blockRows))
					if err != nil {
						return err
					}
					local[id] = record
				}
			}
			mu.Lock()
			for id, record := range local {
				found[id] = record
			}
			mu.Unlock()
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return nil, err
	}
	return found, nil
}

package catalog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPremisesPrefetchOverlapsAndPreservesOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	editions := []premisesDataset{{ID: 1, Key: "old"}, {ID: 2, Key: "middle"}, {ID: 3, Key: "new"}}
	secondStarted := make(chan struct{})
	firstStarted := make(chan struct{})
	var active, maximum atomic.Int32
	var mu sync.Mutex
	starts := map[int16]int32{}
	read := func(ctx context.Context, d premisesDataset, after int32, want int) ([]result, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for old := maximum.Load(); n > old && !maximum.CompareAndSwap(old, n); old = maximum.Load() {
		}
		mu.Lock()
		starts[d.ID] = after
		mu.Unlock()
		if want != 4 {
			t.Errorf("unbounded edition limit: %d", want)
		}
		if d.ID == 1 {
			close(firstStarted)
			select {
			case <-secondStarted:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if d.ID == 2 {
			close(secondStarted)
			select {
			case <-firstStarted:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		return []result{{ID: int64(d.ID) * 10}}, nil
	}
	items, next, err := collectPremisesEditions(ctx, editions, premisesCursor{Dataset: 1, ID: 7}, 3, "query", read)
	if err != nil || next != "" || len(items) != 3 {
		t.Fatalf("page: %+v %q %v", items, next, err)
	}
	for i, item := range items {
		if item.ID != int64((i+1)*10) || item.Dataset != editions[i].Key {
			t.Fatalf("out of order: %+v", items)
		}
	}
	if maximum.Load() != 2 || active.Load() != 0 {
		t.Fatalf("concurrency=%d remaining=%d", maximum.Load(), active.Load())
	}
	if !reflect.DeepEqual(starts, map[int16]int32{1: 7, 2: 0, 3: 0}) {
		t.Fatal(starts)
	}
}

func TestPremisesPrefetchCursorAndUnusedError(t *testing.T) {
	boom := errors.New("unused future edition")
	items, next, err := collectPremisesEditions(context.Background(), []premisesDataset{{ID: 1, Key: "old"}, {ID: 2, Key: "new"}, {ID: 3, Key: "unused"}}, premisesCursor{}, 2, "fingerprint", func(ctx context.Context, d premisesDataset, after int32, want int) ([]result, error) {
		if d.ID == 3 {
			t.Error("read beyond bounded lookahead")
		}
		if d.ID == 2 {
			return nil, boom
		}
		return []result{{ID: 10}, {ID: 20}, {ID: 30}}, nil
	})
	if err != nil || len(items) != 2 || next == "" {
		t.Fatalf("%+v %q %v", items, next, err)
	}
	raw, err := base64.RawURLEncoding.DecodeString(next)
	if err != nil {
		t.Fatal(err)
	}
	var cursor premisesCursor
	if err = json.Unmarshal(raw, &cursor); err != nil {
		t.Fatal(err)
	}
	if cursor != (premisesCursor{Query: "fingerprint", Dataset: 1, ID: 20}) {
		t.Fatal(cursor)
	}
}

func TestPremisesPrefetchRequiresLookaheadAcrossEditionBoundary(t *testing.T) {
	boom := errors.New("required edition failed")
	_, _, err := collectPremisesEditions(context.Background(), []premisesDataset{{ID: 1}, {ID: 2}}, premisesCursor{}, 2, "query", func(ctx context.Context, d premisesDataset, after int32, want int) ([]result, error) {
		if d.ID == 2 {
			return nil, boom
		}
		return []result{{ID: 1}, {ID: 2}}, nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("required failure lost: %v", err)
	}
}

func TestPremisesPrefetchCancelsAndJoinsUnusedWorker(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	_, _, err := collectPremisesEditions(context.Background(), []premisesDataset{{ID: 1}, {ID: 2}}, premisesCursor{}, 1, "query", func(ctx context.Context, d premisesDataset, after int32, want int) ([]result, error) {
		if d.ID == 2 {
			close(started)
			<-ctx.Done()
			close(stopped)
			return nil, ctx.Err()
		}
		<-started
		return []result{{ID: 1}, {ID: 2}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-stopped:
	default:
		t.Fatal("worker outlived request")
	}
}

func TestPremisesPrefetchParentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var started, stopped atomic.Int32
	_, _, err := collectPremisesEditions(ctx, []premisesDataset{{ID: 1}, {ID: 2}, {ID: 3}}, premisesCursor{}, 1, "query", func(ctx context.Context, d premisesDataset, after int32, want int) ([]result, error) {
		if started.Add(1) == 2 {
			cancel()
		}
		<-ctx.Done()
		stopped.Add(1)
		return nil, ctx.Err()
	})
	if !errors.Is(err, context.Canceled) || started.Load() != 2 || stopped.Load() != 2 {
		t.Fatalf("%v started=%d stopped=%d", err, started.Load(), stopped.Load())
	}
}

func TestPremisesPrefetchPaginationMatchesSequentialOrder(t *testing.T) {
	editions := []premisesDataset{{ID: 1, Key: "old"}, {ID: 2, Key: "empty"}, {ID: 3, Key: "new"}, {ID: 4, Key: "last"}}
	data := map[int16][]int64{1: {2, 9, 20}, 3: {1, 3}, 4: {10, 11, 12, 20}}
	for _, limit := range []int{1, 2, 5, 100} {
		seen := []string{}
		after := premisesCursor{Query: "fingerprint"}
		start := 0
		for page := 0; page < 20; page++ {
			items, next, err := collectPremisesEditions(context.Background(), editions[start:], after, limit, "fingerprint", func(ctx context.Context, d premisesDataset, after int32, want int) ([]result, error) {
				matches := []result{}
				for _, id := range data[d.ID] {
					if id > int64(after) {
						matches = append(matches, result{ID: id})
						if len(matches) == want {
							break
						}
					}
				}
				return matches, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range items {
				seen = append(seen, fmt.Sprintf("%s:%d", item.Dataset, item.ID))
			}
			if next == "" {
				break
			}
			raw, err := base64.RawURLEncoding.DecodeString(next)
			if err != nil {
				t.Fatal(err)
			}
			if err = json.Unmarshal(raw, &after); err != nil {
				t.Fatal(err)
			}
			start = -1
			for i, d := range editions {
				if d.ID == after.Dataset {
					start = i
					break
				}
			}
			if start < 0 {
				t.Fatal("unknown cursor edition")
			}
		}
		expected := []string{"old:2", "old:9", "old:20", "new:1", "new:3", "last:10", "last:11", "last:12", "last:20"}
		if !reflect.DeepEqual(seen, expected) {
			t.Fatalf("limit=%d: %+v", limit, seen)
		}
	}
}

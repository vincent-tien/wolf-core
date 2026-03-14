// sharded.go — Data-parallel processing by splitting work across goroutine shards.
package concurrency

import (
	"context"
	"sync"
)

// ProcessSharded divides items into disjoint sub-slices and processes each
// shard in a separate goroutine. Each goroutine writes to its own index in
// the results slice — no coordination beyond the WaitGroup is needed.
//
// workers is clamped to [1, len(items)]. Remainder items are distributed
// round-robin to the first N shards. Returns nil when items is empty.
func ProcessSharded[T, R any](items []T, workers int, fn func(shard []T) R) []R {
	n := len(items)
	if n == 0 {
		return nil
	}

	workers = max(1, min(workers, n))

	base := n / workers
	remainder := n % workers

	results := make([]R, workers)
	var wg sync.WaitGroup

	offset := 0
	for i := range workers {
		size := base
		if i < remainder {
			size++
		}
		shard := items[offset : offset+size]
		offset += size

		idx := i
		wg.Go(func() {
			results[idx] = fn(shard)
		})
	}

	wg.Wait()
	return results
}

// ProcessShardedWithError is like ProcessSharded but propagates errors. The
// first error encountered by any shard cancels the context passed to remaining
// shards. The merge function combines per-shard results into a single value.
//
// Returns the zero value of R and the first encountered error if any shard
// fails. On success, returns merge(results).
func ProcessShardedWithError[T, R any](
	ctx context.Context,
	items []T,
	workers int,
	fn func(ctx context.Context, shard []T) (R, error),
	merge func(results []R) R,
) (R, error) {
	var zero R
	n := len(items)
	if n == 0 {
		return zero, nil
	}

	workers = max(1, min(workers, n))

	base := n / workers
	remainder := n % workers

	type shardResult struct {
		value R
		err   error
	}

	results := make([]shardResult, workers)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	offset := 0
	for i := range workers {
		size := base
		if i < remainder {
			size++
		}
		shard := items[offset : offset+size]
		offset += size

		idx := i
		wg.Go(func() {
			v, err := fn(ctx, shard)
			results[idx] = shardResult{value: v, err: err}
			if err != nil {
				cancel()
			}
		})
	}

	wg.Wait()

	merged := make([]R, 0, workers)
	for _, sr := range results {
		if sr.err != nil {
			return zero, sr.err
		}
		merged = append(merged, sr.value)
	}

	return merge(merged), nil
}

package concurrency_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/vincent-tien/wolf-core/infra/concurrency"
)

func TestProcessSharded_SumsCorrectly(t *testing.T) {
	t.Parallel()

	// Arrange
	items := make([]int, 100)
	for i := range items {
		items[i] = i + 1
	}

	// Act
	sums := concurrency.ProcessSharded(items, 4, func(shard []int) int {
		total := 0
		for _, v := range shard {
			total += v
		}
		return total
	})

	// Assert — sum 1..100 = 5050
	total := 0
	for _, s := range sums {
		total += s
	}
	assert.Equal(t, 5050, total)
}

func TestProcessSharded_EmptySlice(t *testing.T) {
	t.Parallel()

	// Act
	result := concurrency.ProcessSharded([]int{}, 4, func(shard []int) int {
		return len(shard)
	})

	// Assert
	assert.Nil(t, result)
}

func TestProcessSharded_SingleItem(t *testing.T) {
	t.Parallel()

	// Act
	result := concurrency.ProcessSharded([]int{42}, 4, func(shard []int) int {
		total := 0
		for _, v := range shard {
			total += v
		}
		return total
	})

	// Assert
	assert.Equal(t, []int{42}, result)
}

func TestProcessSharded_MoreWorkersThanItems(t *testing.T) {
	t.Parallel()

	// Arrange — 3 items, 10 workers → clamped to 3
	items := []int{1, 2, 3}

	// Act
	result := concurrency.ProcessSharded(items, 10, func(shard []int) int {
		total := 0
		for _, v := range shard {
			total += v
		}
		return total
	})

	// Assert — workers clamped to len(items)
	assert.Len(t, result, 3)
	total := 0
	for _, s := range result {
		total += s
	}
	assert.Equal(t, 6, total)
}

func TestProcessSharded_ZeroWorkers(t *testing.T) {
	t.Parallel()

	// Act — 0 workers defaults to 1
	result := concurrency.ProcessSharded([]int{1, 2, 3}, 0, func(shard []int) int {
		return len(shard)
	})

	// Assert — single shard containing all items
	assert.Equal(t, []int{3}, result)
}

func TestProcessSharded_EvenDistribution(t *testing.T) {
	t.Parallel()

	// Arrange — 12 items / 4 workers = 3 each
	items := make([]int, 12)
	for i := range items {
		items[i] = 1
	}

	// Act
	sizes := concurrency.ProcessSharded(items, 4, func(shard []int) int {
		return len(shard)
	})

	// Assert
	assert.Equal(t, []int{3, 3, 3, 3}, sizes)
}

func TestProcessSharded_UnevenDistribution(t *testing.T) {
	t.Parallel()

	// Arrange — 10 items / 3 workers = 4+3+3
	items := make([]int, 10)
	for i := range items {
		items[i] = 1
	}

	// Act
	sizes := concurrency.ProcessSharded(items, 3, func(shard []int) int {
		return len(shard)
	})

	// Assert — first shard gets the extra item
	assert.Equal(t, []int{4, 3, 3}, sizes)
}

// Package vo provides immutable value objects for the wolf-be domain model.
package vo

import (
	"fmt"
)

const (
	defaultPageLimit = 20
	maxPageLimit     = 100
)

// PageRequest encapsulates cursor-based pagination parameters for list queries.
// Use Validate to check the request before use.
type PageRequest struct {
	// Cursor is the opaque continuation token returned by the previous page.
	// An empty string requests the first page.
	Cursor string `json:"cursor"`
	// Limit is the maximum number of items to return. Zero means use the default.
	// Values above 100 are clamped to 100 by EffectiveLimit.
	Limit int `json:"limit"`
}

// Validate checks that the PageRequest fields are within acceptable bounds.
// Returns a non-nil error if Limit is negative.
func (p PageRequest) Validate() error {
	if p.Limit < 0 {
		return fmt.Errorf("pagination limit must be >= 0, got %d", p.Limit)
	}
	return nil
}

// EffectiveLimit returns the resolved page size, applying the default (20)
// when Limit is zero and capping to the maximum (100) otherwise.
func (p PageRequest) EffectiveLimit() int {
	if p.Limit <= 0 {
		return defaultPageLimit
	}
	if p.Limit > maxPageLimit {
		return maxPageLimit
	}
	return p.Limit
}

// PageResponse is the generic envelope for a single page of results.
// T is the item type (e.g., *OrderSummary).
type PageResponse[T any] struct {
	// Items contains the result set for this page.
	Items []T `json:"items"`
	// NextCursor is the opaque token to pass as Cursor in the next request.
	// Empty when HasMore is false.
	NextCursor string `json:"next_cursor"`
	// HasMore indicates whether additional pages exist beyond this one.
	HasMore bool `json:"has_more"`
	// TotalCount is the total number of matching records across all pages.
	// Zero values are omitted from JSON responses for backward compatibility.
	TotalCount int64 `json:"total_count,omitempty"`
}

// MapPage transforms the items in a PageResponse using the provided mapping function.
// The resulting page preserves NextCursor, HasMore, and TotalCount.
func MapPage[T, U any](page PageResponse[T], fn func(T) U) PageResponse[U] {
	items := make([]U, len(page.Items))
	for i, item := range page.Items {
		items[i] = fn(item)
	}
	return PageResponse[U]{
		Items:      items,
		NextCursor: page.NextCursor,
		HasMore:    page.HasMore,
		TotalCount: page.TotalCount,
	}
}

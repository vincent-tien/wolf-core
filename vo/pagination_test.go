// Package vo provides immutable value objects for the wolf-be domain model.
package vo

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPageRequest_Validate_Valid(t *testing.T) {
	tests := []struct {
		name  string
		req   PageRequest
	}{
		{"zero limit is valid", PageRequest{Limit: 0}},
		{"positive limit is valid", PageRequest{Limit: 50}},
		{"max limit is valid", PageRequest{Limit: 100}},
		{"with cursor", PageRequest{Cursor: "abc123", Limit: 20}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, tc.req.Validate())
		})
	}
}

func TestPageRequest_Validate_Invalid(t *testing.T) {
	tests := []struct {
		name string
		req  PageRequest
	}{
		{"negative limit", PageRequest{Limit: -1}},
		{"very negative limit", PageRequest{Limit: -100}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, tc.req.Validate())
		})
	}
}

func TestPageRequest_EffectiveLimit(t *testing.T) {
	tests := []struct {
		name      string
		req       PageRequest
		wantLimit int
	}{
		{"zero uses default 20", PageRequest{Limit: 0}, 20},
		{"negative uses default 20", PageRequest{Limit: -5}, 20},
		{"explicit 50 is returned", PageRequest{Limit: 50}, 50},
		{"100 is returned as-is", PageRequest{Limit: 100}, 100},
		{"101 is clamped to 100", PageRequest{Limit: 101}, 100},
		{"200 is clamped to 100", PageRequest{Limit: 200}, 100},
		{"1 is returned as-is", PageRequest{Limit: 1}, 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantLimit, tc.req.EffectiveLimit())
		})
	}
}

func TestPageResponse_JSONTags(t *testing.T) {
	// Ensure the generic struct can be instantiated with different item types.
	strPage := PageResponse[string]{
		Items:      []string{"a", "b"},
		NextCursor: "cursor-xyz",
		HasMore:    true,
	}
	assert.Len(t, strPage.Items, 2)
	assert.Equal(t, "cursor-xyz", strPage.NextCursor)
	assert.True(t, strPage.HasMore)

	type item struct{ ID int }
	itemPage := PageResponse[item]{
		Items:   []item{{1}, {2}, {3}},
		HasMore: false,
	}
	assert.Len(t, itemPage.Items, 3)
	assert.False(t, itemPage.HasMore)
	assert.Empty(t, itemPage.NextCursor)
}

func TestPageResponse_EmptyPage(t *testing.T) {
	page := PageResponse[string]{
		Items:   []string{},
		HasMore: false,
	}
	assert.Empty(t, page.Items)
	assert.False(t, page.HasMore)
}

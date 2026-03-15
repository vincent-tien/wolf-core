// Package vo provides immutable value objects for the wolf-be domain model.
package vo

import (
	"testing"
	"time"

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

func TestEncodeCursor_RoundTrip(t *testing.T) {
	ts := time.Date(2026, 3, 15, 10, 30, 45, 123456000, time.UTC)
	id := "550e8400-e29b-41d4-a716-446655440000"

	encoded := EncodeCursor(ts, id)
	assert.NotEmpty(t, encoded)

	gotTime, gotID, err := DecodeCursor(encoded)
	require.NoError(t, err)
	assert.True(t, ts.Equal(gotTime), "timestamps should match")
	assert.Equal(t, id, gotID)
}

func TestEncodeCursor_PreservesSubSecondPrecision(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 999999000, time.UTC)

	encoded := EncodeCursor(ts, "test-id")
	gotTime, _, err := DecodeCursor(encoded)
	require.NoError(t, err)
	assert.Equal(t, ts.UnixNano(), gotTime.UnixNano(), "sub-second precision must be preserved")
}

func TestEncodeCursor_NormalizesToUTC(t *testing.T) {
	loc := time.FixedZone("UTC+7", 7*3600)
	ts := time.Date(2026, 3, 15, 17, 0, 0, 0, loc) // 17:00 UTC+7 = 10:00 UTC

	encoded := EncodeCursor(ts, "id")
	gotTime, _, err := DecodeCursor(encoded)
	require.NoError(t, err)
	assert.Equal(t, time.UTC, gotTime.Location())
	assert.Equal(t, 10, gotTime.Hour())
}

func TestDecodeCursor_InvalidBase64(t *testing.T) {
	_, _, err := DecodeCursor("!!!not-base64!!!")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "decode base64")
}

func TestDecodeCursor_MissingSeparator(t *testing.T) {
	_, _, err := DecodeCursor("bm8tcGlwZS1oZXJl") // base64("no-pipe-here")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid format")
}

func TestDecodeCursor_MalformedTimestamp(t *testing.T) {
	_, _, err := DecodeCursor("bm90LWEtdGltZXxzb21lLWlk") // base64("not-a-time|some-id")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parse time")
}

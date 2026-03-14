package seed

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestValidTableName(t *testing.T) {
	tests := []struct {
		name  string
		table string
		valid bool
	}{
		{"simple", "users", true},
		{"underscore", "user_roles", true},
		{"schema qualified", "public.users", true},
		{"uppercase", "Users", true},
		{"alphanumeric", "table2", true},
		{"starts with underscore", "_temp", true},
		{"sql injection semicolon", "users; DROP TABLE", false},
		{"sql injection dash", "users--", false},
		{"empty", "", false},
		{"space", "user roles", false},
		{"starts with number", "2table", false},
		{"special chars", "table$", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, validTableName.MatchString(tt.table))
		})
	}
}

func TestTruncateTables_InvalidName(t *testing.T) {
	err := TruncateTables(t.Context(), nil, []string{"valid_table", "invalid; DROP"})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid table name")
}

func TestTruncateTables_EmptyList(t *testing.T) {
	err := TruncateTables(t.Context(), nil, nil)
	assert.NoError(t, err)
}

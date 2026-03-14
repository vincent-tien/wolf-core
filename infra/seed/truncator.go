// truncator.go — Cascading TRUNCATE for fresh-seed mode.
package seed

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

// validTableName matches a PostgreSQL table name: "name" or "schema.name".
var validTableName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*(\.[a-zA-Z_][a-zA-Z0-9_]*)?$`)

// TruncateTables truncates the given tables with RESTART IDENTITY CASCADE.
// Table names are validated to prevent SQL injection.
func TruncateTables(ctx context.Context, db DBTX, tables []string) error {
	if len(tables) == 0 {
		return nil
	}

	for _, t := range tables {
		if !validTableName.MatchString(t) {
			return fmt.Errorf("seed: invalid table name %q", t)
		}
	}

	query := fmt.Sprintf("TRUNCATE TABLE %s RESTART IDENTITY CASCADE", strings.Join(tables, ", "))
	_, err := db.ExecContext(ctx, query)
	return err
}

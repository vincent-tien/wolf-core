package deadletter_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/vincent-tien/wolf-core/infra/events/deadletter"
)

// ---------------------------------------------------------------------------
// Minimal in-memory SQL driver for tests
// ---------------------------------------------------------------------------
//
// We do not have go-sqlmock in go.mod, so we implement the minimum driver
// surface required by the deadletter.Store methods under test.
//
// The driver supports three operations identified by the query prefix:
//   INSERT  — records the row in the in-memory table, returns 1 row affected.
//   SELECT  — returns rows from the in-memory table.
//   DELETE  — removes the row by id, returns 1 row affected.

func init() {
	sql.Register("deadletter-test", &testDriver{})
}

// testDB holds the shared in-memory table for a single test.
type testDB struct {
	rows []deadletter.DLQEntry
}

// testDriver / testConn / testStmt / testRows implement database/sql/driver.

type testDriver struct{}

func (d *testDriver) Open(name string) (driver.Conn, error) {
	// name is ignored; every Open returns a fresh connection to the same shared
	// store that is embedded in the dsn parameter as a pointer address.  For
	// simplicity we use a package-level store swapped per test.
	return &testConn{db: currentTestDB}, nil
}

// currentTestDB is swapped by each test via withTestDB().
var currentTestDB *testDB

func withTestDB(t *testing.T) (*sql.DB, *testDB) {
	t.Helper()
	tdb := &testDB{}
	currentTestDB = tdb
	db, err := sql.Open("deadletter-test", "")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = db.Close()
		currentTestDB = nil
	})
	return db, tdb
}

// --- driver.Conn ---

type testConn struct {
	db *testDB
}

func (c *testConn) Prepare(query string) (driver.Stmt, error) {
	return &testStmt{conn: c, query: query}, nil
}

func (c *testConn) Close() error { return nil }

func (c *testConn) Begin() (driver.Tx, error) { return &testTx{}, nil }

type testTx struct{}

func (t *testTx) Commit() error   { return nil }
func (t *testTx) Rollback() error { return nil }

// --- driver.Stmt ---

type testStmt struct {
	conn  *testConn
	query string
}

func (s *testStmt) Close() error                                    { return nil }
func (s *testStmt) NumInput() int                                   { return -1 }
func (s *testStmt) Query(args []driver.Value) (driver.Rows, error) { return s.exec(args) }

func (s *testStmt) Exec(args []driver.Value) (driver.Result, error) {
	rows, err := s.exec(args)
	if err != nil {
		return nil, err
	}
	_ = rows
	return driver.RowsAffected(1), nil
}

func (s *testStmt) exec(args []driver.Value) (driver.Rows, error) {
	db := s.conn.db

	switch detectOp(s.query) {
	case opInsert:
		// args: id, subject, data, headers, error, attempts, original_at
		id := stringVal(args[0])
		// ON CONFLICT DO NOTHING — skip if id already exists
		for _, e := range db.rows {
			if e.ID == id {
				return &emptyRows{}, nil
			}
		}
		originalAt, _ := args[6].(time.Time)
		entry := deadletter.DLQEntry{
			ID:         id,
			Subject:    stringVal(args[1]),
			Data:       bytesVal(args[2]),
			Headers:    bytesVal(args[3]),
			Error:      stringVal(args[4]),
			Attempts:   int(intVal(args[5])),
			OriginalAt: originalAt,
			DeadAt:     time.Now().UTC(),
		}
		db.rows = append(db.rows, entry)
		return &emptyRows{}, nil

	case opSelect:
		limit := int(intVal(args[0]))
		end := limit
		if end > len(db.rows) {
			end = len(db.rows)
		}
		return newDLQRows(db.rows[:end]), nil

	case opDelete:
		id := stringVal(args[0])
		newRows := db.rows[:0]
		for _, e := range db.rows {
			if e.ID != id {
				newRows = append(newRows, e)
			}
		}
		db.rows = newRows
		return &emptyRows{}, nil
	}

	return &emptyRows{}, nil
}

// --- query operation detection ---

type sqlOp int

const (
	opUnknown sqlOp = iota
	opInsert
	opSelect
	opDelete
)

func detectOp(q string) sqlOp {
	// Trim leading whitespace (queries often start with \n\t\t...).
	for i, r := range q {
		if r != ' ' && r != '\t' && r != '\n' && r != '\r' {
			q = q[i:]
			break
		}
	}
	if len(q) < 6 {
		return opUnknown
	}
	upper := strings.ToUpper(q[:6])
	switch upper {
	case "INSERT":
		return opInsert
	case "SELECT":
		return opSelect
	case "DELETE":
		return opDelete
	}
	return opUnknown
}

// --- driver helpers ---

func stringVal(v driver.Value) string {
	if v == nil {
		return ""
	}
	s, _ := v.(string)
	return s
}

func bytesVal(v driver.Value) []byte {
	if v == nil {
		return nil
	}
	b, _ := v.([]byte)
	return b
}

func intVal(v driver.Value) int64 {
	if v == nil {
		return 0
	}
	switch x := v.(type) {
	case int64:
		return x
	case float64:
		return int64(x)
	}
	return 0
}

// --- driver.Rows ---

type emptyRows struct{}

func (r *emptyRows) Columns() []string                   { return nil }
func (r *emptyRows) Close() error                        { return nil }
func (r *emptyRows) Next(_ []driver.Value) error         { return io.EOF }

type dlqRows struct {
	cols []string
	rows [][]driver.Value
	pos  int
}

func newDLQRows(entries []deadletter.DLQEntry) *dlqRows {
	r := &dlqRows{
		cols: []string{"id", "subject", "data", "headers", "error", "attempts", "original_at", "dead_at"},
		rows: make([][]driver.Value, len(entries)),
	}
	for i, e := range entries {
		r.rows[i] = []driver.Value{
			e.ID, e.Subject, e.Data, e.Headers,
			e.Error, int64(e.Attempts), e.OriginalAt, e.DeadAt,
		}
	}
	return r
}

func (r *dlqRows) Columns() []string { return r.cols }
func (r *dlqRows) Close() error      { return nil }

func (r *dlqRows) Next(dest []driver.Value) error {
	if r.pos >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.pos])
	r.pos++
	return nil
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestDLQStore_Insert_AddsEntry(t *testing.T) {
	// Arrange
	db, tdb := withTestDB(t)
	store := deadletter.NewStore(db)

	headers, err := deadletter.MarshalHeaders(map[string]string{"event_type": "order.failed.v1"})
	require.NoError(t, err)

	entry := deadletter.DLQEntry{
		ID:         "dlq-001",
		Subject:    "orders",
		Data:       []byte(`{"order_id":"o-1"}`),
		Headers:    headers,
		Error:      "broker timeout",
		Attempts:   5,
		OriginalAt: time.Now().UTC().Add(-time.Hour),
	}

	// Act
	err = store.Insert(context.Background(), entry)

	// Assert
	require.NoError(t, err)
	require.Len(t, tdb.rows, 1)
	got := tdb.rows[0]
	assert.Equal(t, entry.ID, got.ID)
	assert.Equal(t, entry.Subject, got.Subject)
	assert.Equal(t, entry.Data, got.Data)
	assert.Equal(t, entry.Error, got.Error)
	assert.Equal(t, entry.Attempts, got.Attempts)
}

func TestDLQStore_Insert_OnConflictDoNothing(t *testing.T) {
	// Arrange — insert same ID twice; second should be a no-op.
	db, tdb := withTestDB(t)
	store := deadletter.NewStore(db)

	entry := deadletter.DLQEntry{
		ID:         "dlq-dup",
		Subject:    "orders",
		Data:       []byte(`{}`),
		Error:      "err",
		Attempts:   3,
		OriginalAt: time.Now().UTC(),
	}

	// Act
	require.NoError(t, store.Insert(context.Background(), entry))
	require.NoError(t, store.Insert(context.Background(), entry))

	// Assert — only one row
	assert.Len(t, tdb.rows, 1)
}

func TestDLQStore_GetDeadLetters_ReturnsEntries(t *testing.T) {
	// Arrange
	db, tdb := withTestDB(t)
	store := deadletter.NewStore(db)

	// Seed the in-memory table directly.
	now := time.Now().UTC()
	tdb.rows = []deadletter.DLQEntry{
		{ID: "a", Subject: "s1", Data: []byte(`{}`), Headers: []byte(`{}`),
			Error: "e1", Attempts: 1, OriginalAt: now, DeadAt: now},
		{ID: "b", Subject: "s2", Data: []byte(`{}`), Headers: []byte(`{}`),
			Error: "e2", Attempts: 2, OriginalAt: now, DeadAt: now},
		{ID: "c", Subject: "s3", Data: []byte(`{}`), Headers: []byte(`{}`),
			Error: "e3", Attempts: 3, OriginalAt: now, DeadAt: now},
	}

	// Act
	entries, err := store.GetDeadLetters(context.Background(), 2)

	// Assert
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "a", entries[0].ID)
	assert.Equal(t, "b", entries[1].ID)
}

func TestDLQStore_GetDeadLetters_LimitRespected(t *testing.T) {
	// Arrange
	db, tdb := withTestDB(t)
	store := deadletter.NewStore(db)

	now := time.Now().UTC()
	tdb.rows = []deadletter.DLQEntry{
		{ID: "x", Subject: "s", Data: nil, Headers: nil, Error: "", Attempts: 0, OriginalAt: now, DeadAt: now},
	}

	// Act — ask for up to 10 but only 1 exists.
	entries, err := store.GetDeadLetters(context.Background(), 10)

	// Assert
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestDLQStore_Retry_RemovesEntry(t *testing.T) {
	// Arrange
	db, tdb := withTestDB(t)
	store := deadletter.NewStore(db)

	now := time.Now().UTC()
	tdb.rows = []deadletter.DLQEntry{
		{ID: "keep", Subject: "s", Data: nil, Headers: nil, Error: "", Attempts: 0, OriginalAt: now, DeadAt: now},
		{ID: "remove", Subject: "s", Data: nil, Headers: nil, Error: "", Attempts: 0, OriginalAt: now, DeadAt: now},
	}

	// Act
	err := store.Retry(context.Background(), "remove")

	// Assert
	require.NoError(t, err)
	require.Len(t, tdb.rows, 1)
	assert.Equal(t, "keep", tdb.rows[0].ID)
}

func TestDLQStore_Retry_IdempotentWhenNotFound(t *testing.T) {
	// Arrange — retry on a non-existent ID must not error.
	db, _ := withTestDB(t)
	store := deadletter.NewStore(db)

	// Act
	err := store.Retry(context.Background(), "does-not-exist")

	// Assert
	assert.NoError(t, err)
}

func TestMarshalHeaders_EmptyMap_ReturnsEmptyJSON(t *testing.T) {
	b, err := deadletter.MarshalHeaders(map[string]string{})
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(b))
}

func TestMarshalHeaders_PopulatedMap_RoundTrips(t *testing.T) {
	in := map[string]string{
		"event_type":     "order.created.v1",
		"aggregate_type": "Order",
	}
	b, err := deadletter.MarshalHeaders(in)
	require.NoError(t, err)

	var out map[string]string
	require.NoError(t, json.Unmarshal(b, &out))
	assert.Equal(t, in, out)
}

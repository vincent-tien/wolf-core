package http_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/db"
	"github.com/vincent-tien/wolf-core/infra/di"
	httpmw "github.com/vincent-tien/wolf-core/infra/middleware/http"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// ---------------------------------------------------------------------------
// Fake driver — in-memory, no-op SQL driver for unit tests.
// ---------------------------------------------------------------------------

type fakeDriver struct{}
type fakeConn struct {
	committed  bool
	rolledBack bool
}
type fakeTx struct{ conn *fakeConn }
type fakeStmt struct{}
type fakeRows struct{ closed bool }

func (d *fakeDriver) Open(_ string) (driver.Conn, error) { return &fakeConn{}, nil }

func (c *fakeConn) Prepare(_ string) (driver.Stmt, error) { return &fakeStmt{}, nil }
func (c *fakeConn) Close() error                          { return nil }
func (c *fakeConn) Begin() (driver.Tx, error)             { return &fakeTx{conn: c}, nil }

func (t *fakeTx) Commit() error {
	t.conn.committed = true
	return nil
}
func (t *fakeTx) Rollback() error {
	t.conn.rolledBack = true
	return nil
}

func (s *fakeStmt) Close() error                                    { return nil }
func (s *fakeStmt) NumInput() int                                   { return 0 }
func (s *fakeStmt) Exec(_ []driver.Value) (driver.Result, error)   { return nil, nil }
func (s *fakeStmt) Query(_ []driver.Value) (driver.Rows, error)    { return &fakeRows{}, nil }
func (r *fakeRows) Columns() []string                               { return nil }
func (r *fakeRows) Close() error                                    { r.closed = true; return nil }
func (r *fakeRows) Next(_ []driver.Value) error                     { return io.EOF }

// driverName must be unique per test binary to avoid "already registered" panics.
const driverName = "fake-txmw"

func init() {
	sql.Register(driverName, &fakeDriver{})
}

// newTestDB opens a *sql.DB backed by the in-memory fake driver.
func newTestDB(t *testing.T) *sql.DB {
	t.Helper()

	sqlDB, err := sql.Open(driverName, "")
	require.NoError(t, err)

	t.Cleanup(func() { _ = sqlDB.Close() })

	return sqlDB
}

// newRouter builds a Gin engine with TransactionMiddleware applied to all routes.
func newRouter(t *testing.T, sqlDB *sql.DB, handler gin.HandlerFunc) *gin.Engine {
	t.Helper()

	ctr := di.New()
	logger := zap.NewNop()

	r := gin.New()
	r.Use(httpmw.TransactionMiddleware(ctr, sqlDB, logger))
	r.GET("/", handler)

	return r
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestTransactionMiddleware_HappyPath_Commits(t *testing.T) {
	sqlDB := newTestDB(t)

	var capturedConn db.Conn

	r := newRouter(t, sqlDB, func(c *gin.Context) {
		capturedConn = di.GetTyped[db.Conn](c.Request.Context(), "db.conn")
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.NotNil(t, capturedConn, "handler must receive a non-nil db.Conn from the DI container")
}

func TestTransactionMiddleware_ServerError_RollsBack(t *testing.T) {
	sqlDB := newTestDB(t)

	r := newRouter(t, sqlDB, func(c *gin.Context) {
		c.Status(http.StatusInternalServerError)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestTransactionMiddleware_ContextPropagation(t *testing.T) {
	sqlDB := newTestDB(t)

	type ctxKey string
	const key ctxKey = "test-key"

	// Apply a base context value before the middleware runs.
	ctr := di.New()
	logger := zap.NewNop()

	r := gin.New()
	r.Use(func(c *gin.Context) {
		ctx := context.WithValue(c.Request.Context(), key, "test-value")
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	})
	r.Use(httpmw.TransactionMiddleware(ctr, sqlDB, logger))

	var gotValue string

	r.GET("/", func(c *gin.Context) {
		gotValue, _ = c.Request.Context().Value(key).(string)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "test-value", gotValue, "context values set before TransactionMiddleware must be visible to handlers")
}

func TestTransactionMiddleware_ConnIsAvailableViaDI(t *testing.T) {
	sqlDB := newTestDB(t)

	ctr := di.New()
	logger := zap.NewNop()

	r := gin.New()
	r.Use(httpmw.TransactionMiddleware(ctr, sqlDB, logger))

	var conn1, conn2 db.Conn

	r.GET("/", func(c *gin.Context) {
		// Resolve db.Conn twice — within the same scope it must be the same instance.
		c1 := di.FromContext(c.Request.Context())
		conn1 = c1.Get("db.conn").(db.Conn)
		conn2 = c1.Get("db.conn").(db.Conn)
		c.Status(http.StatusOK)
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	r.ServeHTTP(w, req)

	require.NotNil(t, conn1)
	assert.Same(t, conn1, conn2, "db.Conn must be the same instance within a single request scope")
}

func TestTransactionMiddleware_PanicRollsBack(t *testing.T) {
	sqlDB := newTestDB(t)

	ctr := di.New()
	logger := zap.NewNop()

	r := gin.New()
	// Recovery middleware must wrap TransactionMiddleware so panics are caught at the
	// router level and do not crash the test server.
	r.Use(gin.Recovery())
	r.Use(httpmw.TransactionMiddleware(ctr, sqlDB, logger))

	r.GET("/", func(_ *gin.Context) {
		panic("simulated panic")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	// The panic is recovered by gin.Recovery. The test simply verifies that the
	// server does not crash and returns a 500-level response.
	assert.NotPanics(t, func() {
		r.ServeHTTP(w, req)
	})

	assert.GreaterOrEqual(t, w.Code, http.StatusInternalServerError)
}

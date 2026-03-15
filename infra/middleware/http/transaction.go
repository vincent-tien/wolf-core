// transaction.go — Per-request database transaction middleware (commit on <500, rollback on 5xx).
package http

import (
	"database/sql"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/db"
	"github.com/vincent-tien/wolf-core/infra/di"
	wolfhttp "github.com/vincent-tien/wolf-core/infra/http"
)

// connKey is the DI container key under which the active *sql.Tx is registered
// as a db.Conn for the duration of a request.
const connKey = "db.conn"

// TransactionMiddleware returns a Gin middleware that wraps each request in a
// database transaction.
//
// On every request:
//  1. A new DI scope is created from ctr and stored in the request context.
//  2. A transaction is started against writeDB and registered in the scope
//     under the key "db.conn" as a db.Conn.
//  3. Downstream handlers retrieve the transaction-backed db.Conn via
//     di.GetTyped[db.Conn](ctx, "db.conn").
//  4. After all handlers return, if the HTTP response status is < 500 the
//     transaction is committed; otherwise it is rolled back. A panic in any
//     handler also triggers a rollback (and re-panics after).
//
// The scoped context (carrying the DI container) is propagated by replacing
// c.Request with the new context so that c.Request.Context() always returns
// the enriched context.
func TransactionMiddleware(ctr di.Container, writeDB *sql.DB, logger *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 1. Create a scoped DI container for this request.
		ctx := ctr.Scoped(c.Request.Context())
		c.Request = c.Request.WithContext(ctx)

		// 2. Begin the transaction.
		tx, err := writeDB.BeginTx(ctx, nil)
		if err != nil {
			logger.Error("transaction middleware: begin tx failed", zap.Error(err))
			wolfhttp.AbortInternalError(c, "failed to begin database transaction")

			return
		}

		// 3. Register the transaction as a db.Conn in the scoped container.
		scopedCtr := di.FromContext(ctx)
		scopedCtr.AddScoped(connKey, func(_ di.Container) any {
			return db.Conn(tx)
		})

		// 4. Ensure rollback on panic before re-propagating.
		defer func() {
			if p := recover(); p != nil {
				_ = tx.Rollback()
				panic(p)
			}
		}()

		// 5. Execute the handler chain.
		c.Next()

		// 6. Commit on success, rollback on application error.
		if c.Writer.Status() >= http.StatusInternalServerError {
			if rbErr := tx.Rollback(); rbErr != nil {
				logger.Error("transaction middleware: rollback failed",
					zap.Int("status", c.Writer.Status()),
					zap.Error(rbErr),
				)
			}

			return
		}

		if commitErr := tx.Commit(); commitErr != nil {
			logger.Error("transaction middleware: commit failed", zap.Error(commitErr))
			wolfhttp.AbortInternalError(c, "failed to commit database transaction")
		}
	}
}

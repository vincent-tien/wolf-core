// transaction.go — Per-request database transaction interceptor for gRPC.
package grpc

import (
	"context"
	"database/sql"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	grpcstatus "google.golang.org/grpc/status"

	"github.com/vincent-tien/wolf-core/infra/db"
	"github.com/vincent-tien/wolf-core/infra/di"
)

// connKey is the DI container key under which the active *sql.Tx is registered
// as a db.Conn for the duration of a gRPC call.
const connKey = "db.conn"

// TransactionInterceptor returns a gRPC unary server interceptor that wraps
// each RPC in a database transaction.
//
// On every call:
//  1. A new DI scope is created from ctr and stored in the call context.
//  2. A transaction is started against writeDB and registered in the scope
//     under the key "db.conn" as a db.Conn.
//  3. Downstream handlers retrieve the transaction-backed db.Conn via
//     di.GetTyped[db.Conn](ctx, "db.conn").
//  4. If the handler returns nil error the transaction is committed; any
//     non-nil error triggers a rollback. A panic also triggers a rollback
//     before being re-propagated.
func TransactionInterceptor(ctr di.Container, writeDB *sql.DB, logger *zap.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		// 1. Create a scoped DI container for this RPC call.
		ctx = ctr.Scoped(ctx)

		// 2. Begin the transaction.
		tx, beginErr := writeDB.BeginTx(ctx, nil)
		if beginErr != nil {
			logger.Error("transaction interceptor: begin tx failed", zap.Error(beginErr))
			return nil, grpcstatus.Error(codes.Internal, "failed to begin database transaction")
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

		// 5. Execute the handler.
		resp, err = handler(ctx, req)

		// 6. Commit on success, rollback on handler error.
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil {
				logger.Error("transaction interceptor: rollback failed",
					zap.NamedError("handler_error", err),
					zap.NamedError("rollback_error", rbErr),
				)
			}

			return nil, err
		}

		if commitErr := tx.Commit(); commitErr != nil {
			logger.Error("transaction interceptor: commit failed", zap.Error(commitErr))
			return nil, grpcstatus.Error(codes.Internal, "failed to commit database transaction")
		}

		return resp, nil
	}
}

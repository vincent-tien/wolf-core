// notifier.go — PostgreSQL LISTEN/NOTIFY listener for near-real-time outbox relay.
package outbox

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

// NotifyChannel is the PostgreSQL LISTEN channel name. A trigger on the
// outbox_events table must issue:
//
//	PERFORM pg_notify('outbox_events_inserted', '');
const NotifyChannel = "outbox_events_inserted"

// reconnectDelay is the pause between reconnect attempts when the LISTEN
// connection drops.
const reconnectDelay = 3 * time.Second

// Notifier maintains a dedicated pgx connection that LISTENs for
// outbox_events_inserted notifications. Each notification sends a signal on
// the Wake channel so the outbox Worker can poll immediately instead of
// waiting for the next ticker tick.
//
// Architecture decisions:
//   - Dedicated pgx.Conn (not from sql.DB pool) — PostgreSQL LISTEN/NOTIFY
//     requires a persistent, non-pooled connection. Pooled connections may be
//     returned to the pool, losing the LISTEN subscription silently.
//   - Buffered channel(1) — coalesces multiple rapid INSERTs into a single
//     wake signal. The worker polls the full batch regardless, so queuing
//     multiple signals would waste poll cycles.
//   - Auto-reconnect with time.NewTimer — not time.After (which leaks timers
//     in loops). On connection loss, the worker falls back to ticker polling
//     until the Notifier reconnects.
//   - FOR EACH STATEMENT trigger (not FOR EACH ROW) — a batch INSERT of N
//     events fires one notification, not N. This prevents notification storms
//     during bulk outbox writes.
type Notifier struct {
	dsn    string
	logger *zap.Logger
	wake   chan struct{}
}

// NewNotifier creates a Notifier that connects to dsn and LISTENs for
// outbox inserts. Call Start to begin listening.
func NewNotifier(dsn string, logger *zap.Logger) *Notifier {
	return &Notifier{
		dsn:    dsn,
		logger: logger,
		wake:   make(chan struct{}, 1),
	}
}

// Wake returns the channel that receives a signal on every NOTIFY.
// Pass this to Worker.WithNotify.
func (n *Notifier) Wake() <-chan struct{} {
	return n.wake
}

// Start connects to PostgreSQL, issues LISTEN, and loops waiting for
// notifications until ctx is cancelled. It reconnects automatically on
// connection loss.
func (n *Notifier) Start(ctx context.Context) error {
	reconnect := time.NewTimer(0)
	reconnect.Stop()
	defer reconnect.Stop()

	for {
		if err := n.listenLoop(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			n.logger.Warn("outbox: notifier connection lost, reconnecting",
				zap.Error(err),
				zap.Duration("delay", reconnectDelay),
			)
			reconnect.Reset(reconnectDelay)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-reconnect.C:
			}
		}
	}
}

// listenLoop opens a connection, issues LISTEN, and blocks on
// WaitForNotification until an error occurs or ctx is cancelled.
func (n *Notifier) listenLoop(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, n.dsn)
	if err != nil {
		return fmt.Errorf("outbox: notifier connect: %w", err)
	}
	defer conn.Close(context.Background())

	if _, err := conn.Exec(ctx, "LISTEN "+NotifyChannel); err != nil {
		return fmt.Errorf("outbox: notifier LISTEN: %w", err)
	}

	n.logger.Info("outbox: notifier listening", zap.String("channel", NotifyChannel))

	for {
		_, err := conn.WaitForNotification(ctx)
		if err != nil {
			return fmt.Errorf("outbox: notifier wait: %w", err)
		}

		// Non-blocking send — if a signal is already pending the worker
		// will process it; we don't need to queue multiple signals.
		select {
		case n.wake <- struct{}{}:
		default:
		}
	}
}

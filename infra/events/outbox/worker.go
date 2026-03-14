// worker.go — Outbox relay: polls unpublished entries, publishes to broker, escalates to DLQ.
package outbox

import (
	"context"
	"fmt"
	"time"

	"github.com/sony/gobreaker"
	"go.uber.org/zap"

	"github.com/vincent-tien/wolf-core/infra/events/deadletter"
	"github.com/vincent-tien/wolf-core/infra/observability/metrics"
	"github.com/vincent-tien/wolf-core/event"
)

const (
	defaultPollTimeout = 30 * time.Second
	maxBackoffShift    = 4 // 2^4 = 16x max multiplier
	cleanupEvery       = 100
)

// WorkerStore is the subset of Store operations the Worker needs. Extracting
// an interface allows unit testing with mocks and keeps the Worker decoupled
// from the concrete SQL implementation.
type WorkerStore interface {
	ClaimBatch(ctx context.Context, batchSize int) ([]OutboxEntry, error)
	MarkPublished(ctx context.Context, ids []string, leaseToken string) error
	ReleaseClaim(ctx context.Context, id string, lastError string) error
	ReleaseClaims(ctx context.Context, entries []ReleaseEntry) error
	Cleanup(ctx context.Context, retention time.Duration) (int64, error)
	CountPending(ctx context.Context) (int64, error)
}

// DLQInserter is the subset of deadletter.Store the Worker needs. Using an
// interface instead of the concrete *deadletter.Store allows unit testing of
// the DLQ failure path without a database.
type DLQInserter interface {
	Insert(ctx context.Context, entry deadletter.DLQEntry) error
}

// Worker polls the outbox store on a configurable interval, publishes pending
// entries to the event bus, marks them as published, and periodically removes
// entries older than the configured retention period.
//
// When an entry's retry count reaches maxRetries the worker moves it to the
// dead letter queue via dlqStore and marks it as published so it is no longer
// returned by ClaimBatch. If no dlqStore is provided, exhausted entries are
// only logged at warn level (backward-compatible behaviour).
//
// The worker uses a circuit breaker around publish calls. When the broker is
// down, the breaker opens and poll cycles short-circuit until it recovers.
//
// Consecutive poll errors trigger exponential backoff on the ticker interval
// (up to 16x the base interval) to avoid log storms and wasted CPU.
type Worker struct {
	store          WorkerStore
	dlqStore       DLQInserter
	bus            event.Publisher
	cb             *gobreaker.CircuitBreaker
	notify         <-chan struct{}
	pollInterval   time.Duration
	pollTimeout    time.Duration
	batchSize      int
	maxRetries     int
	retention      time.Duration
	logger         *zap.Logger
	metrics        *metrics.Metrics
	consecutiveErr int
}

// NewWorker constructs a Worker with the given configuration.
func NewWorker(
	store WorkerStore,
	bus event.Publisher,
	pollInterval time.Duration,
	batchSize, maxRetries int,
	retention time.Duration,
	logger *zap.Logger,
	m *metrics.Metrics,
) *Worker {
	return &Worker{
		store:        store,
		bus:          bus,
		pollInterval: pollInterval,
		pollTimeout:  defaultPollTimeout,
		batchSize:    batchSize,
		maxRetries:   maxRetries,
		retention:    retention,
		logger:       logger,
		metrics:      m,
	}
}

// WithDLQ configures dead letter persistence. Accepts any DLQInserter
// implementation (the concrete *deadletter.Store satisfies this interface).
func (w *Worker) WithDLQ(dlqStore DLQInserter) *Worker {
	w.dlqStore = dlqStore
	return w
}

// WithCircuitBreaker configures a circuit breaker around publish calls.
func (w *Worker) WithCircuitBreaker(cb *gobreaker.CircuitBreaker) *Worker {
	w.cb = cb
	return w
}

// WithNotify configures a channel that triggers an immediate poll cycle when
// a signal is received. Used with Notifier (LISTEN/NOTIFY) to reduce latency
// between outbox insert and publish. The ticker remains as a fallback.
func (w *Worker) WithNotify(ch <-chan struct{}) *Worker {
	w.notify = ch
	return w
}

// WithPollTimeout overrides the default 30s per-cycle context timeout.
func (w *Worker) WithPollTimeout(d time.Duration) *Worker {
	if d > 0 {
		w.pollTimeout = d
	}
	return w
}

// Start runs the outbox polling loop until ctx is cancelled. When a notify
// channel is configured (via WithNotify / LISTEN/NOTIFY), the worker also
// wakes immediately on database insert signals. The ticker remains as a
// fallback to handle missed notifications or reconnect gaps.
func (w *Worker) Start(ctx context.Context) error {
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	cycle := 0

	w.logger.Info("outbox: worker started",
		zap.Duration("poll_interval", w.pollInterval),
		zap.Duration("poll_timeout", w.pollTimeout),
		zap.Int("batch_size", w.batchSize),
		zap.Bool("notify_enabled", w.notify != nil),
	)

	// notifyCh is nil-safe: selecting on a nil channel blocks forever,
	// which is exactly what we want when no notifier is configured.
	notifyCh := w.notify

	for {
		select {
		case <-ctx.Done():
			w.logger.Info("outbox: worker stopping", zap.Error(ctx.Err()))
			return ctx.Err()

		case <-ticker.C:
			w.doPoll(ctx, ticker, &cycle)

		case <-notifyCh:
			w.doPoll(ctx, ticker, &cycle)
			// Reset ticker so the next tick is a full interval from now,
			// avoiding a redundant poll shortly after the notify-driven one.
			ticker.Reset(w.pollInterval)
		}
	}
}

// doPoll executes a single poll cycle and handles errors, backoff, and
// periodic cleanup. Extracted to avoid duplicating the logic between the
// ticker and notify select cases.
func (w *Worker) doPoll(ctx context.Context, ticker *time.Ticker, cycle *int) {
	pollCtx, pollCancel := context.WithTimeout(ctx, w.pollTimeout)
	defer pollCancel()

	if err := w.poll(pollCtx); err != nil {
		w.logger.Error("outbox: poll cycle error", zap.Error(err))
		w.consecutiveErr++
		w.adjustBackoff(ticker)
	} else {
		if w.consecutiveErr > 0 {
			w.consecutiveErr = 0
			ticker.Reset(w.pollInterval)
		}
	}

	*cycle++
	if *cycle%cleanupEvery == 0 {
		cleanupCtx, cleanupCancel := context.WithTimeout(ctx, w.pollTimeout)
		n, err := w.store.Cleanup(cleanupCtx, w.retention)
		cleanupCancel() // cancel immediately — Cleanup is synchronous
		if err != nil {
			w.logger.Error("outbox: cleanup error", zap.Error(err))
		} else if n > 0 {
			w.logger.Info("outbox: cleanup complete", zap.Int64("deleted", n))
		}

		// Sample queue depth on the cleanup cadence (every 10 polls) to
		// avoid a COUNT(*) round-trip on every active poll cycle.
		if w.metrics != nil {
			if pending, err := w.store.CountPending(ctx); err == nil {
				w.metrics.OutboxQueueDepth.Set(float64(pending))
			}
		}
	}
}

// adjustBackoff increases the ticker interval exponentially on consecutive
// errors, capped at 2^maxBackoffShift times the base interval.
func (w *Worker) adjustBackoff(ticker *time.Ticker) {
	shift := w.consecutiveErr
	if shift > maxBackoffShift {
		shift = maxBackoffShift
	}
	backoff := w.pollInterval * time.Duration(1<<shift)
	ticker.Reset(backoff)
	w.logger.Warn("outbox: backing off",
		zap.Duration("next_interval", backoff),
		zap.Int("consecutive_errors", w.consecutiveErr),
	)
}

// poll claims one batch of entries, publishes each to the broker, then marks
// successfully delivered entries as published. On publish failure, ReleaseClaim
// returns the entry to the unclaimed pool with an incremented retry count.
//
// The key safety guarantee: published_at is set AFTER broker acknowledgement,
// not before. If the worker crashes between claim and publish, entries become
// re-claimable after leaseTimeout expires.
func (w *Worker) poll(ctx context.Context) error {
	if w.metrics != nil {
		start := time.Now()
		defer func() {
			w.metrics.OutboxPollDuration.Observe(time.Since(start).Seconds())
		}()
	}

	entries, err := w.store.ClaimBatch(ctx, w.batchSize)
	if err != nil {
		return fmt.Errorf("outbox: claim batch: %w", err)
	}

	if w.metrics != nil {
		w.metrics.OutboxPolledTotal.Add(float64(len(entries)))
	}

	if len(entries) == 0 {
		return nil
	}

	publishedIDs := make([]string, 0, len(entries))
	failed := make([]ReleaseEntry, 0, len(entries))

	for _, entry := range entries {
		if entry.RetryCount >= w.maxRetries {
			if err := w.handleExhaustedEntry(ctx, entry); err != nil {
				// DLQ persistence failed — release claim so the entry is retried
				// on the next poll cycle. Do NOT mark published.
				failed = append(failed, ReleaseEntry{ID: entry.ID, LastError: err.Error()})
				continue
			}
			// DLQ insert succeeded (or no DLQ configured) — safe to mark published.
			publishedIDs = append(publishedIDs, entry.ID)
			continue
		}

		if err := w.publishEntry(ctx, entry); err != nil {
			failed = append(failed, ReleaseEntry{ID: entry.ID, LastError: err.Error()})
			continue
		}

		publishedIDs = append(publishedIDs, entry.ID)
		if w.metrics != nil {
			w.metrics.OutboxPublishTotal.WithLabelValues("success").Inc()
		}
	}

	// Mark published AFTER broker ack — the core durability guarantee.
	// All entries in a batch share the same lease token from ClaimBatch.
	if len(publishedIDs) > 0 {
		leaseToken := entries[0].LeaseToken
		if markErr := w.store.MarkPublished(ctx, publishedIDs, leaseToken); markErr != nil {
			w.logger.Error("outbox: mark published failed",
				zap.Int("count", len(publishedIDs)),
				zap.Error(markErr),
			)
			if w.metrics != nil {
				w.metrics.OutboxMarkPublishedErrors.Inc()
			}
		}
		w.logger.Debug("outbox: batch published", zap.Int("count", len(publishedIDs)))
	}

	if len(failed) > 0 {
		if releaseErr := w.store.ReleaseClaims(ctx, failed); releaseErr != nil {
			w.logger.Error("outbox: batch release claims failed",
				zap.Int("count", len(failed)),
				zap.Error(releaseErr),
			)
		}
	}

	return nil
}

// publishEntry publishes a single outbox entry, optionally wrapping the call
// in a circuit breaker.
func (w *Worker) publishEntry(ctx context.Context, entry OutboxEntry) error {
	evt := &envelopeEvent{entry: entry}

	publish := func() error {
		return w.bus.Publish(ctx, evt)
	}

	var err error
	if w.cb != nil {
		_, cbErr := w.cb.Execute(func() (any, error) {
			return nil, publish()
		})
		err = cbErr
	} else {
		err = publish()
	}

	if err != nil {
		w.logger.Error("outbox: publish failed",
			zap.String("id", entry.ID),
			zap.String("event_type", entry.EventType),
			zap.Error(err),
		)
		if w.metrics != nil {
			w.metrics.OutboxPublishTotal.WithLabelValues("error").Inc()
		}
		return err
	}

	return nil
}

// handleExhaustedEntry moves an entry that exceeded max retries to the DLQ
// (if configured). Returns nil only when the entry is safely persisted in the
// DLQ (or no DLQ is configured). The caller MUST NOT mark the entry as
// published unless this returns nil — doing so would silently lose the event.
func (w *Worker) handleExhaustedEntry(ctx context.Context, entry OutboxEntry) error {
	w.logger.Warn("outbox: entry exceeded max retries, moving to DLQ",
		zap.String("id", entry.ID),
		zap.String("event_type", entry.EventType),
		zap.Int("retry_count", entry.RetryCount),
		zap.String("last_error", entry.LastError),
	)

	if w.dlqStore != nil {
		if dlqErr := w.moveToDLQ(ctx, entry); dlqErr != nil {
			w.logger.Error("outbox: failed to move entry to DLQ — entry will NOT be marked published",
				zap.String("id", entry.ID),
				zap.Error(dlqErr),
			)
			if w.metrics != nil {
				w.metrics.OutboxDLQInsertErrors.Inc()
			}
			return fmt.Errorf("outbox: DLQ insert failed for %q: %w", entry.ID, dlqErr)
		}
	}

	if w.metrics != nil {
		w.metrics.OutboxDLQTotal.Inc()
	}
	return nil
}

// moveToDLQ builds a DLQEntry from entry and inserts it into the dead letter
// queue.
func (w *Worker) moveToDLQ(ctx context.Context, entry OutboxEntry) error {
	headers := map[string]string{
		"aggregate_type": entry.AggregateType,
		"aggregate_id":   entry.AggregateID,
		"event_type":     entry.EventType,
	}
	for k, v := range entry.Metadata.ToMap() {
		headers[k] = v
	}

	headersJSON, err := deadletter.MarshalHeaders(headers)
	if err != nil {
		return fmt.Errorf("outbox: marshal headers for DLQ entry %q: %w", entry.ID, err)
	}

	dlqEntry := deadletter.DLQEntry{
		ID:         entry.ID,
		Subject:    entry.EventType,
		Data:       entry.Payload,
		Headers:    headersJSON,
		Error:      entry.LastError,
		Attempts:   entry.RetryCount,
		OriginalAt: entry.CreatedAt,
	}

	if err := w.dlqStore.Insert(ctx, dlqEntry); err != nil {
		return fmt.Errorf("outbox: DLQ insert for %q: %w", entry.ID, err)
	}

	return nil
}

// envelopeEvent adapts an OutboxEntry to the event.Event interface.
type envelopeEvent struct {
	entry OutboxEntry
}

func (e *envelopeEvent) EventID() string       { return e.entry.ID }
func (e *envelopeEvent) EventType() string     { return e.entry.EventType }
func (e *envelopeEvent) AggregateID() string   { return e.entry.AggregateID }
func (e *envelopeEvent) AggregateType() string { return e.entry.AggregateType }
func (e *envelopeEvent) OccurredAt() time.Time { return e.entry.CreatedAt }
func (e *envelopeEvent) Version() int          { return 1 }
func (e *envelopeEvent) Payload() any          { return e.entry.Payload }
func (e *envelopeEvent) GetMetadata() event.Metadata {
	return e.entry.Metadata
}

// Package uow provides the Unit of Work pattern for atomic aggregate persistence
// with transactional outbox event publishing.
//
// The core problem it solves: every command handler that must persist an
// aggregate AND emit domain events previously duplicated ~15 lines of
// tx+outbox boilerplate, and some implementations called ClearEvents BEFORE
// the transaction started — meaning events could be silently dropped if the
// transaction rolled back. UnitOfWork fixes both issues by collecting events
// INSIDE the transaction, after the aggregate has been persisted successfully.
package uow

import (
	"context"
	"fmt"

	"github.com/vincent-tien/wolf-core/event"
	"github.com/vincent-tien/wolf-core/tx"
)

// Aggregate is the interface constraint for domain aggregates that participate
// in the Unit of Work. It is satisfied by any type embedding aggregate.Base.
type Aggregate interface {
	// ClearEvents drains and returns all pending domain events. The UnitOfWork
	// calls this INSIDE the open transaction so that events are only consumed
	// when the persistence step has already succeeded.
	ClearEvents() []event.Event
}

// OutboxInserter is the write-side port for inserting a domain event into the
// transactional outbox. The transaction is carried in ctx (via tx.Inject),
// keeping this interface free of database/sql imports.
type OutboxInserter interface {
	Insert(ctx context.Context, evt event.Event, meta event.Metadata) error
}

// UnitOfWork coordinates aggregate persistence and outbox event publishing
// within a single database transaction. It eliminates per-handler boilerplate
// and guarantees that ClearEvents is called only after the domain save
// succeeds, preventing silent event loss on rollback.
type UnitOfWork struct {
	txRunner    tx.Runner
	outboxStore OutboxInserter
	source      string
}

// New constructs a UnitOfWork with the given transaction runner, outbox
// inserter, and source identifier. The source string is stamped on every
// outbox event so the relay worker can identify the originating service.
func New(txRunner tx.Runner, outboxStore OutboxInserter, source string) *UnitOfWork {
	if txRunner == nil {
		panic("uow: txRunner must not be nil")
	}
	if outboxStore == nil {
		panic("uow: outboxStore must not be nil")
	}
	if source == "" {
		panic("uow: source must not be empty")
	}
	return &UnitOfWork{
		txRunner:    txRunner,
		outboxStore: outboxStore,
		source:      source,
	}
}

// Execute runs fn inside a database transaction, then collects events from agg
// and inserts them into the outbox table within the same transaction.
//
// Ordering guarantee: fn (aggregate persistence) runs first. Only if fn
// succeeds are events drained from the aggregate via ClearEvents and written
// to the outbox. If either step fails the entire transaction rolls back,
// leaving both domain state and outbox unchanged.
func (u *UnitOfWork) Execute(ctx context.Context, agg Aggregate, fn func(txCtx context.Context) error) error {
	return u.txRunner.RunInTx(ctx, func(txCtx context.Context) error {
		if err := fn(txCtx); err != nil {
			return err
		}
		return u.publishEvents(txCtx, agg)
	})
}

// ExecuteMulti runs fn inside a database transaction, then collects events from
// every aggregate in aggs and inserts them into the outbox table.
//
// Aggregates are drained in iteration order. The first insertion error aborts
// the transaction, rolling back all domain state and all previously inserted
// outbox rows for this transaction.
func (u *UnitOfWork) ExecuteMulti(ctx context.Context, aggs []Aggregate, fn func(txCtx context.Context) error) error {
	return u.txRunner.RunInTx(ctx, func(txCtx context.Context) error {
		if err := fn(txCtx); err != nil {
			return err
		}
		for _, agg := range aggs {
			if err := u.publishEvents(txCtx, agg); err != nil {
				return err
			}
		}
		return nil
	})
}

// publishEvents drains pending events from agg and writes each one to the
// outbox inside the already-open transaction carried in txCtx. Source is
// stamped on the metadata so the relay worker can identify the originating service.
func (u *UnitOfWork) publishEvents(txCtx context.Context, agg Aggregate) error {
	events := agg.ClearEvents()
	meta := event.Metadata{Source: u.source}
	for _, evt := range events {
		if err := u.outboxStore.Insert(txCtx, evt, meta); err != nil {
			return fmt.Errorf("uow: publish event %q: %w", evt.EventType(), err)
		}
	}
	return nil
}

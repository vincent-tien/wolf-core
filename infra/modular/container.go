// Package modular provides the self-registering module system for wolf-be.
//
// Architecture boundary: modular sits between bootstrap (which owns concrete
// infrastructure) and modules (which must not depend on bootstrap internals).
// Container acts as a typed dependency bag — the only coupling point between
// platform and module layers.
//
// Modules describe themselves via a Factory function, and the global catalog
// feeds them into bootstrap without manual wiring. See catalog.go for the
// registration pattern and registry.go for boot ordering.
package modular

import (
	"database/sql"

	"go.uber.org/zap"

	platformauth "github.com/vincent-tien/wolf-core/infra/auth"
	"github.com/vincent-tien/wolf-core/infra/cache"
	"github.com/vincent-tien/wolf-core/infra/config"
	"github.com/vincent-tien/wolf-core/infra/db"
	"github.com/vincent-tien/wolf-core/infra/events/outbox"
	httpmw "github.com/vincent-tien/wolf-core/infra/middleware/http"
	"github.com/vincent-tien/wolf-core/infra/observability/metrics"
	"github.com/vincent-tien/wolf-core/event"
	"github.com/vincent-tien/wolf-core/messaging"
	"github.com/vincent-tien/wolf-core/tx"
)

// Container is the typed dependency bag that module factories receive.
// It exposes every platform dependency a module might need, avoiding
// the need for modules to know about bootstrap internals.
type Container struct {
	Config         *config.Config
	Logger         *zap.Logger
	WriteDB        *sql.DB
	ReadDB         *sql.DB
	TxManager      db.TxManager
	TxRunner       tx.Runner
	Cache          cache.Client
	OutboxStore    *outbox.Store
	EventBus       event.Bus
	Stream         messaging.Stream
	JWTService     *platformauth.JWTService
	AuthMiddleware *httpmw.AuthMiddleware
	RBACMiddleware *httpmw.RBACMiddleware
	TypeRegistry   *event.TypeRegistry
	EventStream    *messaging.EventStream
	Metrics        *metrics.Metrics
}

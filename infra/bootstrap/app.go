// Package bootstrap is the composition root that wires all platform
// dependencies and modules together. It owns the construction order,
// readiness registration, and the concurrent startup of every server.
package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	platformauth "github.com/vincent-tien/wolf-core/infra/auth"
	"github.com/vincent-tien/wolf-core/infra/cache"
	"github.com/vincent-tien/wolf-core/infra/config"
	"github.com/vincent-tien/wolf-core/infra/db"
	"github.com/vincent-tien/wolf-core/infra/events"
	"github.com/vincent-tien/wolf-core/infra/events/deadletter"
	"github.com/vincent-tien/wolf-core/infra/events/outbox"
	platformgrpc "github.com/vincent-tien/wolf-core/infra/grpc"
	platformhttp "github.com/vincent-tien/wolf-core/infra/http"
	grpcmw "github.com/vincent-tien/wolf-core/infra/middleware/grpc"
	httpmw "github.com/vincent-tien/wolf-core/infra/middleware/http"
	"github.com/vincent-tien/wolf-core/infra/modular"
	"github.com/vincent-tien/wolf-core/infra/resilience"
	"github.com/vincent-tien/wolf-core/infra/observability/logging"
	"github.com/vincent-tien/wolf-core/infra/observability/metrics"
	"github.com/vincent-tien/wolf-core/infra/observability/tracing"
	sharedauth "github.com/vincent-tien/wolf-core/auth"
	"github.com/vincent-tien/wolf-core/clock"
	sharedevent "github.com/vincent-tien/wolf-core/event"
	"github.com/vincent-tien/wolf-core/messaging"
	"github.com/vincent-tien/wolf-core/runtime"
	"github.com/vincent-tien/wolf-core/tx"
)

// App is the fully wired application. It owns every platform dependency and
// drives the lifecycle of all registered modules and servers.
type App struct {
	cfg            *config.Config
	logger         *zap.Logger
	writeDB        *sql.DB
	readDB         *sql.DB
	cacheClient    cache.Client
	eventBus       sharedevent.Bus
	txManager      db.TxManager
	txRunner       tx.Runner
	outboxStore    *outbox.Store
	outboxWorker   *outbox.Worker
	outboxNotifier *outbox.Notifier
	appMetrics     *metrics.Metrics
	tracerProvider *sdktrace.TracerProvider
	readiness      *platformhttp.ReadinessChecker
	jwtService     *platformauth.JWTService
	blacklist      sharedauth.TokenBlacklist
	authMiddleware *httpmw.AuthMiddleware
	rbacMiddleware *httpmw.RBACMiddleware
	perIPLimiter   *httpmw.PerIPRateLimiter
	httpServer     *platformhttp.Server
	metricsServer  *platformhttp.MetricsServer
	grpcServer     *platformgrpc.Server
	modules        []runtime.Module
	typeRegistry   *sharedevent.TypeRegistry
	stream         messaging.Stream
	eventStream    *messaging.EventStream
}

// New constructs a fully wired *App from the configuration file at cfgPath.
// Dependencies are initialised in strict dependency order so that any failure
// surfaces with a clear error before the process proceeds further:
//
//  1. Config
//  2. Logging
//  3. Metrics
//  4. Tracing (optional — skipped when TracingConfig.Enabled is false)
//  5. Write DB pool
//  6. Read DB pool
//  7. Transaction manager (backed by write pool)
//  8. Cache client
//  9. Event bus
//  10. Outbox store + worker
//  11. HTTP server with middleware chain
//  12. gRPC server with interceptor chain
//  13. Readiness checker with DB and cache probes
func New(cfgPath string) (_ *App, retErr error) {
	// closers accumulates cleanup functions for resources created during init.
	// On error, they run in LIFO order so dependencies are released correctly.
	var closers []func()
	defer func() {
		if retErr != nil {
			for i := len(closers) - 1; i >= 0; i-- {
				closers[i]()
			}
		}
	}()

	// 1. Config
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: load config: %w", err)
	}

	// 2. Logger
	logger, err := logging.New(
		cfg.Log.Level,
		cfg.Log.Format,
		cfg.App.Name,
		cfg.App.Env,
	)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: init logger: %w", err)
	}

	// 3. Metrics
	appMetrics := metrics.New()

	// 3b. Runtime metrics — goroutine count, GOMAXPROCS.
	metrics.RegisterRuntimeMetrics(prometheus.DefaultRegisterer)

	// 4. Tracing (optional)
	var tp *sdktrace.TracerProvider
	if cfg.Otel.Enabled {
		tp, err = tracing.Init(
			context.Background(),
			cfg.App.Name,
			cfg.App.Env,
			cfg.Otel.Endpoint,
			cfg.Otel.SampleRate,
			cfg.Otel.Insecure,
		)
		if err != nil {
			return nil, fmt.Errorf("bootstrap: init tracing: %w", err)
		}
		closers = append(closers, func() { _ = tp.Shutdown(context.Background()) })
	}

	// 5. Write DB pool
	writeDB, err := db.NewWritePool(cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: write db pool: %w", err)
	}
	closers = append(closers, func() { _ = writeDB.Close() })

	// 6. Read DB pool
	readDB, err := db.NewReadPool(cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: read db pool: %w", err)
	}
	closers = append(closers, func() { _ = readDB.Close() })

	// 7. Transaction manager + domain-friendly runner
	txManager := db.NewTxManager(writeDB)
	txRunner := db.NewTxRunner(txManager)

	// 8. Cache client
	cacheClient, err := cache.NewClient(cfg.Cache.Driver, cfg.Redis, cfg.Cache.Local)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: cache client: %w", err)
	}
	closers = append(closers, func() { _ = cacheClient.Close() })

	// 8b. Token blacklist
	var blacklist sharedauth.TokenBlacklist
	if cfg.Cache.Driver == "redis" {
		blacklist = platformauth.NewRedisBlacklist(cacheClient)
	} else {
		logger.Warn("redis disabled — token revocation will not work until natural expiry")
		blacklist = platformauth.NewNoopBlacklist()
	}

	// 8c. JWT service
	jwtSvc, err := platformauth.NewJWTService(cfg.JWT, clock.RealClock{}, blacklist)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: jwt service: %w", err)
	}

	// 8d. Auth middleware
	authMiddleware := httpmw.NewAuthMiddleware(jwtSvc, logger)
	rbacMiddleware := httpmw.NewRBACMiddleware(logger)

	// 9. Event bus
	eventBus, err := events.NewBus(cfg.Broker.Driver, logger)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: event bus: %w", err)
	}
	closers = append(closers, func() { _ = eventBus.Close() })

	// 10. Outbox store (worker is wired after stream initialisation)
	outboxStore := outbox.NewStore(writeDB, logger)
	dlqStore := deadletter.NewStore(writeDB)

	// 10b. Messaging stream
	streamResult, err := events.NewStream(cfg.Broker.Driver, events.StreamConfigs{
		NATS:     cfg.NATS,
		Kafka:    cfg.Kafka,
		RabbitMQ: cfg.RabbitMQ,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("bootstrap: messaging stream: %w", err)
	}
	stream := streamResult.Stream
	closers = append(closers, func() { _ = stream.Close() })

	// 10c. Outbox worker publishes durable events through the messaging stream.
	outboxPublisher := events.NewStreamEventPublisher(stream)
	outboxCB := resilience.NewCircuitBreaker(
		"outbox-publish",
		uint32(cfg.CB.MaxRequests),
		cfg.CB.Interval,
		cfg.CB.Timeout,
		logger,
	)
	outboxWorker := outbox.NewWorker(
		outboxStore,
		outboxPublisher,
		cfg.Outbox.PollInterval,
		cfg.Outbox.BatchSize,
		cfg.Outbox.MaxRetries,
		cfg.Outbox.Retention,
		logger,
		appMetrics,
	).WithDLQ(dlqStore).
		WithCircuitBreaker(outboxCB).
		WithPollTimeout(cfg.Outbox.PollTimeout)

	// 10d. Outbox LISTEN/NOTIFY notifier (optional — reduces poll latency).
	// Uses the write DSN to open a dedicated pgx connection for LISTEN.
	var outboxNotifier *outbox.Notifier
	if cfg.Outbox.NotifyEnabled {
		outboxNotifier = outbox.NewNotifier(cfg.DB.Write.DSN, logger)
		outboxWorker.WithNotify(outboxNotifier.Wake())
	}

	// 10d. Event type registry — module events are registered via RegisterModules
	typeRegistry := sharedevent.NewTypeRegistry()

	// 10e. Event stream — typed publish/subscribe over the raw stream
	eventStream := messaging.NewEventStream(stream, typeRegistry)

	// 11. HTTP server with middleware chain
	var perIPLimiter *httpmw.PerIPRateLimiter
	if cfg.RateLimit.RPS > 0 {
		perIPLimiter = httpmw.NewPerIPRateLimiter(cfg.RateLimit.RPS, cfg.RateLimit.Burst)
	}

	httpServer := platformhttp.New(cfg.HTTP, cfg.App, logger, appMetrics)
	httpServer.Engine().Use(httpmw.BuildChain(httpmw.ChainDeps{
		Logger:          logger,
		Metrics:         appMetrics,
		PerIPLimiter:    perIPLimiter,
		LoadShed:        cfg.LoadShed,
		Timeout:         cfg.HTTP.WriteTimeout,
		ServiceName:     cfg.App.Name,
		SecurityHeaders: cfg.SecurityHeaders,
		CORS:            cfg.HTTP.CORS,
	})...)

	// 12. Dedicated metrics server on a separate port so Prometheus scrapes
	// bypass API middleware (auth, rate-limit, load-shed) and match the
	// deployment annotation prometheus.io/port.
	var metricsServer *platformhttp.MetricsServer
	if cfg.Metrics.Enabled {
		metricsServer = platformhttp.NewMetricsServer(cfg.Metrics.Port, logger)
	}

	// 13. gRPC server with interceptor chain
	grpcServer := platformgrpc.New(
		cfg.GRPC,
		logger,
		grpcmw.BuildInterceptors(logger, appMetrics)...,
	)

	// 13. DB pool metrics — export sql.DBStats as Prometheus gauges
	_ = db.NewPoolMetrics(writeDB, readDB)

	// 14. Readiness checker — register DB and cache probes
	dbHealth := db.NewHealthChecker(writeDB, readDB)
	readiness := httpServer.Readiness()
	readiness.Add("postgres_write", dbHealth.CheckWrite)
	readiness.Add("postgres_read", dbHealth.CheckRead)
	readiness.Add("cache", cacheClient.Ping)
	if streamResult.HealthCheck != nil {
		readiness.Add("broker", streamResult.HealthCheck)
	}
	if cfg.Outbox.HealthThreshold > 0 {
		outboxLag := outbox.NewLagChecker(writeDB, cfg.Outbox.HealthThreshold)
		readiness.Add("outbox_lag", outboxLag.HealthCheck)
	}

	app := &App{
		cfg:            cfg,
		logger:         logger,
		writeDB:        writeDB,
		readDB:         readDB,
		cacheClient:    cacheClient,
		eventBus:       eventBus,
		txManager:      txManager,
		txRunner:       txRunner,
		outboxStore:    outboxStore,
		outboxWorker:   outboxWorker,
		perIPLimiter:   perIPLimiter,
		outboxNotifier: outboxNotifier,
		appMetrics:     appMetrics,
		tracerProvider: tp,
		jwtService:     jwtSvc,
		blacklist:      blacklist,
		authMiddleware: authMiddleware,
		rbacMiddleware: rbacMiddleware,
		readiness:      readiness,
		httpServer:     httpServer,
		metricsServer:  metricsServer,
		grpcServer:     grpcServer,
		typeRegistry:   typeRegistry,
		stream:         stream,
		eventStream:    eventStream,
	}

	return app, nil
}

// RegisterModule registers a runtime.Module with the application. It mounts
// the module's HTTP handlers onto the /api/v1 router group, registers its
// gRPC service implementations with the gRPC server, attaches its event
// subscribers to the event bus, and — if the module implements
// runtime.StreamModule — registers its stream-based subscriptions.
// RegisterModule must be called before Run.
func (a *App) RegisterModule(m runtime.Module) error {
	if err := m.RegisterSubscribers(a.eventBus); err != nil {
		return fmt.Errorf("bootstrap: register subscribers for module %q: %w", m.Name(), err)
	}

	if sm, ok := m.(runtime.StreamModule); ok {
		if err := sm.RegisterStreams(a.stream); err != nil {
			return fmt.Errorf("bootstrap: register streams for module %q: %w", m.Name(), err)
		}
	}

	// Apply module-level HTTP middleware if the module declares any.
	router := a.httpServer.Router()
	if mp, ok := m.(runtime.HTTPMiddlewareProvider); ok {
		raw := mp.HTTPMiddleware()
		if len(raw) > 0 {
			handlers := make([]gin.HandlerFunc, len(raw))
			for i, h := range raw {
				hf, ok := h.(gin.HandlerFunc)
				if !ok {
					return fmt.Errorf("bootstrap: module %q: HTTPMiddleware element %d is not gin.HandlerFunc (got %T)", m.Name(), i, h)
				}
				handlers[i] = hf
			}
			router = router.Group("", handlers...)
			a.logger.Info("module middleware applied",
				zap.String("module", m.Name()),
				zap.Int("count", len(handlers)),
			)
		}
	}

	// Register module-level health probes if the module declares any.
	if hp, ok := m.(runtime.HealthProbeProvider); ok {
		for name, check := range hp.HealthChecks() {
			a.readiness.Add(m.Name()+"_"+name, check)
		}
	}

	m.RegisterHTTP(router)
	m.RegisterGRPC(a.grpcServer.GRPCServer())

	a.modules = append(a.modules, m)
	a.logger.Info("module registered", zap.String("module", m.Name()))

	return nil
}

// Run starts all background workers and servers concurrently. It calls OnStart
// for every registered module before opening either server for traffic.
//
// Run blocks until all servers have exited or ctx is cancelled. The first
// non-nil error from any server or module start is returned. A cancelled
// context is not treated as an error.
func (a *App) Run(ctx context.Context) error {
	// Start modules before accepting traffic.
	for _, m := range a.modules {
		if err := m.OnStart(ctx); err != nil {
			return fmt.Errorf("bootstrap: module %q OnStart: %w", m.Name(), err)
		}
	}

	g, gCtx := errgroup.WithContext(ctx)

	g.Go(func() error {
		if err := a.httpServer.Start(); err != nil {
			return fmt.Errorf("bootstrap: http server: %w", err)
		}
		return nil
	})

	if a.metricsServer != nil {
		g.Go(func() error {
			if err := a.metricsServer.Start(); err != nil {
				return fmt.Errorf("bootstrap: metrics server: %w", err)
			}
			return nil
		})
	}

	// Reflection registers 2 services automatically. If no modules added
	// business services, log a warning so operators know the port is idle.
	if svcCount := len(a.grpcServer.GRPCServer().GetServiceInfo()); svcCount <= 2 {
		a.logger.Warn("gRPC server starting with no registered business services",
			zap.Int("port", a.cfg.GRPC.Port))
	}

	g.Go(func() error {
		if err := a.grpcServer.Start(); err != nil {
			return fmt.Errorf("bootstrap: grpc server: %w", err)
		}
		return nil
	})

	g.Go(func() error {
		if err := a.outboxWorker.Start(gCtx); err != nil && gCtx.Err() == nil {
			return fmt.Errorf("bootstrap: outbox worker: %w", err)
		}
		return nil
	})

	if a.outboxNotifier != nil {
		g.Go(func() error {
			if err := a.outboxNotifier.Start(gCtx); err != nil && gCtx.Err() == nil {
				return fmt.Errorf("bootstrap: outbox notifier: %w", err)
			}
			return nil
		})
	}

	// Context-watcher: when the parent context is cancelled (signal received),
	// stop servers so their errgroup goroutines can return and g.Wait() unblocks.
	// Without this, HTTP/gRPC servers block forever on ListenAndServe/Serve
	// because they don't respond to context cancellation — only to explicit Stop.
	g.Go(func() error {
		<-gCtx.Done()
		a.logger.Info("context cancelled, stopping servers")

		timeout := a.cfg.App.ShutdownTimeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		stopCtx, stopCancel := context.WithTimeout(context.Background(), timeout)
		defer stopCancel()

		if err := a.httpServer.Stop(stopCtx); err != nil {
			a.logger.Error("http server stop in context-watcher", zap.Error(err))
		}
		if a.metricsServer != nil {
			if err := a.metricsServer.Stop(stopCtx); err != nil {
				a.logger.Error("metrics server stop in context-watcher", zap.Error(err))
			}
		}
		if err := a.grpcServer.Stop(stopCtx); err != nil {
			a.logger.Error("grpc server stop in context-watcher", zap.Error(err))
		}
		return nil
	})

	err := g.Wait()

	if a.perIPLimiter != nil {
		a.perIPLimiter.Close()
	}

	return err
}

// Config returns the loaded application configuration. It is exposed so that
// callers composing modules at the entry point can pass config sub-sections to
// module constructors without repeating config.Load.
func (a *App) Config() *config.Config {
	return a.cfg
}

// Logger returns the application-wide structured logger.
func (a *App) Logger() *zap.Logger {
	return a.logger
}

// WriteDB returns the primary (write) database connection pool.
func (a *App) WriteDB() *sql.DB {
	return a.writeDB
}

// ReadDB returns the replica (read) database connection pool.
func (a *App) ReadDB() *sql.DB {
	return a.readDB
}

// TxManager returns the transaction manager backed by the write pool.
func (a *App) TxManager() db.TxManager {
	return a.txManager
}

// Cache returns the cache client.
func (a *App) Cache() cache.Client {
	return a.cacheClient
}

// EventBus returns the application event bus.
func (a *App) EventBus() sharedevent.Bus {
	return a.eventBus
}

// OutboxStore returns the transactional outbox store.
func (a *App) OutboxStore() *outbox.Store {
	return a.outboxStore
}

// Metrics returns the Prometheus metrics collectors.
func (a *App) Metrics() *metrics.Metrics {
	return a.appMetrics
}

// JWTService returns the platform JWT token service.
func (a *App) JWTService() *platformauth.JWTService {
	return a.jwtService
}

// AuthMiddleware returns the HTTP auth middleware for use by modules.
func (a *App) AuthMiddleware() *httpmw.AuthMiddleware {
	return a.authMiddleware
}

// RBACMiddleware returns the HTTP RBAC middleware for use by modules.
func (a *App) RBACMiddleware() *httpmw.RBACMiddleware {
	return a.rbacMiddleware
}

// TypeRegistry returns the global event type registry with all module event
// payload types registered. Use it for stream-based serialization/deserialization.
func (a *App) TypeRegistry() *sharedevent.TypeRegistry {
	return a.typeRegistry
}

// Stream returns the raw messaging stream backed by the configured broker
// driver. Use EventStream for typed domain event publishing/subscribing.
func (a *App) Stream() messaging.Stream {
	return a.stream
}

// EventStream returns a typed event stream that serializes and deserializes
// domain events using the global type registry.
func (a *App) EventStream() *messaging.EventStream {
	return a.eventStream
}

// Container builds a modular.Container from the App's platform dependencies.
// Module factories receive this container to construct themselves without
// knowing about bootstrap internals.
func (a *App) Container() *modular.Container {
	return &modular.Container{
		Config:         a.cfg,
		Logger:         a.logger,
		WriteDB:        a.writeDB,
		ReadDB:         a.readDB,
		TxManager:      a.txManager,
		TxRunner:       a.txRunner,
		Cache:          a.cacheClient,
		OutboxStore:    a.outboxStore,
		EventBus:       a.eventBus,
		Stream:         a.stream,
		JWTService:     a.jwtService,
		AuthMiddleware: a.authMiddleware,
		RBACMiddleware: a.rbacMiddleware,
		TypeRegistry:   a.typeRegistry,
		EventStream:    a.eventStream,
		Metrics:        a.appMetrics,
	}
}

// RegisterModules accepts an explicit module manifest, checks the modules
// config, creates each module via its factory, registers events, and wires
// the lifecycle (HTTP, gRPC, subscribers, streams).
func (a *App) RegisterModules(manifest []modular.CatalogEntry) error {
	ctr := a.Container()
	registry := modular.NewRegistry()

	for _, entry := range manifest {
		if !a.cfg.Modules.IsEnabled(entry.Name) {
			a.logger.Info("module disabled by config, skipping",
				zap.String("module", entry.Name),
			)
			continue
		}

		m := entry.Factory(ctr)
		m.RegisterEvents(a.typeRegistry)

		if err := registry.Register(m); err != nil {
			return fmt.Errorf("bootstrap: %w", err)
		}
	}

	ordered, err := registry.Modules()
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}

	// Wire optional module-provided capabilities into platform services.
	// Only one SessionRevocationProvider is expected; first match wins.
	for _, m := range ordered {
		if p, ok := m.(runtime.SessionRevocationProvider); ok {
			a.jwtService.SetSessionRevocationChecker(p.SessionRevocationChecker())
			a.logger.Info("session revocation checker wired from module",
				zap.String("module", m.Name()),
			)
			break
		}
	}

	for _, m := range ordered {
		if err := a.RegisterModule(m); err != nil {
			return err
		}
	}

	return nil
}

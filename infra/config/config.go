// Package config provides configuration loading and validation for the wolf-be service.
// It uses Viper to read YAML files and supports environment variable overrides.
package config

import (
	"errors"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config is the top-level configuration struct for the entire application.
// It is populated from a YAML file and can be overridden by environment variables.
//
// The struct is intentionally flat so that Viper's automatic env-var mapping
// (SetEnvKeyReplacer "." → "_") produces short, operator-friendly names:
//
//	db.write.dsn           → DB_WRITE_DSN
//	redis.addr             → REDIS_ADDR
//	log.level              → LOG_LEVEL
//	jwt.secret_key         → JWT_SECRET_KEY
//	cb.max_requests        → CB_MAX_REQUESTS
type Config struct {
	App    AppConfig    `mapstructure:"app"`
	HTTP   HTTPConfig   `mapstructure:"http"`
	GRPC   GRPCConfig   `mapstructure:"grpc"`
	DB     DBConfig     `mapstructure:"db"`
	Redis  RedisConfig  `mapstructure:"redis"`
	Cache  CacheConfig  `mapstructure:"cache"`
	Broker BrokerConfig `mapstructure:"broker"`
	Kafka  KafkaConfig  `mapstructure:"kafka"`

	RabbitMQ        RabbitMQConfig        `mapstructure:"rabbitmq"`
	NATS            NATSConfig            `mapstructure:"nats"`
	Log             LoggingConfig         `mapstructure:"log"`
	Otel            TracingConfig         `mapstructure:"otel"`
	Metrics         MetricsConfig         `mapstructure:"metrics"`
	CB              CircuitBreakerConfig  `mapstructure:"cb"`
	RateLimit       RateLimitConfig       `mapstructure:"rate_limit"`
	LoadShed        LoadShedConfig        `mapstructure:"load_shed"`
	SecurityHeaders SecurityHeadersConfig `mapstructure:"security_headers"`
	Outbox          OutboxConfig          `mapstructure:"outbox"`
	Modules         ModulesConfig         `mapstructure:"modules"`
	JWT             JWTConfig             `mapstructure:"jwt"`
	Bcrypt          BcryptConfig          `mapstructure:"bcrypt"`
	Session         SessionConfig         `mapstructure:"session"`
}

// Environment constants for AppConfig.Env.
const (
	EnvDevelopment = "development"
	EnvStaging     = "staging"
	EnvProduction  = "production"
)

// Broker driver constants for BrokerConfig.Driver.
const (
	BrokerInProcess = "inprocess"
	BrokerKafka     = "kafka"
	BrokerRabbitMQ  = "rabbitmq"
	BrokerNATS      = "nats"
)

// Cache driver constants for CacheConfig.Driver.
const (
	CacheDriverRedis = "redis"
	CacheDriverNoop  = "noop"
)

// AppConfig holds application-level metadata.
type AppConfig struct {
	// Name is the human-readable application name.
	Name string `mapstructure:"name"`
	// Env is the deployment environment: development, staging, or production.
	Env string `mapstructure:"env"`
	// Debug enables verbose debug output when true.
	Debug bool `mapstructure:"debug"`
	// ShutdownTimeout is the maximum duration to wait for graceful shutdown.
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// HTTPConfig holds configuration for the HTTP/REST server.
type HTTPConfig struct {
	// Port is the TCP port the HTTP server listens on.
	Port int `mapstructure:"port"`
	// ReadTimeout is the maximum duration for reading the entire request.
	ReadTimeout time.Duration `mapstructure:"read_timeout"`
	// ReadHeaderTimeout limits how long the server waits to read request
	// headers. Mitigates slow-loris attacks. Defaults to ReadTimeout if zero.
	ReadHeaderTimeout time.Duration `mapstructure:"read_header_timeout"`
	// WriteTimeout is the maximum duration before timing out writes of the response.
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	// IdleTimeout is the maximum amount of time to wait for the next request.
	IdleTimeout time.Duration `mapstructure:"idle_timeout"`
	// MaxHeaderBytes controls the maximum number of bytes the server will read
	// parsing the request header's keys and values.
	MaxHeaderBytes int `mapstructure:"max_header_bytes"`
	// TrustedProxies is a list of CIDR ranges or IP addresses of trusted
	// reverse proxies. When set, Gin uses this to determine the real client IP
	// from forwarded headers. Required for correct per-IP rate limiting,
	// audit logging, and abuse detection behind load balancers.
	TrustedProxies []string `mapstructure:"trusted_proxies"`
	// CORS holds cross-origin resource sharing configuration.
	CORS CORSConfig `mapstructure:"cors"`
}

// CORSConfig holds cross-origin resource sharing settings.
type CORSConfig struct {
	// AllowedOrigins is the list of origins that are allowed to make cross-site requests.
	AllowedOrigins []string `mapstructure:"allowed_origins"`
	// AllowedMethods is the list of HTTP methods allowed for cross-origin requests.
	AllowedMethods []string `mapstructure:"allowed_methods"`
	// AllowedHeaders is the list of HTTP headers allowed in cross-origin requests.
	AllowedHeaders []string `mapstructure:"allowed_headers"`
	// AllowCredentials indicates whether credentials (cookies, auth headers) are allowed.
	AllowCredentials bool `mapstructure:"allow_credentials"`
	// MaxAge specifies how long (in seconds) the results of a preflight request can be cached.
	MaxAge int `mapstructure:"max_age"`
}

// GRPCConfig holds configuration for the gRPC server.
type GRPCConfig struct {
	// Enabled controls whether the gRPC server starts. Defaults to true when nil (backward compat).
	Enabled *bool `mapstructure:"enabled"`
	// Port is the TCP port the gRPC server listens on.
	Port int `mapstructure:"port"`
	// MaxRecvMsgSize is the maximum message size in bytes the server can receive.
	MaxRecvMsgSize int `mapstructure:"max_recv_msg_size"`
	// MaxSendMsgSize is the maximum message size in bytes the server can send.
	MaxSendMsgSize int `mapstructure:"max_send_msg_size"`
	// KeepaliveTime is the duration after which the server pings the client to check liveness.
	KeepaliveTime time.Duration `mapstructure:"keepalive_time"`
	// KeepaliveTimeout is the duration the server waits for a keepalive ping ack.
	KeepaliveTimeout time.Duration `mapstructure:"keepalive_timeout"`
}

// IsEnabled returns true if gRPC is enabled. Defaults to true when Enabled is nil (backward compat).
func (c GRPCConfig) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
}

// ---------------------------------------------------------------------------
// Database
// ---------------------------------------------------------------------------

// DBConfig holds configuration for database connection pools.
type DBConfig struct {
	// Write is the connection pool configuration for the primary (write) database.
	Write PoolConfig `mapstructure:"write"`
	// Read is the connection pool configuration for the replica (read) database.
	Read PoolConfig `mapstructure:"read"`
}

// PoolConfig holds configuration for a single database connection pool.
type PoolConfig struct {
	// DSN is the data source name (connection string) for the database.
	DSN string `mapstructure:"dsn"`
	// MaxOpenConns is the maximum number of open connections to the database.
	MaxOpenConns int `mapstructure:"max_open_conns"`
	// MaxIdleConns is the maximum number of idle connections in the pool.
	MaxIdleConns int `mapstructure:"max_idle_conns"`
	// ConnMaxLifetime is the maximum amount of time a connection may be reused.
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
	// ConnMaxIdleTime is the maximum amount of time a connection may be idle before closing.
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time"`
	// StatementTimeout aborts any statement that takes longer than the specified
	// duration. Prevents runaway queries from holding connections indefinitely.
	// Value is in milliseconds as required by PostgreSQL. 0 disables.
	StatementTimeout int `mapstructure:"statement_timeout"`
	// IdleInTransactionTimeout terminates sessions idle inside a transaction
	// for longer than the specified duration (milliseconds). 0 disables.
	IdleInTransactionTimeout int `mapstructure:"idle_in_transaction_session_timeout"`
	// SimpleProtocol forces pgx to use the simple query protocol instead of the
	// extended protocol. Required when connecting through PgBouncer in transaction
	// pooling mode, which cannot track server-side prepared statements.
	SimpleProtocol bool `mapstructure:"simple_protocol"`
}

// ---------------------------------------------------------------------------
// Cache — top-level "cache" holds driver + L1; "redis" is a peer section.
// ---------------------------------------------------------------------------

// CacheConfig holds cache driver selection and L1 settings.
// Redis-specific settings live in the top-level RedisConfig.
type CacheConfig struct {
	// Driver specifies the cache backend: "redis" or "noop".
	Driver string `mapstructure:"driver"`
	// Local holds in-process L1 cache configuration.
	Local LocalCacheConfig `mapstructure:"local"`
}

// LocalCacheConfig holds in-process L1 cache settings.
type LocalCacheConfig struct {
	// Enabled activates the L1 in-memory cache in front of the remote backend.
	Enabled bool `mapstructure:"enabled"`
	// Size is the maximum number of entries in the LRU. Default: 10000.
	Size int `mapstructure:"size"`
	// TTL is how long entries stay in the local cache before expiring. Default: 10s.
	TTL time.Duration `mapstructure:"ttl"`
}

// RedisConfig holds configuration for a Redis connection.
type RedisConfig struct {
	// Addr is the Redis server address in "host:port" format.
	Addr string `mapstructure:"addr"`
	// Password is the Redis AUTH password (empty string means no auth).
	Password string `mapstructure:"password"`
	// DB is the Redis database number to select.
	DB int `mapstructure:"db"`
	// PoolSize is the maximum number of socket connections.
	PoolSize int `mapstructure:"pool_size"`
	// MinIdleConns is the minimum number of idle connections to maintain.
	MinIdleConns int `mapstructure:"min_idle_conns"`
	// ReadTimeout is the timeout for socket reads.
	ReadTimeout time.Duration `mapstructure:"read_timeout"`
	// WriteTimeout is the timeout for socket writes.
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	// DialTimeout is the timeout for establishing new connections.
	DialTimeout time.Duration `mapstructure:"dial_timeout"`
}

// ---------------------------------------------------------------------------
// Broker — top-level "broker" holds the driver; individual backends are peers.
// ---------------------------------------------------------------------------

// BrokerConfig holds just the driver selector for the message broker.
// Backend-specific configs (Kafka, RabbitMQ, NATS) are top-level peers.
type BrokerConfig struct {
	// Driver specifies the broker backend: "kafka", "rabbitmq", "nats", or "inprocess".
	Driver string `mapstructure:"driver"`
}

// KafkaConfig holds configuration for a Kafka connection.
type KafkaConfig struct {
	// Brokers is the list of Kafka broker addresses.
	Brokers []string `mapstructure:"brokers"`
	// ConsumerGroup is the consumer group ID for this service.
	ConsumerGroup string `mapstructure:"consumer_group"`
}

// RabbitMQConfig holds configuration for a RabbitMQ connection.
type RabbitMQConfig struct {
	// URL is the AMQP connection URL (e.g. amqp://guest:guest@localhost:5672/).
	URL string `mapstructure:"url"`
	// PrefetchCount controls how many messages are fetched at once per consumer.
	PrefetchCount int `mapstructure:"prefetch_count"`
}

// NATSConfig holds configuration for a NATS JetStream connection.
type NATSConfig struct {
	// URL is the NATS server connection URL (e.g. "nats://localhost:4222").
	URL string `mapstructure:"url"`
	// Stream is the JetStream stream name to create or update at startup.
	Stream string `mapstructure:"stream"`
	// Subjects is the list of NATS subjects the stream will listen on.
	Subjects []string `mapstructure:"subjects"`
	// Replicas is the number of stream replicas for clustered JetStream.
	// Defaults to 1 when unset.
	Replicas int `mapstructure:"replicas"`
}

// ---------------------------------------------------------------------------
// Observability — split into top-level "log", "otel", "metrics".
// ---------------------------------------------------------------------------

// LoggingConfig holds structured logging settings.
type LoggingConfig struct {
	// Level is the minimum log level: debug, info, warn, error.
	Level string `mapstructure:"level"`
	// Format is the log output format: json or console.
	Format string `mapstructure:"format"`
}

// TracingConfig holds distributed tracing settings.
type TracingConfig struct {
	// Enabled controls whether distributed tracing is active.
	Enabled bool `mapstructure:"enabled"`
	// Exporter specifies the trace exporter: "otlp", "jaeger", or "stdout".
	Exporter string `mapstructure:"exporter"`
	// Endpoint is the address of the trace collector.
	Endpoint string `mapstructure:"endpoint"`
	// SampleRate controls what fraction of traces to sample (0.0–1.0).
	SampleRate float64 `mapstructure:"sample_rate"`
	// Insecure disables TLS for the OTLP exporter connection. Set to true only
	// for local development. Production deployments MUST use TLS (default).
	Insecure bool `mapstructure:"insecure"`
}

// MetricsConfig holds Prometheus metrics settings.
type MetricsConfig struct {
	// Enabled controls whether the metrics endpoint is active.
	Enabled bool `mapstructure:"enabled"`
	// Port is the TCP port the metrics server listens on.
	Port int `mapstructure:"port"`
}

// ---------------------------------------------------------------------------
// Resilience — split into top-level "cb", "rate_limit", "load_shed",
// "security_headers".
// ---------------------------------------------------------------------------

// CircuitBreakerConfig holds circuit breaker settings (using gobreaker).
type CircuitBreakerConfig struct {
	// MaxRequests is the maximum number of requests allowed in the half-open state.
	MaxRequests int `mapstructure:"max_requests"`
	// Interval is the cyclic period of the closed state to clear the counts.
	Interval time.Duration `mapstructure:"interval"`
	// Timeout is the period of the open state before moving to half-open.
	Timeout time.Duration `mapstructure:"timeout"`
}

// RateLimitConfig holds token-bucket rate limiting settings.
type RateLimitConfig struct {
	// RPS is the maximum number of requests per second (token refill rate).
	RPS int `mapstructure:"rps"`
	// Burst is the maximum number of tokens in the bucket (burst capacity).
	Burst int `mapstructure:"burst"`
}

// LoadShedConfig holds concurrency-based load shedding settings.
type LoadShedConfig struct {
	// MaxConcurrent is the maximum number of in-flight requests before the
	// server starts returning 503. 0 disables load shedding.
	MaxConcurrent int `mapstructure:"max_concurrent"`
}

// SecurityHeadersConfig holds configuration for HTTP security response headers.
// Used directly by the middleware layer via BuildChain.
type SecurityHeadersConfig struct {
	// Enabled activates security header injection globally.
	Enabled bool `mapstructure:"enabled"`
	// HSTS enables the Strict-Transport-Security header.
	HSTS bool `mapstructure:"hsts"`
	// HSTSMaxAge is the max-age directive in seconds. Default: 31536000 (1 year).
	HSTSMaxAge int `mapstructure:"hsts_max_age"`
	// ContentTypeNosniff enables X-Content-Type-Options: nosniff.
	ContentTypeNosniff bool `mapstructure:"content_type_nosniff"`
	// FrameDeny enables X-Frame-Options: DENY.
	FrameDeny bool `mapstructure:"frame_deny"`
	// XSSProtection enables X-XSS-Protection: 1; mode=block.
	XSSProtection bool `mapstructure:"xss_protection"`
	// ContentSecurityPolicy is the Content-Security-Policy header value.
	// An empty string means the header is omitted.
	ContentSecurityPolicy string `mapstructure:"content_security_policy"`
	// ReferrerPolicy is the Referrer-Policy header value.
	// Default: strict-origin-when-cross-origin.
	ReferrerPolicy string `mapstructure:"referrer_policy"`
}

// ---------------------------------------------------------------------------
// Outbox
// ---------------------------------------------------------------------------

// OutboxConfig holds configuration for the transactional outbox relay.
type OutboxConfig struct {
	// PollInterval is how often the relay polls for new outbox messages.
	PollInterval time.Duration `mapstructure:"poll_interval"`
	// BatchSize is the number of messages to process per polling cycle.
	BatchSize int `mapstructure:"batch_size"`
	// MaxRetries is the number of delivery attempts before marking a message as failed.
	MaxRetries int `mapstructure:"max_retries"`
	// Retention is how long to keep processed outbox records before deletion.
	Retention time.Duration `mapstructure:"retention"`
	// PollTimeout is the per-cycle context timeout. Defaults to 30s when zero.
	PollTimeout time.Duration `mapstructure:"poll_timeout"`
	// HealthThreshold is how long an unpublished entry may linger before the
	// outbox lag health check reports unhealthy. Defaults to 60s when zero.
	HealthThreshold time.Duration `mapstructure:"health_threshold"`
	// NotifyEnabled activates PostgreSQL LISTEN/NOTIFY for the outbox worker.
	// When true, the worker wakes immediately on INSERT instead of waiting
	// for the next poll tick. Requires the outbox_notify trigger migration.
	NotifyEnabled bool `mapstructure:"notify_enabled"`
}

// ---------------------------------------------------------------------------
// Modules
// ---------------------------------------------------------------------------

// ModulesConfig controls which application modules are enabled at startup.
// It is a dynamic map so that new modules do not require config struct changes.
// Modules not listed in the map are enabled by default.
type ModulesConfig map[string]bool

// IsEnabled returns whether the named module is enabled. Modules not
// explicitly listed in the config are enabled by default.
func (m ModulesConfig) IsEnabled(name string) bool {
	enabled, ok := m[name]
	return !ok || enabled
}

// ---------------------------------------------------------------------------
// Auth — split into top-level "jwt", "bcrypt", "session".
// ---------------------------------------------------------------------------

// JWTConfig holds JWT signing key material and token lifetime settings.
type JWTConfig struct {
	// SigningMethod is the algorithm used to sign tokens: "HS256" or "RS256".
	SigningMethod string `mapstructure:"signing_method"`
	// SecretKey is the HMAC secret used when SigningMethod is "HS256".
	// Must be at least 32 characters.
	SecretKey string `mapstructure:"secret_key"`
	// PrivateKeyPath is the path to the RSA private key PEM file (RS256 only).
	PrivateKeyPath string `mapstructure:"private_key_path"`
	// PublicKeyPath is the path to the RSA public key PEM file (RS256 only).
	PublicKeyPath string `mapstructure:"public_key_path"`
	// AccessTokenTTL is how long access tokens remain valid (1m–1h).
	AccessTokenTTL time.Duration `mapstructure:"access_token_ttl"`
	// RefreshTokenTTL is how long refresh tokens remain valid (1h–2160h).
	RefreshTokenTTL time.Duration `mapstructure:"refresh_token_ttl"`
	// Issuer is the "iss" claim embedded in every token.
	Issuer string `mapstructure:"issuer"`
	// Audience is the list of "aud" claims embedded in every token.
	Audience []string `mapstructure:"audience"`
}

// BcryptConfig holds bcrypt password hashing parameters.
type BcryptConfig struct {
	// Cost is the bcrypt work factor. Must be between 10 and 14.
	Cost int `mapstructure:"cost"`
}

// SessionConfig controls multi-device session behaviour.
type SessionConfig struct {
	// RevokeOnRoleChange when true forces session revocation after role assignment/revocation,
	// ensuring JWT claims refresh immediately rather than waiting for access token expiry.
	RevokeOnRoleChange bool `mapstructure:"revoke_on_role_change"`
}

// ---------------------------------------------------------------------------
// Convenience re-composition types — used as parameter objects by factories
// that need grouped configs. NOT embedded in Config; constructed at call sites.
// ---------------------------------------------------------------------------

// AuthBundle groups auth-related configs for modules that need them together.
type AuthBundle struct {
	JWT     JWTConfig
	Bcrypt  BcryptConfig
	Session SessionConfig
}

// Load reads and parses the application configuration from the YAML file at
// the given path. Environment variables override file values automatically.
//
// Environment variable mapping example:
//
//	DB_WRITE_DSN=...        overrides db.write.dsn
//	JWT_SECRET_KEY=...      overrides jwt.secret_key
//	LOG_LEVEL=info          overrides log.level
func Load(path string) (*Config, error) {
	v := viper.New()

	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	// Register all known keys with defaults so Viper recognises them during
	// AutomaticEnv lookups. Without this, env-only bootstrap (no YAML file)
	// produces zero-values because Viper only maps env vars to keys it knows.
	setDefaults(v)

	// Allow environment variables to override config values.
	// With no prefix and "." → "_" replacement, Viper auto-maps:
	//   db.write.dsn → DB_WRITE_DSN
	v.SetEnvPrefix("")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		// If the config file simply doesn't exist (e.g. container without a
		// mounted YAML), fall back to environment-variable-only bootstrap.
		// Any other read error (malformed YAML, permission denied) still fails fast.
		if !errors.Is(err, os.ErrNotExist) {
			var notFoundErr viper.ConfigFileNotFoundError
			if !errors.As(err, &notFoundErr) {
				return nil, err
			}
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}
